package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// ------------------------------------------------------------------
// T43：风格修复大循环
//
// 旧行为（多数派门）：超过半数章节汇报样式问题才打回原样式会话。
// 新行为：只要有任何章节汇报手册/样式问题就进入修复流程——
//   1. 整合会话（stylefix_integrate_r<N>）：类似章节划分会话，把各章
//      汇报整合成不重复的问题清单（submit_problems，固定格式）。
//   2. 风格修复会话（stylefix_style_r<N>）：复用原样式会话上下文（信息
//      不丢失；缓存失效只是成本问题，按用户口径"衡量"后选复用）修复
//      cls/手册，提交后编译 example 验证。
//   3. 影响评估会话（stylefix_assess_r<N>）：按问题划分章节块
//      （submit_blocks，每块带问题编号与章节范围）——样式问题常是全局
//      性的，不能只修汇报章。
//   4. 分块修复会话（stylefix_blk_<N>_<i>，并行）：每块一个会话，在临时
//      工作区里对块内章节做**定位修改**（绝不重做），逐章汇报是否仍有
//      问题。
//   5. 任一章仍有问题且未到 latex.style_fix.max_rounds → 下一轮
//      style-f<N+1>；到上限则告警保留现状（编译与终审仍是硬关卡）。
// ------------------------------------------------------------------

// StyleProblem 是整合会话交出的一个问题。
type StyleProblem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Chapters []string `json:"chapters"`
}

// StyleBlock 是评估会话交出的一个修复块。
type StyleBlock struct {
	Title      string   `json:"title"`
	ProblemIDs []string `json:"problem_ids"`
	Chapters   []string `json:"chapters"`
}

// SubmitProblemsTool 收整合会话的问题清单。
type SubmitProblemsTool struct {
	Problems []StyleProblem
	Set      bool
}

func (t *SubmitProblemsTool) Name() string { return "submit_problems" }

func (t *SubmitProblemsTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit_problems",
		"description": "Submit the consolidated, de-duplicated problem list. Each problem: {id (P1, P2, ...), title, detail (what is wrong and how to recognise it), chapters (chapter base names that reported it)}.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"problems": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string"},
						"title":    map[string]any{"type": "string"},
						"detail":   map[string]any{"type": "string"},
						"chapters": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"id", "title", "detail", "chapters"},
				}},
			},
			"required": []string{"problems"},
		},
	}}
}

func (t *SubmitProblemsTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["problems"].([]interface{})
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: problems must be a non-empty array."}, nil
	}
	var out []StyleProblem
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		p := StyleProblem{
			ID:     strArg(m, "id"),
			Title:  strArg(m, "title"),
			Detail: strArg(m, "detail"),
		}
		if cs, ok := m["chapters"].([]interface{}); ok {
			for _, c := range cs {
				if s, ok := c.(string); ok {
					p.Chapters = append(p.Chapters, s)
				}
			}
		}
		if p.ID == "" || p.Title == "" || p.Detail == "" {
			return session.ToolResult{Text: "REJECTED: each problem needs id/title/detail."}, nil
		}
		out = append(out, p)
	}
	t.Problems, t.Set = out, true
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. %d problems recorded.", len(out))}, nil
}

// SubmitBlocksTool 收评估会话的修复块划分。
type SubmitBlocksTool struct {
	Blocks []StyleBlock
	Set    bool
}

func (t *SubmitBlocksTool) Name() string { return "submit_blocks" }

func (t *SubmitBlocksTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit_blocks",
		"description": "Submit the chapter fix blocks. Each block: {title, problem_ids, chapters} — group chapters that share the same problems into ONE block; every affected chapter must appear in exactly one block.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blocks": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":       map[string]any{"type": "string"},
						"problem_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"chapters":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"title", "problem_ids", "chapters"},
				}},
			},
			"required": []string{"blocks"},
		},
	}}
}

