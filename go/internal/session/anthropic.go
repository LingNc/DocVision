package session

// Anthropic Messages API 适配（P4）：models.*.<条目>.api: anthropic 的
// 条目改走 POST {base_url}/messages（base_url 约定含 /v1，如
// https://api.anthropic.com/v1 或兼容网关的等价地址）。
//
// 只实现非流式：请求强制 stream=false，一次 JSON 应答。Anthropic 的
// 应答是 content block 数组（text / thinking / tool_use），这里翻译回
// 内部的 ChatMessage（Content / ReasoningContent / ToolCalls），用量
// 映射进 Usage——cache_read_input_tokens 归入 PromptTokensDetails.
// CachedTokens，会话层的前缀缓存统计、[cache-probe] 指纹因此照常工作。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicVersion pins the Messages API revision (current 2023-06-01).
const anthropicVersion = "2023-06-01"

// buildAnthropicBody translates the internal ChatRequest into an
// Anthropic /v1/messages request body, then merges request_body and the
// thinking config exactly like the OpenAI path (explicit config wins).
func (c *Client) buildAnthropicBody(payload *ChatRequest) ([]byte, error) {
	body := map[string]any{
		"model":      c.model,
		"max_tokens": payload.MaxTokens,
	}
	if payload.Temperature > 0 {
		body["temperature"] = payload.Temperature
	}
	if payload.User != "" {
		// OpenAI 的 user 路由字段对应 Anthropic 的 metadata.user_id。
		body["metadata"] = map[string]any{"user_id": payload.User}
	}
	if len(payload.Tools) > 0 {
		tools := make([]map[string]any, 0, len(payload.Tools))
		for _, t := range payload.Tools {
			fn, _ := t["function"].(map[string]any)
			if fn == nil {
				continue
			}
			tools = append(tools, map[string]any{
				"name":        fn["name"],
				"description": fn["description"],
				"input_schema": func() any {
					if s, ok := fn["parameters"]; ok && s != nil {
						return s
					}
					return map[string]any{"type": "object"}
				}(),
			})
		}
		if len(tools) > 0 {
			body["tools"] = tools
		}
	}
	// tool_choice：Anthropic 缺省即 auto；"none" 是唯一需要显式表达的。
	if choice, ok := payload.ToolChoice.(string); ok && choice == "none" {
		body["tool_choice"] = map[string]any{"type": "none"}
	}

	msgs := make([]map[string]any, 0, len(payload.Messages))
	for _, m := range payload.Messages {
		// system 消息提升为顶层 system 字段（Anthropic 不允许 system
		// 出现在 messages 里）。我们自己的会话永远以一条 system 开头。
		if m.Role == "system" {
			if s := ContentString(m); s != "" {
				body["system"] = s
			}
			continue
		}
		converted, err := anthropicMessage(m)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, converted...)
	}
	body["messages"] = msgs

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}
	if len(c.requestBody) > 0 {
		raw, err = injectRequestBody(raw, c.requestBody)
		if err != nil {
			return nil, err
		}
	}
	if c.thinking != nil {
		// GLM/Qwen 风格 thinking 配置原样透传（Anthropic 兼容网关通常
		// 自己认 thinking:{type:enabled,budget_tokens:N}）；非 thinking
		// 键（如 clear_thinking）由网关忽略。
		raw, err = injectRequestBody(raw, map[string]any{"thinking": c.thinking})
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// anthropicMessage converts one internal message to Anthropic wire
// shape. Assistant turns become text/thinking/tool_use content blocks;
// tool receipts become a user turn of tool_result blocks; user turns
// become text blocks (plus image blocks for multimodal parts).
func anthropicMessage(m ChatMessage) ([]map[string]any, error) {
	switch m.Role {
	case "assistant":
		var blocks []map[string]any
		if think := strings.TrimSpace(m.ReasoningContent); think != "" {
			// GLM 保留式思考随历史回传——Anthropic 对应 thinking block
			// （需要输出端开 thinking 的模型/网关才有意义，纯透传）。
			blocks = append(blocks, map[string]any{"type": "thinking", "thinking": think})
		}
		if s := ContentString(m); s != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": s})
		}
		for _, tc := range m.ToolCalls {
			var input any
			if tc.Function.Arguments == "" {
				input = map[string]any{}
			} else if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				// 模型生成的非法 JSON 参数按字符串原样上交（Anthropic
				// 会在服务端校验失败，行为与 OpenAI 网关一致）。
				input = tc.Function.Arguments
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}
		if len(blocks) == 0 {
			// Anthropic 不接受空 content 的助手轮——占位一个空文本块，
			// 与 OpenAI 路径上空字符串 content 的语义对齐。
			blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		}
		return []map[string]any{{"role": "assistant", "content": blocks}}, nil

	case "tool":
		s := ContentString(m)
		return []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     s,
			}},
		}}, nil

	case "user":
		var blocks []map[string]any
		switch c := m.Content.(type) {
		case string:
			if c != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": c})
			}
		case nil:
		default:
			// 多模态 parts：{type:text,text} 与 {type:image_url,
			// image_url:{url:"data:image/...;base64,..."}}。
			parts, ok := c.([]map[string]interface{})
			if !ok {
				raw, err := json.Marshal(c)
				if err != nil {
					return nil, fmt.Errorf("anthropic: encode user content: %w", err)
				}
				if err := json.Unmarshal(raw, &parts); err != nil {
					return nil, fmt.Errorf("anthropic: decode user content parts: %w", err)
				}
			}
			for _, p := range parts {
				switch p["type"] {
				case "text":
					blocks = append(blocks, map[string]any{"type": "text", "text": p["text"]})
				case "image_url":
					// image_url 的 map 类型随构造路径不同
					// （map[string]string / map[string]interface{}），
					// 统一经 JSON 归一化再取 url。
					var iu map[string]interface{}
					iuRaw, err := json.Marshal(p["image_url"])
					if err == nil {
						_ = json.Unmarshal(iuRaw, &iu)
					}
					url, _ := iu["url"].(string)
					media, data, ok := parseDataURL(url)
					if !ok {
						// 非 data: URL（http 图床等）按 Anthropic URL source 上交。
						blocks = append(blocks, map[string]any{
							"type":   "image",
							"source": map[string]any{"type": "url", "url": url},
						})
						continue
					}
					blocks = append(blocks, map[string]any{
						"type":   "image",
						"source": map[string]any{"type": "base64", "media_type": media, "data": data},
					})
				}
			}
		}
		if len(blocks) == 0 {
			blocks = append(blocks, map[string]any{"type": "text", "text": ""})
		}
		return []map[string]any{{"role": "user", "content": blocks}}, nil
	}
	return nil, fmt.Errorf("anthropic: unsupported role %q", m.Role)
}

