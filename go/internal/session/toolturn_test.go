package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// fakeTool is a scripted session tool.
type fakeTool struct {
	name string
	res  ToolResult
}

func (t *fakeTool) Name() string { return t.name }
func (t *fakeTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":       t.name,
		"parameters": map[string]any{"type": "object"},
	}}
}
func (t *fakeTool) Execute(string) (ToolResult, error) { return t.res, nil }

// TestToolImageTurnDoesNotBreakToolBlock is the regression test for the
// "insufficient tool messages following tool_calls message" 400: a tool
// that returns an image must not inject a user turn BETWEEN tool
// responses when several tools are called in one round.
func TestToolImageTurnDoesNotBreakToolBlock(t *testing.T) {
	var mu sync.Mutex
	round := 0
	var problems []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Messages []struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &body)

		// Validate the tool-call block: every assistant tool_calls turn
		// must be followed immediately by matching tool messages.
		for i, m := range body.Messages {
			if m.Role != "assistant" || len(m.ToolCalls) == 0 {
				continue
			}
			for j, tc := range m.ToolCalls {
				idx := i + 1 + j
				if idx >= len(body.Messages) {
					problems = append(problems, fmt.Sprintf("missing tool reply for %s", tc.ID))
					continue
				}
				next := body.Messages[idx]
				if next.Role != "tool" || next.ToolCallID != tc.ID {
					problems = append(problems, fmt.Sprintf("message %d after tool_calls is role=%s id=%s, want tool/%s", idx, next.Role, next.ToolCallID, tc.ID))
				}
			}
		}

		mu.Lock()
		round++
		cur := round
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if cur == 1 {
			io.WriteString(w, `data: {"choices":[{"delta":{"content":"working"}}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"view_image","arguments":"{}"}}]}}]}`+"\n\n")
			io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"image_context","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
			io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	client.SetLogger(log)

	tools := []Tool{
		&fakeTool{name: "view_image", res: ToolResult{Text: "image attached", ImageBase64: "aGk=", ImageMIME: "image/png"}},
		&fakeTool{name: "image_context", res: ToolResult{Text: "context text"}},
	}
	sess := NewSession(client, config.ModelConfig{Model: "m"}, config.SessionTuning{MaxTokens: 100}, "sys", tools, log, 1, "test")
	got, err := sess.Run(RunOptions{UserText: "draw it"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "final answer" {
		t.Errorf("final = %q", got)
	}
	if len(problems) > 0 {
		t.Fatalf("invalid tool block: %s", strings.Join(problems, "; "))
	}
	// Both tool responses must be present, and the vision turn must come
	// after the whole tool block.
	msgs := sess.Messages()
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	joined := strings.Join(roles, ",")
	if !strings.Contains(joined, "assistant,tool,tool,user") {
		t.Fatalf("roles = %s, want the image user turn after both tool replies", joined)
	}
}
