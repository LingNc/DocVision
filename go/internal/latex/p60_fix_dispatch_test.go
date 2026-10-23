package latex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// p60Stats 统计一次递归转换里真正发生的会话/请求：P60 第二步的验收全看这些
// 数字——关着开关时它们必须与今天一模一样，打开并且有计划时才多出"复查→修复
// →重核"这三样东西。
type p60Stats struct {
	mu              sync.Mutex
	requests        int            // 一共多少个 HTTP 请求（≈ 一次会话轮次）
	reviews         int            // 整合前的"复查子章节"对话轮次
	integrations    int            // 父章节整合对话轮次
	fixes           map[string]int // 子章节修复会话的请求数（按子章节名）
	checkers        map[string]int // 核对会话请求数（按章节 ID）
	parentSessions  int            // 父章节**转换会话**的轮数（请求拆分的那条）：≈ 整章尝试次数
	parentCheckers  int            // 父章节核对会话的次数
	runErr          string         // convertOneChapter 的错误（空 = 这一章收下了）
	dispatchOffered bool           // 有没有任何请求把 dispatch_fix 放进工具表
	convertSystem   string         // 转换会话的系统提示词（第一份）
	receipts        []string       // 模型看到的 dispatch_fix 回执（P60 第五步同步执行）
}

// p60Options 是 p60RunWith 的场景旋钮（p60Run 的三个变体只是它们的简写）。
type p60Options struct {
	enabled bool // latex.fix.enabled
	failFix bool // 修复会话怎么提醒都不提交
	// maxRounds 是 latex.fix.max_rounds（0 = 配置默认 2）。
	maxRounds int
	// parentIssues 让**父章节核对**每次都报 issues（"整合不顺"）——整章重开
	// 与修复预算收口这两条判据都只在它成立时才有区别。
	parentIssues bool
	// checkerRounds 是 latex.checker_rounds（0 = 默认 6）。测试把它压到 1，
	// 反馈轮与尝试次数的下限（3）就够验证判据，不必跑满六轮。
	checkerRounds int
	// expectError 表示这次转换**本来就该失败**（例如整章重开到次数上限）。
	expectError bool
}

func (s *p60Stats) bump(m *map[string]int, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if *m == nil {
		*m = map[string]int{}
	}
	(*m)[key]++
}

// p60Run 跑一次最小的两子章节递归转换。enabled 打开 latex.fix.enabled；
// failFix 让修复会话"怎么提醒都不提交"（模拟一轮修复失败）。返回统计与日志。
func p60Run(t *testing.T, enabled, failFix bool) (*p60Stats, string, string) {
	t.Helper()
	return p60RunWith(t, p60Options{enabled: enabled, failFix: failFix})
}

