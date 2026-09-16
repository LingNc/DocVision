package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// P4 补全：OpenAI Responses API 适配的端到端测试——httptest 网关断言
// 请求路径/认证头/翻译后的体形（instructions/input items/store:false/
// include），回一个固定 output 数组，校验 decode 回内部 ChatResponse。

func TestResponsesEndToEnd(t *testing.T) {
	var gotHeaders http.Header
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1","object":"response","status":"completed","model":"gpt-test",
			"output":[
				{"type":"reasoning","id":"rs_1","encrypted_content":"ENC-CHAIN"},
				{"type":"message","id":"msg_1","role":"assistant","status":"completed",
				 "content":[{"type":"output_text","text":"looks fine"}]},
				{"type":"function_call","id":"fc_1","call_id":"call_1","name":"submit","arguments":"{\"status\":\"pass\"}"}
			],
			"incomplete_reason":null,
			"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,
				"input_tokens_details":{"cached_tokens":80}}
		}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL + "/v1", APIKey: "sk-resp-test", Model: "gpt-test",
		API: "responses", MaxTokens: 512,
	})

	req := &ChatRequest{
		Model: "gpt-test",
		Messages: []ChatMessage{
			{Role: "system", Content: "you verify chapters"},
			{Role: "user", Content: "check it"},
			{Role: "assistant", Content: "reading",
				ReasoningContent: "ENC-CHAIN-EARLIER",
				ToolCalls: []ToolCall{{ID: "call_0", Type: "function", Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read_file", Arguments: `{"path":"check:ch.md"}`}}}},
			{Role: "tool", ToolCallID: "call_0", Content: "FILE CONTENT"},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": "with pic"},
				{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64,QUJD"}},
			}},
		},
		Tools: []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "read_file",
				"description": "read a file",
				"parameters":  map[string]any{"type": "object"},
			},
		}},
		ToolChoice:  "auto",
		MaxTokens:   512,
		Temperature: 0.1,
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	// --- 请求面 ---
	if gotPath != "/v1/responses" {
		t.Errorf("path = %q, want /v1/responses", gotPath)
	}
	if gotHeaders.Get("Authorization") != "Bearer sk-resp-test" {
		t.Errorf("Authorization = %q", gotHeaders.Get("Authorization"))
	}
	if gotBody["instructions"] != "you verify chapters" {
		t.Errorf("instructions = %v", gotBody["instructions"])
	}
	if gotBody["store"] != false || gotBody["stream"] != false {
		t.Errorf("store/stream = %v / %v", gotBody["store"], gotBody["stream"])
	}
	if gotBody["max_output_tokens"] != float64(512) {
		t.Errorf("max_output_tokens = %v", gotBody["max_output_tokens"])
	}
	inc, _ := gotBody["include"].([]any)
	if len(inc) != 1 || inc[0] != "reasoning.encrypted_content" {
		t.Errorf("include = %v", gotBody["include"])
	}
	// 工具翻译：type:function、parameters 保留。
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", gotBody["tools"])
	}
	tool0 := tools[0].(map[string]any)
	if tool0["type"] != "function" || tool0["name"] != "read_file" || tool0["parameters"] == nil {
		t.Errorf("tool0 = %v", tool0)
	}
	if gotBody["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v", gotBody["tool_choice"])
	}

	// input 里没有 system，各角色翻译正确。
	input, _ := gotBody["input"].([]any)
	if len(input) != 6 { // user / reasoning / assistant-msg / function_call / function_call_output / user(text+image)
		t.Fatalf("input count = %d (%v)", len(input), gotBody["input"])
	}
	var sawReasoningReplay, sawFuncCall, sawFuncOutput, sawImage bool
	for _, it := range input {
		ii := it.(map[string]any)
		if ii["type"] == "reasoning" {
			sawReasoningReplay = true
			if ii["encrypted_content"] != "ENC-CHAIN-EARLIER" {
				t.Errorf("reasoning replay = %v", ii)
			}
		}
		if ii["type"] == "function_call" {
			sawFuncCall = true
			if ii["call_id"] != "call_0" || ii["name"] != "read_file" ||
				ii["arguments"] != `{"path":"check:ch.md"}` {
				t.Errorf("function_call = %v", ii)
			}
		}
		if ii["type"] == "function_call_output" {
			sawFuncOutput = true
			if ii["call_id"] != "call_0" || ii["output"] != "FILE CONTENT" {
				t.Errorf("function_call_output = %v", ii)
			}
		}
		if ii["role"] == "user" {
			content, _ := ii["content"].([]any)
			for _, b := range content {
				bb := b.(map[string]any)
				if bb["type"] == "input_image" && bb["image_url"] == "data:image/jpeg;base64,QUJD" {
					sawImage = true
				}
			}
		}
		if ii["role"] == "system" {
			t.Error("system role must not appear in input")
		}
	}
	if !sawReasoningReplay || !sawFuncCall || !sawFuncOutput || !sawImage {
		t.Errorf("items missing: reasoning=%v funcCall=%v funcOutput=%v image=%v",
			sawReasoningReplay, sawFuncCall, sawFuncOutput, sawImage)
	}

	// --- 响应面 ---
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.Content != "looks fine" {
		t.Errorf("content = %q", msg.Content)
	}
	// 加密思考链原样存回 ReasoningContent（回放时重新上交）。
	if msg.ReasoningContent != "ENC-CHAIN" {
		t.Errorf("reasoning = %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_1" ||
		msg.ToolCalls[0].Function.Name != "submit" || !strings.Contains(msg.ToolCalls[0].Function.Arguments, `"pass"`) {
		t.Errorf("toolcalls = %+v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].Type != "function" {
		t.Errorf("toolcall type = %q", msg.ToolCalls[0].Type)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("finish = %q", resp.FinishReason)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 100 || resp.Usage.CompletionTokens != 20 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.Usage.CachedTokens() != 80 {
		t.Errorf("cached = %d, want 80", resp.Usage.CachedTokens())
	}
	if resp.Streamed {
		t.Error("responses replies are non-streaming")
	}
}

func TestResponsesStopAndIncomplete(t *testing.T) {
	// 无工具调用的 completed → stop；incomplete_reason=max_output_tokens → length。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"status":"incomplete","incomplete_reason":"max_output_tokens",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"cut"}]}],
			"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}
		}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m", API: "responses", MaxTokens: 16})
	resp, err := c.ChatCompletion(&ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp.FinishReason != "length" {
		t.Errorf("finish = %q, want length", resp.FinishReason)
	}
	if resp.Choices[0].Message.Content != "cut" {
		t.Errorf("content = %q", resp.Choices[0].Message.Content)
	}
}

func TestResponsesFailedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"failed","error":{"message":"safety block"}}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m", API: "responses", MaxTokens: 16})
	_, err := c.ChatCompletion(&ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "safety block") {
		t.Errorf("err = %v", err)
	}
}

func TestResponsesReasoningEffortPassthrough(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	effort := "low"
	cfg := config.ModelConfig{
		BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m", API: "responses",
		MaxTokens: 16, ReasoningEffort: effort,
	}
	c := NewClient(cfg)
	if _, err := c.ChatCompletion(&ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	reasoning, _ := gotBody["reasoning"].(map[string]any)
	if reasoning["effort"] != "low" {
		t.Errorf("reasoning = %v", gotBody["reasoning"])
	}
}
