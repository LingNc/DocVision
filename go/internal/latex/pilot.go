package latex

// pilot.go — T97 重量档（v2）：样式试点，全部发生在 style 阶段内部、
// 章节划分之前，与真实章节无关。
//
// 三个子会话（都不复用彼此/原样式会话的上下文，各自独立转录）：
//
//	1. 选材会话：从全书挑"有代表性/有特点"的内容——md 行段 + 原书 PDF
//	   页，不论是否完整章节，pilot_spec 工具落盘 work/style/pilot/
//	   spec.json（T108，与 split/problems 同族，不裸写 JSON）；
//	2. 试转会话：把拼合出的 sample.md 用当前样式包转成 sample.tex、
//	   编译、对照选中的原书页核对到贴合为止；
//	3. 修订会话：拿试转产物与原书页修订 cls/manual/example，交出的
//	   样式包即最终样式。
//
// pilot 是建议性的：任何一步失败只警告并写 marker（内容即失败原因），
// 绝不挡后续阶段；marker 也让中断续跑不重复 pilot。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// pilotMarkerPath 是 pilot 的完成标记（内容为人可读的结果摘要）。
func pilotMarkerPath(proj string) string {
	return filepath.Join(proj, "work", "style", "pilot.done")
}

// pilotSpec 是选材会话的产出：md 行段（1 起、含端点）+ 原书 PDF 页。
type pilotSpec struct {
	MDRanges []struct {
		File  string `json:"file"`
		Start int    `json:"start"`
		End   int    `json:"end"`
	} `json:"md_ranges"`
	PDFPages []struct {
		File  string `json:"file"`
		Pages []int  `json:"pages"`
	} `json:"pdf_pages"`
}

// stylePilotPhase 依次跑「选材 → 试转 → 样式修订」三步，把样式包再磨一遍。
//
// T288：**会话级失败必须让整轮停下**。试点是在样式包之上做的一轮修订，它没跑
// 完就等于拿没修订的样式包往下走——接着划分、逐章转换，等发现不对时下游全是
// 废产物，只能整个删掉重跑。真实事故：选材会话被上游流式接口掐断（
// `SESSION_API_ERROR: stream error ... Upstream request failed`），日志只留一句
// 「选材失败（跳过试点）」，运行照旧进了划分与转换。
//
// 内容级失败（选材结果不合法、样例拼不出来）仍旧只警告跳过试点：那不是环境
// 问题，跳过试点并不影响后续阶段拿到一份可用的样式包。
func (r *Runner) stylePilotPhase(proj string) error {
	marker := pilotMarkerPath(proj)
	if fileExists(marker) {
		return nil
	}
	writeMarker := func(note string) {
		_ = os.WriteFile(marker, []byte(note+"\n"), 0o644)
	}
	sourceDir := filepath.Join(proj, "source")
	mds, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
	if len(mds) == 0 {
		return nil
	}
	sort.Strings(mds)
	mainMD := mds[0]
	for _, f := range mds { // 与 stylePhase 同规则：最大文件为正文
		if fi, err := os.Stat(f); err == nil {
			if mj, e2 := os.Stat(mainMD); e2 == nil && fi.Size() > mj.Size() {
				mainMD = f
			}
		}
	}
	r.phaseNote()("[style/pilot] 样式试点：选材 → 试转 → 修订（latex.style_pilot: false 可关）")

	spec, err := r.pilotSelectSession(proj, mainMD)
	if err != nil {
		if !pilotContentSkip(err) {
			return fmt.Errorf("样式试点选材会话中止（接口/会话级错误，整轮停在这里，样式包与下游都没动）: %w", err)
		}
		r.log.LogWarning(1, "[pilot] 选材失败（跳过试点）:", err)
		writeMarker("select failed: " + err.Error())
		return nil
	}
	sample, pagesDesc, err := r.buildPilotSample(proj, spec)
	if err != nil {
		r.log.LogWarning(1, "[pilot] 语料组装失败（跳过试点）:", err)
		writeMarker("sample failed: " + err.Error())
		return nil
	}
	if err := r.pilotTrialSession(proj, sample, pagesDesc); err != nil {
		if !pilotContentSkip(err) {
			return fmt.Errorf("样式试点试转会话中止（接口/会话级错误，整轮停在这里）: %w", err)
		}
		r.log.LogWarning(1, "[pilot] 试转失败（跳过样式修订）:", err)
		writeMarker("trial failed: " + err.Error())
		return nil
	}
	if err := r.pilotReviseSession(proj, pagesDesc); err != nil {
		if !pilotContentSkip(err) {
			return fmt.Errorf("样式试点修订会话中止（接口/会话级错误，整轮停在这里）: %w", err)
		}
		r.log.LogWarning(1, "[pilot] 样式修订失败（沿用样式阶段提交的样式包）:", err)
		writeMarker("revise failed: " + err.Error())
		return nil
	}
	writeMarker("done")
	r.log.Log(1, "[pilot] 样式试点完成，最终样式包已就位")
	return nil
}