// p60RunWith 与 p60Run 同一条路径，只是把场景旋钮摊开（P60 第四步的预算收口
// 要能同时拨动"父章节核对不过"与"修复轮数上限"）。
func p60RunWith(t *testing.T, opts p60Options) (*p60Stats, string, string) {
	t.Helper()
	proj := t.TempDir()
	workDir := filepath.Join(proj, "work")
	chapters := filepath.Join(proj, "chapters")
	style := filepath.Join(proj, "style")
	for _, dir := range []string{workDir, chapters, style} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	chapterPath := filepath.Join(chapters, "chapter_01.md")
	if err := os.WriteFile(chapterPath, []byte("# First\nFirst text\n# Second\nSecond text"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"book.cls":    "\\ProvidesClass{book}\n",
		"manual.md":   "# Manual\n",
		"example.tex": "\\section{Example}\n",
	} {
		if err := os.WriteFile(filepath.Join(style, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	stats := &p60Stats{}
	checkerBase := regexp.MustCompile(`check:(chapter_[0-9a-z_]+)\.md`)
	dispatchOnce := false

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
		lastUser, system := "", ""
		var receipts []string
		for _, m := range apiReq.Messages {
			switch m.Role {
			case "user":
				lastUser = m.Content
			case "system":
				system = m.Content
			case "tool":
				// P60 第五步：dispatch_fix 的回执是同步执行的唯一"结果面"，
				// 它随 tool 消息回到模型——这里留一份给断言用。
				if strings.HasPrefix(m.Content, "FIX RESULT") {
					receipts = append(receipts, m.Content)
				}
			}
		}
		toolset := map[string]bool{}
		submitDescription := ""
		for _, tool := range apiReq.Tools {
			toolset[tool.Function.Name] = true
			if tool.Function.Name == "submit" {
				submitDescription = tool.Function.Description
			}
		}

		stats.mu.Lock()
		stats.requests++
		if toolset["dispatch_fix"] {
			stats.dispatchOffered = true
		}
		if stats.convertSystem == "" && toolset["request_chapter_split"] {
			stats.convertSystem = system
		}
		stats.receipts = append(stats.receipts, receipts...)
		stats.mu.Unlock()

		switch {
		// 修复会话（P60 第二步）：可写、但没有 request_chapter_split（那是转换
		// 会话独有的）；它后面还有"提醒提交"的轮次，所以按工具表 + submit 的
		// 描述认，而不是只看首句。
		case toolset["write_file"] && !toolset["request_chapter_split"] && toolset["submit"]:
			base := "chapter_01_child_01"
			if strings.Contains(submitDescription, "chapter_01_child_02.tex") {
				base = "chapter_01_child_02"
			}
			stats.bump(&stats.fixes, base)
			if opts.failFix {
				// 不调 submit：runner 会提醒两次，仍然不提交——一轮修复失败。
				p60Text(w, "I could not fix this.")
				return
			}
			subdivisionReply(w,
				subdivisionWrite("work:"+base+".tex", "\\section{Fixed "+base+"}\n"),
				subdivisionWrite("work:"+base+"/part.tex", "fixed part\n"),
				subdivisionCall("fix-submit-"+base, "submit", map[string]any{
					"path": "work:" + base + ".tex", "dir": "work:" + base + "/",
					"report": map[string]any{"status": "pass"},
				}),
			)

		// 核对会话：read_file + grep，没有写权限。
		case toolset["read_file"] && toolset["grep"] && !toolset["write_file"]:
			base := ""
			if m := checkerBase.FindStringSubmatch(lastUser); len(m) == 2 {
				base = m[1]
			}
			if base == "" {
				t.Errorf("认不出核对会话的章节名: %q", lastUser)
				p60Text(w, "no verdict")
				return
			}
			stats.bump(&stats.checkers, base)
			if base == "chapter_01" {
				stats.mu.Lock()
				stats.parentCheckers++
				stats.mu.Unlock()
				if opts.parentIssues {
					// 父章节核对报问题 = "整合不顺"：今天会作废这一版、整章重开；
					// P60 第四步要看修复预算用光时收不收得住。
					subdivisionReply(w, subdivisionCall("check-"+base, "submit", map[string]any{
						"report": map[string]any{"status": "issues", "issues": "第 3 页有一段没还原"},
					}))
					return
				}
			}
			subdivisionReply(w, subdivisionCall("check-"+base, "submit", map[string]any{
				"report": map[string]any{"status": "pass"},
			}))

		// 整合前的复查对话（P60 第二步）：第一次调用 dispatch_fix，第二次收尾。
		case strings.Contains(lastUser, "only hands work down"):
			stats.mu.Lock()
			stats.reviews++
			seen := dispatchOnce
			dispatchOnce = true
			stats.mu.Unlock()
			if seen {
				p60Text(w, "nothing else to hand down")
				return
			}
			subdivisionReply(w, subdivisionCall("dispatch-fix", "dispatch_fix", map[string]any{
				"issues": []any{map[string]any{
					"id": "ISSUE-1", "child": "child_01", "where": "chapter_01_child_01.tex:3",
					"what": "缺一段正文", "rule": "与原书该页一致",
				}},
			}))

		// 父章节整合会话（今天的行为）。
		case strings.Contains(lastUser, "All child chapters have passed"):
			stats.mu.Lock()
			stats.integrations++
			stats.mu.Unlock()
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Integrated}\n"),
				subdivisionWrite("work:chapter_01/part.tex", "integrated part\n"),
				subdivisionCall("parent-submit", "submit", map[string]any{
					"path": "work:chapter_01.tex", "dir": "work:chapter_01/",
					"report": map[string]any{"status": "pass"},
				}),
			)

		// 子章节转换会话。
		case strings.Contains(lastUser, "input:chapter.md"):
			base := "chapter_01_child_01"
			if strings.Contains(submitDescription, "chapter_01_child_02.tex") {
				base = "chapter_01_child_02"
			}
			subdivisionReply(w,
				subdivisionWrite("work:"+base+".tex", "\\section{"+base+"}\n"),
				subdivisionWrite("work:"+base+"/part.tex", "child part\n"),
				subdivisionCall("child-submit-"+base, "submit", map[string]any{
					"path": "work:" + base + ".tex", "dir": "work:" + base + "/",
					"report": map[string]any{"status": "pass"},
				}),
			)

		// 父章节的反馈轮（核对打了 issues 之后回到同一个转换会话）：重交同一份
		// 整合产物。内容不变，所以要看的只是"有没有再问一次核对"。
		case strings.Contains(lastUser, "The checker reviewed your submitted chapter"):
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Integrated}\n"),
				subdivisionWrite("work:chapter_01/part.tex", "integrated part\n"),
				subdivisionCall("parent-feedback-submit", "submit", map[string]any{
					"path": "work:chapter_01.tex", "dir": "work:chapter_01/",
					"report": map[string]any{"status": "pass"},
				}),
			)

		// 父章节第一次会话：请求拆分。
		default:
			stats.mu.Lock()
			stats.parentSessions++
			stats.mu.Unlock()
			subdivisionReply(w,
				subdivisionWrite("work:chapter_01.tex", "\\section{Parent draft}\n"),
				subdivisionWrite("work:chapter_01/part.tex", "draft part\n"),
				subdivisionCall("split", "request_chapter_split", map[string]any{"children": []any{
					map[string]any{"title": "First", "start_line": 1, "end_line": 2},
					map[string]any{"title": "Second", "start_line": 3, "end_line": 4},
				}}),
			)
		}
	}))
	defer server.Close()

	logPath := filepath.Join(t.TempDir(), "run.log")
	log, err := logger.NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	log.SetQuiet(true)

	cfg := &config.Config{}
	cfg.Latex.Concurrency = 2
	cfg.Latex.ConvertModel = "fake"
	cfg.Latex.CheckerModel = "fake"
	if opts.checkerRounds > 0 {
		cfg.Latex.CheckerRounds = opts.checkerRounds
	}
	on := opts.enabled
	cfg.Latex.Fix = config.FixConfig{Enabled: &on, MaxRounds: opts.maxRounds}
	off := false
	cfg.Models = map[string]config.ModelConfig{"fake": {BaseURL: server.URL, APIKey: "test", Model: "fake", Stream: &off}}
	r := &Runner{cfg: cfg, log: log, noChapterGate: true, projDir: proj,
		clients: map[string]*session.Client{}, models: map[string]config.ModelConfig{}}
	runErr := r.convertOneChapter(proj, "book", filepath.Join(style, "manual.md"), chapterPath, workDir, 1, 1, false)
	stats.mu.Lock()
	if runErr != nil {
		stats.runErr = runErr.Error()
	}
	stats.mu.Unlock()
	if runErr != nil && !opts.expectError {
		t.Fatalf("convertOneChapter: %v", runErr)
	}
	if runErr == nil && !fileExists(filepath.Join(workDir, "chapters", "chapter_01.tex")) {
		t.Fatal("父章节没有交出产物")
	}
	raw, _ := os.ReadFile(logPath)
	_ = log.Close()
	return stats, string(raw), proj
}

