package latex

// P58 扩展（2026-10-22 第二轮，用户口径）：复盘不再只认"跑完但没 submit"。
//
//	① Run **报错终止**（非致命错误）也走同一条复盘链路；
//	② 会话层纠正过「你的工具确实可用」之后仍然没提交，同样复盘（并且复盘者
//	   要知道这件事——它是退化的强信号）；
//	③ 致命错误（余额不足 / 无可用资源包）**不**复盘：换会话也救不回来；
//	④ `latex.rescue.enabled: false` 时三种触发全部退回引入本功能之前的行为，
//	   请求数、重试队列、审计日志与转录文件一个字节都不多。
//
// 这些用例都跑真实的 `convertOneChapter`（假客户端），断言的是**请求数与磁盘
// 上留下了什么**，而不是内部函数被调了几次。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

type rescueChapterOptions struct {
	// enabled 是 latex.rescue.enabled；nil = 不写这一项（默认开）。
	enabled *bool
	// chapterError > 0 时转换会话用这个 HTTP 状态码回错误（模拟"报错终止"）。
	chapterError int
	// chapterErrorBody 是错误响应体（余额判定靠它）。
	chapterErrorBody string
	// claimNoTools 为真时会话先声称"工具不可用"（触发会话层纠正），纠正后的
	// 那一轮正常收尾、不提交。
	claimNoTools bool
	// noSubmit 为真时会话每轮都正常收尾、从不调 submit（老的"没提交"触发）。
	// 每轮正文带轮次号，模拟"还在推进但就是不提交"。
	noSubmit bool
	// repeatAnswer 让 noSubmit 的会话每轮返回**一字不差**的正文：模型在复读，
	// 第二次提醒只换来同一段话（钉住"不再重复追提醒"）。
	repeatAnswer bool
	// checkerNoSubmit 为真时**核对会话**每轮都正常收尾、从不调 submit：现场那句
	// "checker 会话已退化，已排队从干净轮次继续核对"就是这么来的。
	checkerNoSubmit bool
	// verdicts 是复盘结论序列（第 n 次复盘取第 n 个，超出取最后一个）。
	verdicts []string
	from     int
	hint     string
	// rescueFailures 是前几次复盘会话"不给结论"的方式（1 起，第 n 项对应第 n
	// 次复盘）：1 = 报错终止，2 = 跑完不调 submit。用完之后的复盘照常按
	// verdicts 给结论（P61/B 的丢弃重来）。
	rescueFailures []int
	// keepRescueRecords 打开 latex.rescue.keep_records，让复盘转录真的落盘
	// （要断言"丢弃/挪进 .trash"的用例必须开）。
	keepRescueRecords bool
	// preseedNoVerdict 预铺一份"产物在、但上次核对没给结论"的现场（T222）：
	// `work/chapters/<章>.tex` 与带退化文案的 `.checker` 都已存在，用来钉住
	// 「已有产物，跳过」不会静默采纳一份没被判定过的产物。
	preseedNoVerdict bool
	// seedCheckerReminders > 0 时预铺一份"已经退回 n 次都没提交"的核对转录
	// （T230）：`work/sessions/checker_chapter_01.jsonl` 里已经有 n 条提交提醒，
	// 用来钉住「这种转录不再回放、也不再往上追加」。
	seedCheckerReminders int
	// checkerStatuses 是逐次核对会话给出的结论（第 n 次取第 n 项，超出取最后
	// 一项）；空 = 每次都 pass。给多项时用来钉住"打回一轮之后必须重新真的核一
	// 遍"（T234）：第二项是 pass 就说明修复后的那一版确实被重新看过了。
	checkerStatuses []string
	// checkerEmptyIssues 是前几次**核对请求**给出"issues 但问题清单为空"的次数
	// （T242）：现场实测整章核对跑完、issues 是空串，旧行为据此作废整章重转。
	// 之后的请求照 checkerStatuses 回答。
	checkerEmptyIssues int
}

type rescueChapterStats struct {
	mu         sync.Mutex
	requests   int
	convert    int
	checker    int
	rescue     int
	rescueText []string // 每次复盘请求里的第一段 user 文本
	userTexts  []string // 所有请求里的最后一条 user 文本
}