// pilotContentSkip 区分"试点这一步内容上没做成"（可以只跳过试点）与"会话/接口
// 本身断了"（必须停）。接口不通、余额不足、会话错误都不是试点本身的问题——
// 让运行带着没修订的样式包往下走，代价是全下游白跑（T288）。
func pilotContentSkip(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return !isSessionAPIError(msg) && !isSessionFatalError(msg)
}

// pilotDirOf 是试点的工作根（work/style/pilot/）。
func pilotDirOf(proj string) string { return filepath.Join(proj, "work", "style", "pilot") }

// pilotSpecFromPrevRun 判断选材是否已经做过：转录里已有 submit 收账、且
// spec.json 里的条目仍然齐全时返回上次的结果，调用方据此跳过选材会话。
//
// 判据只认磁盘事实（转录有 submit + spec.json 合法），不看进程内存——重跑
// 是另一个进程，工具实例里什么都没有（T296）。
func (r *Runner) pilotSpecFromPrevRun(proj, trPath string) (*pilotSpec, bool) {
	specPath := filepath.Join(pilotDirOf(proj), "spec.json")
	if !fileExists(specPath) || !fileExists(trPath) {
		return nil, false
	}
	_, st, err := session.LoadTranscriptStats(trPath)
	if err != nil {
		return nil, false
	}
	submit := &SubmitDoneTool{Label: "the pilot selection"}
	if !replaySubmit(r.log, 1, "pilot-select", submit, st) {
		return nil, false
	}
	tool := NewPilotSpecTool(specPath, filepath.Join(proj, "source"))
	spec := specFromTool(tool.loadLocked())
	if len(spec.MDRanges) == 0 || len(spec.PDFPages) == 0 {
		return nil, false
	}
	r.log.Log(1, "[pilot] 上次已经选好材（转录已提交、spec.json 就绪）——跳过选材会话，直接沿用")
	return spec, true
}