// p60Text 是"模型这一轮没有工具调用、正常收尾"的回复。
func p60Text(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{
		"role": "assistant", "content": text,
	}}}})
}

// P60 第二步（a）：默认关闭时，父章节这条路径上一条多余会话都不许有——
// 不多一次复查对话、不多一次修复、不多一次核对，工具表里也没有 dispatch_fix。
func TestFixDispatchDisabledKeepsTodayPath(t *testing.T) {
	stats, _, _ := p60Run(t, false, false)
	if stats.dispatchOffered {
		t.Error("关闭时不该把 dispatch_fix 注册给任何会话")
	}
	if strings.Contains(stats.convertSystem, "dispatch_fix") {
		t.Error("关闭时转换系统提示词不该出现 dispatch_fix")
	}
	if stats.reviews != 0 || len(stats.fixes) != 0 {
		t.Errorf("关闭时不该有复查/修复会话: reviews=%d fixes=%v", stats.reviews, stats.fixes)
	}
	if stats.integrations != 1 {
		t.Errorf("父章节整合会话 = %d，期望 1", stats.integrations)
	}
	// 今天这条路径一共 7 个请求：父会话拆分、两个子会话、两个子核对、
	// 父整合、父核对。任何"默认也会多跑一条会话"的回归都会把这个数字顶上去。
	if stats.requests != 7 {
		t.Errorf("请求数 = %d，期望 7（与引入 P60 之前一致）", stats.requests)
	}
	if got := stats.checkers["chapter_01_child_01"]; got != 1 {
		t.Errorf("child_01 核对次数 = %d，期望 1", got)
	}
	if got := stats.checkers["chapter_01_child_02"]; got != 1 {
		t.Errorf("child_02 核对次数 = %d，期望 1", got)
	}
	if got := stats.checkers["chapter_01"]; got != 1 {
		t.Errorf("父章节核对次数 = %d，期望 1", got)
	}
}

