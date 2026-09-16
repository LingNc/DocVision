package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// BookOptions drives the level-1 full-book pipeline.
type BookOptions struct {
	// Step limits the run to one phase: "" (all missing), "style",
	// "chapters", "convert", "assemble".
	Step string
	// SourceDir overrides paths.output_dir.
	SourceDir string
	// Project overrides the project name (--project): the workspace
	// becomes <paths.latex_project>/<项目名>/. Default: the book's
	// 主题名 (main markdown base name, the images/<主题>/ name).
	Project string
	// Restart re-runs phases already marked done.
	Restart bool
	// TestMode / Number / Seed are forwarded to the level-2 image pass.
	TestMode bool
	Number   int
	Seed     string
	// Files selects specific markdown files (empty = all).
	Files []string
	// Verbose keeps per-phase detail output on the console (session
	// rounds, tool calls, compile results). Default (false) shows one
	// compact progress line per phase — like the level-2 process line —
	// and sends every detail line to the log file only.
	Verbose bool
}

// fmtDuration renders a duration compactly for phase progress lines:
// "42.3s" under a minute, "4m12s" above.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// livePhaseRow manages the compact console status line of a
// single-session phase / sub-session (style / chapters / style-feedback /
// per-chapter convert / checker / style-fix): ONE live row per owner, fed by
// the session's progress notifications plus a ticker keeping the elapsed time
// fresh, so concurrent sub-sessions show up side by side instead of fighting
// over a single status line. In verbose mode everything is a no-op (the
// logger already streams the details). fin() is idempotent: it removes the
// row and (optionally) leaves a final line behind.
//
// id names the owner and MUST be unique per concurrent owner — a shared id
// would make two sub-sessions overwrite each other's line (the bug this
// replaced: "[style-feedback] 轮次 33 · 已用 7m06s" and "[style-fix] …"
// alternating with both timers running, because the single live slot held
// whichever owner had drawn last).
func (r *Runner) livePhaseRow(id, label string) (hook func(rounds, tools int), fin func()) {
	return r.livePhaseRowAt(id, label, time.Now())
}

// livePhaseRowAt 是 livePhaseRow 的续跑形态（T28）：start 可以前移到
// 「现在 − 转录历史活跃跨度」，让"已用"从上次中断处继续累计，而不是
// 每次续跑都从十几秒看起来像重新开了一场。
func (r *Runner) livePhaseRowAt(id, label string, start time.Time) (hook func(rounds, tools int), fin func()) {
	if r.consoleVerbose {
		return func(int, int) {}, func() {}
	}
	var mu sync.Mutex
	var rounds, tools int
	row := r.log.LiveRow(id)
	render := func() {
		mu.Lock()
		rounds, tools := rounds, tools
		mu.Unlock()
		row.Update("[%s] 轮次 %d · 工具调用 %d · 已用 %s",
			label, rounds, tools, fmtDuration(time.Since(start)))
	}
	render() // 立即出现起始行，会话一开始就有进度
	stop := make(chan struct{})
	done := make(chan struct{})
	// 终端每秒刷新"已用"时长（用户实测：轮次/工具数在动、时间却像不动）；
	// 管道模式下每次变化只追加一行（logger 的 paint 自己会去重），
	// 所以同样保持 10 秒节拍免得刷爆日志。
	tick := 10 * time.Second
	if r.log.TTY() {
		tick = time.Second
	}
	ticker := time.NewTicker(tick)
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				render()
			}
		}
	}()
	var once sync.Once
	return func(rr, tt int) {
			mu.Lock()
			rounds, tools = rr, tt
			mu.Unlock()
			render()
		}, func() {
			once.Do(func() {
				close(stop)
				ticker.Stop()
				<-done // 汇合：已在途的一帧不得画在清行之后
				// 行是"会消失的"：阶段结束时从实时块里摘掉。T28：
				// 把定格行（轮次/工具调用/已用）作为普通行留在滚动区——
				// 每个跑过的流程在终端上都留着最终状态，下一个流程开新
				// 行实时刷，不会被顶掉。
				row.Remove()
				mu.Lock()
				finalText := fmt.Sprintf("[%s] 轮次 %d · 工具调用 %d · 已用 %s",
					label, rounds, tools, fmtDuration(time.Since(start)))
				mu.Unlock()
				r.log.PrintConsole(finalText)
			})
		}
}

// convertProgressText renders the level-1 [convert] progress line. It does
// NOT add a trailing "]" or a "\r": the label already carries its brackets
// and the caller (liveProgress) owns redraw/termination — the old inline
// Fprintf produced "[convert] 0/3] 0.00%" with a stray bracket and bare CRs
// in pipes.
func convertProgressText(label string, done, failed, running, total int, elapsed time.Duration) string {
	pct := 0.0
	if total > 0 {
		pct = float64(done) * 100.0 / float64(total)
	}
	return fmt.Sprintf("%s %d/%d %.2f%% (done: %d, errors: %d, running: %d, %s)",
		label, done, total, pct, done-failed, failed, running, fmtDuration(elapsed))
}

// bookProgress is the persisted phase state of a book project.
type bookProgress struct {
	Style    string `json:"style"`
	Images   string `json:"images"`
	Chapters string `json:"chapters"`
	Convert  string `json:"convert"`
	Assemble string `json:"assemble"`
}

// classNameRe extracts the class name from \ProvidesClass{...}.
var classNameRe = regexp.MustCompile(`\\ProvidesClass\{([^}]*)\}`)

