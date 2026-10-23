package latex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

/*
 * P60 第五步：dispatch_fix 同步执行。
 *
 * 与 p60_fix_dispatch_test.go 的整章流程不同，这里直接把工具指向一棵已经铺好
 * 的细分树：父会话（Work）停在这条工具调用里，假模型只负责两条会话——子章节
 * 修复会话与它之后的核对。要钉住的是**回执本身**：每个子章节修没修好、核对
 * 说了什么、还剩几轮，以及计划只被执行一次。
 */

type fixSyncStats struct {
	mu       sync.Mutex
	fixes    int
	checkers int
}

func (s *fixSyncStats) add(fixes, checkers int) {
	s.mu.Lock()
	s.fixes += fixes
	s.checkers += checkers
	s.mu.Unlock()
}

func (s *fixSyncStats) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fixes, s.checkers
}

type fixSyncHarness struct {
	proj     string
	work     string
	trPath   string // 父会话转录（fix_result 行写在这里）
	r        *Runner
	tool     *DispatchFixTool
	children []ChapterSubdivision
	stats    *fixSyncStats
}

// newFixSyncHarness 铺一棵最小的两子章节细分树（只给 child_01 修复对象），
// 起一个只认"修复会话 / 核对会话"的假模型，并把 Runner、父会话与工具接好。
func newFixSyncHarness(t *testing.T, maxRounds int) *fixSyncHarness {
	t.Helper()
	proj := t.TempDir()
	work := filepath.Join(proj, "work", "temp", "conv_chapter_01", "work")
	childRoot := filepath.Join(work, "_subdivision", "child_01")
	for _, dir := range []string{
		filepath.Join(childRoot, "input"),
		filepath.Join(childRoot, "node", "work"),
		filepath.Join(childRoot, "node", "sessions"),
		filepath.Join(childRoot, "node", "result"),
		filepath.Join(childRoot, "node", "reports"),
		filepath.Join(work, "chapter_01", "children"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(childRoot, "input", "chapter.md"), []byte("# child\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stats := &fixSyncStats{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var apiReq subdivisionAPIRequest
		if err := json.Unmarshal(body, &apiReq); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		toolset := map[string]bool{}
		for _, tl := range apiReq.Tools {
			toolset[tl.Function.Name] = true
		}
		switch {
		// 子章节修复会话：可写 + submit，没有 request_chapter_split。
		case toolset["write_file"] && toolset["submit"] && !toolset["request_chapter_split"]:
			stats.add(1, 0)
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01_child_01.tex", "\\section{Fixed child}\n"),
				subdivisionWrite("work:chapter_01_child_01/part.tex", "fixed part\n"),
				subdivisionCall("fix-submit", "submit", map[string]any{
					"path": "work:chapter_01_child_01.tex", "dir": "work:chapter_01_child_01/",
					"report": map[string]any{"status": "pass"},
				}),
			)
		// 修复后的重核：只读（read_file/grep），结论 pass。
		case toolset["read_file"] && toolset["grep"] && !toolset["write_file"]:
			stats.add(0, 1)
			subdivisionReply(w, subdivisionCall("check-submit", "submit", map[string]any{
				"report": map[string]any{"status": "pass"},
			}))
		default:
			t.Errorf("意料之外的会话（工具表 %v）", toolset)
			p60Text(w, "unexpected session")
		}
	}))
	t.Cleanup(server.Close)

	logPath := filepath.Join(t.TempDir(), "run.log")
	log, err := logger.NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	log.SetQuiet(true)
	t.Cleanup(func() { _ = log.Close() })

	cfg := &config.Config{}
	cfg.Latex.Concurrency = 2
	cfg.Latex.ConvertModel = "fake"
	cfg.Latex.CheckerModel = "fake"
	on := true
	cfg.Latex.Fix = config.FixConfig{Enabled: &on, MaxRounds: maxRounds}
	streamOff := false
	cfg.Models = map[string]config.ModelConfig{
		"fake": {BaseURL: server.URL, APIKey: "test", Model: "fake", Stream: &streamOff},
	}
	r := &Runner{cfg: cfg, log: log, noChapterGate: true, projDir: proj,
		clients: map[string]*session.Client{}, models: map[string]config.ModelConfig{}}

	// 父会话：只用来接 fix_result 那一行（它此刻停在工具调用里）。
	parent := session.NewSession(r.clientFor("fake"), r.modelOf("fake"), cfg.LatexSession("convert"),
		"parent system prompt", nil, log, 1, "convert:chapter_01")
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_01.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	tr, err := session.NewTranscript(trPath)
	if err != nil {
		t.Fatal(err)
	}
	parent.SetTranscript(tr)
	t.Cleanup(func() { _ = tr.Close() })

	children := []ChapterSubdivision{{Title: "First", StartLine: 1, EndLine: 2}}
	tool := &DispatchFixTool{
		Proj: proj, Root: "chapter_01", Work: work,
		Runner: r, Parent: parent,
		Children: children, ChildDepth: 1, Seq: r.nextConvertSeq(), Tid: 1,
	}
	return &fixSyncHarness{proj: proj, work: work, trPath: trPath, r: r, tool: tool, children: children, stats: stats}
}