// P60 第二步（b）：打开开关、计划点名一个子章节时，只有它多一条修复会话与
// 一次重核；另一个子章节一动不动，父章节的整合会话照常跑。
func TestFixDispatchRunsPlannedChildOnly(t *testing.T) {
	stats, _, proj := p60Run(t, true, false)
	if !stats.dispatchOffered {
		t.Fatal("打开时 dispatch_fix 应当出现在转换会话的工具表里")
	}
	if !strings.Contains(stats.convertSystem, "## Handing a problem child back") ||
		!strings.Contains(stats.convertSystem, "dispatch_fix") {
		t.Error("打开时转换系统提示词应当带上下派说明")
	}
	if stats.reviews == 0 {
		t.Error("打开时整合前应当有一次复查子章节的对话")
	}
	if got := stats.fixes["chapter_01_child_01"]; got != 1 {
		t.Errorf("child_01 的修复会话请求数 = %d，期望 1（P60 第五步：dispatch_fix 同步跑一次，runner 不再重复执行同一份计划）: %v", got, stats.fixes)
	}
	if got := stats.fixes["chapter_01_child_02"]; got != 0 {
		t.Errorf("计划没点名的 child_02 不该跑修复会话: %v", stats.fixes)
	}
	if got := stats.checkers["chapter_01_child_01"]; got != 2 {
		t.Errorf("child_01 核对次数 = %d，期望 2（原核对 + 修复后重核）", got)
	}
	if got := stats.checkers["chapter_01_child_02"]; got != 1 {
		t.Errorf("child_02 核对次数 = %d，期望 1（不动它）", got)
	}
	if stats.integrations != 1 {
		t.Errorf("父章节整合会话 = %d，期望 1", stats.integrations)
	}
	// 回执里必须有结局（P60 第五步）：模型在整合前就知道 child_01 修好了、
	// 核对结论与还剩几轮。
	receipts := strings.Join(stats.receipts, "\n")
	for _, want := range []string{"FIX RESULT", "child_01: FIXED", "checker: pass", "rounds 1/5, 4 left"} {
		if !strings.Contains(receipts, want) {
			t.Errorf("dispatch_fix 的回执缺 %q:\n%s", want, receipts)
		}
	}
	// 同步执行过的计划已标成 applied：runner 侧看到它就该空转。
	plan, err := ReadFixPlan(proj, "chapter_01")
	if err != nil {
		t.Fatalf("读不回修复计划: %v", err)
	}
	if plan.Applied == "" {
		t.Error("同步执行的计划应当带 applied 时间戳")
	}

	// 修复会话写下的产物要真的刷新进父章节的 children/ 副本，否则整合读到的
	// 还是修复前的旧文件。
	fixed := filepath.Join(proj, "work", "temp", "conv_chapter_01", "work",
		"chapter_01", "children", "chapter_01_child_01.tex")
	raw, err := os.ReadFile(fixed)
	if err != nil {
		t.Fatalf("父章节的 child_01 副本不存在: %v", err)
	}
	if !strings.Contains(string(raw), "Fixed chapter_01_child_01") {
		t.Errorf("父章节的 child_01 副本没有刷新成修复后的产物:\n%s", raw)
	}
	// 修复转录里要有一条 fix_target 锚点（P59/P60：挂回同一次转换）。
	tr := filepath.Join(proj, "work", "temp", "conv_chapter_01", "work", "_subdivision",
		"child_01", "node", "sessions", "fix_chapter_01_child_01.jsonl")
	trRaw, err := os.ReadFile(tr)
	if err != nil {
		t.Fatalf("修复转录不存在: %v", err)
	}
	if !strings.Contains(string(trRaw), "fix_target") {
		t.Error("修复转录缺少 fix_target 锚点")
	}
}