// RunBook executes the level-1 pipeline on top of the level-2 image
// pass:
//
//  1. images    : level-2 processing of the source md (into <proj>/source)
//  2. style     : style-analysis session -> book.cls + manual + example
//  3. chapters  : chapter-split session -> <proj>/chapters/*.md
//  4. convert   : concurrent per-chapter md -> tex sessions
//  5. assemble  : main.tex + full-book compile (+ fix session) + standalone.tex
func (r *Runner) RunBook(opts BookOptions) error {
	cfg := r.cfg
	// 项目根只在这里决定一次：<paths.latex_project>/<项目名>/（项目名
	// 默认取本书主题名，可用 --project 覆盖），或者——当输出根本身已是
	// 一个工程（旧版单项目布局）且未指定 --project——继续沿用输出根。
	// 下游（source/ style/ chapters/ work/ build/ out/ progress.json、
	// doc_index、pdfview、临时目录）全部从 proj 派生。
	sourceDir := opts.SourceDir
	if sourceDir == "" {
		sourceDir = cfg.Paths.OutputDir
	}
	proj, err := r.useProjectDir(cfg.Paths.LatexProject, opts.Project, projectSourceName(sourceDir, opts.Files), 1)
	if err != nil {
		return err
	}
	for _, d := range []string{
		proj, filepath.Join(proj, "source"), filepath.Join(proj, "style"),
		filepath.Join(proj, "chapters"), filepath.Join(proj, "work"),
		filepath.Join(proj, "build"), filepath.Join(proj, "out"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	writeFontsReadme(r.cfg.Paths.Fonts)
	progPath := filepath.Join(proj, "progress.json")
	prog := bookProgress{}
	if data, err := os.ReadFile(progPath); err == nil {
		_ = json.Unmarshal(data, &prog)
	}
	saveProg := func() {
		data, _ := json.MarshalIndent(prog, "", "  ")
		_ = os.WriteFile(progPath, data, 0o644)
	}
	// Default console behaviour mirrors the level-2 image pass: one
	// compact progress line per phase; every detail line (session
	// rounds, tool calls, compile results) goes to the log file only.
	verbose := opts.Verbose
	r.consoleVerbose = verbose
	if !verbose {
		r.log.SetQuiet(true)
		defer func() { r.log.SetQuiet(false) }()
	}
	// note prints a user-facing phase line directly to the console —
	// used only in compact mode (in verbose mode the logger already
	// shows everything).
	note := func(format string, a ...any) {
		if !verbose {
			// PrintConsole ends/redraws the live progress line, so a phase
			// line never lands on top of it (the log lines were fixed the
			// same way).
			r.log.PrintConsole(fmt.Sprintf(format, a...))
		}
	}

	// 续跑摘要（T28/T39）：压缩模式下控制台只看得到"从哪续上"， Completed
	// 阶段一行都不少地列出来，用户不用翻上一次运行的日志。日志文件同步
	// 记一份——note 只上控制台，不进文件。
	if !opts.Restart && opts.Step == "" {
		var done []string
		for _, p := range []string{"style", "images", "chapters", "convert", "assemble"} {
			if prog2Field(prog, p) == "done" {
				done = append(done, p)
			}
		}
		if len(done) > 0 {
			msg := "[book] 续跑：已完成 " + strings.Join(done, " / ") + "，只跑剩余阶段"
			note(msg)
			r.log.Log(0, msg)
		}
	}

	runPhase := func(name string, fn func() error) error {
		// images 阶段不做 phase 级跳过：RunImages 自身就是增量的
		// （done 跳过、fallback 重试），phase 级 done 标记会让上次
		// 失败（fallback）的图片永远得不到重试。
		if name != "images" && prog2Field(prog, name) == "done" && !opts.Restart && opts.Step == "" {
			note("[book] phase %s already done, skipping", name)
			r.log.Log(0, "[book] phase "+name+" already done, skipping")
			return nil
		}
		if opts.Step != "" && opts.Step != name {
			return nil
		}
		note("[book] === phase: %s ===", name)
		phaseStart := time.Now()
		if err := fn(); err != nil {
			note("[book] === phase: %s FAILED (%s) ===", name, fmtDuration(time.Since(phaseStart)))
			return fmt.Errorf("phase %s: %w", name, err)
		}
		note("[book] === phase: %s done (%s) ===", name, fmtDuration(time.Since(phaseStart)))
		setProgField(&prog, name, "done")
		saveProg()
		return nil
	}

	// Phase: images (level-2 pass on the source markdown). r.projDir is
	// already set (by useProjectDir above), so the watermark memory, the
	// pages cache and the sessions of the image pass land inside THIS
	// book's workspace rather than at the output root.
	err = runPhase("images", func() error {
		return r.RunImages(ImagesOptions{
			TestMode: opts.TestMode, Number: opts.Number, Seed: opts.Seed,
			SourceDir: opts.SourceDir, Files: opts.Files,
			OutDir:  filepath.Join(proj, "source"),
			Inline:  true, // 档位1：矢量图内嵌 tikz 代码，不产 figures 资源
			Verbose: verbose,
		})
	})
	if err != nil {
		return err
	}
	// 原始文档索引 + 原书页表：把 MinerU 中间产物（content_list/
	// layout/origin.pdf）加工为只读检索索引。样式会话最先用到它
	// （doc_search 定位文本页、list_source_pages 列章节起点与逐页图片），
	// 所以放在 style 之前构建。
	r.buildPDFViewQuiet(opts.SourceDir, opts.Files)
	r.buildDocIndexQuiet(proj, opts.SourceDir, opts.Files)
	r.checkPageAlignment()

	err = runPhase("style", func() error { return r.stylePhase(proj) })
	if err != nil {
		return err
	}
	err = runPhase("chapters", func() error { return r.chaptersPhase(proj) })
	if err != nil {
		return err
	}

	err = runPhase("convert", func() error {
		// 前置检查：样式包完整 + 图片全部处理完毕，有问题直接停。
		if err := r.preflightConvert(proj); err != nil {
			return err
		}
		return r.convertPhase(proj, 0)
	})
	if err != nil {
		return err
	}
	return runPhase("assemble", func() error { return r.assemblePhase(proj) })
}

func prog2Field(p bookProgress, name string) string {
	switch name {
	case "images":
		return p.Images
	case "style":
		return p.Style
	case "chapters":
		return p.Chapters
	case "convert":
		return p.Convert
	case "assemble":
		return p.Assemble
	}
	return ""
}

func setProgField(p *bookProgress, name, v string) {
	switch name {
	case "images":
		p.Images = v
	case "style":
		p.Style = v
	case "chapters":
		p.Chapters = v
	case "convert":
		p.Convert = v
	case "assemble":
		p.Assemble = v
	}
}

// ------------------------------------------------------------------
// phase: style
// ------------------------------------------------------------------

func (r *Runner) stylePhase(proj string) error {
	start := time.Now()
	notef := r.phaseNote()
	defer func() { notef("[style] 耗时 %s", fmtDuration(time.Since(start))) }()
	sourceDir := filepath.Join(proj, "source")
	mds, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
	if len(mds) == 0 {
		return fmt.Errorf("source 目录中没有处理后的 markdown")
	}
	sort.Strings(mds)
	mainMD := mds[0]
	for _, f := range mds { // prefer the largest file as the book body
		if fi, err := os.Stat(f); err == nil {
			if mj, e2 := os.Stat(mainMD); e2 == nil && fi.Size() > mj.Size() {
				mainMD = f
			}
		}
	}

	// 水印工作记忆：档位1 也先做一次检测（结果与档位2 共享缓存），
	// 让 style/convert/checker 全程带着同一份水印描述。
	if r.cfg.Latex.RemoveWatermark {
		if data, err := os.ReadFile(mainMD); err == nil {
			r.detectWatermarkPhase([]watermarkSample{{
				name: filepath.Base(mainMD), content: string(data),
			}})
		}
	}

	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.modelOf(r.cfg.Latex.StyleModel)
	// style 会话的内置 max_tokens 默认即 32768（cls+manual+example 较大），
	// 用户显式配置 latex.sessions.style.max_tokens 时以配置为准。
	tuning := r.cfg.LatexSession("style")

	comp := r.comp
	if err := comp.Available(); err != nil {
		return fmt.Errorf("样式阶段需要 LaTeX 工具链: %w", err)
	}

	// Analyst workspace: drafts persist as real files under the project
	// so later turns edit diffs instead of re-emitting full contents.
	workDir := filepath.Join(proj, "work", "style")
	_ = os.MkdirAll(workDir, 0o755)
	submit := &SubmitStyleTool{Workspace: workDir}
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_style")
	defer cleanBashTmp()
	tools := r.styleSessionTools(proj, workDir, sourceDir, "style", bashTmp, submit)

	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, "style")
	var styleHook func(rounds, tools int)
	styleFin := func() {}
	styleStart := time.Now()
	// 会话上下文实时持久化（JSONL 转录，图片走 file:// 引用）：样式
	// 反馈回路直接复用这个上下文打回（不开新会话，避免丢失信息）；
	// 运行中断时下次从转录恢复，不重烧 token。成功/失败路径都会保存
	// 最新状态。
	ctxPath := filepath.Join(proj, "work", "style_session.jsonl")
	styleResumed := false
	styleResumeSkip := false
	var styleMsgs []session.ChatMessage
	if legacyMsgs, lerr := loadSessionContext(ctxPath); lerr == nil && len(legacyMsgs) > 0 {
		// 上次运行中断（phase 未标 done）→ 从历史上下文续跑。
		styleResumed = true
		styleMsgs = legacyMsgs
		sess.SetMessages(legacyMsgs)
		_, statErr := os.Stat(ctxPath)
		freshTranscript := os.IsNotExist(statErr)
		if tr, terr := session.NewTranscript(ctxPath); terr == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
			// 迁移旧上下文（旧版单 JSON 文件）：不写一遍的话新转录从本轮
			// 才开头，历史在下次续跑时就找不回来了。
			if freshTranscript {
				for _, m := range legacyMsgs {
					_ = tr.Append(m)
				}
			}
		}
		if _, st, serr := session.LoadTranscriptStats(ctxPath); serr == nil {
			sess.SeedCounters(st.Rounds, st.Tools)
			styleStart = time.Now().Add(-st.Active)
			resumeLog(r.log, 1, "style", filepath.Base(ctxPath), legacyMsgs, st)
			if replaySubmit(r.log, 1, "style", submit, st) {
				styleResumeSkip = true
			}
		} else {
			// 旧版单 JSON 上下文：至少轮次/工具数照常显示（估算历史条数）。
			r.log.Log(1, "[style] 恢复中断的样式会话 (", strconv.Itoa(len(legacyMsgs)), "条历史消息 )")
		}
	} else if tr, terr := session.NewTranscript(ctxPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	styleHook, styleFin = r.livePhaseRowAt("style", "style", styleStart)
	sess.SetProgressHook(styleHook)
	defer styleFin()
	defer func() {
		if err := saveSessionContext(sess, ctxPath); err != nil {
			r.log.LogWarning(1, "[style] 会话上下文保存失败:", err)
		}
	}()

	initial := prompts.Render(prompts.StyleUser, map[string]string{
		"MAIN_MD": "- Organized markdown (high-quality text): " + filepath.Base(mainMD) + " (readable as project:source/" + filepath.Base(mainMD) + " with read_file)",
	})
	if r.pdfView != nil {
		initial += fmt.Sprintf("\n- ORIGINAL pages: %d in total (%s) — list_source_pages lists them with the detected section starts; view any page with view_pdf {path:\"source:<file>\", page:N} — in bash that directory is /source with the SAME file names (use one form or the other; do not paste the /source/... path into view_pdf).", r.pdfView.total, strings.Join(r.pdfView.names(), ", "))
	}

	scratch, cleanScratch, err := r.tempDir(proj, "style")
	if err != nil {
		return err
	}
	defer cleanScratch()

	var lastExampleCompile string
	if styleResumed && !styleResumeSkip {
		// T29：历史能自己续（tool 回执收尾）就不插入新内容；模型自己停了
		// 才发一句最小推动。
		initial = resumeUserText(styleMsgs, "The session was interrupted earlier. Continue from where you left off: check your workspace state, finish the cls/manual/example, and call submit_style once everything is consistent.")
	}
	replayRunPending := styleResumeSkip // 重放已提交：第一轮跳过 Run 直接走提交后处理
	for attempt := 0; attempt <= r.cfg.Latex.Compile.MaxFixRounds; attempt++ {
		if replayRunPending {
			replayRunPending = false
		} else {
			userText := initial
			if attempt > 0 {
				userText = "The example failed to compile:\n" + lastExampleCompile +
					"\n\nFix the cls/example and call submit_style again with the corrected package."
			}
			if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
				return fmt.Errorf("样式会话失败: %w", err)
			}
		}
		if !submit.Set {
			return fmt.Errorf("样式会话未提交 submit_style")
		}

		// Persist + test-compile the example.
		clsName := classNameOf(submit.Cls)
		if clsName == "" {
			lastExampleCompile = "cls 缺少 \\ProvidesClass{...}"
			submit.Set = false
			continue
		}
		styleDir := filepath.Join(proj, "style")
		if err := os.WriteFile(filepath.Join(styleDir, clsName+".cls"), []byte(submit.Cls), 0o644); err != nil {
			return err
		}
		_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
		_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
		r.writeStyleExtras(styleDir, submit)

		// 重新编译当前提交：清空同一个临时目录后重放 cls/example。
		if entries, derr := os.ReadDir(scratch); derr == nil {
			for _, e := range entries {
				_ = os.RemoveAll(filepath.Join(scratch, e.Name()))
			}
		}
		copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls"))
		exFile := filepath.Join(scratch, "example.tex")
		copyFile(filepath.Join(styleDir, "example.tex"), exFile)
		// 提交的额外文件（.sty / TikZ 样式 / 字体表）也要在示例编译时可见，
		// 否则"示例编译通过"只是因为没有用到它们。
		copyStyleExtras(styleDir, scratch, submit, clsName)
		start := time.Now()
		res := comp.Compile(scratch, "example.tex")
		LogCompileResult(r.log, 1, "style-example", res, time.Since(start))
		if res.OK {
			r.log.Log(1, "[style] example 编译通过，样式包已就绪:", clsName+".cls")
			return nil
		}
		lastExampleCompile = res.Err
		r.log.LogWarning(1, "[style] example 编译失败（第", strconv.Itoa(attempt+1), "轮）:", res.Err)
		submit.Set = false
	}
	return fmt.Errorf("样式 example 在 %d 轮内未能编译通过", r.cfg.Latex.Compile.MaxFixRounds)
}