func (t *SubmitBlocksTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["blocks"].([]interface{})
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: blocks must be a non-empty array."}, nil
	}
	var out []StyleBlock
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		b := StyleBlock{Title: strArg(m, "title")}
		for _, k := range []string{"problem_ids", "chapters"} {
			if vs, ok := m[k].([]interface{}); ok {
				for _, v := range vs {
					if s, ok := v.(string); ok {
						if k == "problem_ids" {
							b.ProblemIDs = append(b.ProblemIDs, s)
						} else {
							b.Chapters = append(b.Chapters, s)
						}
					}
				}
			}
		}
		if b.Title == "" || len(b.Chapters) == 0 {
			return session.ToolResult{Text: "REJECTED: each block needs a title and at least one chapter."}, nil
		}
		out = append(out, b)
	}
	t.Blocks, t.Set = out, true
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. %d blocks recorded.", len(out))}, nil
}

// SubmitBlockFixTool 收分块修复会话的逐章结论。提交后代码把工作区里
// 块内章节的 .tex 与 parts 目录拷回 work/chapters/。
type SubmitBlockFixTool struct {
	// ChapterResults: chapter base → 是否已解决（false=仍有问题）。
	ChapterResults map[string]bool
	Notes          map[string]string
	Submitted      bool
}

func (t *SubmitBlockFixTool) Name() string { return "submit" }

func (t *SubmitBlockFixTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit",
		"description": "Submit the fixed chapters of this block. chapters: [{chapter (base name, e.g. chapter_003), resolved (true/false), note}] — one entry per chapter in your block.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapters": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"chapter":  map[string]any{"type": "string"},
						"resolved": map[string]any{"type": "boolean"},
						"note":     map[string]any{"type": "string"},
					},
					"required": []string{"chapter", "resolved"},
				}},
			},
			"required": []string{"chapters"},
		},
	}}
}

func (t *SubmitBlockFixTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["chapters"].([]interface{})
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: chapters must be a non-empty array."}, nil
	}
	t.ChapterResults, t.Notes = map[string]bool{}, map[string]string{}
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		base := strArg(m, "chapter")
		resolved, _ := m["resolved"].(bool)
		t.ChapterResults[base] = resolved
		t.Notes[base] = strArg(m, "note")
	}
	if len(t.ChapterResults) == 0 {
		return session.ToolResult{Text: "REJECTED: no valid chapter entries."}, nil
	}
	t.Submitted = true
	return session.ToolResult{Text: "SUBMITTED. Reply with a one-line confirmation and nothing else."}, nil
}

// collectIssueReports 汇总工作汇报，返回 (报告总数, 有问题的报告 base→内容)。
func collectIssueReports(proj string) (int, map[string]string) {
	reportsDir := filepath.Join(proj, "work", "reports")
	files, _ := filepath.Glob(filepath.Join(reportsDir, "*.md"))
	issues := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "- 结论: 存在问题") {
			issues[strings.TrimSuffix(filepath.Base(f), ".md")] = string(data)
		}
	}
	return len(files), issues
}

