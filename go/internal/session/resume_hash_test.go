package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTranscriptLines(t *testing.T, path string, lines []map[string]any) {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func msgLine(role, text, ts string, calls []map[string]any) map[string]any {
	l := map[string]any{"t": "msg", "role": role, "text": text, "ts": ts}
	if calls != nil {
		l["tool_calls"] = calls
	}
	return l
}

// P10：轮次哈希是内容的稳定函数（重放同一轮哈希一致），且 Append 落盘的
// 行携带 h。
func TestRoundHashStableAndAppended(t *testing.T) {
	line := transcriptLine{T: "msg", Role: "assistant", Text: "hello"}
	h1 := roundHash(line)
	line2 := transcriptLine{T: "msg", Role: "assistant", Text: "hello"}
	if roundHash(line2) != h1 {
		t.Fatalf("同内容两轮哈希不一致: %s vs %s", h1, roundHash(line2))
	}
	line.Text = "world"
	if roundHash(line) == h1 {
		t.Fatal("内容变化哈希没变")
	}
	if len(h1) != 12 {
		t.Fatalf("哈希长度 %d，期望 12 hex", len(h1))
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	w, err := NewTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(ChatMessage{Role: "user", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	data, _ := os.ReadFile(path)
	var written map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[0]), &written); err != nil {
		t.Fatal(err)
	}
	if h, _ := written["h"].(string); h == "" || len(h) != 12 {
		t.Fatalf("落盘行缺 h 或长度不对: %v", written["h"])
	}
}

// T28/T29：续跑统计——轮次/工具数/活跃跨度/提交识别，悬空调用合成占位回执。
func TestLoadTranscriptStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	base := time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339Nano)
	mid := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	last := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339Nano)
	writeTranscriptLines(t, path, []map[string]any{
		msgLine("user", "do it", base, nil),
		msgLine("assistant", "", mid, []map[string]any{
			{"id": "c1", "type": "function", "function": map[string]any{"name": "compile", "arguments": "{}"}},
			{"id": "c2", "type": "function", "function": map[string]any{"name": "submit", "arguments": `{"path":"figure.tex"}`}},
			{"id": "c3", "type": "function", "function": map[string]any{"name": "view_image", "arguments": "{}"}},
		}),
		{"t": "msg", "role": "tool", "tool_call_id": "c1", "text": "OK", "ts": last},
		{"t": "msg", "role": "tool", "tool_call_id": "c2", "text": "SUBMITTED. figure.tex", "ts": last},
	})
	msgs, st, err := LoadTranscriptStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Rounds != 1 || st.Tools != 2 {
		t.Fatalf("Rounds=%d Tools=%d，期望 1/2", st.Rounds, st.Tools)
	}
	if st.SubmitName != "submit" || !strings.Contains(st.SubmitArgs, "figure.tex") {
		t.Fatalf("提交识别失败: %q %q", st.SubmitName, st.SubmitArgs)
	}
	if st.Active < time.Minute {
		t.Fatalf("Active=%v，期望 ≥1m（首尾时间跨度）", st.Active)
	}
	// c3 是悬空调用（没有回执）→ 合成占位回执，历史 wire 合法。
	found := false
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "c3" {
			found = true
			if !strings.Contains(ContentString(m), "INTERRUPTED") {
				t.Fatalf("占位回执文本不对: %q", ContentString(m))
			}
		}
	}
	if !found {
		t.Fatal("悬空调用 c3 没有合成占位回执")
	}
	// 悬空的 submit 调用（无回执）不算已提交 —— 已有回执的 c2 才是。
	if st.SubmitName != "submit" || !strings.Contains(st.SubmitArgs, "figure.tex") {
		t.Fatalf("提交识别失败: %q %q", st.SubmitName, st.SubmitArgs)
	}
}

// T28：提交重放的参数取自最后一个**有回执**的 submit 调用。
func TestLoadTranscriptStatsSubmittedArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	writeTranscriptLines(t, path, []map[string]any{
		msgLine("user", "do it", now, nil),
		msgLine("assistant", "", now, []map[string]any{
			{"id": "s1", "type": "function", "function": map[string]any{"name": "submit", "arguments": `{"path":"old.tex"}`}},
		}),
		{"t": "msg", "role": "tool", "tool_call_id": "s1", "text": "SUBMITTED.", "ts": now},
		msgLine("assistant", "", now, []map[string]any{
			{"id": "s2", "type": "function", "function": map[string]any{"name": "submit", "arguments": `{"path":"new.tex"}`}},
		}),
		{"t": "msg", "role": "tool", "tool_call_id": "s2", "text": "SUBMITTED.", "ts": now},
	})
	_, st, err := LoadTranscriptStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.SubmitArgs, "new.tex") {
		t.Fatalf("应取最后一次已收账提交的参数: %q", st.SubmitArgs)
	}
}
