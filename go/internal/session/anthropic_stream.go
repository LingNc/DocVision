package session

// T270：Anthropic Messages API 的 SSE 流式读取。
//
// 之前 anthropic 条目被强制 stream=false（一次 JSON 应答）：思考型模型动辄
// 静默几分钟，转录里看不到任何进行中的字符，超时判据也只能吃总时长。这里按
// Anthropic 的事件模型把一条流拼回同一个 ChatMessage：
//
//	message_start        用量（input/cache 两个基数）
//	content_block_start  开一块：text / thinking / tool_use（带 id、name）
//	content_block_delta  text_delta / thinking_delta / input_json_delta（工具入参分片）
//	message_delta        stop_reason 与 output_tokens
//	message_stop         结束
//
// 语义与 OpenAI 那条流保持一致：返回值同样是 ChatResponse（Streamed=true、
// TTFT=首个增量到达耗时），渐进快照通过同一个 hook 回调（content/reasoning
// 两个 phase，传的是"已累积内容的尾巴"）。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicToolUse 是流里拼出来的一次工具调用。
type anthropicToolUse struct {
	ID   string
	Name string
	Args strings.Builder
}

// anthropicStreamResult 是一条流攒出来的全部事实。
type anthropicStreamResult struct {
	Text           string
	Thinking       string
	ToolUses       []anthropicToolUse
	StopReason     string
	InputTokens    int
	CacheRead      int
	CacheCreation  int
	OutputTokens   int
	ContentChars   int
	ReasoningChars int
}

// readAnthropicStream 读 SSE 应答并转成内部 ChatResponse。
func (c *Client) readAnthropicStream(resp *http.Response, start time.Time, hook func(phase, text string)) (*ChatResponse, error) {
	defer resp.Body.Close()
	res, err := collectAnthropicStream(resp.Body, c.streamIdle, hook)
	if err != nil {
		return nil, err
	}
	var msg ChatMessage
	msg.Role = "assistant"
	msg.Content = res.Text
	msg.ReasoningContent = res.Thinking
	for _, tu := range res.ToolUses {
		args := tu.Args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:   tu.ID,
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: tu.Name, Arguments: args},
		})
	}
	out := &ChatResponse{
		Choices:        []ChatResponseChoice{{Message: msg}},
		Elapsed:        time.Since(start),
		Streamed:       true,
		ReasoningChars: res.ReasoningChars,
	}
	switch res.StopReason {
	case "tool_use":
		out.FinishReason = "tool_calls"
	case "max_tokens":
		out.FinishReason = "length"
	default:
		out.FinishReason = "stop"
	}
	if res.InputTokens > 0 || res.OutputTokens > 0 {
		prompt := res.InputTokens + res.CacheCreation
		out.Usage = &Usage{
			PromptTokens:     prompt,
			CompletionTokens: res.OutputTokens,
			TotalTokens:      prompt + res.OutputTokens,
			PromptTokensDetails: &struct {
				CachedTokens int `json:"cached_tokens"`
			}{CachedTokens: res.CacheRead},
		}
	}
	return out, nil
}

