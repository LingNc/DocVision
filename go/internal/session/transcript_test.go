package session

import (
	"encoding/json"
	"os"
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

// 元信息行（t=meta）记录"这次运行模型被交代了什么"：系统提示词 + 工具定义。
// 它**不参与回放**（LoadTranscript 只认 t=="msg"），因此续跑语义不变；
// 同一份系统提示词重复挂载时不会写第二条。
func TestTranscriptMetaLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	tools := []ToolSnapshot{{Name: "read_file", Description: "读文件", Parameters: json.RawMessage(`{"type":"object"}`)}}
	if err := w.AppendMeta("system", "style", "glm-4.6", "你是样式助手", tools); err != nil {
		t.Fatal(err)
	}
	if err := w.AppendMeta("system", "style", "glm-4.6", "你是样式助手", tools); err != nil {
		t.Fatal(err) // 同一提示词：应被去重
	}
	if err := w.Append(ChatMessage{Role: "user", Content: "开始"}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(body), `"t":"meta"`); n != 1 {
		t.Fatalf("meta 行应只写一条（去重），实际 %d 条:\n%s", n, body)
	}
	if !strings.Contains(string(body), `"system_sha"`) || !strings.Contains(string(body), "read_file") {
		t.Fatalf("meta 行缺少系统提示词哈希或工具定义:\n%s", body)
	}

	// 回放必须只看到消息
	msgs, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("回放应只有 1 条 user 消息，实际 %d 条: %+v", len(msgs), msgs)
	}

	// 换了系统提示词（新一次运行）→ 再写一条
	w2, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w2.AppendMeta("system", "style", "glm-4.6", "你是样式助手（新）", tools); err != nil {
		t.Fatal(err)
	}
	_ = w2.Close()
	body, _ = os.ReadFile(path)
	if n := strings.Count(string(body), `"t":"meta"`); n != 2 {
		t.Fatalf("提示词变化后应追加一条，实际 %d 条", n)
	}
	if msgs, _ = LoadTranscript(path); len(msgs) != 1 {
		t.Fatalf("新增 meta 行后回放仍应只有 1 条消息，实际 %d", len(msgs))
	}
}

// P7: partial sidecar —— WritePartial 原子覆写、ClearPartial 清干净、
// Close 顺手清（会话中途消失也不能留下过期快照）。
func TestPartialSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WritePartial(PartialRecord{Phase: "reasoning", Text: "思考中…", Ts: 123}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path + ".partial")
	if err != nil {
		t.Fatal(err)
	}
	var p PartialRecord
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Phase != "reasoning" || p.Text != "思考中…" {
		t.Fatalf("unexpected partial: %+v", p)
	}
	// 覆写：tmp 文件不能残留
	if err := w.WritePartial(PartialRecord{Phase: "content", Text: "输出中", Ts: 456}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".partial.tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file left behind: %v", err)
	}
	// Append 一条消息后 sidecar 应被清（真实流程经 appendTranscript，这里直接验证语义）
	if err := w.Append(ChatMessage{Role: "assistant", Content: "done"}); err != nil {
		t.Fatal(err)
	}
	w.ClearPartial()
	if _, err := os.Stat(path + ".partial"); !os.IsNotExist(err) {
		t.Fatalf("partial not cleared: %v", err)
	}
	// Close 也要清：再写一个然后 Close
	if err := w.WritePartial(PartialRecord{Phase: "content", Text: "x", Ts: 1}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".partial"); !os.IsNotExist(err) {
		t.Fatalf("partial survived Close: %v", err)
	}
}