// styleFeedbackLoop 是 T43 的入口：任何章节汇报问题即进入风格修复大循环。
func (r *Runner) styleFeedbackLoop(proj string, round int) error {
	total, issues := collectIssueReports(proj)
	if len(issues) == 0 {
		return nil
	}
	r.log.Log(0, "[style-fix] 工作汇报:", strconv.Itoa(total), "份，其中",
		strconv.Itoa(len(issues)), "份报告 cls/手册问题")
	r.phaseNote()("[style-fix] 工作汇报 %d 份，其中 %d 份报告 cls/手册问题", total, len(issues))

	maxRounds := r.cfg.Latex.StyleFix.MaxRounds
	if maxRounds < 0 {
		r.log.LogWarning(0, "[style-fix] style_fix.max_rounds<0，跳过风格修复（问题留给 checker/终审）")
		return nil
	}

	// 问题清单（整合会话跑一轮，跨轮复用——修复轮之间问题集不变，变的是修复结果）。
	problems, err := r.styleFixIntegrate(proj, issues)
	if err != nil {
		return fmt.Errorf("问题整合会话失败: %w", err)
	}
	r.log.Log(0, "[style-fix] 问题清单:", strconv.Itoa(len(problems)), "项")

	for round = 1; round <= maxRounds; round++ {
		r.phaseNote()("[style-fix] 第 %d/%d 轮风格修复", round, maxRounds)

		// 1. 风格修复会话：按问题清单修 cls/手册。
		if err := r.styleFixStyleSession(proj, round, problems); err != nil {
			return fmt.Errorf("风格修复会话（第 %d 轮）失败: %w", round, err)
		}

		// 2. 影响评估：按问题划分章节块。
		blocks, err := r.styleFixAssess(proj, round, problems, issues)
		if err != nil {
			return fmt.Errorf("影响评估会话（第 %d 轮）失败: %w", round, err)
		}
		r.log.Log(0, "[style-fix] 修复块:", strconv.Itoa(len(blocks)), "块")

		// 3. 分块并行修复（定位修改，绝不重做）。
		remaining := r.styleFixBlocks(proj, round, blocks, problems)

		// 4. 收尾判定。
		if len(remaining) == 0 {
			r.log.Log(0, "[style-fix] 第", strconv.Itoa(round), "轮后所有问题已解决")
			r.phaseNote()("[style-fix] 第 %d 轮后所有问题已解决", round)
			return nil
		}
		r.log.LogWarning(0, "[style-fix] 第", strconv.Itoa(round), "轮后仍有",
			strconv.Itoa(len(remaining)), "章报告问题:", strings.Join(remaining, ", "))
	}
	r.log.LogWarning(0, "[style-fix] 已达最大轮数("+strconv.Itoa(maxRounds)+")，停止风格修复循环（遗留问题交给编译/终审）")
	r.phaseNote()("[style-fix] 已达最大轮数(%d)，遗留问题交给编译/终审", maxRounds)
	return nil
}

// styleFixIntegrate 跑问题整合会话。
func (r *Runner) styleFixIntegrate(proj string, issues map[string]string) ([]StyleProblem, error) {
	var b strings.Builder
	keys := make([]string, 0, len(issues))
	for k := range issues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("\n--- " + k + " ---\n")
		b.WriteString(issues[k])
		b.WriteString("\n")
	}

	submit := &SubmitProblemsTool{}
	tools := []session.Tool{submit}
	sess := session.NewSession(r.clientFor(r.cfg.Latex.StyleModel), r.modelOf(r.cfg.Latex.StyleModel),
		r.cfg.LatexSession("style"), renderPrompt(prompts.Must(prompts.StyleFixIntegrateSystem), r.cfg.LatexSession("style"), r.outputLang()),
		tools, r.log, 1, "stylefix-integrate")
	trPath := filepath.Join(proj, "work", "sessions", "stylefix_integrate.jsonl")
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("stylefix-integrate", "stylefix-integrate")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := "These are the work reports from the chapter conversion sessions that flagged style/manual problems:\n" + b.String() +
		"\nConsolidate them into a de-duplicated problem list and call submit_problems."
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, err
	}
	if !submit.Set {
		return nil, fmt.Errorf("整合会话未提交 submit_problems")
	}
	r.keepSessionFile(trPath)
	return submit.Problems, nil
}