// collectAnthropicStream 是纯读取器（不碰 http.Response），单测直接喂字节。
func collectAnthropicStream(r io.Reader, idle time.Duration, hook func(phase, text string)) (*anthropicStreamResult, error) {
	type lineMsg struct {
		line string
		err  error
	}
	lines := make(chan lineMsg, 64)
	stop := make(chan struct{})
	go func() {
		defer close(lines)
		br := bufio.NewReaderSize(r, 64*1024)
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
	defer close(stop)

	res := &anthropicStreamResult{}
	var texts, thinks strings.Builder
	tools := map[int]*anthropicToolUse{}
	var order []int

	// 渐进快照：150ms 一次、只回传尾巴 16KB（与 OpenAI 那条流同一口径）。
	var lastHook time.Time
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

	var timer *time.Timer
	var timerC <-chan time.Time
	if idle > 0 {
		timer = time.NewTimer(idle)
		timerC = timer.C
		defer timer.Stop()
	}

	for {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		}
		var msg lineMsg
		var ok bool
		select {
		case msg, ok = <-lines:
			if !ok {
				return res, nil
			}
		case <-timerC:
			return nil, fmt.Errorf("stream idle timeout: no data for %s", idle)
		}
		if msg.err != nil {
			return nil, msg.err
		}
		line := strings.TrimRight(msg.line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue // event: / ping / 注释行都不带数据
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if err := applyAnthropicEvent(payload, res, &texts, &thinks, tools, &order); err != nil {
			return nil, err
		}
		// 每个事件后都把累积内容推给 hook（内部自己节流）。
		emit("content", &texts)
		emit("reasoning", &thinks)
	}
}

// applyAnthropicEvent 消化一个 data 事件。
func applyAnthropicEvent(payload string, res *anthropicStreamResult, texts, thinks *strings.Builder, tools map[int]*anthropicToolUse, order *[]int) error {
	var ev struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		Message *struct {
			Usage *struct {
				InputTokens              int `json:"input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				OutputTokens             int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
		ContentBlock *struct {
			Type  string `json:"type"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Text  string `json:"text"`
			Think string `json:"thinking"`
		} `json:"content_block"`
		Delta *struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			PartialJSON string `json:"partial_json"`
			StopReason  string `json:"stop_reason"`
		} `json:"delta"`
		Usage *struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		// 单条坏事件不该毁掉整条流：Anthropic 的心跳/扩展字段都可能出现。
		return nil
	}
	switch ev.Type {
	case "error":
		if ev.Error != nil {
			return fmt.Errorf("anthropic stream error: %s: %s", ev.Error.Type, ev.Error.Message)
		}
		return fmt.Errorf("anthropic stream error: %s", payload)
	case "message_start":
		if ev.Message != nil && ev.Message.Usage != nil {
			u := ev.Message.Usage
			res.InputTokens, res.CacheRead, res.CacheCreation = u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens
			if u.OutputTokens > 0 {
				res.OutputTokens = u.OutputTokens
			}
		}
	case "content_block_start":
		if ev.ContentBlock == nil {
			return nil
		}
		switch ev.ContentBlock.Type {
		case "text":
			if ev.ContentBlock.Text != "" {
				texts.WriteString(ev.ContentBlock.Text)
				res.ContentChars += len(ev.ContentBlock.Text)
			}
		case "thinking":
			if ev.ContentBlock.Think != "" {
				thinks.WriteString(ev.ContentBlock.Think)
				res.ReasoningChars += len(ev.ContentBlock.Think)
			}
		case "tool_use":
			tu := &anthropicToolUse{ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}
			tools[ev.Index] = tu
			*order = append(*order, ev.Index)
		}
	case "content_block_delta":
		if ev.Delta == nil {
			return nil
		}
		switch ev.Delta.Type {
		case "text_delta":
			texts.WriteString(ev.Delta.Text)
			res.ContentChars += len(ev.Delta.Text)
		case "thinking_delta":
			thinks.WriteString(ev.Delta.Thinking)
			res.ReasoningChars += len(ev.Delta.Thinking)
		case "input_json_delta":
			if tu := tools[ev.Index]; tu != nil {
				tu.Args.WriteString(ev.Delta.PartialJSON)
			}
		}
	case "message_delta":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			res.StopReason = ev.Delta.StopReason
		}
		if ev.Usage != nil && ev.Usage.OutputTokens > 0 {
			res.OutputTokens = ev.Usage.OutputTokens
		}
	}
	res.Text = texts.String()
	res.Thinking = thinks.String()
	res.ToolUses = res.ToolUses[:0]
	for _, idx := range *order {
		if tu := tools[idx]; tu != nil {
			res.ToolUses = append(res.ToolUses, *tu)
		}
	}
	return nil
}
