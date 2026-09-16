package session

// OpenAI Responses API 适配（P4 补全）：models.*.<条目>.api: responses 的
// 条目改走 POST {base_url}/responses（base_url 约定含 /v1，如
// https://api.openai.com/v1）。gpt-5 / o 系等新模型只开在这个协议上。
//
// 与 anthropic 线路相同的取舍：只实现非流式（一次 JSON 应答），流式/
// 实时快照专属能力不适用。Responses API 有状态（store/previous_response_id）
// 而我们每次回放慢整个历史，因此固定 store:false + 全量 input。
//
// 思考链的处理：请求 include:["reasoning.encrypted_content"]，decode 时
// 把 encrypted_content 存进 ReasoningContent（不透明 token，回放时原样
// 交回）；没有 encrypted_content 时退回 summary 文本。注意加密思考有
// 有效期（约半小时级），长间隔续跑可能失效——这是 Responses API 的固
// 有约束，摘要不受影响。

import (
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
		"stream": false,
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

// postResponses performs the /responses call (OpenAI bearer auth).
func (c *Client) postResponses(raw []byte) (*http.Response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+"/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	return c.http.Do(httpReq)
}

// responsesWireResponse is the subset of the /responses response we use.
type responsesWireResponse struct {
	Status     string `json:"status"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error"`
	Output           []map[string]json.RawMessage `json:"output"`
	IncompleteReason string                       `json:"incomplete_reason"`
	Usage            *struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// decodeResponsesResponse reads one JSON /responses response back into
// the internal ChatResponse.
func (c *Client) decodeResponsesResponse(resp *http.Response, start time.Time) (*ChatResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
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
	} else if wire.IncompleteReason == "max_output_tokens" {
		finish = "length"
	}
	out := &ChatResponse{
		Choices: []ChatResponseChoice{{Message: msg}},
		Elapsed: time.Since(start),
		TTFT:    time.Since(start), // 非流式：整体到达
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
