// Package chatstream assembles an OpenAI-compatible chat-completion
// server-sent-event (SSE) stream into a single assistant message.
//
// Streaming is the default request mode (see models.*.stream): it keeps
// long thinking / long output requests alive with visible progress
// instead of one silent wait, and it lets the caller see reasoning
// output as it happens. Both the session client (latex pipelines) and
// the img2text client share this collector so the wire handling exists
// in exactly one place.
package chatstream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Options tunes stream collection.
type Options struct {
	// IdleTimeout aborts the stream when no data arrives for this long.
	// 0 disables the watchdog (the HTTP layer's own timeout still applies).
	IdleTimeout time.Duration
	// OnContent receives every content delta (may be nil).
	OnContent func(string)
	// OnReasoning receives every reasoning/thinking delta (may be nil).
	OnReasoning func(string)
}

// Result is the assembled assistant message plus diagnostics that make
// the debug log useful: how much content/reasoning arrived and the
// provider-reported token usage.
type Result struct {
	// Message is the assembled assistant message:
	// {"role":"assistant","content":"...","tool_calls":[...]}.
	Message map[string]any
	// Usage is the provider usage object (nil when the provider did not
	// send one; stream_options.include_usage is requested by the clients).
	Usage map[string]any
	// ContentChars / ReasoningChars count the deltas seen.
	ContentChars   int
	ReasoningChars int
	// Chunks is the number of SSE payloads consumed.
	Chunks int
	// FinishReason is the last non-empty finish_reason (stop, tool_calls,
	// length...).
	FinishReason string
}

// toolCallAcc accumulates one streamed tool call (arguments arrive as
// fragments, keyed by index).
type toolCallAcc struct {
	id   string
	typ  string
	name string
	args strings.Builder
}

type streamPayload struct {
	Choices []struct {
		Delta struct {
			Role             string           `json:"role"`
			Content          json.RawMessage  `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			Reasoning        string           `json:"reasoning"`
			ToolCalls        []streamToolCall `json:"tool_calls"`
		} `json:"delta"`
		Message *struct {
			Role      string           `json:"role"`
			Content   json.RawMessage  `json:"content"`
			ToolCalls []streamToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]any  `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type streamToolCall struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Collect reads an SSE body until [DONE], EOF, an error payload or an
// idle timeout, and returns the assembled assistant message.
//
// The caller keeps ownership of the body and must close it: closing is
// what unblocks the internal reader goroutine after an early return.
func Collect(r io.Reader, opt Options) (*Result, error) {
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

	a := &assembler{opt: opt, tools: map[int]*toolCallAcc{}}
	var dataLines []string

	flush := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" {
			return false, nil
		}
		return a.addData(payload)
	}

	var timer *time.Timer
	var timerC <-chan time.Time
	if opt.IdleTimeout > 0 {
		timer = time.NewTimer(opt.IdleTimeout)
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
			timer.Reset(opt.IdleTimeout)
		}

		var msg lineMsg
		var ok bool
		select {
		case msg, ok = <-lines:
			if !ok {
				if _, err := flush(); err != nil {
					return nil, err
				}
				return a.result(), nil
			}
		case <-timerC:
			return nil, fmt.Errorf("stream idle timeout: no data for %s", opt.IdleTimeout)
		}
		if msg.err != nil {
			return nil, msg.err
		}

		line := strings.TrimRight(msg.line, "\r\n")
		switch {
		case line == "":
			// Event boundary: process the buffered data lines.
			done, err := flush()
			if err != nil {
				return nil, err
			}
			if done {
				return a.result(), nil
			}
		case strings.HasPrefix(line, ":"):
			// SSE comment / keep-alive.
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		default:
			// event:, id:, retry: and any other field are ignored.
		}
	}
}

// assembler accumulates the deltas of one stream.
type assembler struct {
	content        strings.Builder
	reasoning      strings.Builder // 思维链全文：回传历史用（保留式思考/缓存命中）
	tools          map[int]*toolCallAcc
	toolOrder      []int
	usage          map[string]any
	finish         string
	chunks         int
	reasoningChars int
	opt            Options
}

