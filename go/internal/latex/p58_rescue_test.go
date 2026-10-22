package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// 造一条最小转录：两条正常消息，一条"把 submit 写成正文"的助手消息，末尾用量行。
func writeRescueTranscript(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "convert_chapter_001.jsonl")
	lines := []string{
		`{"ts":"2026-10-22T10:00:00Z","t":"msg","role":"user","text":"convert chapter_001"}`,
		`{"ts":"2026-10-22T10:00:10Z","t":"msg","role":"assistant","text":"I will read the files.","tool_calls":[{"id":"c1","function":{"name":"read_file","arguments":"{\"path\":\"x.md\"}"}}]}`,
		`{"ts":"2026-10-22T10:00:11Z","t":"msg","role":"tool","text":"file content"}`,
		`{"ts":"2026-10-22T10:00:20Z","t":"msg","role":"assistant","text":"All done. \\submit{ \"report\": { \"status\": \"pass\" } }"}`,
		`{"ts":"2026-10-22T10:00:21Z","t":"usage","finish_reason":"stop","prompt_tokens":10,"completion_tokens":5}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadSessionToolTailAndFullMessage(t *testing.T) {
	dir := t.TempDir()
	path := writeRescueTranscript(t, dir)
	lines, err := loadTranscriptLines(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d", len(lines))
	}
	tool := &ReadSessionTool{Path: path, Lines: lines, Total: len(lines)}

	// 默认：末尾 12 条消息（这里只有 3 条），每条截到 2000 字符。
	res, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "#1 user") || !strings.Contains(res.Text, "#4 assistant") {
		t.Fatalf("默认窗口应该覆盖整条转录：\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "read_file(") {
		t.Fatalf("工具调用参数应出现在窗口里：\n%s", res.Text)
	}

	// 显式 from：只看第 4 行起。
	res, err = tool.Execute(`{"from":4,"count":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "#1 user") || !strings.Contains(res.Text, "#4 assistant") {
		t.Fatalf("from 应该只从第 4 行开始：\n%s", res.Text)
	}

	// max_chars=0：看全文（这里正文很短，不出现 clipped 标记）。
	res, err = tool.Execute(`{"from":4,"count":1,"max_chars":0}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "clipped") {
		t.Fatalf("max_chars=0 不应截断：\n%s", res.Text)
	}
	if !strings.Contains(res.Text, `\submit{`) {
		t.Fatalf("全文里应看到正文里的 \\submit：\n%s", res.Text)
	}
}

func TestReadSessionToolClipsAndTellsHowToWiden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checker_chapter_002.jsonl")
	long := strings.Repeat("x", 3000)
	lines := []string{
		`{"ts":"2026-10-22T10:00:00Z","t":"msg","role":"assistant","text":"` + long + `"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ls, err := loadTranscriptLines(path)
	if err != nil {
		t.Fatal(err)
	}
	tool := &ReadSessionTool{Lines: ls, Total: len(ls)}
	res, _ := tool.Execute(`{}`)
	if !strings.Contains(res.Text, "clipped") || !strings.Contains(res.Text, "max_chars=0") {
		t.Fatalf("超长消息应截断并告诉模型怎么读全文：%s", res.Text)
	}
	res, _ = tool.Execute(`{"max_chars":0}`)
	if strings.Contains(res.Text, "clipped") {
		t.Fatalf("max_chars=0 时不应再有 clipped")
	}
}

func TestSessionStatsToolFlagsProseToolCalls(t *testing.T) {
	dir := t.TempDir()
	path := writeRescueTranscript(t, dir)
	ls, err := loadTranscriptLines(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&SessionStatsTool{Lines: ls, Total: len(ls)}).Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "#4") {
		t.Fatalf("应点名第 4 条正文像工具调用：\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "finish_reason per round: stop") {
		t.Fatalf("应给出每轮 finish_reason：\n%s", res.Text)
	}
}

func TestRescueSubmitToolValidation(t *testing.T) {
	tool := &RescueSubmitTool{}
	if res, _ := tool.Execute(`{"verdict":"truncate","reason":"garbled"}`); !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("truncate 缺 from 应被拒：%s", res.Text)
	}
	if res, _ := tool.Execute(`{"verdict":"maybe","reason":"x"}`); !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("非法 verdict 应被拒：%s", res.Text)
	}
	if res, _ := tool.Execute(`{"verdict":"retry","reason":""}`); !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("空 reason 应被拒：%s", res.Text)
	}
	res, err := tool.Execute(`{"verdict":"truncate","from":3,"reason":"prose submit"}`)
	if err != nil || !tool.Submitted || tool.Verdict != "truncate" || tool.From != 3 {
		t.Fatalf("合法提交应被收下：%v %+v %s", err, tool, res.Text)
	}
	if !res.Terminal {
		t.Fatalf("提交回执应是终态")
	}
}