// pilotSelectSession 跑选材会话：产出 pilot/spec.json（write_file 写入，
// submit 终态；代码复核文件存在，缺失打回一次）。试点会话短，中断后不
// 续跑——重跑前归档旧转录。
func (r *Runner) pilotSelectSession(proj, mainMD string) (*pilotSpec, error) {
	pilotDir := pilotDirOf(proj)
	if err := os.MkdirAll(pilotDir, 0o755); err != nil {
		return nil, err
	}
	selectKey := r.cfg.Latex.PilotSelectModel // T108：空 = 沿用章节划分模型
	if selectKey == "" {
		selectKey = r.cfg.Latex.ChapterModel
	}
	client := r.clientFor(selectKey)
	modelCfg := r.modelOf(selectKey)
	tuning := r.cfg.LatexSession("style")
	goal := NewGoalTool(filepath.Join(pilotDir, "plan_select.md"))
	submit := &SubmitDoneTool{Label: "the pilot selection"}
	specTool := NewPilotSpecTool(filepath.Join(pilotDir, "spec.json"), filepath.Join(proj, "source"))
	tools := []session.Tool{
		&ReadFileTool{Root: pilotDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&GrepTool{Root: pilotDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		specTool,
		&ViewPDFTool{Root: pilotDir, Mounts: r.sessionMounts(kindStyle, pilotDir), Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		goal, submit,
	}
	tools = append(tools, r.sourcePageTools()...)

	trPath := filepath.Join(proj, "work", "sessions", "style_pilot_select.jsonl")
	// 选材是"读 md + 读原书页挑段落"，与样式包、模型都无关：上次已经选好
	// （转录里 submit 收账、spec.json 条目仍合法）就沿用，别再花四十分钟把
	// 同样的段落重挑一遍——重跑试点/样式只为换交上去的 cls 时才需要（T296）。
	if spec, ok := r.pilotSpecFromPrevRun(proj, trPath); ok {
		return spec, nil
	}

	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.StylePilotSelectSystem), tuning, r.outputLang()),
		tools, r.log, 1, "pilot-select")
	archivePrevTranscript(trPath)
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("pilot-select", "pilot-select")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := prompts.Render(prompts.StylePilotSelectUser, map[string]string{
		"MAIN_MD": "project:source/" + filepath.Base(mainMD),
	})
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return nil, fmt.Errorf("选材会话失败: %w", err)
		}
		if !r.remindSubmit(sess, 1, "[pilot]", "pilot-select", "submit", func() bool { return submit.Submitted }) {
			return nil, fmt.Errorf("选材会话未提交（已提醒两次）")
		}
		if fileExists(filepath.Join(pilotDir, "spec.json")) {
			break
		}
		userText = "You submitted but the selection is empty — add your md ranges and pdf pages with pilot_spec, then submit again."
		submit.Submitted = false
	}
	spec := specFromTool(specTool.loadLocked())
	if len(spec.MDRanges) == 0 || len(spec.PDFPages) == 0 {
		return nil, fmt.Errorf("选材需要至少一条 md 行段与一条 pdf 页集合")
	}
	return spec, nil
}

// buildPilotSample 把 spec 拼成 pilot/sample.md（越界行段/页码裁剪并
// 警告，全无效才报错），返回样本文路径与给人/模型看的页清单描述。
func (r *Runner) buildPilotSample(proj string, spec *pilotSpec) (string, string, error) {
	pilotDir := pilotDirOf(proj)
	if err := os.MkdirAll(pilotDir, 0o755); err != nil {
		return "", "", err
	}
	sourceDir := filepath.Join(proj, "source")
	var sb strings.Builder
	used := 0
	for _, rg := range spec.MDRanges {
		data, err := os.ReadFile(filepath.Join(sourceDir, filepath.Base(rg.File)))
		if err != nil {
			r.log.LogWarning(1, "[pilot] 选材行段文件不可读（跳过）:", rg.File)
			continue
		}
		lines := strings.Split(string(data), "\n")
		start, end := rg.Start, rg.End
		if start < 1 {
			start = 1
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > end {
			r.log.LogWarning(1, "[pilot] 选材行段越界（跳过）:", rg.File, rg.Start, "-", rg.End)
			continue
		}
		fmt.Fprintf(&sb, "<!-- pilot 选材：%s 第 %d-%d 行 -->\n\n", filepath.Base(rg.File), start, end)
		sb.WriteString(strings.Join(lines[start-1:end], "\n"))
		sb.WriteString("\n\n")
		used++
	}
	if used == 0 {
		return "", "", fmt.Errorf("所有选材行段都无效")
	}
	sample := filepath.Join(pilotDir, "sample.md")
	if err := os.WriteFile(sample, []byte(sb.String()), 0o644); err != nil {
		return "", "", err
	}
	// 页清单描述（试转/修订会话拿它 view_pdf 对照原书页）；越界页丢弃。
	var parts []string
	for _, pp := range spec.PDFPages {
		var ok []string
		for _, p := range pp.Pages {
			if p >= 1 && (r.pdfView == nil || p <= r.pdfView.total) {
				ok = append(ok, strconv.Itoa(p))
			}
		}
		if len(ok) > 0 {
			parts = append(parts, fmt.Sprintf("%s 页 %s", pp.File, strings.Join(ok, ",")))
		}
	}
	if len(parts) == 0 {
		return "", "", fmt.Errorf("所有选材页都无效")
	}
	return sample, strings.Join(parts, "；"), nil
}

// pilotImageRef 匹配 sample.md 里的图片引用（](images/…)）。
// 路径里允许空格：项目名/分片目录带空格时，图片路径就是
// `images/<项目名> /<hash>.jpg`，按空白截断只会取到半个目录名。
var pilotImageRef = regexp.MustCompile(`\]\((images/[^)]+)\)`)

// stagePilotImages 把 sample.md 引用的插图按相对路径复制进试转工作区，
// 返回没能复制过去的相对路径（调用方据此告警）。重复引用只复制一次。
func stagePilotImages(proj, samplePath, work string) []string {
	data, err := os.ReadFile(samplePath)
	if err != nil {
		return []string{"（读不到 " + filepath.Base(samplePath) + "）"}
	}
	var missing []string
	seen := map[string]bool{}
	for _, m := range pilotImageRef.FindAllStringSubmatch(string(data), -1) {
		rel := filepath.FromSlash(strings.TrimSpace(m[1]))
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		dst := filepath.Join(work, rel)
		// copyFile 不建父目录（图片在 images/<目录>/ 下面，试转工作区里还没有），
		// 不先建出来就只会在 ReadFile/WriteFile 上失败。
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			missing = append(missing, rel)
			continue
		}
		if err := copyFile(filepath.Join(proj, "source", rel), dst); err != nil {
			missing = append(missing, rel)
		}
	}
	return missing
}