func (a *assembler) addData(payload string) (bool, error) {
	if payload == "[DONE]" {
		return true, nil
	}
	if !strings.HasPrefix(payload, "{") {
		// Not JSON (some gateways emit bare keep-alives): ignore.
		return false, nil
	}
	var p streamPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return false, fmt.Errorf("decode stream chunk: %w (chunk=%q)", err, truncate(payload, 200))
	}
	a.chunks++

	if len(p.Error) > 0 && string(p.Error) != "null" {
		return false, fmt.Errorf("stream error: %s", truncate(string(p.Error), 300))
	}
	if p.Usage != nil {
		a.usage = p.Usage
	}
	for i := range p.Choices {
		ch := &p.Choices[i]
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			a.finish = *ch.FinishReason
		}
		if ch.Message != nil {
			// Some gateways send the full message instead of deltas.
			a.addContent(ch.Message.Content)
			a.addToolCalls(ch.Message.ToolCalls)
			continue
		}
		a.addContent(ch.Delta.Content)
		if ch.Delta.ReasoningContent != "" {
			a.addReasoning(ch.Delta.ReasoningContent)
		}
		if ch.Delta.Reasoning != "" {
			a.addReasoning(ch.Delta.Reasoning)
		}
		a.addToolCalls(ch.Delta.ToolCalls)
	}
	return false, nil
}

func (a *assembler) addContent(raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return
		}
		a.content.WriteString(s)
		if a.opt.OnContent != nil {
			a.opt.OnContent(s)
		}
		return
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err == nil {
		for _, part := range parts {
			if t, _ := part["type"].(string); t == "text" {
				if txt, ok := part["text"].(string); ok && txt != "" {
					a.content.WriteString(txt)
					if a.opt.OnContent != nil {
						a.opt.OnContent(txt)
					}
				}
			}
		}
	}
}

func (a *assembler) addReasoning(text string) {
	if text == "" {
		return
	}
	a.reasoning.WriteString(text)
	a.reasoningChars += len(text)
	if a.opt.OnReasoning != nil {
		a.opt.OnReasoning(text)
	}
}

func (a *assembler) addToolCalls(calls []streamToolCall) {
	for _, tc := range calls {
		idx := len(a.tools)
		if tc.Index != nil {
			idx = *tc.Index
		}
		acc, ok := a.tools[idx]
		if !ok {
			acc = &toolCallAcc{}
			a.tools[idx] = acc
			a.toolOrder = append(a.toolOrder, idx)
		}
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Type != "" {
			acc.typ = tc.Type
		}
		if tc.Function.Name != "" {
			acc.name = tc.Function.Name
		}
		acc.args.WriteString(tc.Function.Arguments)
	}
}

func (a *assembler) result() *Result {
	msg := map[string]any{"role": "assistant", "content": a.content.String()}
	if a.reasoning.Len() > 0 {
		// 思维链随 assistant 消息存回历史（GLM 保留式思考要求完整回传，
		// 同时保证前缀逐字节一致以提高 prompt cache 命中）。
		msg["reasoning_content"] = a.reasoning.String()
	}
	if len(a.toolOrder) > 0 {
		calls := make([]map[string]any, 0, len(a.toolOrder))
		for _, idx := range a.toolOrder {
			acc := a.tools[idx]
			typ := acc.typ
			if typ == "" {
				typ = "function"
			}
			calls = append(calls, map[string]any{
				"id":   acc.id,
				"type": typ,
				"function": map[string]any{
					"name":      acc.name,
					"arguments": acc.args.String(),
				},
			})
		}
		msg["tool_calls"] = calls
	}
	return &Result{
		Message:        msg,
		Usage:          a.usage,
		ContentChars:   a.content.Len(),
		ReasoningChars: a.reasoningChars,
		Chunks:         a.chunks,
		FinishReason:   a.finish,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