func classNameOf(cls string) string {
	m := classNameRe.FindStringSubmatch(cls)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// ------------------------------------------------------------------
// phase: chapters
// ------------------------------------------------------------------

func (r *Runner) chaptersPhase(proj string) error {
	start := time.Now()
	notef := r.phaseNote()
	defer func() { notef("[chapters] 耗时 %s", fmtDuration(time.Since(start))) }()
	sourceDir := filepath.Join(proj, "source")
	mds, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
	if len(mds) == 0 {
		return fmt.Errorf("source 目录中没有处理后的 markdown")
	}
	sort.Strings(mds)
	mainMD := mds[0]
	data, err := os.ReadFile(mainMD)
	if err != nil {
		return err
	}
	totalLines := strings.Count(string(data), "\n") + 1

	// Sandbox with ONLY the source file, as book.md + 一个可编辑的
	// 缓冲区 buffer.md（工作记忆：划分结果先增量写入这里，最后
	// submit_split 按缓冲区内容提交）。书多章多时单次会话可能放不下，
	// 缓冲区让进度跨轮保留。
	sandbox, cleanSandbox, err := r.tempDir(proj, "chapters")
	if err != nil {
		return err
	}
	defer cleanSandbox()
	if err := copyFile(mainMD, filepath.Join(sandbox, "book.md")); err != nil {
		return err
	}
	bufferPath := filepath.Join(sandbox, "buffer.md")
	_ = os.WriteFile(bufferPath, []byte("# split buffer\n"), 0o644)

	client := r.clientFor(r.cfg.Latex.ChapterModel)
	modelCfg := r.modelOf(r.cfg.Latex.ChapterModel)
	tuning := r.cfg.LatexSession("chapter")
	submit := &SubmitSplitTool{}
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_chapters")
	defer cleanBashTmp()
	sess := session.NewSession(client, modelCfg, tuning, renderPrompt(prompts.Must(prompts.ChaptersSystem), tuning, r.outputLang()), []session.Tool{
		&GrepTool{Root: sandbox},
		&ReadFileTool{Root: sandbox},
		&WorkBashTool{Root: sandbox, Mounts: r.sessionMounts(kindChapters, sandbox), TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		&EditWorkFileTool{Root: sandbox},
		submit,
	}, r.log, 1, "chapters")
	// 划分会话也带转录：大部头一本书可能分多次跑，中断后从转录续上。
	// （转录里的 book.md 内容以 file:// 引用，恢复时自动还原。）
	trChap := filepath.Join(proj, "work", "sessions", "chapters.jsonl")
	chaptersSkip := false
	chaptersStart := time.Now()
	var chapterMsgs []session.ChatMessage
	if msgsCh, st, errC := session.LoadTranscriptStats(trChap); errC == nil && len(msgsCh) > 0 {
		sess.SetMessages(msgsCh)
		sess.SeedCounters(st.Rounds, st.Tools)
		chapterMsgs = msgsCh
		chaptersStart = time.Now().Add(-st.Active)
		resumeLog(r.log, 1, "chapters", filepath.Base(trChap), msgsCh, st)
		if replaySubmit(r.log, 1, "chapters", submit, st) {
			chaptersSkip = true
		}
	}
	if tr, errT := session.NewTranscript(trChap); errT == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRowAt("chapters", "chapters", chaptersStart)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	granularity := r.cfg.Latex.ChapterGranularity
	if granularity == "" {
		granularity = "small"
	}
	gran := "SMALL granularity (default): one chapter = one SECTION. Split at the finest heading level that yields coherent, self-contained units (a top-level chapter containing several sections becomes several files). Never split mid-section."
	if granularity == "large" {
		gran = "LARGE granularity: one chapter = one TOP-LEVEL chapter of the book. Never split a top-level chapter into pieces; if a chapter is huge, it stays one file (the converter handles it)."
	}
	initial := prompts.Render(prompts.ChaptersUser, map[string]string{
		"GRANULARITY": gran,
		"TOTAL_LINES": strconv.Itoa(totalLines),
	})

	if len(chapterMsgs) > 0 && !chaptersSkip {
		initial = resumeUserText(chapterMsgs, "The session was interrupted earlier. Continue from where you left off and call submit_split with the final chapter ranges.")
	}
	replayRunPending := chaptersSkip // 重放已提交：第一轮跳过 Run 直接走校验写盘
	for attempt := 0; attempt < 3; attempt++ {
		if replayRunPending {
			replayRunPending = false
		} else {
			userText := initial
			if attempt > 0 {
				userText = "Your split was rejected: " + r.lastSplitError + "\nFix the ranges and call submit_split again."
			}
			if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
				return fmt.Errorf("章节划分会话失败: %w", err)
			}
		}
		if !submit.Set {
			return fmt.Errorf("章节划分会话未提交 submit_split")
		}
		if err := validateSplit(submit.Chapters, totalLines); err != nil {
			r.lastSplitError = err.Error()
			r.log.LogWarning(1, "[chapters] 划分无效:", err)
			submit.Set = false
			continue
		}
		// Write chapter files.
		lines := strings.Split(string(data), "\n")
		chapDir := filepath.Join(proj, "chapters")
		for i, c := range submit.Chapters {
			name := fmt.Sprintf("chapter_%03d.md", i+1)
			body := strings.Join(lines[c.StartLine-1:c.EndLine], "\n")
			if err := os.WriteFile(filepath.Join(chapDir, name), []byte(body), 0o644); err != nil {
				return err
			}
			r.log.Log(1, "[chapters]", name, "= lines", strconv.Itoa(c.StartLine)+"-"+strconv.Itoa(c.EndLine), "|", c.Title)
		}
		notef("[chapters] 划分完成: %d 章 (granularity=%s)", len(submit.Chapters), granularity)
		r.keepSessionFile(filepath.Join(proj, "work", "sessions", "chapters.jsonl")) // 已完成
		return nil
	}
	return fmt.Errorf("章节划分在 3 次尝试内未通过校验: %s", r.lastSplitError)
}

func validateSplit(chapters []ChapterRange, totalLines int) error {
	if len(chapters) == 0 {
		return fmt.Errorf("空划分")
	}
	if chapters[0].StartLine != 1 {
		return fmt.Errorf("划分必须从第 1 行开始")
	}
	for i, c := range chapters {
		if c.EndLine > totalLines {
			return fmt.Errorf("章节 %d 结束行 %d 超出文件（共 %d 行）", i+1, c.EndLine, totalLines)
		}
		if i > 0 && c.StartLine != chapters[i-1].EndLine+1 {
			return fmt.Errorf("章节 %d 与 %d 之间存在间隙或重叠", i, i+1)
		}
	}
	if chapters[len(chapters)-1].EndLine != totalLines {
		return fmt.Errorf("划分必须覆盖到最后一行（%d）", totalLines)
	}
	return nil
}

// ------------------------------------------------------------------
// mainMDFile picks the book's main markdown (the largest *.md), the same
// way the style phase does.
func mainMDFile(sourceDir string, files []string) string {
	mds := files
	if len(mds) == 0 && sourceDir != "" {
		matches, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
		mds = matches
	}
	if len(mds) == 0 {
		return ""
	}
	main := mds[0]
	fi, err := os.Stat(main)
	for _, f := range mds[1:] {
		mj, e := os.Stat(f)
		if e == nil && err == nil && mj.Size() > fi.Size() {
			main, fi, err = f, mj, e
		}
	}
	return main
}

// buildPDFViewQuiet mounts ONLY this book's original PDFs (clean names)
// as the session "source" view. The whole mineru_output tree is never
// exposed to a session.
func (r *Runner) buildPDFViewQuiet(sourceDir string, files []string) {
	if r.cfg.Paths.MineruOutput == "" || r.projDir == "" {
		return
	}
	main := mainMDFile(sourceDir, files)
	if main == "" {
		return
	}
	view, err := buildPDFView(r.projDir, r.cfg.Paths.MineruOutput, subjectOf(filepath.Base(main)))
	if err != nil {
		r.log.LogWarning(0, "[pdfview] 原书 PDF 视图不可用（source 挂载点关闭）:", err)
		return
	}
	r.pdfView = view
	r.log.Log(0, "[pdfview] 原书 PDF 视图就绪:", strings.Join(view.names(), ", "),
		"("+strconv.Itoa(view.total)+" 页) ->", view.Dir)
}

// checkPageAlignment cross-checks the OCR page index against the real
// origin PDFs: a mismatch means global page numbers can drift, while the
// part-based lookups (used by doc_search and list_source_pages) stay
// exact. Reported once per run, never fatal.
func (r *Runner) checkPageAlignment() {
	if r.pdfView == nil || r.docPages == nil {
		return
	}
	for _, msg := range r.pdfView.alignmentProblems(r.docPages) {
		r.log.LogWarning(0, "[pdfview] 页数不一致:", msg)
	}
}

// buildDocIndexQuiet compiles the read-only original-document index
// from the MinerU intermediate output. Failures are non-fatal: the
// convert sessions simply run without doc_search/source pages.
func (r *Runner) buildDocIndexQuiet(proj, sourceDir string, files []string) {
	mds := files
	if len(mds) == 0 && sourceDir != "" {
		matches, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
		mds = matches
	}
	if len(mds) == 0 || r.cfg.Paths.MineruOutput == "" {
		return
	}
	// DOCVISION 注释（图片条目的类型/描述/原文）只存在于 images 阶段
	// 加工过的 md 里——也就是 <proj>/source（RunBook 把 RunImages 的
	// OutDir 指到这里）。没有该目录（例如只跑 --step 子阶段）就退回
	// 输入 md 目录，保持旧行为可用。
	markerDir := filepath.Join(proj, "source")
	if st, serr := os.Stat(markerDir); serr != nil || !st.IsDir() {
		markerDir = sourceDir
	}
	outPath := filepath.Join(proj, "doc_index", "doc_index.json")
	idx, pageIdx, err := buildDocIndex(r.cfg.Paths.MineruOutput, mds, markerDir, outPath)
	if err != nil {
		r.log.Log(0, "[docindex] 原始文档索引不可用（doc_search/原书页面工具关闭）:", err)
		return
	}
	r.docIndex = idx
	r.docPages = pageIdx
	withNotes := 0
	for _, e := range idx.Entries {
		if e.Marker != "" {
			withNotes++
		}
	}
	r.log.Log(0, "[docindex] 原始文档索引就绪:", strconv.Itoa(len(idx.Entries)), "个块 /", strconv.Itoa(len(idx.Parts)),
		"个 part（带 DOCVISION 注释:", strconv.Itoa(withNotes), "）->", outPath)
}

// phase: convert (concurrent per-chapter sessions)
// ------------------------------------------------------------------

func (r *Runner) convertPhase(proj string, fbRound int) error {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	if clsName == "" {
		return fmt.Errorf("未找到样式 cls（style 阶段未完成？）")
	}
	manualPath := filepath.Join(proj, "style", "manual.md")
	chapDir := filepath.Join(proj, "chapters")
	workDir := filepath.Join(proj, "work")

	chapters, err := filepath.Glob(filepath.Join(chapDir, "chapter_*.md"))
	if err != nil || len(chapters) == 0 {
		return fmt.Errorf("没有章节文件可转换")
	}
	sort.Strings(chapters)

	conc := r.cfg.Latex.Concurrency
	if conc <= 0 {
		conc = 3
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var failsMu sync.Mutex
	var failures []string

	// Compact console progress, mirroring the level-2 [process k/N] line.
	// Details stay in the log file; the console only shows the counter.
	verbose := r.consoleVerbose
	label := "[convert]"
	if fbRound > 0 {
		label = fmt.Sprintf("[convert#%d]", fbRound+1)
	}
	r.phaseNote()("%s %d 章，并发 %d", label, len(chapters), conc)
	// 并发数同时进日志文件：控制台进度行不进日志，事后看日志无法判断
	// 当时跑了几个会话（所有阶段共用 latex.concurrency）。
	r.log.Log(0, fmt.Sprintf("%s 并发 %d（latex.concurrency），共 %d 章", label, conc, len(chapters)))
	var done, failed, running int
	var progMu sync.Mutex
	total := len(chapters)
	start := time.Now()
	// 进度行统一走 liveProgress：终端里原地覆写、管道里按节拍整行输出。
	// 这里原来是自己 fmt.Fprintf(os.Stdout, "\r…")，有两个真实毛病：
	// ① 格式串写成了 "%s %d/%d]"，多了个 `]`，实际输出
	//    "[convert] 0/3] 0.00% …"；② 裸 `\r` 在管道/日志里就是乱码，
	//    用户看到的 "[convert] 0/3] 0.00% (done: 0,[convert] 3/3] …" 正是
	//    两次重绘被当成普通文本落在一起。
	var progLive *liveProgress
	if !verbose && total > 0 {
		progLive = newLiveProgressRow(r.log, "convert", func() string {
			progMu.Lock()
			defer progMu.Unlock()
			return convertProgressText(label, done, failed, running, total, time.Since(start))
		})
	}
	progress := func() { progLive.render() }

	for idx, chapPath := range chapters {
		wg.Add(1)
		tid := <-tidPool
		if verbose {
			r.log.Log(0, label, "start", filepath.Base(chapPath),
				fmt.Sprintf("(%d/%d)", idx+1, total))
		}
		progMu.Lock()
		running++
		progMu.Unlock()
		go func(i int, chap string, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			err := r.convertOneChapter(proj, clsName, manualPath, chap, workDir, i, tid, false)
			progMu.Lock()
			done++
			running--
			if err != nil {
				failed++
			}
			progMu.Unlock()
			progress()
			if err != nil {
				r.log.LogError(tid, label, "章节失败:", filepath.Base(chap), err)
				failsMu.Lock()
				failures = append(failures, filepath.Base(chap)+": "+err.Error())
				failsMu.Unlock()
			}
		}(idx, chapPath, tid)
	}
	wg.Wait()
	progLive.Close()
	if len(failures) > 0 {
		return fmt.Errorf("%d 个章节转换失败: %s", len(failures), strings.Join(failures, "; "))
	}
	r.log.Log(0, label, "全部", strconv.Itoa(len(chapters)), "章转换完成")
	// 样式反馈回路：多数章节汇报 cls/手册问题 → 打回原样式会话修正
	// 后，用全新上下文重新并发转换。
	return r.styleFeedbackLoop(proj, fbRound)
}

// writeStyleExtras persists the files a submit_style call shipped
// besides cls/manual/example (helper .sty, TikZ style files, font
// tables…) into the style package, preserving relative paths, and
// records the session's report for the operator (missing fonts and
// other limitations the class cannot express yet).
func (r *Runner) writeStyleExtras(styleDir string, submit *SubmitStyleTool) {
	for rel, content := range submit.Extra {
		clean := filepath.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		full := filepath.Join(styleDir, clean)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			r.log.LogWarning(0, "[style] 额外文件写入失败:", clean, err)
			continue
		}
		r.log.Log(0, "[style] 额外文件已收录:", clean)
	}
	if submit.Reported != "" {
		path := filepath.Join(styleDir, "REPORT.md")
		_ = os.WriteFile(path, []byte(submit.Reported+"\n"), 0o644)
		r.log.Log(0, "[style] 样式会话报告已保存:", path)
	}
}

// copyStyleExtras replays the submitted extra files into a compile
// scratch so the example really exercises them.
func copyStyleExtras(styleDir, scratch string, submit *SubmitStyleTool, clsName string) {
	for rel := range submit.Extra {
		clean := filepath.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		if clean == clsName+".cls" {
			continue
		}
		src := filepath.Join(styleDir, clean)
		dst := filepath.Join(scratch, clean)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		copyFile(src, dst)
	}
}

func classNameOfFile(styleDir string) string {
	files, _ := filepath.Glob(filepath.Join(styleDir, "*.cls"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if n := classNameOf(string(data)); n != "" {
			return n
		}
	}
	return ""
}

// convertOneChapter converts ONE chapter in its own session. The session
// works in a disposable TEMP workspace that already carries everything it
// needs (the class package, the manual, example.tex, the illustrations)
// and it may experiment there freely: nothing it writes reaches the book
// until it submits the TWO paths of the chapter — the main .tex and the
// folder with its \input parts — which the runner copies into
// <proj>/work/chapters. Probe files, test PDFs and other scratch can
// therefore never leak into the book.
func (r *Runner) convertOneChapter(proj, clsName, manualPath, chapPath, workDir string, idx, tid int, retry bool) error {
	base := strings.TrimSuffix(filepath.Base(chapPath), filepath.Ext(chapPath))
	texPath := filepath.Join(workDir, "chapters", base+".tex")
	if fileExists(texPath) {
		r.log.Log(tid, "[convert]", base, "已有产物，跳过")
		// 产物已写出的章节不再需要会话转录。
		r.keepSessionFile(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"))
		return nil
	}

	client := r.clientFor(r.cfg.Latex.ConvertModel)
	modelCfg := r.modelOf(r.cfg.Latex.ConvertModel)
	tuning := r.cfg.LatexSession("convert")
	// 手册不在首条消息里内联：它就在会话工作区里（work:manual.md）以及
	// project:style/manual.md，内联 11k 字符会跟着每轮请求重发。
	_ = manualPath

	// 本章会话的临时工作区；只有"转录与工作树都在"才算中断续跑（否则
	// 回放一段没有文件的历史只会让会话空转，不如重来）。
	trPath := filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
	workRoot := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	resuming := fileExists(trPath) && fileExists(workRoot)
	work, cleanWork, err := r.chapterWorkTree(proj, clsName, base, resuming, "")
	if err != nil {
		return err
	}
	defer cleanWork()

	submit := &SubmitChapterTool{
		WorkRoot:   work,
		SubmitRoot: filepath.Join(workDir, "chapters"),
		Base:       base,
		ReportPath: filepath.Join(workDir, "reports", base+".md"), // 工作汇报（实时落盘）
	}
	// 挂载表：work = 本章临时工作区（唯一可写），project/source = 只读。
	convertMounts := r.sessionMounts(kindConvert, work)
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_"+base)
	defer cleanBashTmp()
	tools := []session.Tool{
		&ReadFileTool{Mounts: convertMounts},
		// 工作区里什么都能写（探针/实验都留着当草稿），提交时才拷两个路径。
		&WriteWorkFileTool{Root: work, AnyExt: true, Hint: "Your chapter is " + base + ".tex plus the folder " + base +
			"/ for its \\input parts. Everything else in this workspace is scratch you may experiment with."},
		&EditWorkFileTool{Root: work},
		&GrepTool{Mounts: convertMounts},
		// 看 markdown 里引用的原图（传 markdown 中的引用路径即可）
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewPDFTool{Mounts: convertMounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&CompileChapterTool{Comp: r.comp, Dir: work, MainFile: base + ".tex", WrapperFile: base + "_wrapper.tex", Log: r.log, Tid: 1},
		&WorkBashTool{Mounts: convertMounts, Root: work, TmpDir: bashTmp,
			MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		submit,
	}
	// 原始文档只读工具：片段→原 PDF 页定位（doc_search）+ 原书页面索引/
	// 逐页内容（list_source_pages）；原书 PDF 通过 view_pdf 的 source
	// 挂载查看。
	tools = append(tools, r.sourcePageTools()...)
	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.ConvertSystem), tuning, r.outputLang()),
		tools, r.log, tid, "convert:"+base)
	// 每章一行实时行：并发的 4 章并排显示各自的轮次/工具数/已用时间，
	// 谁跑完谁那行消失（用户要的"箭头指过去看每个子会话状态"）。聚合进度
	// 行是另一行（id "convert"），两者互不覆盖。
	// 会话转录（JSONL，图片走 file:// 引用）：单章转换中断后（进程被
	// 杀 / 网络断连）下次从转录恢复上下文继续，不重烧 token。
	// 中断续跑时才回放转录（resuming 已确认工作树也在）。
	resumed := false
	resumeSkip := false
	convertStart := time.Now()
	var resumeMsgs []session.ChatMessage
	if resuming {
		if msgs, st, err := session.LoadTranscriptStats(trPath); err != nil {
			r.log.LogWarning(tid, "[convert] 转录读取失败（忽略，按全新会话继续）:", err)
		} else if len(msgs) > 0 {
			sess.SetMessages(msgs)
			sess.SeedCounters(st.Rounds, st.Tools)
			resumed, resumeMsgs = true, msgs
			resumeLog(r.log, tid, "convert", base, msgs, st)
			if replaySubmit(r.log, tid, "convert:"+base, submit, st) {
				resumeSkip = true
			}
			convertStart = time.Now().Add(-st.Active)
		}
	}
	convHook, convClose := r.livePhaseRowAt("convert:"+base, "convert:"+base, convertStart)
	sess.SetProgressHook(convHook)
	defer convClose()
	if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}

	chapData, err := os.ReadFile(chapPath)
	if err != nil {
		return err
	}
	initial := prompts.Render(prompts.ConvertUser, map[string]string{
		"CHAPTER_FILE":    "chapter markdown path: project:chapters/" + base + ".md",
		"CHAPTER_PREVIEW": truncateRunes(string(chapData), 2000),
		"TEX_PATH":        "Write it as work:" + base + ".tex (extra \\input parts go in the folder work:" + base + "/), compile until clean, then submit those TWO paths.",
	})

	if r.cfg.Latex.RemoveWatermark {
		initial += "\n\n" + r.watermarkGuidance("WATERMARK: exclude watermark artifacts from the .tex output (repeated decorative overlay text such as institution marks, faint background strings). Skip such content entirely - do not typeset it.")
	}

	if resumed && !resumeSkip {
		// T29：历史以 tool 回执收尾就原样续行（不插内容、不重发任务提示）；
		// 只有模型自己停了才发一句最小推动。
		initial = resumeUserText(resumeMsgs, "The session was interrupted earlier. Continue from where you left off: read your last written .tex state (read_file), finish the conversion, compile until clean, then submit.")
	}
	if !resumeSkip {
		if _, err := sess.Run(session.RunOptions{UserText: initial}); err != nil {
			return fmt.Errorf("会话失败: %w", err)
		}
	}
	if !submit.Submitted {
		// Accept a chapter that compiles clean even without an explicit
		// submit (rounds may have run out): the same two paths are placed.
		if !fileExists(filepath.Join(work, base+".tex")) {
			return fmt.Errorf("会话未提交且未写出 .tex")
		}
		start := time.Now()
		res := r.comp.Compile(work, base+"_wrapper.tex")
		LogCompileResult(r.log, tid, "convert-check", res, time.Since(start))
		if !res.OK {
			return fmt.Errorf("会话未提交且编译失败: %s", res.Err)
		}
		if _, perr := placeChapterFiles(work, filepath.Join(workDir, "chapters"), base); perr != nil {
			return perr
		}
		r.log.LogWarning(tid, "[convert]", base, "未显式提交，但编译通过，予以采纳")
	}
	if !fileExists(texPath) {
		return fmt.Errorf("会话已提交但没有写出 .tex")
	}
	// Checker pass: a small dedicated session (read_file/grep/submit only,
	// read-only view holding the chapter markdown + the submitted .tex +
	// its parts folder) verifies the chapter against the markdown. Hard
	// problems are fed straight back to the SAME conversion session (its
	// context is still live, so it is the cheapest and most accurate
	// fixer), up to maxCheckerRounds rounds; only then the chapter is
	// discarded and re-converted with a fresh session.
	ok, issues := r.checkChapter(proj, base, chapPath, texPath, filepath.Join(workDir, "chapters", base), tid)
	for round := 0; !ok && round < maxCheckerRounds; round++ {
		r.log.LogWarning(tid, "[checker]", base, "第"+strconv.Itoa(round+1)+"轮反馈:", issues)
		r.phaseNote()("[checker] %s 第 %d 轮反馈", base, round+1)
		msg := "The checker reviewed your submitted chapter and found problems that must be fixed:\n" + issues +
			"\n\nFix it in your workspace (edit_file/write_file), compile until clean, then submit the two paths again."
		if _, err := sess.Run(session.RunOptions{UserText: msg}); err != nil {
			r.log.LogWarning(tid, "[checker]", base, "反馈轮失败:", err)
			break
		}
		ok, issues = r.checkChapter(proj, base, chapPath, texPath, filepath.Join(workDir, "chapters", base), tid)
	}
	if ok {
		r.log.Log(tid, "[checker]", base, "通过")
		_ = os.Remove(texPath + ".checker")
	} else {
		_ = os.WriteFile(texPath+".checker", []byte(issues), 0o644)
		if !retry {
			r.log.LogWarning(tid, "[checker]", base, strconv.Itoa(maxCheckerRounds)+" 轮反馈后仍有问题 — 作废产物，改用全新会话重转换")
			r.phaseNote()("[checker] %s 反馈 %d 轮未过 — 重转换", base, maxCheckerRounds)
			_ = os.Remove(texPath)
			_ = os.RemoveAll(filepath.Join(workDir, "chapters", base))
			r.keepSessionFile(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"))
			return r.convertOneChapter(proj, clsName, manualPath, chapPath, workDir, idx, tid, true)
		}
		r.log.LogWarning(tid, "[checker]", base, "重转换后仍未过（已记录，供终审处理）:", issues)
	}
	return nil
}

// sessionKind names the role of a session: it decides which trees the
// session may see (and thus which trees the sandboxed bash mounts).
type sessionKind string

const (
	kindTikz     sessionKind = "tikz"     // one vector figure
	kindStyle    sessionKind = "style"    // class/manual analysis
	kindChapters sessionKind = "chapters" // chapter splitting
	kindConvert  sessionKind = "convert"  // one chapter .tex
	kindBook     sessionKind = "book"     // build fix + final review
)

// sessionMounts is the ONE place that defines what a session can see.
// workDir is the session's own workspace (the only writable tree); the
// project and the original PDFs are read-only, and a session only gets
// what its role needs — never the whole mineru_output tree.
func (r *Runner) sessionMounts(kind sessionKind, workDir string) []Mount {
	mounts := []Mount{{Name: "work", Dir: workDir, Writable: true}}
	switch kind {
	case kindTikz, kindChapters:
		// A figure session only draws; a chapter session only splits the
		// single book markdown in its own workspace.
		return mounts
	}
	// Everything else reads part of the project (source markdown +
	// images, the submitted style package, the chapter markdown) and the
	// original book PDFs. The project is mounted through the NARROW view:
	// work/sessions, work/reports, doc_index.json, pages/, build/ and out/
	// are not part of it.
	if r.projDir != "" {
		r.projView = ensureProjectView(r.projDir)
		if r.projView != "" {
			mounts = append(mounts, Mount{Name: "project", Dir: r.projView})
		}
	}
	mounts = append(mounts, r.pdfView.Mounts()...)
	return mounts
}

// projectRoot is the directory a session reads as "the project": the
// narrow view when available, otherwise the project root (degraded).
func (r *Runner) projectRoot() string {
	if r.projView != "" {
		return r.projView
	}
	if r.projDir != "" {
		if v := ensureProjectView(r.projDir); v != "" {
			return v
		}
	}
	return r.projDir
}

// sourcePageTools gives a session the original-document tooling: a text
// index (doc_search) plus the page index that also lists what each page
// contains. The original PDFs themselves are viewed with view_pdf
// through the read-only "source" mount — there is no separate viewer.
func (r *Runner) sourcePageTools() []session.Tool {
	var tools []session.Tool
	if r.docIndex != nil {
		tools = append(tools, &DocSearchTool{Index: r.docIndex, View: r.pdfView, Mount: "source"})
	}
	if r.pdfView == nil || r.pdfView.total == 0 {
		return tools
	}
	tools = append(tools, &ListSourcePagesTool{
		View: r.pdfView, Index: r.docIndex, Mount: "source",
	})
	return tools
}

// chapterWorkTree builds the disposable TEMP workspace of ONE chapter
// session: everything the session needs to work and compile (the class,
// helper .sty files, manual.md, example.tex, the illustrations) plus the
// two paths the chapter is submitted as (<base>.tex + <base>/, created
// empty). The style package is COPIED, not linked: the session may
// rewrite or break those files at will without touching the real package.
//
// resume=true keeps whatever the previous attempt left behind (an
// interrupted session continues in place); seed is an optional directory
// holding an already converted chapter to start from (the style-fix
// session adapts the existing chapter instead of re-converting).
func (r *Runner) chapterWorkTree(proj, clsName, base string, resume bool, seed string) (string, func(), error) {
	root := filepath.Join(proj, "work", "temp", "conv_"+base)
	if !resume {
		_ = os.RemoveAll(root)
	}
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", func() {}, err
	}
	if entries, err := os.ReadDir(filepath.Join(proj, "style")); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			switch strings.ToLower(filepath.Ext(name)) {
			case ".cls", ".sty", ".tex", ".md":
				_ = copyFile(filepath.Join(proj, "style", name), filepath.Join(work, name))
			}
		}
	}
	for _, asset := range []string{"images", "figures"} {
		src := filepath.Join(proj, "source", asset)
		if !fileExists(src) {
			continue
		}
		if err := linkAbs(src, filepath.Join(work, asset)); err != nil {
			_ = copyDir(src, filepath.Join(work, asset))
		}
	}
	// 待提交的两个路径：主文件 + 它的 \input 分片文件夹。
	if err := os.MkdirAll(filepath.Join(work, base), 0o755); err != nil {
		return "", func() {}, err
	}
	if seed != "" {
		if fileExists(filepath.Join(seed, base+".tex")) {
			if err := copyFile(filepath.Join(seed, base+".tex"), filepath.Join(work, base+".tex")); err != nil {
				return "", func() {}, fmt.Errorf("读取已转换的章节失败: %w", err)
			}
		}
		if fileExists(filepath.Join(seed, base)) {
			_ = os.RemoveAll(filepath.Join(work, base))
			if err := copyDir(filepath.Join(seed, base), filepath.Join(work, base)); err != nil {
				return "", func() {}, err
			}
		}
	}
	wrapper := "\\documentclass{" + clsName + "}\n" +
		"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
		"\\graphicspath{{figures/}}\n" +
		"\\begin{document}\n\\input{" + base + ".tex}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(work, base+"_wrapper.tex"), []byte(wrapper), 0o644); err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		if r.keepTemp() {
			r.log.Log(0, "[temp] 保留临时工作目录:", root)
			return
		}
		_ = os.RemoveAll(root)
	}
	return work, cleanup, nil
}

