package img2text

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T76：get_more_context 轮的 tool_calls 助手回复不得在转录里出现两次。
func TestExchangeRecorderNoDuplicateToolCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.jsonl")
	rec, err := NewExchangeRecorder(path, "测试图", "m", "sys")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()

	asstToolCall := ChatMessage{Role: "assistant", Content: "", ToolCalls: []ToolCall{{
		ID: "c1", Type: "function",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "get_more_context", Arguments: "{}"},
	}}}
	toolReceipt := ChatMessage{Role: "tool", ToolCallID: "c1", Content: "Added lines."}
	final := ChatMessage{Role: "assistant", Content: "answer"}

	// 第 1 轮：请求 [sys,user] → 回复 tool_calls
	rec.Record(&ChatRequest{Messages: []ChatMessage{
		{Role: "system", Content: "sys"}, {Role: "user", Content: "u1"},
	}}, &ChatResponse{Choices: []ChatResponseChoice{{Message: asstToolCall}}})
	// 第 2 轮：请求带尾巴 [tool 回执] → 最终回复
	rec.Record(&ChatRequest{Messages: []ChatMessage{
		{Role: "system", Content: "sys"}, {Role: "user", Content: "u1"},
		asstToolCall, toolReceipt,
	}}, &ChatResponse{Choices: []ChatResponseChoice{{Message: final}}})

	data, _ := os.ReadFile(path)
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		var head struct {
			T    string `json:"t"`
			Role string `json:"role"`
		}
		if json.Unmarshal([]byte(line), &head) != nil || head.T != "msg" {
			continue
		}
		if strings.Contains(line, "get_more_context") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("get_more_context 应只出现 1 次，实际 %d", count)
	}
}
