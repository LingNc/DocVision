package session

// OpenAI Responses API 适配（P4 补全）：models.*.<条目>.api: responses 的
// 条目改走 POST {base_url}/responses（base_url 约定含 /v1，如
// https://api.openai.com/v1）。gpt-5 / o 系等新模型只开在这个协议上。
//
// 流式（P34 后续）：models.*.stream 为真时请求带 stream:true，按
// response.output_text.delta / reasoning_summary_text.delta 增量喂实时
// 快照与首字延迟，最终仍以 response.completed 里的完整 response 对象
// 收账（output/usage/status 与非流式同源，所以两条路径共用同一个
// decoder，转录内容不随传输方式变化）。网关不支持 SSE 时自动降级成
// 一次 JSON 应答重试一次。
//
// store:false + 全量 input（我们每次回放整个历史，不用服务端状态）。
//
// 思考链的处理：请求 include:["reasoning.encrypted_content"]，decode 时
// 把 encrypted_content 存进 ReasoningContent（不透明 token，回放时原样
// 交回）；没有 encrypted_content 时退回 summary 文本。注意加密思考有
// 有效期（约半小时级），长间隔续跑可能失效——这是 Responses API 的固
// 有约束，摘要不受影响。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// buildResponsesBody translates the internal ChatRequest into an
// OpenAI Responses API request body, then merges request_body (explicit
// config wins, same rule as the other protocols).
func (c *Client) buildResponsesBody(payload *ChatRequest) ([]byte, error) {
	body := map[string]any{
		"model":  c.model,
		"store":  false, // 无状态：历史每次全量回传，不用服务端存储
		"stream": payload.Stream,
	}
	if payload.MaxTokens > 0 {
		// Responses API 的输出预算字段名不同（reasoning 模型的硬上限语义）。
		body["max_output_tokens"] = payload.MaxTokens
	}
	if payload.Temperature > 0 {
		body["temperature"] = payload.Temperature
	}

	input := make([]map[string]any, 0, len(payload.Messages))
	for _, m := range payload.Messages {
		// system 消息 → 顶层 instructions（Responses API 的 input 里没有
		// system 角色）。我们的会话永远以一条 system 开头。
		if m.Role == "system" {
			if s := ContentString(m); s != "" {
				body["instructions"] = s
			}
			continue
		}
		items, err := responsesItems(m)
		if err != nil {
			return nil, err
		}
		input = append(input, items...)
	}
	body["input"] = input

	if len(payload.Tools) > 0 {
		tools := make([]map[string]any, 0, len(payload.Tools))
		for _, t := range payload.Tools {
			fn, _ := t["function"].(map[string]any)
			if fn == nil {
				continue
			}
			tools = append(tools, map[string]any{
				"type":        "function",
				"name":        fn["name"],
				"description": fn["description"],
				"parameters": func() any {
					if p, ok := fn["parameters"]; ok && p != nil {
						return p
					}
					return map[string]any{"type": "object"}
				}(),
			})
		}
		if len(tools) > 0 {
			body["tools"] = tools
		}
	}
	// tool_choice：Responses API 直接收 "auto"/"none" 字符串。
	if choice, ok := payload.ToolChoice.(string); ok && choice != "" {
		body["tool_choice"] = choice
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal responses request: %w", err)
	}
	// 让服务端回传可回放的加密思考（历史思维链续跑的前提）。
	raw, err = injectRequestBody(raw, map[string]any{
		"include": []any{"reasoning.encrypted_content"},
	})
	if err != nil {
		return nil, err
	}
	if len(c.requestBody) > 0 {
		raw, err = injectRequestBody(raw, c.requestBody)
		if err != nil {
			return nil, err
		}
	}
	if c.thinking != nil || c.reasoningEffort != "" {
		// reasoning 控制（o 系：effort 级别等）原样透传，网关自己认。
		extra := map[string]any{}
		if c.thinking != nil {
			extra["reasoning"] = c.thinking
		}
		if c.reasoningEffort != "" {
			extra["reasoning"] = map[string]any{"effort": c.reasoningEffort}
		}
		raw, err = injectRequestBody(raw, extra)
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// responsesItems converts one internal message to Responses API input
// items: user turns → {role:"user", content:[input_text/input_image]},
// assistant turns → output_text message (+ reasoning item with the
// encrypted chain + function_call items), tool receipts →
// function_call_output items.
func responsesItems(m ChatMessage) ([]map[string]any, error) {
	switch m.Role {
	case "user":
		var content []map[string]any
		switch c := m.Content.(type) {
		case string:
			if c != "" {
				content = append(content, map[string]any{"type": "input_text", "text": c})
			}
		case nil:
		default:
			parts, err := normalizeParts(c)
			if err != nil {
				return nil, fmt.Errorf("responses: decode user content parts: %w", err)
			}
			for _, p := range parts {
				switch p["type"] {
				case "text":
					content = append(content, map[string]any{"type": "input_text", "text": p["text"]})
				case "image_url":
					iu, err := normalizeMap(p["image_url"])
					if err != nil {
						return nil, fmt.Errorf("responses: decode image_url: %w", err)
					}
					url, _ := iu["url"].(string)
					// Responses API 直接收 data: URL 或 http URL。
					content = append(content, map[string]any{"type": "input_image", "image_url": url})
				}
			}
		}
		if len(content) == 0 {
			content = append(content, map[string]any{"type": "input_text", "text": ""})
		}
		return []map[string]any{{"role": "user", "content": content}}, nil

	case "assistant":
		var items []map[string]any
		if think := strings.TrimSpace(m.ReasoningContent); think != "" {
			// decode 时存进来的要么是 encrypted_content（不透明 token），
			// 要么是 summary 文本——都原样放回 reasoning item。
			items = append(items, map[string]any{
				"type":              "reasoning",
				"encrypted_content": think,
			})
		}
		var outText []map[string]any
		if s := ContentString(m); s != "" {
			outText = append(outText, map[string]any{"type": "output_text", "text": s})
		}
		items = append(items, map[string]any{"role": "assistant", "content": outText})
		for _, tc := range m.ToolCalls {
			args := tc.Function.Arguments
			if args == "" {
				args = "{}"
			}
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   tc.ID,
				"name":      tc.Function.Name,
				"arguments": args, // Responses API 要 JSON **字符串**
			})
		}
		return items, nil

	case "tool":
		return []map[string]any{{
			"type":    "function_call_output",
			"call_id": m.ToolCallID,
			"output":  ContentString(m),
		}}, nil
	}
	return nil, fmt.Errorf("responses: unsupported role %q", m.Role)
}

