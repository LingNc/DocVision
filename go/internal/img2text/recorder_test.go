package img2text

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// T37: the exchange recorder reconstructs the wire transcript from a
// monotonically growing conversation: message cursor + reply + usage.
func TestExchangeRecorder_ReconstructsTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions", "ch1.md", "abc123.jpg.jsonl")
	rec, err := NewExchangeRecorder(path, "ch1.md::images/abc123.jpg", "model-x", "SYS")
	if err != nil {
		t.Fatalf("NewExchangeRecorder: %v", err)
	}

	user := ChatMessage{Role: "user", Content: "hello"}
	req1 := &ChatRequest{Model: "model-x", Messages: []ChatMessage{
		{Role: "system", Content: "SYS"}, user,
	}}
	resp1 := &ChatResponse{
		Choices: []ChatResponseChoice{{Message: ChatMessage{Role: "assistant", Content: "bad mermaid"}}},
		Usage:   &Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		Elapsed: 2 * time.Second,
	}
	rec.Record(req1, resp1)

	// Second call replays the history and appends a fix user turn: the
	// recorder must persist ONLY the tail (the fix turn), not re-write
	// the first exchange.
	fixTurn := ChatMessage{Role: "user", Content: "fix it"}
	req2 := &ChatRequest{Model: "model-x", Messages: append(append([]ChatMessage{},
		req1.Messages...), choiceMessages(resp1)[0], fixTurn)}
	resp2 := &ChatResponse{
		Choices: []ChatResponseChoice{{Message: ChatMessage{Role: "assistant", Content: "fixed"}}},
		Usage:   &Usage{PromptTokens: 20, CompletionTokens: 6, TotalTokens: 26},
	}
	rec.Record(req2, resp2)
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	// meta + (system, user, assistant) + (fix user, assistant) + 2 usage
	if len(lines) != 9 {
		t.Fatalf("lines = %d, want 9:\n%s", len(lines), raw)
	}
	if !strings.Contains(lines[0], `"t":"meta"`) {
		t.Errorf("line0 not meta: %s", lines[0])
	}
	// The user turn appears exactly once (no duplication on the second call).
	if n := strings.Count(string(raw), `"role":"user"`); n != 2 {
		t.Errorf("user roles = %d, want 2 (initial + fix)", n)
	}
	if !strings.Contains(string(raw), `"role":"tool"`) && !strings.Contains(string(raw), "fix it") {
		t.Errorf("fix turn missing from transcript")
	}
	if n := strings.Count(string(raw), `"t":"usage"`); n != 2 {
		t.Errorf("usage lines = %d, want 2", n)
	}
}

// choiceMessages mirrors how the processor echoes the assistant reply.
func choiceMessages(r *ChatResponse) []ChatMessage {
	out := make([]ChatMessage, len(r.Choices))
	for i, c := range r.Choices {
		out[i] = c.Message
	}
	return out
}