// pilotTrialSession 跑试转会话：pilot/work/ 里铺好 cls+手册+示例+
// sample.md+引用的插图，转成 sample.tex 并编译、对照原书页核对到贴合。
// 提交后代码复核 sample.tex 存在且编译通过（一次打回机会）。
func (r *Runner) pilotTrialSession(proj, samplePath, pagesDesc string) error {
	styleDir := filepath.Join(proj, "style")
	clsName := classNameOfFile(styleDir)
	if clsName == "" {
		return fmt.Errorf("样式包缺失")
	}
	pilotDir := pilotDirOf(proj)
	work := filepath.Join(pilotDir, "work")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(work, clsName+".cls"))
	copyFile(filepath.Join(styleDir, "manual.md"), filepath.Join(work, "manual.md"))
	copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(work, "example.tex"))
	copyFile(samplePath, filepath.Join(work, "sample.md"))
	// 铺插图：sample.md 引用的图片按相对路径复制进试转工作区。取不到的
	// 逐条告警——以前 copyFile 的错误被直接丢掉，路径里带空格时整批图片
	// 一张都没铺进去，会话只能拿占位框顶替，还在报告里写"没有原始位图"。
	for _, miss := range stagePilotImages(proj, samplePath, work) {
		r.log.LogWarning(0, "[pilot] 试转工作区缺这张插图的副本:", miss)
	}

	trialKey := r.cfg.Latex.PilotTrialModel // T108：空 = 沿用转换模型
	if trialKey == "" {
		trialKey = r.cfg.Latex.ConvertModel
	}
	client := r.clientFor(trialKey)
	modelCfg := r.modelOf(trialKey)
	tuning := r.cfg.LatexSession("convert")
	goal := NewGoalTool(filepath.Join(work, "plan.md"))
	submit := &SubmitDoneTool{Label: "the pilot conversion"}
	mounts := r.sessionMounts(kindStyle, work)
	bashTmp, cleanBash := r.sessionBashTemp(proj, "bash_pilot_trial")
	defer cleanBash()
	tools := []session.Tool{
		&ReadFileTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&WriteWorkFileTool{Root: work, AnyExt: true},
		&EditWorkFileTool{Root: work},
		&GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&CompileTexTool{Comp: r.comp, Root: work, MainFile: "sample.tex", Tag: "pilot-trial", Log: r.log, Tid: 1},
		&ViewPDFTool{Root: work, Mounts: mounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		&WorkBashTool{Root: work, Mounts: mounts, TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		goal, submit,
	}
	tools = append(tools, r.sourcePageTools()...)

	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.StylePilotTrialSystem), tuning, r.outputLang()),
		tools, r.log, 1, "pilot-trial")
	trPath := filepath.Join(proj, "work", "sessions", "style_pilot_trial.jsonl")
	archivePrevTranscript(trPath)
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("pilot-trial", "pilot-trial")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := prompts.Render(prompts.StylePilotTrialUser, map[string]string{
		"CLS": clsName, "PAGES": pagesDesc,
	})
	var compileErr string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			userText = "sample.tex still fails to compile:\n" + truncateStr(compileErr, 8000) +
				"\n\nFix it (you may also fix the class file), compile again, then submit."
			submit.Submitted = false
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("试转会话失败: %w", err)
		}
		if !r.remindSubmit(sess, 1, "[pilot]", "pilot-trial", "submit", func() bool { return submit.Submitted }) {
			return fmt.Errorf("试转会话未提交（已提醒两次）")
		}
		if !fileExists(filepath.Join(work, "sample.tex")) {
			return fmt.Errorf("试转会话提交了但没有产出 sample.tex")
		}
		res := r.comp.Compile(work, "sample.tex")
		if res.OK {
			r.log.Log(1, "[pilot] 试转编译通过:", work)
			return nil
		}
		compileErr = res.Err
	}
	return fmt.Errorf("sample.tex 打回一次后仍编译失败: %s", truncateStr(compileErr, 200))
}