// normalizeParts / normalizeMap coerce content part structures built
// with different map element types (map[string]string vs
// map[string]interface{}) through JSON.
func normalizeParts(c any) ([]map[string]interface{}, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var parts []map[string]interface{}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	return parts, nil
}

func normalizeMap(v any) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// postResponses performs the /responses call (OpenAI bearer auth). A
// streamed call uses the streaming HTTP client (no whole-body timeout).
func (c *Client) postResponses(raw []byte, stream bool) (*http.Response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+"/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream, application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpClient := c.http
	if stream {
		httpClient = c.streamHTTP
	}
	return httpClient.Do(httpReq)
}

// responsesWireResponse is the subset of the /responses response we use.
type responsesWireResponse struct {
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
	Output []map[string]json.RawMessage `json:"output"`
	// The REST API reports truncation as incomplete_details.reason; the
	// older flat incomplete_reason is still tolerated.
	IncompleteReason  string `json:"incomplete_reason"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage *struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// decodeResponsesResponse reads one JSON /responses response back into
// the internal ChatResponse (non-streaming path).
func (c *Client) decodeResponsesResponse(resp *http.Response, start time.Time) (*ChatResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return c.decodeResponsesBody(body, start)
}

// decodeResponsesBody turns one /responses JSON object (the body itself,
// or the "response" payload of a stream's response.completed event) into
// a ChatResponse. Streamed and non-streamed replies therefore land on
// exactly the same decode: transcripts do not depend on the transport.
func (c *Client) decodeResponsesBody(body []byte, start time.Time) (*ChatResponse, error) {
	var wire responsesWireResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%q)", err, truncate(string(body), 256))
	}
	if wire.Status == "failed" || wire.Error != nil {
		msg := "unknown error"
		if wire.Error != nil && wire.Error.Message != "" {
			msg = wire.Error.Message
		}
		return nil, fmt.Errorf("responses failed: %s", msg)
	}

	var msg ChatMessage
	msg.Role = "assistant"
	var texts, thinks []string
	sawToolCall := false
	for _, item := range wire.Output {
		var typ string
		_ = json.Unmarshal(item["type"], &typ)
		switch typ {
		case "message":
			var m struct {
				Content []struct {
					Typ  string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if json.Unmarshal(mustJSON(item), &m) == nil {
				for _, blk := range m.Content {
					if blk.Typ == "output_text" {
						texts = append(texts, blk.Text)
					}
				}
			}
		case "function_call":
			var fc struct {
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if json.Unmarshal(mustJSON(item), &fc) == nil {
				sawToolCall = true
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{
					ID:   fc.CallID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: fc.Name, Arguments: fc.Arguments},
				})
			}
		case "reasoning":
			var r struct {
				EncryptedContent string `json:"encrypted_content"`
				Summary          []struct {
					Text string `json:"text"`
				} `json:"summary"`
			}
			if json.Unmarshal(mustJSON(item), &r) == nil {
				if r.EncryptedContent != "" {
					thinks = append(thinks, r.EncryptedContent)
				} else {
					for _, s := range r.Summary {
						thinks = append(thinks, s.Text)
					}
				}
			}
		}
	}
	msg.Content = strings.Join(texts, "")
	msg.ReasoningContent = strings.Join(thinks, "\n\n")

	finish := "stop"
	if sawToolCall {
		finish = "tool_calls"
	} else if wire.IncompleteReason == "max_output_tokens" ||
		(wire.IncompleteDetails != nil && wire.IncompleteDetails.Reason == "max_output_tokens") {
		finish = "length"
	}
	out := &ChatResponse{
		Choices:      []ChatResponseChoice{{Message: msg}},
		Elapsed:      time.Since(start),
		TTFT:         time.Since(start), // 非流式：整体到达
		FinishReason: finish,
	}
	if wire.Usage != nil {
		cached := 0
		if wire.Usage.InputTokensDetails != nil {
			cached = wire.Usage.InputTokensDetails.CachedTokens
		}
		out.Usage = &Usage{
			PromptTokens:     wire.Usage.InputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.TotalTokens,
			PromptTokensDetails: &struct {
				CachedTokens int `json:"cached_tokens"`
			}{CachedTokens: cached},
		}
	}
	return out, nil
}

// mustJSON re-marshals a raw item map for typed decoding.
func mustJSON(m map[string]json.RawMessage) []byte {
	raw, _ := json.Marshal(m)
	return raw
}

// responsesStreamEvent is the subset of a Responses SSE event we use.
// The final reply is carried by response.completed / response.incomplete
// under "response", which has the same shape as the non-streaming body.
type responsesStreamEvent struct {
	Type     string          `json:"type"`
	Delta    string          `json:"delta"`
	Message  string          `json:"message"`
	Response json.RawMessage `json:"response"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// decodeResponses dispatches on the transport: a 2xx streamed reply is
// consumed as SSE, everything else as one JSON body.
func (c *Client) decodeResponses(resp *http.Response, stream bool, start time.Time, hook func(phase, text string)) (*ChatResponse, error) {
	if stream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return c.readResponsesStream(resp, start, hook)
	}
	return c.decodeResponsesResponse(resp, start)
}

