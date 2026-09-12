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

// 现场缺陷（2026-09-11 那次运行，用户问"会话压缩好像没有触发？"）：
// 整个运行里 `[context]` 只出现过 1 行（convert:chapter_003，估算 66,427
// tokens = 窗口的 50%），转录里 **0 条** `COMPRESSED SESSION CONTEXT`，而
// debug 日志里厂商实测 prompt 一路涨到 132k / 206k / 243k —— 阈值
// 0.85×131072=111,411 用的是本地估算，估算漏算太多（reasoning 回传、
// tool_calls 参数、图片按像素编码），于是永远够不着，一次都没压过。
//
// 现在阈值用「本地估算 / 标定后估算 / 厂商实测」三者最大，实测值直接
// 把下一次判定顶上去。
func TestCompactionUsesVendorPromptTokens(t *testing.T) {
	off := false
	var sawSummary int32
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		n := atomic.AddInt32(&requests, 1)
		if strings.Contains(string(raw), "=== CONTEXT COMPACTION REQUEST ===") {
			atomic.StoreInt32(&sawSummary, 1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"SUMMARY"}}],"usage":{"prompt_tokens":1000,"completion_tokens":5}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if n <= 12 {
			// 会话正文很短（本地估算只有几千 tokens），但厂商说这次请求
			// 已经 130,000 tokens —— 就是现场那种"估算看不见的开销"
			// （图片按像素编码、每轮包装）。多跑几轮是为了让"任务与最近 8 条
			// 之间"真的有内容可摘要。
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}}],"usage":{"prompt_tokens":130000,"completion_tokens":5}}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":130000,"completion_tokens":5}}`)
	}))
	defer srv.Close()

	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off})
	tuning := config.SessionTuning{
		ContextLimit: 131072, CompactionAt: 0.85,
		PruneToolChars: 0, KeepImages: -1, MaxToolRounds: 20,
	}
	s := NewSession(client, config.ModelConfig{Model: "m"}, tuning, "SYS",
		[]Tool{&echoTool{}}, testLogger(t), 1, "vendor-usage")
	if est := s.EstimatedTokens(); est > 5000 {
		t.Fatalf("前置条件：本地估算必须远小于阈值，got %d", est)
	}
	if _, err := s.Run(RunOptions{UserText: "keep calling echo"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&sawSummary) == 0 {
		t.Fatalf("厂商实测 prompt 已远超阈值(130k)，必须触发压缩（requests=%d, est=%d, current=%d）",
			requests, s.EstimatedTokens(), s.CurrentTokens())
	}
	if s.Compactions == 0 {
		t.Fatal("Compactions 必须反映真实发生的压缩")
	}
	// 只靠本地估算永远到不了阈值：这条断言说明是"实测值"把它顶上去的。
	if raw := s.EstimatedTokens(); raw >= 111411 {
		t.Fatalf("前置条件被破坏：本地估算自己就越过了阈值（%d）", raw)
	}
}

// 会话还短（任务与"保留的最近 8 条"之间没内容）时不做摘要，也**不得**
// 记账成一次压缩——Compactions 曾经无条件 ++，统计里混进没发生的压缩。
func TestShortSessionDoesNotCountFakeCompaction(t *testing.T) {
	off := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":130000,"completion_tokens":5}}`)
	}))
	defer srv.Close()
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off})
	s := NewSession(client, config.ModelConfig{Model: "m"},
		config.SessionTuning{ContextLimit: 131072, CompactionAt: 0.85, PruneToolChars: 0, KeepImages: -1},
		"SYS", nil, testLogger(t), 1, "short")
	if _, err := s.Run(RunOptions{UserText: "go"}); err != nil {
		t.Fatal(err)
	}
	if s.Compactions != 0 {
		t.Fatalf("短会话没有可摘要的中段，不该记成压缩: %d", s.Compactions)
	}
}

// CurrentTokens 的三段取值：没有实测时就是估算；有实测时不得低于实测。
func TestCurrentTokensNeverBelowMeasured(t *testing.T) {
	off := false
	client := NewClient(config.ModelConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k", Model: "m", Stream: &off})
	s := NewSession(client, config.ModelConfig{Model: "m"},
		config.SessionTuning{ContextLimit: 131072}, "SYS", nil, testLogger(t), 1, "est")
	s.SetMessages([]ChatMessage{{Role: "user", Content: "短"}})
	base := s.EstimatedTokens()
	if got := s.CurrentTokens(); got != base {
		t.Fatalf("没有实测值时 CurrentTokens 应等于估算: %d vs %d", got, base)
	}
	s.observePromptSize(base, 90000)
	if got := s.CurrentTokens(); got < 90000 {
		t.Fatalf("厂商实测 90000 时不得低于实测: %d", got)
	}
	s.forgetMeasuredPromptSize()
	if got := s.CurrentTokens(); got != base {
		t.Fatalf("清掉实测值后应回到估算: %d vs %d", got, base)
	}
	// 标定系数夹在 [1,10]：单次异常值不会把阈值顶到天上。
	s.observePromptSize(1, 100000)
	if s.calibNum > 10 {
		t.Fatalf("标定系数必须夹住，got %v", s.calibNum)
	}
}

// 估算必须覆盖请求体里真正发出去的东西：历史回传的 reasoning_content
// 与 tool_calls 的参数 JSON 曾经完全不计入。
func TestEstimatorCountsReasoningAndToolCalls(t *testing.T) {
	plain := ChatMessage{Role: "assistant", Content: "短答案"}
	rich := ChatMessage{
		Role:             "assistant",
		Content:          "短答案",
		ReasoningContent: strings.Repeat("思考过程。", 200),
		ToolCalls: []ToolCall{{ID: "c1", Type: "function", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "write_file", Arguments: strings.Repeat("{\"path\":\"a.tex\"}", 100)}}},
	}
	if messageTokens(rich, "") <= messageTokens(plain, "")+1000 {
		t.Fatalf("reasoning/tool_calls 必须计入估算: rich=%d plain=%d",
			messageTokens(rich, ""), messageTokens(plain, ""))
	}
}