// pilotReviseSession 跑样式修订会话：**新会话**（不复用原样式会话上下文，
// T97 用户口径），工作区即样式工作区（cls/manual/example 草稿与 pilot/
// 试转产物都在），对照试转产物与原书页修订出最终样式包。修订后
// example 必须仍编译通过（一次打回机会）。
func (r *Runner) pilotReviseSession(proj, pagesDesc string) error {
	workDir := filepath.Join(proj, "work", "style")
	styleDir := filepath.Join(proj, "style")
	reviseKey := r.cfg.Latex.PilotReviseModel // T108：空 = 沿用样式模型
	if reviseKey == "" {
		reviseKey = r.cfg.Latex.StyleModel
	}
	client := r.clientFor(reviseKey)
	modelCfg := r.modelOf(reviseKey)
	tuning := r.cfg.LatexSession("style")
	goal := NewGoalTool(filepath.Join(workDir, "plan.md"))
	submit := &SubmitStyleTool{Workspace: workDir, Gate: func() string { return goalGate(goal) }}
	bashTmp, cleanBash := r.sessionBashTemp(proj, "bash_pilot_revise")
	defer cleanBash()
	tools := r.styleSessionTools(proj, workDir, filepath.Join(proj, "source"), "pilot-revise", bashTmp, submit)
	tools = append(tools, goal)

	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, "pilot-revise")
	trPath := filepath.Join(proj, "work", "sessions", "style_pilot_revise.jsonl")
	archivePrevTranscript(trPath)
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("pilot-revise", "pilot-revise")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := prompts.Render(prompts.StylePilotReviseUser, map[string]string{"PAGES": pagesDesc})
	var compileErr string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			userText = "The revised example failed to compile:\n" + truncateStr(compileErr, 8000) +
				"\n\nFix the cls/example and call submit_style again."
			submit.Set = false
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("样式修订会话失败: %w", err)
		}
		if !r.remindSubmit(sess, 1, "[pilot]", "pilot-revise", "submit_style", func() bool { return submit.Set }) {
			return fmt.Errorf("样式修订会话未提交 submit_style（已提醒两次）")
		}
		cls := classNameOf(submit.Cls)
		if cls == "" {
			compileErr = "cls 缺少 \\ProvidesClass{...}"
			continue
		}
		if err := os.WriteFile(filepath.Join(styleDir, cls+".cls"), []byte(submit.Cls), 0o644); err != nil {
			return err
		}
		_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
		_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
		r.writeStyleExtras(styleDir, submit)
		scratch, cleanScratch, err := r.tempDir(proj, "pilot_revise")
		if err != nil {
			return err
		}
		copyFile(filepath.Join(styleDir, cls+".cls"), filepath.Join(scratch, cls+".cls"))
		copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex"))
		copyStyleExtras(styleDir, scratch, submit, cls)
		res := r.comp.Compile(scratch, "example.tex")
		cleanScratch()
		if res.OK {
			r.log.Log(1, "[pilot] 修订后样式包编译通过:", cls+".cls")
			return nil
		}
		compileErr = res.Err
	}
	return fmt.Errorf("修订后的 example 编译失败: %s", truncateStr(compileErr, 200))
}