// readResponsesStream consumes the Responses API SSE stream: content and
// reasoning deltas drive the progressive snapshot (P7) and the first-token
// latency, while response.completed supplies the authoritative output and
// usage. An idle watchdog aborts a stalled stream instead of hanging.
func (c *Client) readResponsesStream(resp *http.Response, start time.Time, hook func(phase, text string)) (*ChatResponse, error) {
	defer resp.Body.Close()

	type lineMsg struct {
		line string
		err  error
	}
	lines := make(chan lineMsg, 64)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		defer close(lines)
		br := bufio.NewReaderSize(resp.Body, 64*1024)
		for {
			line, err := br.ReadString('\n')
			if len(line) > 0 {
				select {
				case lines <- lineMsg{line: line}:
				case <-stop:
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					select {
					case lines <- lineMsg{err: err}:
					case <-stop:
					}
				}
				return
			}
		}
	}()

	var contentBuf, reasoningBuf strings.Builder
	var ttft time.Duration
	var lastHook time.Time
	markFirst := func() {
		if ttft == 0 {
			ttft = time.Since(start)
		}
	}
	emit := func(phase string, buf *strings.Builder) {
		if hook == nil || buf.Len() == 0 {
			return
		}
		now := time.Now()
		if now.Sub(lastHook) < 150*time.Millisecond {
			return
		}
		lastHook = now
		text := buf.String()
		if len(text) > 16*1024 {
			text = text[len(text)-16*1024:]
		}
		hook(phase, text)
	}

	var final *ChatResponse
	// raw keeps the whole body so a gateway that ignores stream:true and
	// answers with one plain JSON object can still be accounted for.
	var raw strings.Builder
	for {
		var msg lineMsg
		var open bool
		if c.streamIdle > 0 {
			select {
			case msg, open = <-lines:
			case <-time.After(c.streamIdle):
				return nil, fmt.Errorf("流式响应空闲超时（%s 无数据）", c.streamIdle)
			}
		} else {
			msg, open = <-lines
		}
		if !open {
			break // EOF: the stream is over
		}
		if msg.err != nil {
			return nil, fmt.Errorf("read stream: %w", msg.err)
		}
		raw.WriteString(msg.line)
		payload, ok := ssePayload(msg.line)
		if !ok {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var ev responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue // keep-alives / vendor comments
		}
		switch ev.Type {
		case "response.output_text.delta":
			markFirst()
			contentBuf.WriteString(ev.Delta)
			emit("content", &contentBuf)
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			markFirst()
			reasoningBuf.WriteString(ev.Delta)
			emit("reasoning", &reasoningBuf)
		case "response.completed", "response.incomplete":
			if len(ev.Response) == 0 {
				continue
			}
			out, derr := c.decodeResponsesBody(ev.Response, start)
			if derr != nil {
				return nil, derr
			}
			final = out
		case "response.failed", "error":
			// The provider puts the reason inside the nested response
			// object; fall back to the event-level fields when absent.
			if len(ev.Response) > 0 {
				if _, derr := c.decodeResponsesBody(ev.Response, start); derr != nil {
					return nil, derr
				}
			}
			reason := ev.Message
			if ev.Error != nil && ev.Error.Message != "" {
				reason = ev.Error.Message
			}
			if reason == "" {
				reason = "unknown error"
			}
			return nil, fmt.Errorf("responses failed: %s", reason)
		}
	}
	if final == nil {
		// No response.completed: either the endpoint ignored stream:true
		// and sent one JSON body, or the stream was truncated mid-flight.
		out, derr := c.decodeResponsesBody([]byte(raw.String()), start)
		if derr == nil {
			out.TTFT = out.Elapsed
			return out, nil
		}
		// A body that DOES decode but reports failure (status:"failed",
		// safety block) must surface its reason, not a generic message.
		if strings.Contains(derr.Error(), "responses failed") {
			return nil, derr
		}
		return nil, fmt.Errorf("流式响应结束但没有 response.completed（无法收账）")
	}
	final.Streamed = true
	if ttft == 0 {
		ttft = final.Elapsed
	}
	final.TTFT = ttft
	final.ReasoningChars = reasoningBuf.Len()
	return final, nil
}

// ssePayload strips the "data: " prefix of one SSE line.
func ssePayload(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}
