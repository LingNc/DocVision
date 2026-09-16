package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// P4：Anthropic Messages API 适配的端到端测试——httptest 网关断言
// 请求方法/路径/头与翻译后的体形，回一个固定 content block 应答，
// 校验 decode 回内部 ChatResponse 的每个通道。

func anthropicTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := config.ModelConfig{
		BaseURL:   srv.URL + "/v1",
		APIKey:    "sk-ant-test",
		Model:     "claude-test-1",
		API:       "anthropic",
		MaxTokens: 1024,
	}
	return NewClient(cfg), srv
}

func TestAnthropicEndToEnd(t *testing.T) {
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
			"id":"msg_1","type":"message","role":"assistant","model":"claude-test-1",
			"content":[
				{"type":"thinking","thinking":"let me check"},
				{"type":"text","text":"looks fine"},
				{"type":"tool_use","id":"call_1","name":"submit","input":{"status":"pass"}}
			],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":100,"output_tokens":20,
				"cache_read_input_tokens":80,"cache_creation_input_tokens":5}
		}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL + "/v1", APIKey: "sk-ant-test", Model: "claude-test-1",
		API: "anthropic", MaxTokens: 512,
	})

	req := &ChatRequest{
		Model: "claude-test-1",
		Messages: []ChatMessage{
			{Role: "system", Content: "you verify chapters"},
			{Role: "user", Content: "check it"},
			{Role: "assistant", Content: "reading",
				ReasoningContent: "earlier thought",
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
		User:        "sess-1",
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	// --- 请求面 ---
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotHeaders.Get("x-api-key") != "sk-ant-test" {
		t.Errorf("x-api-key header missing/wrong: %q", gotHeaders.Get("x-api-key"))
	}
	if gotHeaders.Get("anthropic-version") == "" {
		t.Error("anthropic-version header missing")
	}
	if gotHeaders.Get("Authorization") != "" {
		t.Error("Authorization header should NOT be sent to Anthropic")
	}
	if gotBody["system"] != "you verify chapters" {
		t.Errorf("system = %v", gotBody["system"])
	}
	if gotBody["model"] != "claude-test-1" || gotBody["stream"] != nil {
		t.Errorf("model/stream = %v / %v", gotBody["model"], gotBody["stream"])
	}
	if gotBody["max_tokens"] != float64(512) {
		t.Errorf("max_tokens = %v", gotBody["max_tokens"])
	}
	meta, _ := gotBody["metadata"].(map[string]any)
	if meta["user_id"] != "sess-1" {
		t.Errorf("metadata.user_id = %v", meta["user_id"])
	}
	// 工具翻译：input_schema。
	tools, _ := gotBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", gotBody["tools"])
	}
	tool0 := tools[0].(map[string]any)
	if tool0["name"] != "read_file" || tool0["input_schema"] == nil {
		t.Errorf("tool0 = %v", tool0)
	}
	// 消息翻译：messages 里不能再有 system。
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("messages count = %d (%v)", len(msgs), gotBody["messages"])
	}
	var sawThinking, sawToolUse, sawToolResult, sawImage bool
	for _, m := range msgs {
		mm := m.(map[string]any)
		if mm["role"] == "system" {
			t.Error("system role must not appear in messages")
		}
		blocks, _ := mm["content"].([]any)
		for _, b := range blocks {
			bb := b.(map[string]any)
			switch bb["type"] {
			case "thinking":
				sawThinking = true
			case "tool_use":
				sawToolUse = true
				if bb["id"] != "call_0" || bb["name"] != "read_file" {
					t.Errorf("tool_use block = %v", bb)
				}
				in, _ := bb["input"].(map[string]any)
				if in["path"] != "check:ch.md" {
					t.Errorf("tool_use input = %v", in)
				}
			case "tool_result":
				sawToolResult = true
				if bb["tool_use_id"] != "call_0" || bb["content"] != "FILE CONTENT" {
					t.Errorf("tool_result block = %v", bb)
				}
			case "image":
				sawImage = true
				src, _ := bb["source"].(map[string]any)
				if src["type"] != "base64" || src["media_type"] != "image/jpeg" || src["data"] != "QUJD" {
					t.Errorf("image source = %v", src)
				}
			}
		}
	}
	if !sawThinking || !sawToolUse || !sawToolResult || !sawImage {
		t.Errorf("blocks missing: thinking=%v toolUse=%v toolResult=%v image=%v",
			sawThinking, sawToolUse, sawToolResult, sawImage)
	}

	// --- 响应面 ---
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.Content != "looks fine" {
		t.Errorf("content = %q", msg.Content)
	}
	if msg.ReasoningContent != "let me check" {
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
	if resp.Usage == nil || resp.Usage.PromptTokens != 105 || resp.Usage.CompletionTokens != 20 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.Usage.CachedTokens() != 80 {
		t.Errorf("cached = %d, want 80", resp.Usage.CachedTokens())
	}
	if resp.Streamed {
		t.Error("anthropic replies are non-streaming")
	}
}

func TestAnthropicToolChoiceNone(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", API: "anthropic", MaxTokens: 16})
	req := &ChatRequest{
		Model:      "m",
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
		ToolChoice: "none",
	}
	if _, err := c.ChatCompletion(req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	tc, _ := gotBody["tool_choice"].(map[string]any)
	if tc["type"] != "none" {
		t.Errorf("tool_choice = %v", gotBody["tool_choice"])
	}
}

func TestAnthropicErrorAndRetryable(t *testing.T) {
	// HTTP 错误必须走与 OpenAI 路径相同的 "HTTP %d: body" 形式，
	// CallWithRetry 的 4xx 不重试分类才成立。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key"}}`))
	}))
	defer srv.Close()
	c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", API: "anthropic", MaxTokens: 16})
	start := time.Now()
	_, sentinel, status := c.CallWithRetry(&ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if status != "error" || !strings.Contains(sentinel, "HTTP 401") {
		t.Errorf("sentinel=%q status=%q", sentinel, status)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("4xx must not be retried")
	}
}

func TestAnthropicInheritsAPIFromText(t *testing.T) {
	// models.*.<条目> 没写 api 时继承 models.text（ResolveModel 零值回落）。
	cfg := &config.Config{}
	cfg.Models = map[string]config.ModelConfig{
		"text":    {BaseURL: "https://api.anthropic.com/v1", APIKey: "k", Model: "claude-x", API: "anthropic"},
		"checker": {Model: "claude-checker"},
	}
	resolved, ok := cfg.ResolveModel("checker")
	if !ok {
		t.Fatal("ResolveModel(checker) not found")
	}
	if resolved.API != "anthropic" {
		t.Errorf("inherited api = %q", resolved.API)
	}
	if resolved.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("inherited base_url = %q", resolved.BaseURL)
	}
	c := NewClient(resolved)
	if !c.Anthropic() {
		t.Error("client should be anthropic after inheritance")
	}
}