// parseDataURL splits "data:<media>;base64,<payload>" into its parts.
func parseDataURL(url string) (media, data string, ok bool) {
	rest, found := strings.CutPrefix(url, "data:")
	if !found {
		return "", "", false
	}
	head, payload, found := strings.Cut(rest, ",")
	if !found {
		return "", "", false
	}
	media = strings.TrimSuffix(head, ";base64")
	return media, payload, media != "" && payload != ""
}

// postAnthropic performs the /messages call with the Anthropic header set.
func (c *Client) postAnthropic(raw []byte) (*http.Response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+"/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	return c.http.Do(httpReq)
}

// anthropicWireResponse is the subset of the /messages response we use.
type anthropicWireResponse struct {
	Content []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		Thinking string          `json:"thinking"`
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      *struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// decodeAnthropicResponse reads one JSON /messages response back into the
// internal ChatResponse (same shape the OpenAI decoder produces).
func (c *Client) decodeAnthropicResponse(resp *http.Response, start time.Time) (*ChatResponse, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var wire anthropicWireResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode response: %w (body=%q)", err, truncate(string(body), 256))
	}

	var msg ChatMessage
	msg.Role = "assistant"
	var texts, thinks []string
	for _, blk := range wire.Content {
		switch blk.Type {
		case "text":
			texts = append(texts, blk.Text)
		case "thinking":
			thinks = append(thinks, blk.Thinking)
		case "tool_use":
			args := string(blk.Input)
			if args == "" {
				args = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:   blk.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: blk.Name, Arguments: args},
			})
		}
	}
	msg.Content = strings.Join(texts, "")
	msg.ReasoningContent = strings.Join(thinks, "\n\n")
	out := &ChatResponse{
		Choices: []ChatResponseChoice{{Message: msg}},
		Elapsed: time.Since(start),
		TTFT:    time.Since(start), // 非流式：整体到达，无独立首字
	}
	// stop_reason → OpenAI finish_reason 词汇（会话层/转录按这个记）。
	switch wire.StopReason {
	case "tool_use":
		out.FinishReason = "tool_calls"
	case "max_tokens":
		out.FinishReason = "length"
	default:
		out.FinishReason = "stop"
	}
	if wire.Usage != nil {
		out.Usage = &Usage{
			PromptTokens:     wire.Usage.InputTokens + wire.Usage.CacheCreationInputTokens,
			CompletionTokens: wire.Usage.OutputTokens,
			TotalTokens:      wire.Usage.InputTokens + wire.Usage.CacheCreationInputTokens + wire.Usage.OutputTokens,
			PromptTokensDetails: &struct {
				CachedTokens int `json:"cached_tokens"`
			}{CachedTokens: wire.Usage.CacheReadInputTokens},
		}
	}
	return out, nil
}