func TestRescueQueueTruncateQueuesContinue(t *testing.T) {
	proj := t.TempDir()
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_001.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}}
	if err := r.rescueQueueTruncate(rescueTarget{ProjDir: proj, Base: "chapter_001", Role: "convert", TrPath: trPath}, 7); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(proj, "work", "retry")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("应排一条请求：%v %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Role, Chapter, Path, Source string
		From                        int
	}
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.Role != "continue" || req.From != 7 || req.Chapter != "chapter_001" || req.Path == "" {
		t.Fatalf("请求内容不对：%+v", req)
	}
	if req.Path != "sessions/convert_chapter_001.jsonl" {
		t.Fatalf("Path 应是 work/ 下的相对路径：%q", req.Path)
	}
}

func TestRescueBudgetDefaultsToTwo(t *testing.T) {
	rescueMu.Lock()
	rescueSeen = map[string]int{}
	rescueMu.Unlock()
	r := &Runner{cfg: &config.Config{}}
	if !r.rescueBudget("k") || !r.rescueBudget("k") {
		t.Fatal("默认预算应是 2 次")
	}
	if r.rescueBudget("k") {
		t.Fatal("第 3 次应超预算")
	}
}

func TestRescueModelFallsBackToChecker(t *testing.T) {
	cfg := &config.Config{}
	cfg.Latex.CheckerModel = "checker-model"
	r := &Runner{cfg: cfg}
	if got := r.rescueModelName(); got != "checker-model" {
		t.Fatalf("latex.rescue.model 与 models.rescue 都空时应跟随 checker，got %q", got)
	}
	cfg.Latex.Rescue.Model = "rescue-x"
	if got := r.rescueModelName(); got != "rescue-x" {
		t.Fatalf("显式配置优先，got %q", got)
	}
}

// P58：复盘会话的会话参数在 latex.sessions.rescue 里；没写的字段继承 checker
// （输出预算同配置），上下文默认 128K、轮次默认 12。
func TestRescueSessionTuningInheritsChecker(t *testing.T) {
	cfg := &config.Config{}
	cfg.Latex.Sessions.Checker.MaxTokens = 4321
	got := cfg.LatexSession("rescue")
	if got.MaxTokens != 4321 {
		t.Fatalf("max_tokens 应继承 checker（4321），got %d", got.MaxTokens)
	}
	if got.ContextLimit != 131072 {
		t.Fatalf("context_limit 默认应是 128K，got %d", got.ContextLimit)
	}
	// checker 自己配了上下文时随它（"继承 checker，默认 128K"）。
	cfg2 := &config.Config{}
	cfg2.Latex.Sessions.Checker.ContextLimit = 65536
	if got := cfg2.LatexSession("rescue"); got.ContextLimit != 65536 {
		t.Fatalf("应继承 checker 的上下文（65536），got %d", got.ContextLimit)
	}
	if got.MaxToolRounds != 12 {
		t.Fatalf("max_tool_rounds 默认应是 12，got %d", got.MaxToolRounds)
	}
	cfg.Latex.Sessions.Rescue.ContextLimit = 262144
	cfg.Latex.Sessions.Rescue.MaxToolRounds = 20
	cfg.Latex.Sessions.Rescue.MaxTokens = 512
	got = cfg.LatexSession("rescue")
	if got.ContextLimit != 262144 || got.MaxToolRounds != 20 || got.MaxTokens != 512 {
		t.Fatalf("latex.sessions.rescue 的显式值应生效：%+v", got)
	}
}

// P58：--debug（logger Debug 级别）必须让复盘转录落盘，否则网页里看不到
// 「错误处理」下的复盘会话（早先按 level>=4 判，debug=1 永远不落盘）。
func TestRescueKeepRecordsOnDebug(t *testing.T) {
	cfg := &config.Config{}
	r := &Runner{cfg: cfg, log: mustTestLogger(t, logger.LevelDebug)}
	if !r.rescueKeepRecords(0) {
		t.Fatal("--debug 下应落盘复盘转录")
	}
	r.log = mustTestLogger(t, logger.LevelInfo)
	if r.rescueKeepRecords(0) {
		t.Fatal("info 级别默认不落盘")
	}
	on := true
	cfg.Latex.Rescue.KeepRecords = &on
	if !r.rescueKeepRecords(0) {
		t.Fatal("latex.rescue.keep_records=true 时应落盘")
	}
}

func mustTestLogger(t *testing.T, level int) *logger.Logger {
	t.Helper()
	l, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	l.SetLevel(level)
	return l
}
