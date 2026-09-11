package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// echoTool returns its argument back, so a fake client can keep the
// loop running without touching the filesystem.
type echoTool struct{ calls int32 }

func (t *echoTool) Name() string { return "echo" }
func (t *echoTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "echo", "description": "echo", "parameters": map[string]any{"type": "object"},
	}}
}
func (t *echoTool) Execute(args string) (ToolResult, error) {
	atomic.AddInt32(&t.calls, 1)
	// 每个工具结果都很长：这正是真实会话膨胀的方式。
	return ToolResult{Text: strings.Repeat("长文本结果。", 400)}, nil
}

// A session must compact WHILE the agentic loop runs, not only when the
// first user turn arrives.
//
// Real defect (2026-09-11 run): maybeCompact() was called once per
// Run(), and for these sessions one Run() IS the entire session — the
// latex sessions hand the initial task to Run() and stay inside its loop
// for hundreds of rounds. So the guard only ever saw "system prompt +
// first message" and never fired: a run reached prompt=323,966 tokens
// with sessions.convert.context_limit=128K and compaction_at=0.85, no
// "COMPRESSED SESSION CONTEXT" ever appeared in its transcript, and the
// account ran out of balance.
func TestCompactionFiresInsideTheToolLoop(t *testing.T) {
	off := false
	var requests int32
	var sawSummary int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		n := atomic.AddInt32(&requests, 1)
		body := string(raw)
		// 摘要请求是"在原会话末尾追加一条指令"的增量请求：靠那条指令识别。
		if strings.Contains(body, "=== CONTEXT COMPACTION REQUEST ===") {
			atomic.StoreInt32(&sawSummary, 1)
			// 摘要请求：返回一段普通文本即可（摘要本身不参与判定）。
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"SUMMARY OF EARLIER WORK"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// 前 6 轮一直调用工具，之后收尾（让 Run 自然结束）。
		if n <= 6 {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
	}))
	defer srv.Close()

	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off})
	tuning := config.SessionTuning{
		ContextLimit: 4000, // 很小的窗口：几轮长工具结果就会越过 0.85
		CompactionAt: 0.85,
		// 本地裁剪关掉（0 / -1），逼出 AI 摘要路径，正是用户要的那条记录。
		PruneToolChars: 0,
		KeepImages:     -1,
		MaxToolRounds:  8,
	}
	tool := &echoTool{}
	s := NewSession(client, config.ModelConfig{Model: "m"}, tuning, "SYS",
		[]Tool{tool}, testLogger(t), 1, "compact-in-loop")

	if _, err := s.Run(RunOptions{UserText: "keep calling echo"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&sawSummary) == 0 {
		t.Fatalf("no compaction request inside the loop (requests=%d, compactions=%d, est=%d)",
			requests, s.Compactions, s.EstimatedTokens())
	}
	if s.Compactions == 0 {
		t.Fatal("Compactions counter must reflect the compaction")
	}
	if atomic.LoadInt32(&tool.calls) < 3 {
		t.Fatalf("the loop must keep running tools after a compaction, calls=%d", tool.calls)
	}
	// 摘要后上下文必须真的降下来（不是"记了一次压缩但没变小"）。
	// 窗口只有 4000 而"系统提示词 + 原始任务 + 最近 8 条"本身就接近它，
	// 所以判据是"显著变小"，不是"低于窗口"。
	if est := s.EstimatedTokens(); est > 5500 {
		t.Fatalf("compaction must shrink the context, still %d tokens", est)
	}
	if s.Compactions > 3 {
		t.Fatalf("compaction must not repeat every round, ran %d times", s.Compactions)
	}
}

// The local pruning stage must only run when the window is actually
// nearly full — pruning every round would keep gutting long tool results
// (and older images) that the model legitimately needs.
func TestLocalPruningOnlyWhenNearTheWindow(t *testing.T) {
	off := false
	var pruned int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "pruned by docvision") {
			atomic.StoreInt32(&pruned, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
	}))
	defer srv.Close()

	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off})
	// 窗口很大 + 很长的工具结果：不该裁剪。
	tuning := config.SessionTuning{ContextLimit: 200000, CompactionAt: 0.85, PruneToolChars: 100, KeepImages: 1}
	s := NewSession(client, config.ModelConfig{Model: "m"}, tuning, "SYS", nil, testLogger(t), 1, "no-prune")
	s.SetMessages([]ChatMessage{
		{Role: "user", Content: "task"},
		{Role: "tool", Content: strings.Repeat("x", 50000)},
	})
	if _, err := s.Run(RunOptions{UserText: "go"}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&pruned) != 0 {
		t.Fatal("a roomy session must not prune its history")
	}
	// 反过来：窗口很小时，同一份历史必须被裁剪（先本地免费裁剪，再考虑摘要）。
	tuning.ContextLimit = 2000
	s2 := NewSession(client, config.ModelConfig{Model: "m"}, tuning, "SYS", nil, testLogger(t), 1, "prune")
	s2.SetMessages([]ChatMessage{
		{Role: "user", Content: "task"},
		{Role: "tool", Content: strings.Repeat("x", 50000)},
	})
	if n := s2.pruneHistory(); n == 0 {
		t.Fatal("a nearly-full session must prune locally first")
	}
}

// A resumed session must continue from the LAST compaction checkpoint,
// not replay the pre-compaction history that the summary replaced.
func TestResumeKeepsOnlyLatestCompactionCheckpoint(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/s.jsonl"
	write := func(msgs ...ChatMessage) {
		for _, m := range msgs {
			line, err := json.Marshal(transcriptLine{T: "msg", Role: m.Role, Text: ContentString(m)})
			if err != nil {
				t.Fatal(err)
			}
			appendLine(t, path, string(line))
		}
	}
	write(
		ChatMessage{Role: "user", Content: "OLD TASK"},
		ChatMessage{Role: "assistant", Content: "old work"},
		ChatMessage{Role: "user", Content: compactedMarker + "\nfirst summary"},
		ChatMessage{Role: "assistant", Content: "work after first compaction"},
		ChatMessage{Role: "user", Content: compactedMarker + "\nsecond summary"},
		ChatMessage{Role: "assistant", Content: "LATEST WORK"},
	)
	got, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, m := range got {
		joined += ContentString(m) + "\n"
	}
	if strings.Contains(joined, "OLD TASK") || strings.Contains(joined, "first summary") {
		t.Fatalf("resume must start at the latest checkpoint:\n%s", joined)
	}
	if !strings.Contains(joined, "second summary") || !strings.Contains(joined, "LATEST WORK") {
		t.Fatalf("resume must keep the latest checkpoint and what followed:\n%s", joined)
	}
}

// testLogger writes into the test's temp dir (nil the log stays quiet).
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	dir := t.TempDir()
	l, err := logger.NewLogger(dir+"/run.log", dir+"/err.log", 4)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}