// styleFixStyleSession 跑一轮风格修复会话（复用原样式会话上下文）。
func (r *Runner) styleFixStyleSession(proj string, round int, problems []StyleProblem) error {
	ctxPath := filepath.Join(proj, "work", "style_session.jsonl")
	msgs, err := loadSessionContext(ctxPath)
	if err != nil {
		return fmt.Errorf("样式会话上下文不可用: %w", err)
	}
	styleDir := filepath.Join(proj, "style")
	workDir := filepath.Join(proj, "work", "style")

	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.modelOf(r.cfg.Latex.StyleModel)
	tuning := r.cfg.LatexSession("style")

	submit := &SubmitStyleTool{Workspace: workDir}
	tag := fmt.Sprintf("style-fix-style-r%d", round)
	bashTmp, cleanBash := r.sessionBashTemp(proj, "bash_stylefix")
	defer cleanBash()
	tools := r.styleSessionTools(proj, workDir, filepath.Join(proj, "source"), tag, bashTmp, submit)

	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, tag)
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()
	sess.SetMessages(msgs)
	trPath := filepath.Join(proj, "work", "sessions", fmt.Sprintf("stylefix_style_r%d.jsonl", round))
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}

	var plist strings.Builder
	for _, p := range problems {
		fmt.Fprintf(&plist, "- %s %s: %s (reported by: %s)\n", p.ID, p.Title, p.Detail, strings.Join(p.Chapters, ", "))
	}
	feedback := "The chapter conversion phase finished and the consolidated problem list below shows where the class/manual did NOT satisfy the book's real formatting. Fix the cls/manual/example so these problems cannot recur, then submit_style.\n\n" + plist.String()
	if _, err := sess.Run(session.RunOptions{UserText: feedback}); err != nil {
		return fmt.Errorf("风格修复会话失败: %w", err)
	}
	if err := saveSessionContext(sess, ctxPath); err != nil {
		r.log.LogWarning(1, "[style-fix] 会话上下文回写失败:", err)
	}
	if !submit.Set {
		return fmt.Errorf("风格修复会话未提交 submit_style")
	}
	clsName := classNameOf(submit.Cls)
	if clsName == "" {
		return fmt.Errorf("风格修复提交的 cls 缺少 \\ProvidesClass{...}")
	}
	if err := os.WriteFile(filepath.Join(styleDir, clsName+".cls"), []byte(submit.Cls), 0o644); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
	_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
	r.writeStyleExtras(styleDir, submit)

	// example 编译验证（与样式阶段同一门槛）。
	scratch, cleanScratch, err := r.tempDir(proj, "stylefix-check")
	if err != nil {
		return err
	}
	defer cleanScratch()
	if err := copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex")); err != nil {
		return err
	}
	copyStyleExtras(styleDir, scratch, submit, clsName)
	start := time.Now()
	res := r.comp.Compile(scratch, "example.tex")
	LogCompileResult(r.log, 1, tag, res, time.Since(start))
	if !res.OK {
		return fmt.Errorf("修复后的 example 编译失败: %s", res.Err)
	}
	r.keepSessionFile(trPath)
	r.log.Log(1, "[style-fix] 样式包已更新:", clsName+".cls")
	return nil
}

// styleFixAssess 跑影响评估会话：按问题划分章节块。
func (r *Runner) styleFixAssess(proj string, round int, problems []StyleProblem, issues map[string]string) ([]StyleBlock, error) {
	chapWork := filepath.Join(proj, "work", "chapters")
	texs, _ := filepath.Glob(filepath.Join(chapWork, "*.tex"))
	var chapterNames []string
	for _, t := range texs {
		chapterNames = append(chapterNames, strings.TrimSuffix(filepath.Base(t), ".tex"))
	}
	sort.Strings(chapterNames)

	var plist strings.Builder
	for _, p := range problems {
		fmt.Fprintf(&plist, "- %s %s: %s (reported by: %s)\n", p.ID, p.Title, p.Detail, strings.Join(p.Chapters, ", "))
	}
	var issueList []string
	for k := range issues {
		issueList = append(issueList, k)
	}
	sort.Strings(issueList)

	submit := &SubmitBlocksTool{}
	mounts := []Mount{{Name: "project", Dir: proj}}
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&GrepTool{Mounts: mounts},
		submit,
	}
	tag := fmt.Sprintf("stylefix-assess-r%d", round)
	sess := session.NewSession(r.clientFor(r.cfg.Latex.StyleModel), r.modelOf(r.cfg.Latex.StyleModel),
		r.cfg.LatexSession("style"), renderPrompt(prompts.Must(prompts.StyleFixAssessSystem), r.cfg.LatexSession("style"), r.outputLang()),
		tools, r.log, 1, tag)
	trPath := filepath.Join(proj, "work", "sessions", fmt.Sprintf("stylefix_assess_r%d.jsonl", round))
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := "Problem list:\n" + plist.String() +
		"\nChapters that explicitly reported problems: " + strings.Join(issueList, ", ") +
		"\nAll converted chapters: " + strings.Join(chapterNames, ", ") +
		"\nTheir .tex files are readable at project:work/chapters/<name>.tex. Style problems are often GLOBAL — assess which chapters each problem actually affects (grep the .tex files for the pattern when in doubt), then partition the affected chapters into fix blocks and call submit_blocks."
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, err
	}
	if !submit.Set {
		return nil, fmt.Errorf("评估会话未提交 submit_blocks")
	}
	r.keepSessionFile(trPath)
	return submit.Blocks, nil
}

