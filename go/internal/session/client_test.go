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

// TestClientStreamsChatCompletion checks the default transport: SSE is
// requested, stream_options asks for usage, the vendor thinking /
// reasoning_effort fields go to the TOP level of the body, and the
// assembled reply carries content + tool call + usage.
func TestClientStreamsChatCompletion(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"f\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		Thinking:        map[string]any{"type": "disabled"},
		ReasoningEffort: "high",
	})
	resp, err := c.ChatCompletion(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
	if opts, ok := body["stream_options"].(map[string]any); !ok || opts["include_usage"] != true {
		t.Errorf("stream_options = %#v", body["stream_options"])
	}
	if th, ok := body["thinking"].(map[string]any); !ok || th["type"] != "disabled" {
		t.Errorf("thinking must be top-level: %#v", body["thinking"])
	}
	if body["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v", body["reasoning_effort"])
	}
	if _, nested := body["extra_body"]; nested {
		t.Error("extra_body must not be sent on the wire")
	}
	if !resp.Streamed {
		t.Error("response should be marked streamed")
	}
	msg := resp.Choices[0].Message
	if ContentString(msg) != "hi" {
		t.Errorf("content = %q", ContentString(msg))
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "f" {
		t.Errorf("tool calls = %#v", msg.ToolCalls)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Errorf("usage = %#v", resp.Usage)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("finish = %q", resp.FinishReason)
	}
}

// TestClientFallsBackWhenStreamUnsupported: a gateway that rejects SSE
// gets exactly one non-streaming retry instead of burning the budget.
func TestClientFallsBackWhenStreamUnsupported(t *testing.T) {
	var streams []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		stream, _ := m["stream"].(bool)
		streams = append(streams, stream)
		if stream {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"message":"stream is not supported"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"plain"}}]}`)
	}))
	defer srv.Close()

	c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	resp, err := c.ChatCompletion(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if ContentString(resp.Choices[0].Message) != "plain" {
		t.Errorf("content = %q", ContentString(resp.Choices[0].Message))
	}
	if len(streams) != 2 || !streams[0] || streams[1] {
		t.Errorf("request modes = %v, want [true false]", streams)
	}
	if resp.Streamed {
		t.Error("fallback reply must not be marked streamed")
	}
}

// TestClientNonStreamingWhenConfigured: stream: false stays a single
// JSON round-trip.
func TestClientNonStreamingWhenConfigured(t *testing.T) {
	off := false
	var gotStream bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotStream = strings.Contains(string(raw), `"stream":true`)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off})
	if _, err := c.ChatCompletion(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}}); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if gotStream {
		t.Error("stream:false must not request a stream")
	}
}

func TestRequestSummaryMentionsThinking(t *testing.T) {
	c := NewClient(config.ModelConfig{
		BaseURL: "http://x", APIKey: "k", Model: "m",
		Thinking: map[string]any{"type": "enabled"}, ReasoningEffort: "max",
	})
	got := c.RequestSummary(&ChatRequest{MaxTokens: 10})
	for _, want := range []string{"model=m", "stream=true", "max_tokens=10", "thinking=enabled", "reasoning_effort=max"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
}