// dispatch 调一次 dispatch_fix（一条 child_01 的 issue），返回回执文本。
func (h *fixSyncHarness) dispatch(t *testing.T, issueID string) string {
	t.Helper()
	return h.dispatchResult(t, issueID).Text
}

// dispatchResult 同上，但连 ToolResult 一起给——P61/A 要钉住回执是**终态**
// （Terminal），这一轮不再为它发一次模型请求。
func (h *fixSyncHarness) dispatchResult(t *testing.T, issueID string) session.ToolResult {
	t.Helper()
	args := `{"issues":[{"id":"` + issueID + `","child":"child_01","where":"chapter_01_child_01.tex:3",` +
		`"what":"缺一段正文","rule":"与原书该页一致"}]}`
	res, err := h.tool.Execute(args)
	if err != nil {
		t.Fatalf("dispatch_fix: %v", err)
	}
	return res
}

func (h *fixSyncHarness) fixTranscript(t *testing.T) string {
	t.Helper()
	return filepath.Join(h.work, "_subdivision", "child_01", "node", "sessions", "fix_chapter_01_child_01.jsonl")
}

// (a) 同步执行：回执按子章节给结局（fixed / 核对结论 / 剩余轮数），重核真的
// 跑过，计划标成 applied，父会话转录里有一行 fix_result。
func TestDispatchFixReportsOutcomeSynchronously(t *testing.T) {
	h := newFixSyncHarness(t, 2)
	res := h.dispatchResult(t, "ISSUE-1")
	receipt := res.Text
	// P61/A：回执是终态——修复已经同步跑完，这一轮不该再为它发一次模型请求。
	if !res.Terminal {
		t.Error("dispatch_fix 的同步回执必须标成 Terminal（Run 就地结束，不再发请求）")
	}
	for _, want := range []string{
		"FIX RESULT", "1 fixed, 0 still failing",
		"child_01: FIXED", "checker: pass", "rounds 1/2, 1 left",
	} {
		if !strings.Contains(receipt, want) {
			t.Errorf("回执缺 %q:\n%s", want, receipt)
		}
	}
	fixes, checkers := h.stats.counts()
	if fixes != 1 || checkers != 1 {
		t.Errorf("修复请求 %d 次、核对请求 %d 次，期望各 1 次（同步跑修复 + 重核）", fixes, checkers)
	}
	plan, err := ReadFixPlan(h.proj, "chapter_01")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applied == "" {
		t.Error("同步执行后计划应当标成 applied（否则 runner 会再执行一遍）")
	}
	raw, err := os.ReadFile(h.trPath)
	if err != nil {
		t.Fatal(err)
	}
	transcript := string(raw)
	if !strings.Contains(transcript, `"kind":"fix_result"`) {
		t.Errorf("父会话转录里应当有一行 fix_result:\n%s", transcript)
	}
	for _, want := range []string{`"summary":"1/1 修正通过"`, `"child":"child_01"`, `"rounds_left":1`} {
		if !strings.Contains(transcript, want) {
			t.Errorf("fix_result 行缺 %q:\n%s", want, transcript)
		}
	}
	// 修复产物已经刷新回父章节的 children/ 副本——整合读的是它。
	copied, err := os.ReadFile(filepath.Join(h.work, "chapter_01", "children", "chapter_01_child_01.tex"))
	if err != nil {
		t.Fatalf("父章节的 child_01 副本不存在: %v", err)
	}
	if !strings.Contains(string(copied), "Fixed child") {
		t.Errorf("父章节副本没有刷新成修复后的产物:\n%s", copied)
	}
}

