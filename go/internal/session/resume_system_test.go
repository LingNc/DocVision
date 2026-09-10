package session

import (
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
)

// 转录（JSONL）里没有 system 行，续跑时 SetMessages 必须把本会话的系统
// 提示补回队首——否则恢复出来的会话完全没有系统提示。
func TestSetMessagesKeepsSystemPrompt(t *testing.T) {
	sess := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "SYS-PROMPT", nil, nil, 1, "test")

	// 模拟从转录恢复：只有 user/assistant/tool 消息。
	sess.SetMessages([]ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "tool", Content: "42", ToolCallID: "c1"},
	})
	msgs := sess.Messages()
	if len(msgs) != 4 || msgs[0].Role != "system" || msgs[0].Content != "SYS-PROMPT" {
		t.Fatalf("恢复后队首不是系统提示: %+v", msgs)
	}

	// 转录里若带有（旧的）system 行，应被本会话的系统提示替换，而不是重复。
	sess.SetMessages([]ChatMessage{
		{Role: "system", Content: "OLD-SYS"},
		{Role: "user", Content: "hi"},
	})
	msgs = sess.Messages()
	if len(msgs) != 2 || msgs[0].Content != "SYS-PROMPT" || msgs[1].Role != "user" {
		t.Fatalf("旧 system 行未被替换: %+v", msgs)
	}
}

// 思维链必须随转录落盘并恢复：GLM 保留式思考（clear_thinking:false）要求
// 历史思维链完整回传，丢了它续跑既不连续、也会打断前缀缓存。
func TestTranscriptKeepsReasoning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []ChatMessage{
		{Role: "user", Content: "draw it"},
		{Role: "assistant", Content: "", ReasoningContent: "I will start with the axes.", ToolCalls: []ToolCall{{ID: "c1", Type: "function"}}},
		{Role: "tool", Content: "ok", ToolCallID: "c1"},
	}
	for _, m := range msgs {
		if err := w.Append(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("消息数 = %d, want 3", len(got))
	}
	if got[1].ReasoningContent != "I will start with the axes." {
		t.Errorf("思维链未恢复: %q", got[1].ReasoningContent)
	}
	if len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "c1" {
		t.Errorf("tool_calls 未恢复: %+v", got[1].ToolCalls)
	}
}
