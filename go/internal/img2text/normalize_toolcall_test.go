package img2text

import "testing"

func TestNormalizeToolCallTypes(t *testing.T) {
	msg := ChatMessage{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: "a", Type: "", Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "get_more_context", Arguments: "{}"}},
			{ID: "b", Type: "function"},
		},
	}
	normalizeToolCallTypes(&msg)
	if msg.ToolCalls[0].Type != "function" {
		t.Fatalf("empty type not normalized: %q", msg.ToolCalls[0].Type)
	}
	if msg.ToolCalls[1].Type != "function" {
		t.Fatalf("existing type should be preserved: %q", msg.ToolCalls[1].Type)
	}
}