// placeChapterFiles copies the TWO submitted paths of one chapter — its
// main .tex and the folder with its \input parts — into the real chapter
// tree, replacing that folder wholesale (so nothing stale survives).
// Returns the number of files placed.
func placeChapterFiles(tree, submitRoot, base string) (int, error) {
	dstTex := filepath.Join(submitRoot, base+".tex")
	dstDir := filepath.Join(submitRoot, base)
	if err := os.MkdirAll(submitRoot, 0o755); err != nil {
		return 0, err
	}
	if err := os.RemoveAll(dstDir); err != nil {
		return 0, err
	}
	n := 0
	if st, err := os.Stat(filepath.Join(tree, base)); err == nil && st.IsDir() {
		if err := copyDir(filepath.Join(tree, base), dstDir); err != nil {
			return 0, fmt.Errorf("复制章节文件夹失败: %w", err)
		}
		n = countTreeFiles(filepath.Join(tree, base))
	}
	if err := copyFile(filepath.Join(tree, base+".tex"), dstTex); err != nil {
		return 0, fmt.Errorf("复制章节主文件失败: %w", err)
	}
	return n + 1, nil
}

// chapterScratch builds a scratch dir that can compile ONE chapter as an
// \input fragment: the book class, a wrapper that inputs <base>.tex, and
// the project images/figures mounted (symlink, copy as fallback) so
// image references in the chapter resolve.
func (r *Runner) chapterScratch(proj, clsName, base, tag string) (string, func(), error) {
	scratch, cleanup, err := r.tempDir(proj, tag+"_"+base)
	if err != nil {
		return "", func() {}, err
	}
	copyFile(filepath.Join(proj, "style", clsName+".cls"), filepath.Join(scratch, clsName+".cls"))
	for _, asset := range []string{"images", "figures"} {
		src := filepath.Join(proj, "source", asset)
		if !fileExists(src) {
			continue
		}
		if err := linkAbs(src, filepath.Join(scratch, asset)); err != nil {
			_ = copyDir(src, filepath.Join(scratch, asset))
		}
	}
	// The chapter tree itself: a chapter may \input its own extra files
	// (chapters/<base>/table1.tex ...) exactly as the assembled book does,
	// so the scratch needs the same relative layout.
	if chapTree := filepath.Join(proj, "work", "chapters"); fileExists(chapTree) {
		if err := linkAbs(chapTree, filepath.Join(scratch, "chapters")); err != nil {
			_ = copyDir(chapTree, filepath.Join(scratch, "chapters"))
		}
	}
	wrapper := "\\documentclass{" + clsName + "}\n" +
		"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
		"\\graphicspath{{figures/}}\n" +
		"\\begin{document}\n\\input{" + base + ".tex}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(scratch, base+"_wrapper.tex"), []byte(wrapper), 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return scratch, cleanup, nil
}