func (s *rescueChapterStats) add(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

type rescueChapterRun struct {
	stats *rescueChapterStats
	proj  string
	err   error
}

// resetRescueBudget 清掉包级的复盘预算表：它按"章节/角色/转录文件名"计数，
// 而每个用例都用 t.TempDir()，键会完全一样。
func resetRescueBudget() {
	rescueMu.Lock()
	rescueSeen = map[string]int{}
	rescueMu.Unlock()
}

// runRescueChapter 用假客户端跑一章真实的转换：转换会话的回复由 opts 决定，
// 复盘会话（工具表里有 read_session）按 opts.verdicts 给结论。
func runRescueChapter(t *testing.T, opts rescueChapterOptions) *rescueChapterRun {
	t.Helper()
	resetRescueBudget()
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
	if err := os.WriteFile(chapterPath, []byte("# First\nFirst text\n"), 0o644); err != nil {
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

	if n := opts.seedCheckerReminders; n > 0 {
		var lines []map[string]any
		lines = append(lines, map[string]any{"t": "msg", "role": "system", "text": "You are a checker."})
		for i := 0; i < n; i++ {
			lines = append(lines,
				map[string]any{"t": "msg", "role": "assistant", "text": `{"submit": {"report": {"status": "pass"}}}`},
				map[string]any{"t": "msg", "role": "user", "text": checkerSubmitReminderMark + " Use the `submit` TOOL."},
			)
		}
		var b strings.Builder
		for _, l := range lines {
			data, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
			b.WriteByte('\n')
		}
		trPath := filepath.Join(workDir, "sessions", "checker_chapter_01.jsonl")
		if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(trPath, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	stats := &rescueChapterStats{}
	if opts.preseedNoVerdict {
		// T222 现场：上一轮交了产物、核对没给结论就收尾了。重跑时"已有产物，
		// 跳过"不能把这份没被判定过的东西直接采纳。
		submitted := filepath.Join(workDir, "chapters")
		if err := os.MkdirAll(submitted, 0o755); err != nil {
			t.Fatal(err)
		}
		texPath := filepath.Join(submitted, "chapter_01.tex")
		if err := os.WriteFile(texPath, []byte("\\section{First}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(texPath+".checker",
			[]byte("checker 会话已退化，已排队从干净轮次继续核对"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var convertRounds int
	verdictAt := func(n int) (string, int, string) {
		if len(opts.verdicts) == 0 {
			return "fail", opts.from, ""
		}
		i := n - 1
		if i >= len(opts.verdicts) {
			i = len(opts.verdicts) - 1
		}
		return opts.verdicts[i], opts.from, opts.hint
	}

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
		lastUser := ""
		toolset := map[string]bool{}
		for _, m := range apiReq.Messages {
			if m.Role == "user" {
				lastUser = m.Content
			}
		}
		for _, tool := range apiReq.Tools {
			toolset[tool.Function.Name] = true
		}
		stats.add(func() {
			stats.requests++
			stats.userTexts = append(stats.userTexts, lastUser)
		})

		switch {
		// 复盘会话（P58）：只读转录 + 交结论，工具表里有 read_session。
		case toolset["read_session"]:
			var n int
			stats.add(func() {
				stats.rescue++
				n = stats.rescue
				stats.rescueText = append(stats.rescueText, lastUser)
			})
			// P61/B：前几次复盘可以"不给结论"（报错 / 不提交），用来钉住丢弃
			// 重来与放弃后的退回行为。
			failure := 0
			if n <= len(opts.rescueFailures) {
				failure = opts.rescueFailures[n-1]
			}
			switch failure {
			case 1:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"message":"review model rejected the request"}}`)
			case 2:
				p60Text(w, "I could not review it.")
			default:
				verdict, from, hint := verdictAt(n)
				subdivisionReply(w, subdivisionCall("rescue-submit", "submit", map[string]any{
					"verdict": verdict, "from": from, "reason": "用例给的结论", "hint": hint,
				}))
			}

		// 核对会话：只读（有 read_file / grep、没有 write_file）。
		case toolset["read_file"] && toolset["grep"] && !toolset["write_file"]:
			var n int
			stats.add(func() {
				stats.checker++
				n = stats.checker
			})
			if opts.checkerNoSubmit {
				// 退化：跑完就给一段正文，从不调 submit。
				p60Text(w, "I could not finish the review.")
				return
			}
			// T242：交了 issues 却一条问题都没写——现场原样（report 里没有 issues）。
			if opts.checkerEmptyIssues > 0 && n <= opts.checkerEmptyIssues {
				subdivisionReply(w, subdivisionCall("check-submit", "submit", map[string]any{
					"report": map[string]any{"status": "issues"},
				}))
				return
			}
			status := "pass"
			if len(opts.checkerStatuses) > 0 {
				i := n - 1
				if i >= len(opts.checkerStatuses) || i < 0 {
					i = len(opts.checkerStatuses) - 1
				}
				status = opts.checkerStatuses[i]
			}
			report := map[string]any{"status": status}
			if status != "pass" {
				report["issues"] = "the marked formula is missing from the submitted chapter"
			}
			subdivisionReply(w, subdivisionCall("check-submit", "submit", map[string]any{"report": report}))

		// 转换会话：可写 + request_chapter_split。
		default:
			var round int
			stats.add(func() {
				stats.convert++
				convertRounds++
				round = convertRounds
			})
			switch {
			case opts.chapterError > 0:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(opts.chapterError)
				_, _ = io.WriteString(w, opts.chapterErrorBody)
			case opts.claimNoTools && round == 1:
				// 声称"工具不可用"：会话层会插一条纠正消息再重发一轮。
				p60Text(w, "The chapter workspace tools are not available in this session, so I cannot convert it.")
			case opts.repeatAnswer:
				// 复读：每一轮的正文一字不差。
				p60Text(w, "I cannot make progress.")
			case opts.claimNoTools || opts.noSubmit:
				// 还在推进（正文带轮次号）但就是不提交：提醒仍然值钱。
				p60Text(w, fmt.Sprintf("I cannot make progress (round %d).", round))
			default:
				// 正常收尾：写产物 + 真调 submit（"一章本来就没问题"的对照用例）。
				subdivisionReply(w,
					subdivisionWrite("work:chapter_01.tex", "\\section{First}\n"),
					subdivisionWrite("work:chapter_01/part.tex", "part\n"),
					subdivisionCall("convert-submit", "submit", map[string]any{
						"path": "work:chapter_01.tex", "dir": "work:chapter_01/",
						"report": map[string]any{"status": "pass"},
					}),
				)
			}
		}
	}))
	t.Cleanup(server.Close)

	r := keepTestRunner(t)
	r.cfg.Latex.Concurrency = 2
	r.cfg.Latex.ConvertModel = "fake"
	r.cfg.Latex.CheckerModel = "fake"
	r.cfg.Latex.Rescue.Enabled = opts.enabled
	if opts.keepRescueRecords {
		on := true
		r.cfg.Latex.Rescue.KeepRecords = &on
	}
	off := false
	r.cfg.Models = map[string]config.ModelConfig{"fake": {BaseURL: server.URL, APIKey: "test", Model: "fake", Stream: &off}}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	r.projDir = proj

	err := r.convertOneChapter(proj, "book", filepath.Join(style, "manual.md"), chapterPath, workDir, 1, 1, false)
	return &rescueChapterRun{stats: stats, proj: proj, err: err}
}

// rescueQueuedRequests 读 <proj>/work/retry/ 里排下的请求（按文件名排序）。
func rescueQueuedRequests(t *testing.T, proj string) []map[string]any {
	t.Helper()
	dir := filepath.Join(proj, "work", "retry")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var rec map[string]any
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		out = append(out, rec)
	}
	return out
}

// rescueAuditLines 读 <proj>/work/rescue/log.jsonl（没有文件就是空）。
func rescueAuditLines(t *testing.T, proj string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(proj, "work", "rescue", "log.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("审计行不是 JSON: %q", line)
		}
		out = append(out, rec)
	}
	return out
}

// rescueTranscripts 列出 <proj>/work/sessions/ 下的复盘转录（rescue_ 前缀）。
func rescueTranscripts(t *testing.T, proj string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(proj, "work", "sessions"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "rescue_") {
			out = append(out, e.Name())
		}
	}
	return out
}

// (a) 报错终止（非致命）的转换会话触发一次复盘；`truncate` 结论排进重试队列
// 从干净轮次继续，这一章按失败收尾，不再往同一个坏会话里发提醒。
func TestRescueOnErrorEndedConvertQueuesTruncate(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		chapterError:     http.StatusBadRequest,
		chapterErrorBody: `{"error":{"message":"model rejected the request"}}`,
		verdicts:         []string{"truncate"},
		from:             1,
	})
	if run.err == nil {
		t.Fatal("报错终止且复盘判 truncate 时，这一章应当按失败收尾（继续跑由队列接手）")
	}
	if !strings.Contains(run.err.Error(), "会话失败") {
		t.Errorf("失败原因应说明是转换会话失败：%v", run.err)
	}
	if run.stats.rescue != 1 {
		t.Fatalf("报错终止应触发一次复盘，实际 %d 次（请求 %d）", run.stats.rescue, run.stats.requests)
	}
	if run.stats.convert != 1 {
		t.Errorf("判 truncate 后不该再提醒同一个会话，转换请求 = %d，期望 1", run.stats.convert)
	}
	if run.stats.requests != 2 {
		t.Errorf("请求数 = %d，期望 2（失败的转换 + 一次复盘）", run.stats.requests)
	}
	// 复盘者要被告知"停下原因是报错"，否则容易把外部故障和模型自己收尾混为一谈。
	if len(run.stats.rescueText) != 1 || !strings.Contains(run.stats.rescueText[0], "ended with an error") {
		t.Errorf("复盘请求应说明这次是报错终止：%v", run.stats.rescueText)
	}
	// truncate 结论 → 与页面 ⤓ 同一条队列通路：role=continue、From 保留。
	reqs := rescueQueuedRequests(t, run.proj)
	if len(reqs) != 1 {
		t.Fatalf("应排一条 continue 请求，实际 %d 条", len(reqs))
	}
	req := reqs[0]
	if req["role"] != "continue" || req["chapter"] != "chapter_01" || req["from"] != float64(1) {
		t.Errorf("排队请求不对：%+v", req)
	}
	if req["source"] != "rescue" {
		t.Errorf("排队请求应标明来源是复盘：%+v", req)
	}
	audit := rescueAuditLines(t, run.proj)
	if len(audit) != 1 || audit[0]["verdict"] != "truncate" {
		t.Errorf("审计记录不对：%+v", audit)
	}
}

// (b) "声称工具不可用"、会话层纠正过一次后仍然没有提交：同样进复盘，并且复盘者
// 要看到"纠正已经用过了"。
func TestRescueOnNoToolClaim(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		claimNoTools: true,
		verdicts:     []string{"truncate"},
		from:         1,
	})
	if run.stats.rescue != 1 {
		t.Fatalf("声称无工具后仍未提交应触发复盘，实际 %d 次", run.stats.rescue)
	}
	// 第一次是幻觉、第二次是纠正后的回复——纠正确实发生了一次。
	if run.stats.convert != 2 {
		t.Errorf("转换请求 = %d，期望 2（幻觉 + 纠正后的一轮）", run.stats.convert)
	}
	if len(run.stats.rescueText) != 1 {
		t.Fatalf("复盘请求文本数量不对：%d", len(run.stats.rescueText))
	}
	if !strings.Contains(run.stats.rescueText[0], "claimed that its tools were not available") {
		t.Errorf("复盘者应知道会话层已经纠正过「工具不可用」：%s", run.stats.rescueText[0])
	}
	if got := len(rescueQueuedRequests(t, run.proj)); got != 1 {
		t.Errorf("truncate 应排一条请求，实际 %d 条", got)
	}
	if got := len(rescueAuditLines(t, run.proj)); got != 1 {
		t.Errorf("审计记录 = %d 行，期望 1", got)
	}
}

// (c) 致命错误（余额不足 / 无可用资源包）不复盘：换会话、道歉、截断都救不回来，
// 复盘只会白烧一次模型调用。
func TestRescueSkipsFatalBalanceError(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		chapterError:     http.StatusTooManyRequests,
		chapterErrorBody: `{"error":{"message":"余额不足或无可用资源包,请充值","code":1113}}`,
		verdicts:         []string{"truncate"},
		from:             1,
	})
	if run.err == nil || !strings.Contains(run.err.Error(), "SESSION_INSUFFICIENT_BALANCE") {
		t.Fatalf("余额错误应原样上报：%v", run.err)
	}
	if run.stats.rescue != 0 {
		t.Errorf("致命错误不该复盘，实际 %d 次", run.stats.rescue)
	}
	if run.stats.requests != 1 {
		t.Errorf("请求数 = %d，期望 1（只有那次失败的转换）", run.stats.requests)
	}
	if got := len(rescueQueuedRequests(t, run.proj)); got != 0 {
		t.Errorf("不该排重试请求，实际 %d 条", got)
	}
	if got := len(rescueAuditLines(t, run.proj)); got != 0 {
		t.Errorf("不该写审计记录，实际 %d 行", got)
	}
}

// (d) latex.rescue.enabled: false —— 三种触发全部退回老行为：没有复盘请求、
// 没有复盘转录、没有审计记录；报错终止立刻上报，"没提交"照旧提醒两次。
func TestRescueDisabledKeepsPreChangeBehaviour(t *testing.T) {
	off := false

	t.Run("报错终止立刻上报", func(t *testing.T) {
		run := runRescueChapter(t, rescueChapterOptions{
			enabled:          &off,
			chapterError:     http.StatusBadRequest,
			chapterErrorBody: `{"error":{"message":"model rejected the request"}}`,
			verdicts:         []string{"truncate"},
			from:             1,
		})
		if run.err == nil {
			t.Fatal("报错终止应当直接让这一章失败")
		}
		if run.stats.requests != 1 {
			t.Errorf("请求数 = %d，期望 1（与引入复盘之前一致：不再多花一次模型调用）", run.stats.requests)
		}
		if run.stats.rescue != 0 {
			t.Errorf("关闭时不该有复盘请求，实际 %d 次", run.stats.rescue)
		}
		if got := len(rescueQueuedRequests(t, run.proj)); got != 0 {
			t.Errorf("关闭时不该排重试请求，实际 %d 条", got)
		}
		if got := rescueTranscripts(t, run.proj); len(got) != 0 {
			t.Errorf("关闭时不该写复盘转录：%v", got)
		}
		if _, err := os.Stat(filepath.Join(run.proj, "work", "rescue")); !os.IsNotExist(err) {
			t.Errorf("关闭时不该创建 work/rescue/（stat err = %v）", err)
		}
	})

	t.Run("没提交仍只发两次提醒", func(t *testing.T) {
		run := runRescueChapter(t, rescueChapterOptions{
			enabled:  &off,
			noSubmit: true,
			verdicts: []string{"truncate"},
			from:     1,
		})
		if run.err == nil {
			t.Fatal("提醒两次后仍未提交，这一章应当失败")
		}
		// 老行为：1 次转换 + 2 次提醒，一次复盘都没有。
		if run.stats.requests != 3 || run.stats.convert != 3 {
			t.Errorf("请求数 = %d / 转换请求 %d，期望 3 / 3（与引入复盘之前一致）",
				run.stats.requests, run.stats.convert)
		}
		if run.stats.rescue != 0 {
			t.Errorf("关闭时不该有复盘请求，实际 %d 次", run.stats.rescue)
		}
		var reminded bool
		for _, text := range run.stats.userTexts {
			if strings.Contains(text, "You have NOT submitted yet") {
				reminded = true
			}
		}
		if !reminded {
			t.Error("提醒文案应保持原样（\"You have NOT submitted yet ...\"）")
		}
		if got := len(rescueQueuedRequests(t, run.proj)); got != 0 {
			t.Errorf("关闭时不该排重试请求，实际 %d 条", got)
		}
		if got := rescueTranscripts(t, run.proj); len(got) != 0 {
			t.Errorf("关闭时不该写复盘转录：%v", got)
		}
		if _, err := os.Stat(filepath.Join(run.proj, "work", "rescue")); !os.IsNotExist(err) {
			t.Errorf("关闭时不该创建 work/rescue/（stat err = %v）", err)
		}
	})
}

// 模型复读时不再重复追提醒：上一次提醒只换来一模一样的答复，第二次提醒
// 就不发了（历史里那串同样的提醒 + 同样的总结就是它堆出来的）。
// 注意口径：只在"答复一字不差"时停，答复变了的会话提醒照发（实测值钱）。
func TestReminderStopsWhenModelRepeatsItself(t *testing.T) {
	off := false
	run := runRescueChapter(t, rescueChapterOptions{
		enabled:      &off,
		noSubmit:     true,
		repeatAnswer: true,
	})
	if run.err == nil {
		t.Fatal("复读且未提交，这一章应当失败")
	}
	if run.stats.convert != 2 {
		t.Errorf("请求数 = %d（转换请求），期望 2（首次 + 一次提醒）", run.stats.convert)
	}
	reminders := 0
	for _, text := range run.stats.userTexts {
		if strings.Contains(text, "You have NOT submitted yet") {
			reminders++
		}
	}
	if reminders != 1 {
		t.Errorf("提交提醒 = %d 次，期望 1（第二次会因为复读而跳过）", reminders)
	}
}

// 对照：答复每轮都变（还在推进、就是不提交）时，两次提醒照发——不能因为
// 怕污染就一律只提醒一次。
func TestReminderKeepsGoingWhenAnswerChanges(t *testing.T) {
	off := false
	run := runRescueChapter(t, rescueChapterOptions{
		enabled:  &off,
		noSubmit: true,
	})
	if run.err == nil {
		t.Fatal("提醒两次后仍未提交，这一章应当失败")
	}
	if run.stats.convert != 3 {
		t.Errorf("转换请求 = %d，期望 3（首次 + 两次提醒）", run.stats.convert)
	}
}

// (e) 不写 latex.rescue.enabled（默认开）时复盘照跑：结论 retry 会把复盘者给的
// hint 拼进提醒再续一轮；下一次复盘判 fail 就停手（不再提醒）。
func TestRescueDefaultOnRunsAndRetryReminds(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		noSubmit: true,
		verdicts: []string{"retry", "fail"},
		hint:     "read work:manual.md then compile and submit",
	})
	if run.stats.rescue != 2 {
		t.Fatalf("默认应复盘两次（retry 后仍未提交 → 再复盘一次），实际 %d 次", run.stats.rescue)
	}
	if run.stats.convert != 2 {
		t.Errorf("转换请求 = %d，期望 2（首次 + retry 后的一轮提醒）", run.stats.convert)
	}
	if !strings.Contains(run.stats.rescueText[0], "ended its turn without calling submit") {
		t.Errorf("普通触发不该带报错措辞：%s", run.stats.rescueText[0])
	}
	var noted bool
	for _, text := range run.stats.userTexts {
		if strings.Contains(text, "Reviewer note: "+"read work:manual.md then compile and submit") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("retry 的 hint 应拼进提醒：%v", run.stats.userTexts)
	}
	if got := len(rescueAuditLines(t, run.proj)); got != 2 {
		t.Errorf("审计记录 = %d 行，期望 2（每次复盘一行）", got)
	}
	if got := len(rescueQueuedRequests(t, run.proj)); got != 0 {
		t.Errorf("retry/fail 都不该排重试请求，实际 %d 条", got)
	}
}

// 正常提交的一章不该多出任何复盘：开关默认开也一样（复盘只认"非正常收尾"）。
func TestRescueNotTriggeredOnNormalCommit(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{})
	if run.err != nil {
		t.Fatalf("正常提交的一章不该失败：%v", run.err)
	}
	if !fileExists(filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")) {
		t.Fatal("正常提交的章节产物应当落进 work/chapters/")
	}
	if run.stats.rescue != 0 {
		t.Errorf("正常收尾不该复盘，实际 %d 次", run.stats.rescue)
	}
	// 转换 1 次 + 核对 1 次；多出任何一次都说明复盘链路被无条件接上了。
	if run.stats.requests != 2 {
		t.Errorf("请求数 = %d，期望 2（转换 + 核对）", run.stats.requests)
	}
	if got := len(rescueAuditLines(t, run.proj)); got != 0 {
		t.Errorf("正常收尾不该写审计记录，实际 %d 行", got)
	}
	if _, err := os.Stat(filepath.Join(run.proj, "work", "rescue")); !os.IsNotExist(err) {
		t.Errorf("正常收尾不该创建 work/rescue/（stat err = %v）", err)
	}
}
