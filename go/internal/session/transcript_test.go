package session

import (
	"path/filepath"
	"strings"
	"testing"
)

// 压缩是 append-only 的：note 写在磁盘末尾，而压缩刻意保留的「原始任务 +
// 最近 8 条」在时间上排在 note **之前**。回放时从 note 起截断就会把这两
// 部分一起丢掉（恢复出来的会话既没有任务、也没有最近上下文）。压缩现在
// 在写完 note 后再把这两部分补写一遍，磁盘回放因此等于内存里的状态。
func TestLoadTranscriptReplaysCompactionKeepTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")

	head := []ChatMessage{{Role: "system", Content: "SYS"}, {Role: "user", Content: "原始任务"}}
	tail := []ChatMessage{
		{Role: "assistant", Content: "最近一轮 A"},
		{Role: "tool", Content: "最近一轮 B", ToolCallID: "c1"},
	}
	note := ChatMessage{Role: "user", Content: "=== COMPRESSED SESSION CONTEXT (auto-generated; the earlier turns were summarised) ===\n摘要正文"}

	// 反面：只有 note（修复前的磁盘形态）→ 任务与 tail 全丢
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range append(append(append([]ChatMessage{}, head...), ChatMessage{Role: "assistant", Content: "被压掉的中间段"}), tail...) {
		if err := w.Append(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Append(note); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	got, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("修复前形态应只剩 note，实际 %d 条", len(got))
	}

	// 正面：note 之后补写「原始任务 + tail」→ 回放包含两者
	w2, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range head[1:] {
		if err := w2.Append(m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range tail {
		if err := w2.Append(m); err != nil {
			t.Fatal(err)
		}
	}
	_ = w2.Close()
	got, err = LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, m := range got {
		texts = append(texts, ContentString(m))
	}
	joined := strings.Join(texts, "|")
	for _, want := range []string{"摘要正文", "原始任务", "最近一轮 A", "最近一轮 B"} {
		if !strings.Contains(joined, want) {
			t.Errorf("回放缺少 %q：%s", want, joined)
		}
	}
	if strings.Contains(joined, "被压掉的中间段") {
		t.Errorf("被压缩的中间段不应复活：%s", joined)
	}
	if strings.Contains(joined, "SYS") {
		t.Errorf("system 提示词不应进入回放（由 SetMessages 重新插入）：%s", joined)
	}
}