// styleFixBlocks 并行跑分块修复会话，返回仍有问题的章节列表。
// 块会话失败的章节退回单章 fixChapterStyle，再失败删产物重转换。
func (r *Runner) styleFixBlocks(proj string, round int, blocks []StyleBlock, problems []StyleProblem) []string {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	chapWork := filepath.Join(proj, "work", "chapters")
	reportsDir := filepath.Join(proj, "work", "reports")

	probText := func(ids []string) string {
		var b strings.Builder
		for _, p := range problems {
			for _, id := range ids {
				if p.ID == id {
					fmt.Fprintf(&b, "- %s %s: %s\n", p.ID, p.Title, p.Detail)
				}
			}
		}
		return b.String()
	}

	conc := r.cfg.Latex.Concurrency
	if conc <= 0 {
		conc = 3
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var remaining []string // 仍有问题
	var fallback []string  // 块会话失败 → 退回单章修复

	for bi, block := range blocks {
		wg.Add(1)
		tid := <-tidPool
		go func(bi int, block StyleBlock, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			results, notes, err := r.styleFixBlockSession(proj, round, bi+1, clsName, block, probText(block.ProblemIDs), tid)
			if err != nil {
				r.log.LogWarning(tid, "[style-fix] 块", strconv.Itoa(bi+1), "会话失败，退回单章修复:", err)
				mu.Lock()
				fallback = append(fallback, block.Chapters...)
				mu.Unlock()
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, base := range block.Chapters {
				resolved, ok := results[base]
				note := notes[base]
				reportPath := filepath.Join(reportsDir, base+".md")
				if !ok {
					// 会话没汇报这一章：保守视为仍有问题。
					resolved = false
					note = "块修复会话未汇报本章"
				}
				conclusion := "无问题"
				if !resolved {
					conclusion = "存在问题"
					remaining = append(remaining, base)
				}
				report := "- 结论: " + conclusion + "\n\n样式修复（第 " + strconv.Itoa(round) + " 轮，块 " + strconv.Itoa(bi+1) + "）: " + note + "\n"
				_ = os.MkdirAll(reportsDir, 0o755)
				_ = os.WriteFile(reportPath, []byte(report), 0o644)
			}
		}(bi, block, tid)
	}
	wg.Wait()

	// 块会话失败的章节退回单章增量修复（复用既有 fixChapterStyle）。
	for _, base := range fallback {
		texPath := filepath.Join(chapWork, base+".tex")
		if !fileExists(texPath) {
			continue
		}
		issues := ""
		if data, err := os.ReadFile(filepath.Join(reportsDir, base+".md")); err == nil {
			issues = string(data)
		}
		if err := r.fixChapterStyle(proj, clsName, filepath.Join(proj, "style", "manual.md"),
			filepath.Join(proj, "chapters", base+".md"), filepath.Join(proj, "work"), base, issues, 1); err != nil {
			r.log.LogWarning(1, "[style-fix]", base, "单章修复也失败，删除产物待重转换:", err)
			_ = os.RemoveAll(texPath)
			_ = os.RemoveAll(filepath.Join(chapWork, base))
			r.keepSessionFile(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"))
			mu.Lock()
			remaining = append(remaining, base)
			mu.Unlock()
		}
	}
	sort.Strings(remaining)
	return remaining
}

// styleFixBlockSession 跑一个分块修复会话：块内章节的 .tex/parts 拷进临时
// 工作区，会话在里面做定位修改，提交后拷回 work/chapters/。
func (r *Runner) styleFixBlockSession(proj string, round, blockIdx int, clsName string, block StyleBlock, probText string, tid int) (map[string]bool, map[string]string, error) {
	chapWork := filepath.Join(proj, "work", "chapters")
	tag := fmt.Sprintf("stylefix-blk-r%d-%d", round, blockIdx)

	// 临时工作区：样式包 + 块内章节（.tex + parts 目录）+ 图片软链。
	work, cleanWork, err := r.tempDir(proj, tag)
	if err != nil {
		return nil, nil, err
	}
	defer cleanWork()
	copyFile(filepath.Join(proj, "style", clsName+".cls"), filepath.Join(work, clsName+".cls"))
	copyFile(filepath.Join(proj, "style", "manual.md"), filepath.Join(work, "manual.md"))
	copyFile(filepath.Join(proj, "style", "example.tex"), filepath.Join(work, "example.tex"))
	for _, asset := range []string{"images", "figures"} {
		src := filepath.Join(proj, "source", asset)
		if !fileExists(src) {
			continue
		}
		if err := linkAbs(src, filepath.Join(work, asset)); err != nil {
			_ = copyDir(src, filepath.Join(work, asset))
		}
	}
	chapSub := filepath.Join(work, "chapters")
	_ = os.MkdirAll(chapSub, 0o755)
	for _, base := range block.Chapters {
		tex := filepath.Join(chapWork, base+".tex")
		if !fileExists(tex) {
			continue
		}
		copyFile(tex, filepath.Join(chapSub, base+".tex"))
		if parts := filepath.Join(chapWork, base); fileExists(parts) {
			_ = copyDir(parts, filepath.Join(chapSub, base))
		}
		// 每章一个编译 wrapper（工作区里可直接 compile 验证）。
		wrapper := "\\documentclass{" + clsName + "}\n" +
			"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
			"\\graphicspath{{figures/}}\n" +
			"\\begin{document}\n\\input{chapters/" + base + ".tex}\n\\end{document}\n"
		_ = os.WriteFile(filepath.Join(work, base+"_wrapper.tex"), []byte(wrapper), 0o644)
	}

	mounts := []Mount{{Name: "work", Dir: work}}
	submit := &SubmitBlockFixTool{}
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&EditWorkFileTool{Root: work},
		&WriteWorkFileTool{Root: work, AnyExt: true},
		&GrepTool{Mounts: mounts},
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&CompileTexTool{Comp: r.comp, Root: work, Log: r.log, Tid: tid, Tag: tag},
		submit,
	}
	sess := session.NewSession(r.clientFor(r.cfg.Latex.ConvertModel), r.modelOf(r.cfg.Latex.ConvertModel),
		r.cfg.LatexSession("convert"), renderPrompt(prompts.Must(prompts.StyleFixBlockSystem), r.cfg.LatexSession("convert"), r.outputLang()),
		tools, r.log, tid, tag)
	trPath := filepath.Join(proj, "work", "sessions", tag+".jsonl")
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := "Style problems to fix in this block:\n" + probText +
		"\nYour chapters (in work:chapters/): " + strings.Join(block.Chapters, ", ") +
		"\nThe class and manual in your workspace are the REVISED versions. Make minimal, targeted edits — never re-convert. " +
		"Compile each chapter's wrapper (<base>_wrapper.tex) to verify, then submit with one entry per chapter."
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, nil, err
	}
	if !submit.Submitted {
		return nil, nil, fmt.Errorf("块修复会话未提交")
	}

	// 拷回块内章节的产物。
	for base := range submit.ChapterResults {
		src := filepath.Join(chapSub, base+".tex")
		if fileExists(src) {
			copyFile(src, filepath.Join(chapWork, base+".tex"))
		}
		parts := filepath.Join(chapSub, base)
		if fileExists(parts) {
			_ = copyDir(parts, filepath.Join(chapWork, base))
		}
	}
	r.keepSessionFile(trPath)
	return submit.ChapterResults, submit.Notes, nil
}