// styleSessionTools is the ONE tool set used by the style session AND by the
// feedback round that continues its history. They must be identical: the
// feedback session replays the style session's transcript, so every tool the
// history used has to exist there too. It was written out twice by hand and
// the copy had silently lost bash — the model followed its own history, called
// bash, and got "unknown tool" (real run 2026-09-11 21:12, twice), leaving it
// to guess which of the tools it had just been using still existed. Sharing
// the constructor makes that class of drift impossible.
// tag only names the compile artefact (style / style-feedback).
func (r *Runner) styleSessionTools(proj, workDir, sourceDir, tag, bashTmp string, submit session.Tool) []session.Tool {
	tools := []session.Tool{
		&WriteWorkFileTool{Root: workDir},
		&EditWorkFileTool{Root: workDir},
		&ReadFileTool{Root: workDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&GrepTool{Root: workDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&WorkBashTool{Root: workDir, Mounts: r.sessionMounts(kindStyle, workDir), TmpDir: bashTmp,
			MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		&CompileTexTool{Comp: r.comp, Root: workDir, MainFile: "example.tex", Tag: tag, Log: r.log, Tid: 1},
		&ViewPDFTool{Root: workDir, Mounts: r.sessionMounts(kindStyle, workDir), Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewImageTool{Root: sourceDir, Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}
	return append(tools, r.sourcePageTools()...)
}

// fixChapterStyle runs a targeted style-fix sub-session on ONE already
// converted chapter: the class/manual changed, so the .tex must be
// adapted (NOT re-converted from markdown). Returns nil when the
// chapter compiles and was submitted; otherwise the caller falls back
// to a full re-conversion.
// workRoot is the CHAPTER work tree (proj/work: chapters/<base>.tex +
// chapters/<base>/), NOT the style workspace.
func (r *Runner) fixChapterStyle(proj, clsName, manualPath, chapPath, workRoot, base, issues string, tid int) error {
	texRel := "chapters/" + base + ".tex"
	texPath := filepath.Join(workRoot, texRel)
	if !fileExists(texPath) {
		return fmt.Errorf("没有已转换的 %s", texRel)
	}
	// 同一套临时工作区（类/手册/示例 + 本章现有产物），会话在里面随便改，
	// 提交时同样只交那两个路径，正式树不会被草稿污染。
	work, cleanWork, err := r.chapterWorkTree(proj, clsName, base, false, filepath.Join(workRoot, "chapters"))
	if err != nil {
		return err
	}
	defer cleanWork()
	// 手册不内联：它在会话工作区里（work:manual.md）与 project:style/manual.md。
	_ = manualPath

	submit := &SubmitChapterTool{
		WorkRoot:       work,
		SubmitRoot:     filepath.Join(workRoot, "chapters"),
		Base:           base,
		ReportOptional: true, // 定向修复只要交出适配后的章节
	}
	fixMounts := r.sessionMounts(kindConvert, work)
	tools := []session.Tool{
		&ReadFileTool{Mounts: fixMounts},
		&EditWorkFileTool{Root: work},
		&WriteWorkFileTool{Root: work, AnyExt: true, Hint: "Your chapter is " + base + ".tex plus the folder " + base +
			"/ for its \\input parts; everything else here is scratch."},
		&GrepTool{Mounts: fixMounts},
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewPDFTool{Mounts: fixMounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&CompileChapterTool{Comp: r.comp, Dir: work, MainFile: base + ".tex", WrapperFile: base + "_wrapper.tex", Log: r.log, Tid: tid},
		submit,
	}
	tools = append(tools, r.sourcePageTools()...)
	sess := session.NewSession(r.clientFor(r.cfg.Latex.ConvertModel), r.modelOf(r.cfg.Latex.ConvertModel),
		r.cfg.LatexSession("convert"), renderPrompt(prompts.Must(prompts.StyleFixSystem), r.cfg.LatexSession("convert"), r.outputLang()), tools, r.log, tid, "style-fix:"+base)
	liveHook, liveClose := r.livePhaseRow("style-fix:"+base, "style-fix:"+base)
	sess.SetProgressHook(liveHook)
	defer liveClose()
	// 样式修复会话原先**没有转录**：它改的是真实章节，出问题却无从复盘
	// （docvision sessions 里根本不出现这个会话），中断后重跑也只能从零
	// 再来一遍。与 convert/checker 一样落一份 JSONL。
	trPath := filepath.Join(proj, "work", "sessions", "style_fix_"+base+".jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err == nil {
		if tr, terr := session.NewTranscript(trPath); terr == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
		} else {
			r.log.LogWarning(tid, "[style-fix] 转录创建失败:", terr)
		}
	}

	userText := prompts.Render(prompts.StyleFixUser, map[string]string{
		"CHAPTER_FILE": "The class/manual was revised after your chapter was converted. Adapt work:" + base + ".tex (and work:" + base + "/ if it has \\input parts) so it compiles with the NEW class and follows the NEW manual, then submit those two paths.",
		"ISSUES":       truncateStr(issues, 4000),
	})
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return fmt.Errorf("样式修复会话失败: %w", err)
	}
	if !submit.Submitted {
		return fmt.Errorf("样式修复会话未提交")
	}
	// 复核：编译必须通过（章节内容核对由后续 checker/终审负责）。
	start := time.Now()
	res := r.comp.Compile(work, base+"_wrapper.tex")
	LogCompileResult(r.log, tid, "style-fix-check", res, time.Since(start))
	if !res.OK {
		return fmt.Errorf("样式修复后仍编译失败: %s", res.Err)
	}
	return nil
}

// ------------------------------------------------------------------
// phase: style feedback loop (cls/手册 打回)
// ------------------------------------------------------------------

// maxStyleFeedbackRounds caps how many times conversion results can be
// sent back to the original style session.
const maxStyleFeedbackRounds = 2

// maxCheckerRounds caps how many times a checker verdict is fed back to
// the SAME conversion session before the chapter is discarded and
// re-converted with a fresh session.
const maxCheckerRounds = 3

// styleFeedbackLoop aggregates the per-chapter work reports (工作汇报,
// written in real time at submit). When a MAJORITY reports cls/manual
// conformance problems, the ORIGINAL style session context (persisted
// to work/style_session.json at style phase) is restored — no new
// context, so no information is lost — and asked to fix the style
// package. Afterwards every converted chapter is discarded and
// convertPhase re-runs with FRESH sessions (new context by design).
func (r *Runner) styleFeedbackLoop(proj string, round int) error {
	reportsDir := filepath.Join(proj, "work", "reports")
	files, _ := filepath.Glob(filepath.Join(reportsDir, "*.md"))
	if len(files) == 0 {
		return nil
	}
	var issueFiles []string
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "- 结论: 存在问题") {
			issueFiles = append(issueFiles, f)
			b.WriteString("\n--- " + filepath.Base(f) + " ---\n")
			b.Write(data)
			b.WriteString("\n")
		}
	}
	r.log.Log(0, "[style-feedback] 工作汇报:", strconv.Itoa(len(files)), "份，其中",
		strconv.Itoa(len(issueFiles)), "份报告 cls/手册问题")
	r.phaseNote()("[style-feedback] 工作汇报 %d 份，其中 %d 份报告 cls/手册问题",
		len(files), len(issueFiles))
	if len(issueFiles)*2 <= len(files) {
		return nil // 少数派：不算样式包问题，留给 checker/终审处理
	}
	if round >= maxStyleFeedbackRounds {
		r.log.LogWarning(0, "[style-feedback] 已达最大打回轮数(",
			strconv.Itoa(maxStyleFeedbackRounds), ")，跳过打回")
		r.phaseNote()("[style-feedback] 已达最大打回轮数(%d)，跳过打回", maxStyleFeedbackRounds)
		return nil
	}
	r.log.LogWarning(0, "[style-feedback] 多数章节报告样式问题 — 打回原样式会话（第",
		strconv.Itoa(round+1), "轮）")
	r.phaseNote()("[style-feedback] 多数章节报告样式问题 — 打回原样式会话（第 %d 轮）", round+1)

	ctxPath := filepath.Join(proj, "work", "style_session.jsonl")
	msgs, err := loadSessionContext(ctxPath)
	if err != nil {
		r.log.LogWarning(0, "[style-feedback] 样式会话上下文不可用，跳过打回:", err)
		return nil
	}
	sourceDir := filepath.Join(proj, "source")
	styleDir := filepath.Join(proj, "style")
	manualPath := filepath.Join(styleDir, "manual.md")
	workDir := filepath.Join(proj, "work", "style")

	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.modelOf(r.cfg.Latex.StyleModel)
	tuning := r.cfg.LatexSession("style")

	// 工具集与原样式会话**同一份**（构造器共享，见 styleSessionTools）。
	submit := &SubmitStyleTool{Workspace: workDir}
	feedbackBashTmp, cleanFeedbackBash := r.sessionBashTemp(proj, "bash_style_feedback")
	defer cleanFeedbackBash()
	tools := r.styleSessionTools(proj, workDir, sourceDir, "style-feedback", feedbackBashTmp, submit)

	// 复用原样式会话的上下文（转录里只有 user/assistant/tool，没有 system
	// 行），因此这里必须重新挂上同一份系统提示词——否则打回的这一轮
	// 完全没有系统提示（模板、挂载说明、水印要求全部丢失）。
	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, "style-feedback")
	liveHook, liveClose := r.livePhaseRow("style-feedback", "style-feedback")
	sess.SetProgressHook(liveHook)
	defer liveClose()
	sess.SetMessages(msgs)

	feedback := "The conversion phase finished: the MAJORITY of chapter conversion agents reported that the class/manual did NOT satisfy the book's real formatting." +
		" Their work reports follow (固定格式，结论: 存在问题 = issues):" + b.String() +
		"\n\nThe actual submitted chapters are in the project workspace under work/chapters/ — read any of them with read_file {path:\"work/chapters/<name>.tex\"} to see how the class was used in practice (this is the real submission, the reports above are its summary)." +
		"\n\nRe-inspect the relevant original pages (list_source_pages -> view_pdf on the source mount), fix the cls/manual/example so these problems cannot recur, then submit_style with the corrected package."
	if _, err := sess.Run(session.RunOptions{UserText: feedback}); err != nil {
		return fmt.Errorf("样式反馈会话失败: %w", err)
	}
	if err := saveSessionContext(sess, ctxPath); err != nil {
		r.log.LogWarning(1, "[style-feedback] 会话上下文回写失败:", err)
	}
	if !submit.Set {
		return fmt.Errorf("样式反馈会话未提交 submit_style")
	}
	clsName := classNameOf(submit.Cls)
	if clsName == "" {
		return fmt.Errorf("样式反馈提交的 cls 缺少 \\ProvidesClass{...}")
	}
	if err := os.WriteFile(filepath.Join(styleDir, clsName+".cls"), []byte(submit.Cls), 0o644); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
	_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
	r.writeStyleExtras(styleDir, submit)

	scratch, cleanFBScratch, err := r.tempDir(proj, "style-feedback")
	if err != nil {
		return err
	}
	defer cleanFBScratch()
	if err := copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex")); err != nil {
		return err
	}
	copyStyleExtras(styleDir, scratch, submit, clsName)
	start := time.Now()
	res := r.comp.Compile(scratch, "example.tex")
	LogCompileResult(r.log, 1, "style-feedback", res, time.Since(start))
	if !res.OK {
		return fmt.Errorf("样式反馈 example 编译失败: %s", res.Err)
	}
	r.log.Log(1, "[style-feedback] 更新后的 example 编译通过:", clsName+".cls")

	// 定向修复：只重做"报告样式问题"或"新 cls 下编译不过"的章节，
	// 其余章节产物保留（省 token）。每章先跑一个并发子会话做增量修复
	// （不是重新转换），修复失败才退回整章重转换。
	chapWork := filepath.Join(proj, "work", "chapters")
	var redo []string
	for _, f := range issueFiles {
		base := strings.TrimSuffix(filepath.Base(f), ".md")
		if fileExists(filepath.Join(chapWork, base+".tex")) {
			redo = append(redo, base)
		}
	}
	// 新 cls 下编译不过的章节也必须重做。
	allTex, _ := filepath.Glob(filepath.Join(chapWork, "*.tex"))
	for _, t := range allTex {
		base := strings.TrimSuffix(filepath.Base(t), ".tex")
		if sliceHas(redo, base) {
			continue
		}
		scratch, cleanChk, serr := r.chapterScratch(proj, clsName, base, "convchk")
		if serr != nil {
			continue
		}
		data, rerr := os.ReadFile(t)
		if rerr == nil {
			_ = os.WriteFile(filepath.Join(scratch, base+".tex"), data, 0o644)
			res := r.comp.Compile(scratch, base+"_wrapper.tex")
			if !res.OK {
				redo = append(redo, base)
				r.log.LogWarning(0, "[style-feedback] 新样式下编译失败，需重做:", base)
			}
		}
		cleanChk()
	}
	if len(redo) == 0 {
		r.log.Log(0, "[style-feedback] 样式包已更新，没有章节需要重做")
		_ = os.RemoveAll(reportsDir)
		return nil
	}
	r.log.Log(0, "[style-feedback] 样式包已更新，重做", strconv.Itoa(len(redo)), "个章节:",
		strings.Join(redo, ", "))
	r.phaseNote()("[style-feedback] 样式包已更新，重做 %d 章", len(redo))

	// 并发子会话做增量样式修复；失败者删除产物，退回整章重转换。
	chapDir := filepath.Join(proj, "chapters")
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
	var fallback []string
	for _, base := range redo {
		wg.Add(1)
		tid := <-tidPool
		go func(b string, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			chapPath := filepath.Join(chapDir, b+".md")
			issues := ""
			if data, err := os.ReadFile(filepath.Join(reportsDir, b+".md")); err == nil {
				issues = string(data)
			}
			if err := r.fixChapterStyle(proj, clsName, manualPath, chapPath, filepath.Join(proj, "work"), b, issues, tid); err != nil {
				r.log.LogWarning(tid, "[style-fix]", b, "增量修复失败，将整章重转换:", err)
				mu.Lock()
				fallback = append(fallback, b)
				mu.Unlock()
			} else {
				r.log.Log(tid, "[style-fix]", b, "修复完成")
			}
		}(base, tid)
	}
	wg.Wait()

	// 失败的章节：删除产物 + 转录，交给 convertPhase 用全新会话重转换。
	for _, b := range fallback {
		_ = os.RemoveAll(filepath.Join(chapWork, b+".tex"))
		_ = os.RemoveAll(filepath.Join(chapWork, b))
		r.keepSessionFile(filepath.Join(proj, "work", "sessions", "convert_"+b+".jsonl"))
	}
	_ = os.RemoveAll(reportsDir)
	if len(fallback) == 0 {
		return nil
	}
	r.log.Log(0, "[style-feedback]", strconv.Itoa(len(fallback)), "章需要整章重转换")
	return r.convertPhase(proj, round+1)
}