// P60 第二步（c）：一轮修复失败只留一行警告，整本书照旧整合、照旧提交——
// 落回今天的行为，不因为修复失败而失败。
func TestFixDispatchFailureFallsBackToIntegration(t *testing.T) {
	stats, logText, _ := p60Run(t, true, true)
	if got := stats.fixes["chapter_01_child_01"]; got == 0 {
		t.Fatalf("计划点名的 child_01 应当尝试过修复: %v", stats.fixes)
	}
	if stats.integrations != 1 {
		t.Errorf("修复失败后父章节整合会话 = %d，期望 1", stats.integrations)
	}
	if !strings.Contains(logText, "修复失败") {
		t.Errorf("修复失败应当记一行警告，日志里没有：\n%s", logText)
	}
}

// P60 第二步/第五步：下派说明只按开关追加。关闭时的提示词必须与引入它之前逐字节
// 相同（这正是"默认跑法不变"最容易被破坏的地方）。
func TestConvertSystemPromptDispatchOnlyWhenEnabled(t *testing.T) {
	r := keepTestRunner(t)
	off := false
	r.cfg.Latex.Fix = config.FixConfig{Enabled: &off}
	tuning := config.SessionTuning{MaxToolRounds: 30}
	today := renderPrompt(prompts.Must(prompts.ConvertSystem), tuning, r.outputLang())
	if got := r.convertSystemPrompt(tuning); got != today {
		t.Fatal("latex.fix.enabled 关闭时转换系统提示词必须与今天逐字节相同")
	}
	if strings.Contains(today, "dispatch_fix") {
		t.Fatal("常驻的转换模板不该提到 dispatch_fix（它只在开关打开时注册）")
	}
	// P60 第五步（A）：自包含边界的说明也只跟着开关走——关闭时一个字都不能有。
	// （`_subdivision/` 本身在常驻提示词里就有，所以只钉新增的判据句。）
	for _, want := range []string{"project-level resources", "NOT non-self-contained", "node/result/"} {
		if strings.Contains(today, want) {
			t.Errorf("关闭时不该出现自包含边界说明 %q", want)
		}
	}
	on := true
	r.cfg.Latex.Fix = config.FixConfig{Enabled: &on}
	got := r.convertSystemPrompt(tuning)
	if !strings.HasPrefix(got, today+"\n\n") {
		t.Fatal("打开时应当只是在今天的提示词后面追加一段")
	}
	if !strings.Contains(got, "dispatch_fix") || !strings.Contains(got, "records the plan") {
		t.Errorf("追加的说明缺少关键语义:\n%s", got)
	}
	// 打开时：`figures/`/`images/` 不是断链、只有 `_subdivision/…` 与
	// `node/result/…` 才是——父会话不能把引用项目级资源的子章节退回来重修。
	for _, want := range []string{"`figures/`", "`images/`", "project-level resources", "NOT non-self-contained", "_subdivision/", "node/result/"} {
		if !strings.Contains(got, want) {
			t.Errorf("打开时应当带自包含边界说明 %q:\n%s", want, got)
		}
	}
}
