package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// T270：Anthropic Messages API 的 SSE 流式读取——之前 anthropic 条目被强制
// stream=false，思考型模型整段静默。这里按 Anthropic 的事件模型拼回同一条
// ChatMessage，并钉住"网关不认流式时退回 JSON"这条退路。
func TestAnthropicStreamingEndToEnd(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		e := func(v string) { fmt.Fprintf(w, "event: x\ndata: %s\n\n", v) }
		e(`{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_read_input_tokens":80,"cache_creation_input_tokens":5,"output_tokens":1}}}`)
		e(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
		e(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"ponder"}}`)
		e(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"ing"}}`)
		e(`{"type":"content_block_stop","index":0}`)
		e(`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
		e(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hello"}}`)
		e(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":" world"}}`)
		e(`{"type":"content_block_stop","index":1}`)
		e(`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_9","name":"submit"}}`)
		e(`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"status\":"}}`)
		e(`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"pass\"}"}}`)
		e(`{"type":"content_block_stop","index":2}`)
		e(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}`)
		e(`{"type":"message_stop"}`)
	}))
	defer srv.Close()

	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL + "/v1", APIKey: "sk-ant-test", Model: "claude-test-1",
		API: "anthropic", MaxTokens: 512,
	})
	var hooked []string
	resp, err := c.ChatCompletion(&ChatRequest{
		Model:     "claude-test-1",
		Messages:  []ChatMessage{{Role: "user", Content: "go"}},
		MaxTokens: 512,
		StreamHook: func(phase, text string) {
			hooked = append(hooked, phase+":"+text)
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if gotBody["stream"] != true {
		t.Errorf("流式请求体应带 stream:true，got %v", gotBody["stream"])
	}
	msg := resp.Choices[0].Message
	if msg.Content != "hello world" {
		t.Errorf("content = %q", msg.Content)
	}
	if msg.ReasoningContent != "pondering" {
		t.Errorf("reasoning = %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_9" ||
		msg.ToolCalls[0].Function.Name != "submit" ||
		!strings.Contains(msg.ToolCalls[0].Function.Arguments, `"pass"`) {
		t.Errorf("toolcalls = %+v", msg.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" || !resp.Streamed {
		t.Errorf("finish/streamed = %q/%v", resp.FinishReason, resp.Streamed)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 105 ||
		resp.Usage.CompletionTokens != 42 || resp.Usage.CachedTokens() != 80 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(hooked) == 0 {
		t.Error("流式快照 hook 一次都没被调用")
	}
}

// 网关忽略 stream:true、直接回一份 JSON：仍按非流式解析，不能当成空流。
func TestAnthropicStreamIgnoresJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"plain"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m", API: "anthropic"})
	resp, err := c.ChatCompletion(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Choices[0].Message.Content; got != "plain" {
		t.Errorf("content = %q，期望回退到 JSON 解析", got)
	}
}

// T270：Anthropic 没有 reasoning_effort——effort 必须落进 output_config.effort，
// 否则新 Claude 每个请求都 400（requires adaptive thinking / output_config.effort）。
func TestAnthropicOutputConfigEffort(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{
		BaseURL:         srv.URL + "/v1",
		APIKey:          "k",
		Model:           "claude-haiku-test",
		API:             "anthropic",
		Thinking:        map[string]any{"type": "adaptive"},
		ReasoningEffort: "high",
		RequestBody:     map[string]any{"output_config": map[string]any{"format": "json"}},
	})
	if _, err := c.ChatCompletion(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}, Stream: false}); err != nil {
		t.Fatal(err)
	}
	th, _ := gotBody["thinking"].(map[string]any)
	if th["type"] != "adaptive" {
		t.Errorf("thinking = %v，期望自适应透传", gotBody["thinking"])
	}
	if _, has := gotBody["reasoning_effort"]; has {
		t.Error("anthropic 请求体不该出现 reasoning_effort")
	}
	oc, _ := gotBody["output_config"].(map[string]any)
	if oc["effort"] != "high" {
		t.Errorf("output_config = %v，期望 effort=high", gotBody["output_config"])
	}
	if oc["format"] != "json" {
		t.Errorf("request_body 里已有的 output_config 键被覆盖了：%v", oc)
	}
}