// sliceHas reports whether the slice contains want.
func sliceHas(s []string, want string) bool {
	for _, x := range s {
		if x == want {
			return true
		}
	}
	return false
}

// styleSystemPrompt builds the system prompt of the style session: the
// shared template plus the run-specific mount/watermark/source-page
// guidance. The style-FEEDBACK session (which re-runs in the same
// conversation later) MUST use the same text: a resumed transcript carries
// no system message (system prompts are never written to the JSONL), so
// passing "" there silently left that turn without any system prompt.
func (r *Runner) styleSystemPrompt() string {
	prompt := prompts.Must(prompts.StyleSystem)
	prompt += "\n\nBASH PATHS: the bash tool runs inside a kernel sandbox where your workspace, the project and the original PDFs are the only trees, mounted as /work, /project and /source — exactly the trees of the file tools, so work:x.tex = /work/x.tex, project:source/book.md = /project/source/book.md, source:<file>.pdf = /source/<file>.pdf. The real host paths do not exist there; /tmp is scratch."
	if r.cfg.Latex.RemoveWatermark {
		prompt += "\n\n" + r.watermarkGuidance("WATERMARK: the source may carry watermark artifacts (repeated decorative overlay text such as institution/library marks, faint background strings). Identify the watermark pattern in the manual and instruct conversion to EXCLUDE it entirely - watermark text/graphics must NOT be typeset in the LaTeX output.")
	}
	if r.pdfView != nil {
		prompt += "\n\nIMPORTANT: the ORIGINAL book pages are available — call list_source_pages to get the source PDFs, the detected section starts and (with page=N) that page's text snippets and extracted image file names, then look at the page with view_pdf {\"path\":\"source:<file>\", \"page\":N} (crop/zoom supported). They show the TRUE typography and layout: inspect the title pages, headings, headers/footers and representative figures before writing the class. The OCR markdown itself is readable with read_file {\"path\":\"project:<md>\"} whenever you need exact text."
	}
	tuning := r.cfg.LatexSession("style")
	return renderPrompt(prompt, tuning, r.outputLang())
}