// (b) 第二次下派吃掉最后一轮（1/2 → 2/2、0 left）；第三次在预算上停住，
// 一个修复/核对请求都不再发。
func TestDispatchFixSecondRoundConsumesBudget(t *testing.T) {
	h := newFixSyncHarness(t, 2)
	first := h.dispatch(t, "ISSUE-1")
	if !strings.Contains(first, "rounds 1/2, 1 left") {
		t.Fatalf("第一轮回执 = %q", first)
	}
	second := h.dispatch(t, "ISSUE-2")
	for _, want := range []string{"FIXED", "rounds 2/2, 0 left"} {
		if !strings.Contains(second, want) {
			t.Errorf("第二轮回执缺 %q:\n%s", want, second)
		}
	}
	if fixes, checkers := h.stats.counts(); fixes != 2 || checkers != 2 {
		t.Errorf("两轮后修复请求 %d、核对请求 %d，期望 2/2", fixes, checkers)
	}
	// 第三轮：预算已经用完，工具必须当场报"没跑"，不再烧一次会话。
	third := h.dispatch(t, "ISSUE-3")
	for _, want := range []string{"STILL FAILING", "not run", "轮数已用完", "rounds 2/2, 0 left"} {
		if !strings.Contains(third, want) {
			t.Errorf("预算用完的回执缺 %q:\n%s", want, third)
		}
	}
	if fixes, checkers := h.stats.counts(); fixes != 2 || checkers != 2 {
		t.Errorf("预算用完后不该再发请求: fixes=%d checkers=%d", fixes, checkers)
	}
	if got := fixRoundsUsed(h.fixTranscript(t)); got != 2 {
		t.Errorf("转录里的修复轮数 = %d，期望 2（预算不能因为拒绝执行而少算/多算）", got)
	}
}

// (c) 计划只执行一次：dispatch_fix 已经同步消费的计划，runner 侧的
// applyChildFixPlan 是空操作；没有被消费的计划（老流程）它照旧执行，执行后
// 也标成 applied，再调一次同样不再跑。
func TestFixPlanIsNeverAppliedTwice(t *testing.T) {
	h := newFixSyncHarness(t, 3)
	h.dispatch(t, "ISSUE-1")
	planPath := fixPlanPath(h.proj, "chapter_01")
	beforeFixes, beforeCheckers := h.stats.counts()
	h.r.applyChildFixPlan(h.proj, "cls", "chapter_01", h.work, 1, h.children, planPath, h.tool.Seq, 1)
	if fixes, checkers := h.stats.counts(); fixes != beforeFixes || checkers != beforeCheckers {
		t.Errorf("已消费的计划被 runner 又执行了一遍: fixes %d→%d checkers %d→%d",
			beforeFixes, fixes, beforeCheckers, checkers)
	}
	if got := fixRoundsUsed(h.fixTranscript(t)); got != 1 {
		t.Errorf("修复轮数 = %d，期望 1（同一份计划不能吃两轮）", got)
	}

	// 老流程：计划写好了但没人同步执行（没有 Runner 的调用方）——runner 兜底。
	legacy := fixPlan{Root: "chapter_01", Children: []fixPlanChild{{
		Child:  "child_01",
		Issues: []fixPlanIssue{{ID: "LEGACY-1", What: "w", Rule: "r", Where: "chapter_01_child_01.tex:9"}},
	}}}
	legacyPath, err := WriteFixPlan(h.proj, "chapter_01", legacy)
	if err != nil {
		t.Fatal(err)
	}
	h.r.applyChildFixPlan(h.proj, "cls", "chapter_01", h.work, 1, h.children, legacyPath, h.tool.Seq, 1)
	if fixes, _ := h.stats.counts(); fixes != beforeFixes+1 {
		t.Errorf("未被消费的计划应当由 runner 执行一次: fixes %d→%d", beforeFixes, fixes)
	}
	if plan, err := ReadFixPlan(h.proj, "chapter_01"); err != nil || plan.Applied == "" {
		t.Errorf("runner 执行后也应当标成 applied: %+v / %v", plan, err)
	}
	beforeFixes, beforeCheckers = h.stats.counts()
	h.r.applyChildFixPlan(h.proj, "cls", "chapter_01", h.work, 1, h.children, legacyPath, h.tool.Seq, 1)
	if fixes, checkers := h.stats.counts(); fixes != beforeFixes || checkers != beforeCheckers {
		t.Errorf("同一份计划被 runner 执行了两次: fixes %d→%d checkers %d→%d",
			beforeFixes, fixes, beforeCheckers, checkers)
	}
}
