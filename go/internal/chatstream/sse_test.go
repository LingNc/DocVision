package chatstream

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestCollectAssemblesContentReasoningToolsAndUsage(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","reasoning_content":"think "}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":"more"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"Hello "}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"world"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"view_image","arguments":"{\"path\":"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.jpg\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"completion_tokens_details":{"reasoning_tokens":7}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var content, reasoning strings.Builder
	res, err := Collect(strings.NewReader(stream), Options{
		OnContent:   func(s string) { content.WriteString(s) },
		OnReasoning: func(s string) { reasoning.WriteString(s) },
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := res.Message["content"]; got != "Hello world" {
		t.Errorf("content = %v, want Hello world", got)
	}
	if content.String() != "Hello world" {
		t.Errorf("OnContent saw %q", content.String())
	}
	if reasoning.String() != "think more" {
		t.Errorf("OnReasoning saw %q", reasoning.String())
	}
	if res.ReasoningChars != len("think more") {
		t.Errorf("ReasoningChars = %d", res.ReasoningChars)
	}
	if res.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q", res.FinishReason)
	}
	calls, ok := res.Message["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls = %#v", res.Message["tool_calls"])
	}
	fn := calls[0]["function"].(map[string]any)
	if fn["name"] != "view_image" || fn["arguments"] != `{"path":"a.jpg"}` {
		t.Errorf("assembled tool call = %#v", fn)
	}
	if res.Usage == nil || res.Usage["total_tokens"] != float64(30) {
		t.Errorf("usage = %#v", res.Usage)
	}
	if res.Chunks != 7 {
		t.Errorf("Chunks = %d, want 7", res.Chunks)
	}
}

func TestCollectIgnoresCommentsAndNonJSONKeepalives(t *testing.T) {
	stream := ": ping\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: keep-alive\n\ndata: [DONE]\n\n"
	res, err := Collect(strings.NewReader(stream), Options{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if res.Message["content"] != "ok" {
		t.Errorf("content = %v", res.Message["content"])
	}
}

func TestCollectReturnsErrorPayload(t *testing.T) {
	stream := "data: {\"error\":{\"message\":\"bad_response_body\"}}\n\n"
	if _, err := Collect(strings.NewReader(stream), Options{}); err == nil {
		t.Fatal("expected error payload to fail the stream")
	}
}

func TestCollectIdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Collect(pr, Options{IdleTimeout: 50 * time.Millisecond})
		done <- err
	}()
	// Write one chunk then stay silent: the watchdog must fire.
	if _, err := pw.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "idle timeout") {
			t.Fatalf("err = %v, want idle timeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Collect did not return on idle timeout")
	}
}