// saveSessionContext persists the full conversation of a session as a
// JSONL transcript（图片以 file:// 媒体引用存储，不内联 base64）。
// 兼容旧版单 JSON 文件：若 <path> 不存在而同名 .json 存在，则先迁移。
func saveSessionContext(sess *session.Session, path string) error {
	// A live transcript already appends every message as it happens; writing
	// the whole conversation again produced a duplicated transcript with a
	// system line in the middle (which LoadTranscript does not deduplicate).
	if sess.HasTranscript() {
		return nil
	}
	msgs := sess.Messages()
	if len(msgs) == 0 {
		return fmt.Errorf("空会话")
	}
	migrateLegacyContext(path)
	w, err := session.NewTranscript(path)
	if err != nil {
		return err
	}
	defer w.Close()
	for _, m := range msgs {
		if err := w.Append(m); err != nil {
			return err
		}
	}
	return nil
}

// loadSessionContext restores a persisted conversation (JSONL 转录，
// 兼容旧版单 JSON 文件).
func loadSessionContext(path string) ([]session.ChatMessage, error) {
	if msgs, err := session.LoadTranscript(path); err == nil && len(msgs) > 0 {
		return msgs, nil
	}
	legacy := strings.TrimSuffix(path, ".jsonl") + ".json"
	data, err := os.ReadFile(legacy)
	if err != nil {
		return nil, fmt.Errorf("空会话上下文")
	}
	var msgs []session.ChatMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("空会话上下文")
	}
	return msgs, nil
}

