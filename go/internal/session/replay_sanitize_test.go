package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// toolCall builds one assistant tool call for the fixtures below.
func toolCall(id, name, args string) ToolCall {
	tc := ToolCall{ID: id, Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

// TestSanitizeReplayFixesMalformedToolArguments pins the fix for the
// 400-loop: a replayed assistant tool call whose arguments are not JSON
// (streamed output truncated) is replayed as `{}` and its tool result is
// rewritten, so the resumed request is wire-valid instead of being
// rejected upstream forever.
func TestSanitizeReplayFixesMalformedToolArguments(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "verify"},
		{Role: "assistant", Content: "\n\n", ToolCalls: []ToolCall{
			toolCall("c1", "read_file", `{"path": "a.md"`),
			toolCall("c2", "read_file", `{"path": "b.tex"}`),
		}},
		{Role: "tool", ToolCallID: "c1", Content: "TOOL ERROR: JSON 对象未闭合"},
		{Role: "tool", ToolCallID: "c2", Content: "[b.tex, 10 bytes]\nbody"},
	}
	out, dropped, repaired := sanitizeReplayMessages(msgs)
	if dropped != 0 || repaired != 1 {
		t.Fatalf("dropped=%d repaired=%d, want 0/1", dropped, repaired)
	}
	if got := out[1].ToolCalls[0].Function.Arguments; got != "{}" {
		t.Errorf("malformed args = %q, want {}", got)
	}
	if got := out[1].ToolCalls[1].Function.Arguments; got != `{"path": "b.tex"}` {
		t.Errorf("valid args must be untouched, got %q", got)
	}
	if out[2].Content != replayRepairNote {
		t.Errorf("tool result for the repaired call = %q", out[2].Content)
	}
	if out[3].Content != "[b.tex, 10 bytes]\nbody" {
		t.Errorf("unrelated tool result must be untouched, got %q", out[3].Content)
	}
}

// TestSanitizeReplayDropsEmptyAssistantMessages: a max_tokens-truncated
// empty reply carries no information and may be rejected by the provider;
// it must not be replayed.
func TestSanitizeReplayDropsEmptyAssistantMessages(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "verify"},
		{Role: "assistant", Content: "\n", ReasoningContent: "thinking…"},
		{Role: "user", Content: "Continue."},
		{Role: "assistant", Content: ""},
	}
	out, dropped, repaired := sanitizeReplayMessages(msgs)
	if dropped != 2 || repaired != 0 {
		t.Fatalf("dropped=%d repaired=%d, want 2/0", dropped, repaired)
	}
	if len(out) != 2 {
		t.Fatalf("kept %d messages, want 2: %+v", len(out), out)
	}
	for _, m := range out {
		if m.Role == "assistant" {
			t.Fatalf("an empty assistant message survived: %+v", m)
		}
	}
}

// TestSanitizeReplayKeepsCleanHistory: a healthy history must pass through
// byte-identical (no accidental drops of real answers).
func TestSanitizeReplayKeepsCleanHistory(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "verify"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{
			toolCall("c1", "read_file", `{"path":"a.md"}`),
		}},
		{Role: "tool", ToolCallID: "c1", Content: "file body"},
		{Role: "assistant", Content: "verdict: pass"},
	}
	out, dropped, repaired := sanitizeReplayMessages(msgs)
	if dropped != 0 || repaired != 0 || len(out) != len(msgs) {
		t.Fatalf("clean history changed: dropped=%d repaired=%d len=%d", dropped, repaired, len(out))
	}
	if ContentString(out[3]) != "verdict: pass" {
		t.Errorf("final answer lost: %q", ContentString(out[3]))
	}
}

// TestBudgetCeilingIsWindowAware: the retry boost must never exceed what
// the window can still hold (a larger max_tokens would 400).
func TestBudgetCeilingIsWindowAware(t *testing.T) {
	s := &Session{}
	if got := s.budgetCeiling(16384); got != 16384 {
		t.Errorf("no window configured: got %d, want 16384", got)
	}
	s.tuning.ContextLimit = 32768
	s.messages = []ChatMessage{{Role: "user", Content: strings.Repeat("x", 4*1024)}}
	got := s.budgetCeiling(65536)
	if got <= 0 || got >= 65536 {
		t.Fatalf("must clamp to the free window, got %d", got)
	}
	// A prompt that fills the window leaves no room for output.
	s.messages = []ChatMessage{{Role: "user", Content: strings.Repeat("x", 400*1024)}}
	if got := s.budgetCeiling(65536); got != 0 {
		t.Errorf("exhausted window: got %d, want 0", got)
	}
}

// TestNudgeBoostsBudgetAfterLengthTruncation pins T159's second half: when
// the reply was cut off by max_tokens with no text (the whole budget went
// into reasoning), the retry must ask for MORE room — asking again with the
// same budget just truncates again ("empty final answer" in the field).
func TestNudgeBoostsBudgetAfterLengthTruncation(t *testing.T) {
	var mu sync.Mutex
	var budgets []int
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		calls++
		n := calls
		if b, ok := body["max_tokens"].(float64); ok {
			budgets = append(budgets, int(b))
		}
		mu.Unlock()
		if n == 1 {
			sse(w, "", "length", 8192)
			return
		}
		sse(w, "verdict: pass", "stop", 5)
	}))
	defer srv.Close()

	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	client.SetLogger(log)
	tuning := config.SessionTuning{MaxTokens: 4096, ContextLimit: 1 << 20}
	sess := NewSession(client, config.ModelConfig{Model: "m"}, tuning, "sys", nil, log, 1, "test")
	got, err := sess.Run(RunOptions{UserText: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "verdict: pass" {
		t.Fatalf("final = %q", got)
	}
	if len(budgets) != 2 {
		t.Fatalf("requests = %d (budgets %v), want 2", len(budgets), budgets)
	}
	if budgets[1] <= budgets[0] {
		t.Errorf("retry budget %d must exceed the truncated one %d", budgets[1], budgets[0])
	}
}

// TestBudgetWarningForThinkingModel pins the config-level half of T159:
// max_tokens is the budget for thinking AND answer, so a thinking model with
// an 8K budget can return an empty reply. Warn once per model+budget.
func TestBudgetWarningForThinkingModel(t *testing.T) {
	thinking := config.ModelConfig{Model: "m", Thinking: map[string]any{"type": "enabled"}}
	plain := config.ModelConfig{Model: "m"}
	if got := budgetWarning("m", thinking, 8192); got == "" || !strings.Contains(got, "8192") {
		t.Fatalf("thinking + 8192 budget must warn, got %q", got)
	}
	if got := budgetWarning("m", plain, 8192); got != "" {
		t.Errorf("non-thinking model must not warn, got %q", got)
	}
	if got := budgetWarning("m", thinking, 32768); got != "" {
		t.Errorf("large enough budget must not warn, got %q", got)
	}
	if got := budgetWarning("m", thinking, 0); got != "" {
		t.Errorf("unlimited must not warn, got %q", got)
	}
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	warnBudgetOnce(log, 1, "dup-model", thinking, 8192)
	warnBudgetOnce(log, 2, "dup-model", thinking, 8192)
	if _, ok := warnedBudget.Load("dup-model|8192"); !ok {
		t.Error("warning was not recorded, repeats would flood the log")
	}
}