// migrateLegacyContext converts an old single-JSON style session file
// (.json) into the new JSONL transcript format (best effort).
func migrateLegacyContext(jsonlPath string) {
	legacy := strings.TrimSuffix(jsonlPath, ".jsonl") + ".json"
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	if _, err := os.Stat(jsonlPath); err == nil {
		return // already migrated / new format exists
	}
	msgs, err := loadSessionContext(jsonlPath)
	if err != nil {
		return
	}
	if w, err := session.NewTranscript(jsonlPath); err == nil {
		for _, m := range msgs {
			_ = w.Append(m)
		}
		w.Close()
	}
}

// mainSourceMD picks the largest processed markdown in sourceDir (the
// book body) — shared by the style and style-feedback sessions.
func mainSourceMD(sourceDir string) string {
	mds, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
	if len(mds) == 0 {
		return ""
	}
	sort.Strings(mds)
	mainMD := mds[0]
	for _, f := range mds {
		if fi, err := os.Stat(f); err == nil {
			if mj, e2 := os.Stat(mainMD); e2 == nil && fi.Size() > mj.Size() {
				mainMD = f
			}
		}
	}
	return mainMD
}

// preflightConvert gates the concurrent conversion on clean upstream
// phases: the style package must be complete and every source image
// must be finished (done or fallback-annotated). Problems STOP the
// pipeline here — converting on top of unfinished work would bake
// errors into the whole book.
func (r *Runner) preflightConvert(proj string) error {
	if classNameOfFile(filepath.Join(proj, "style")) == "" {
		return fmt.Errorf("转换前置检查未通过：style 目录缺少可识别的 cls（\\ProvidesClass{...}）")
	}
	if _, err := os.Stat(filepath.Join(proj, "style", "manual.md")); err != nil {
		return fmt.Errorf("转换前置检查未通过：缺少 style/manual.md")
	}
	progDir := filepath.Join(proj, "source", "progress_items")
	matches, _ := filepath.Glob(filepath.Join(progDir, "*", "*.json"))
	unfinished := 0
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var p struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &p) != nil || p.Status == "" {
			continue
		}
		if p.Status != "done" && p.Status != "fallback" {
			unfinished++
		}
	}
	if unfinished > 0 {
		return fmt.Errorf("转换前置检查未通过：source 中有 %d 张图片未完成处理（先补齐图片处理再转换，例如 docvision latex --step images）", unfinished)
	}
	r.log.Log(0, "[preflight] 转换前置检查通过：样式包完整，图片全部处理完毕")
	return nil
}

// writeFontsReadme 保证项目字体目录里有 README：告诉用户缺字体时把
// 什么文件按什么名字放进该目录（编译环境已通过 TEXINPUTS/OSFONTDIR
// 指向这里，cls 里直接用文件名引用即可）。已存在则不覆盖。
func writeFontsReadme(fontsDir string) {
	if fontsDir == "" {
		return
	}
	if err := os.MkdirAll(fontsDir, 0o755); err != nil {
		return
	}
	readme := filepath.Join(fontsDir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		return
	}
	content := `# 字体目录 / Fonts

样式会话发现原书使用的字体本机没有时，会在提交报告与 manual.md 里列出**缺失字体清单**。

## 使用方法

1. 自行下载对应的字体文件（.ttf / .otf），下载渠道自选（官方发布页、开源字体如思源/SIL OFL 家族等）。
2. 把文件放进本目录（fonts/），**文件名保持清单里给出的名字**（例如 SourceHanSerifSC-Regular.otf）。
3. 重新运行 latex 流程（或只重跑 style 后续阶段）。编译环境已把本目录注入 TEXINPUTS / OSFONTDIR，cls 里按文件名引用即可命中，无需安装到系统。

## 命名约定

- 文件名 = 样式会话清单里的名字（通常为原字体英文名）。
- 一个字体家族的多个字重各自单独成文件（Regular / Bold / Italic ...）。
`
	_ = os.WriteFile(readme, []byte(content), 0o644)
}
