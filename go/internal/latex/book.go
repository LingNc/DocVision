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
	// 内存自证（T158/T162）：SIGKILL 捕获不到，所以把基线与采样写进日志，
	// 并**同时追加到项目内**的 work/mem.log——终端被刷掉或进程被杀，那
	// 个文件还在，事后直接看最后几行就知道是不是内存被吃光。前 3 分钟
	// 每 10s 一次（续跑要同时恢复十几条会话、每条都把转录里的图片读成
	// data URL，这是内存尖峰），之后每 60s 一次。
	memLogPath := filepath.Join(proj, "work", "mem.log")
	if stop := r.startMemWatch(10*time.Second, memLogPath); stop != nil {
		defer stop()
	}
	// 只留这一行普通模式提示：被杀之后要看的文件到底在哪。采样行本身在
	// 普通模式不显示（debug 日志与 mem.log 里全量都有）。
	r.log.LogInfo(0, "[book] 内存/被杀现场日志:", memLogPath)
	// 并发账目（T179）：三个池子的大小与模型级上限一次说清——用户调大
	// concurrency 之后"为什么没有更快"通常就卡在模型级上限上。
	r.logConcurrencyPlan()
	// 重试预算与递归层数的匹配检查（T165）：顶层设得太小时，深层永远只能
	// 吃下限 3，用户以为调大了却传不下去——起步就说清楚。
	if warn := checkerBudgetWarning(r.cfg.Latex.CheckerRounds); warn != "" {
		r.log.LogWarning(0, "[convert]", warn)
	}
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
			// T85：assemble 标 done 但上次终审其实**中断**（失败也照常交付
			// 的语义会写下 done）——中断态仍与当前工作区一致时不能跳过，
			// 要重跑 assemble 把终审续上。
			if name == "assemble" && r.assembleNeedsResume(proj) {
				msg := "[book] assemble 标为 done，但检测到上次终审中断——重跑 assemble 续跑终审"
				note(msg)
				r.log.Log(0, msg)
			} else {
				note("[book] phase %s already done, skipping", name)
				r.log.Log(0, "[book] phase "+name+" already done, skipping")
				return nil
			}
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
	// T109：无图片的书老版本不会把 md 落进 source/，且 images 已标 done
	// 续跑时被跳过——style 前确保 source/ 有 md，没有就从原 md 补拷。
	r.ensureSourceMD(proj, opts.SourceDir, opts.Files)

	err = runPhase("style", func() error {
		if err := r.stylePhase(proj); err != nil {
			return err
		}
		// T97 重量档：选材→试转→样式修订（失败只警告，不挡后续阶段）。
		if r.cfg.Latex.LatexStylePilotEnabled() {
			r.stylePilotPhase(proj)
		}
		return nil
	})
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

// ensureSourceMD 兜底：proj/source 下没有任何 md 时，从原始 MinerU 输出
// 把（过滤后的）md 原样补拷过来（T109）。
func (r *Runner) ensureSourceMD(proj, srcDir string, files []string) {
	if srcDir == "" {
		srcDir = r.cfg.Paths.OutputDir
	}
	mds, _ := filepath.Glob(filepath.Join(proj, "source", "*.md"))
	if len(mds) > 0 {
		return
	}
	srcs, _ := filepath.Glob(filepath.Join(srcDir, "*.md"))
	sort.Strings(srcs)
	srcs = filterFiles(srcs, files)
	for _, f := range srcs {
		dst := filepath.Join(proj, "source", filepath.Base(f))
		copyFile(f, dst)
		r.log.Log(1, "[book] source 缺 md，已从原始输出补拷:", filepath.Base(f))
	}
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
	goal := NewGoalTool(filepath.Join(workDir, "plan.md"))
	submit := &SubmitStyleTool{Workspace: workDir, Gate: func() string { return goalGate(goal) }}
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_style")
	defer cleanBashTmp()
	tools := r.styleSessionTools(proj, workDir, sourceDir, "style", bashTmp, submit)
	tools = append(tools, goal)

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
		if !r.remindSubmit(sess, 1, "[style]", "style", "submit_style", func() bool { return submit.Set }) {
			return fmt.Errorf("样式会话未提交 submit_style（已提醒两次）")
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
	// split.json 预置空划分：split 工具的结构化操作与 edit_file 裸改都
	// 从同一份文件出发（edit 直改是逃生舱，工具每次操作前重读文件）。
	splitPath := filepath.Join(sandbox, "split.json")
	_ = os.WriteFile(splitPath, []byte("{\"chapters\": []}\n"), 0o644)

	client := r.clientFor(r.cfg.Latex.ChapterModel)
	modelCfg := r.modelOf(r.cfg.Latex.ChapterModel)
	tuning := r.cfg.LatexSession("chapter")
	goal := NewGoalTool(filepath.Join(sandbox, "plan.md"))
	split := NewSplitTool(splitPath)
	submit := &SubmitSplitTool{Root: sandbox, Gate: func() string { return goalGate(goal) }}
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_chapters")
	defer cleanBashTmp()
	sess := session.NewSession(client, modelCfg, tuning, renderPrompt(prompts.Must(prompts.ChaptersSystem), tuning, r.outputLang()), []session.Tool{
		&GrepTool{Root: sandbox},
		&ReadFileTool{Root: sandbox},
		&WorkBashTool{Root: sandbox, Mounts: r.sessionMounts(kindChapters, sandbox), TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		&EditWorkFileTool{Root: sandbox},
		goal,
		split,
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
		granularity = "medium"
	}
	// T45：三档——large=大章整体；medium（默认）=按这本书自动给出最合理的
	// 拆分（不拆太细、也不太大块）；small=小节级。
	gran := "MEDIUM granularity (default): choose the most sensible split for THIS book — one file per coherent mid-size unit (typically a top-level chapter, split further ONLY when it is clearly too big for one conversion pass). Never split too fine (no per-paragraph fragments) and never leave one oversized block. Never split mid-section."
	if granularity == "large" {
		gran = "LARGE granularity: one chapter = one TOP-LEVEL chapter of the book. Never split a top-level chapter into pieces; if a chapter is huge, it stays one file (the converter handles it)."
	} else if granularity == "small" {
		gran = "SMALL granularity: one chapter = one SECTION. Split at the finest heading level that yields coherent, self-contained units (a top-level chapter containing several sections becomes several files). Never split mid-section."
	}
	initial := prompts.Render(prompts.ChaptersUser, map[string]string{
		"GRANULARITY": gran,
		"TOTAL_LINES": strconv.Itoa(totalLines),
	})

	if len(chapterMsgs) > 0 && !chaptersSkip {
		initial = resumeUserText(chapterMsgs, "The session was interrupted earlier. Continue from where you left off, write split.json and call submit_split.")
	}
	replayRunPending := chaptersSkip // 重放已提交：第一轮跳过 Run 直接走校验写盘
	for attempt := 0; attempt < 3; attempt++ {
		if replayRunPending {
			replayRunPending = false
		} else {
			userText := initial
			if attempt > 0 {
				userText = "Your split was rejected: " + r.lastSplitError + "\nFix split.json and call submit_split again."
			}
			if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
				return fmt.Errorf("章节划分会话失败: %w", err)
			}
		}
		if !r.remindSubmit(sess, 1, "[chapters]", "chapters", "submit_split", func() bool { return submit.Set }) {
			return fmt.Errorf("章节划分会话未提交 submit_split（已提醒两次）")
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

/*
 * logConcurrencyPlan 打印并发账目并在"池子之和超过模型上限"时告警（T179）：
 * 转换与核对各自排队的阶段不同，但落到同一个模型上时挤压的是同一个额度。
 */
func (r *Runner) logConcurrencyPlan() {
	l := r.cfg.Latex
	convert, checker, other := l.ConvertWorkers(), l.CheckerWorkers(), l.OtherWorkers()
	line := "转换 " + strconv.Itoa(convert) + " / 核对 " + strconv.Itoa(checker) + "（独立槽位）/ 其它阶段 " + strconv.Itoa(other)
	caps := r.cfg.ModelConcurrencyLimits()
	if len(caps) == 0 {
		r.log.LogInfo(0, "[book] 并发:", line, "；模型级不限（models.<名>.max_concurrency 未设置）")
		return
	}
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" 上限 "+strconv.Itoa(caps[name]))
	}
	r.log.LogInfo(0, "[book] 并发:", line, "；模型级:", strings.Join(parts, " / "))

	// 想同时用同一个模型的会话数：转换池 + 核对池（两者是这本流程里唯一会
	// 几十个并行的阶段）。
	checkerName := l.CheckerModel
	if checkerName == "" {
		checkerName = l.ConvertModel
	}
	demand := map[string]int{}
	if mc, ok := r.cfg.ResolveModel(l.ConvertModel); ok && mc.Model != "" {
		demand[mc.Model] += convert
	}
	if mc, ok := r.cfg.ResolveModel(checkerName); ok && mc.Model != "" {
		demand[mc.Model] += checker
	}
	for _, name := range names {
		if want := demand[name]; want > caps[name] {
			r.log.LogWarning(0, "[book] 并发:模型", name, "上限", strconv.Itoa(caps[name]),
				"，但转换 "+strconv.Itoa(convert)+" + 核对 "+strconv.Itoa(checker)+" 都在用它——实际最多",
				strconv.Itoa(caps[name]), "个在跑，其余排队等模型槽位；要更快得先调高该模型的 max_concurrency")
		}
	}
}

/*
 * 阶段槽位（T179）：转换与核对各一个池子，各自的上限来自
 * latex.convert_concurrency / checker_concurrency（没写沿用 concurrency）。
 * 两者独立计数，所以"转换 3 + 核对 3"可以同时在途 6 个会话；要限制真正打到
 * 同一个模型上的请求数，用 models.<名>.max_concurrency（会话层还有一个按模型
 * 名的闸门，见 session.SetModelConcurrency）。
 */
func (r *Runner) acquireConvertSlot() func() {
	r.convertSlotsOnce.Do(func() { r.convertSlots = make(chan struct{}, r.cfg.Latex.ConvertWorkers()) })
	r.convertSlots <- struct{}{}
	return func() { <-r.convertSlots }
}

func (r *Runner) acquireCheckerSlot() func() {
	r.checkerSlotsOnce.Do(func() { r.checkerSlots = make(chan struct{}, r.cfg.Latex.CheckerWorkers()) })
	r.checkerSlots <- struct{}{}
	return func() { <-r.checkerSlots }
}

// acquireChapterSlot 是其余阶段的通用池子（样式修复、提交提醒等）。
func (r *Runner) acquireChapterSlot() func() {
	r.chapterSlotsOnce.Do(func() { r.chapterSlots = make(chan struct{}, r.cfg.Latex.OtherWorkers()) })
	r.chapterSlots <- struct{}{}
	return func() { <-r.chapterSlots }
}

func (r *Runner) convertChapterChildren(proj, clsName, manualPath, chapPath, parentBase, parentWork, workDir string, idx, tid int, parent chapterNodeConfig, children []ChapterSubdivision) error {
	data, err := os.ReadFile(chapPath)
	if err != nil {
		return fmt.Errorf("读取父章节失败: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	parentParts := filepath.Join(parentWork, parentBase, "children")
	if err := os.RemoveAll(parentParts); err != nil {
		return err
	}
	if err := os.MkdirAll(parentParts, 0o755); err != nil {
		return err
	}
	sem := make(chan struct{}, r.cfg.Latex.ConvertWorkers())
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []string
	var inherited []ChapterIssue
	for i, child := range children {
		id := fmt.Sprintf("child_%02d", i+1)
		chapter := child
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			childRoot := filepath.Join(parentWork, "_subdivision", id)
			nodeRoot := filepath.Join(childRoot, "node")
			inputDir := filepath.Join(childRoot, "input")
			if err := os.MkdirAll(inputDir, 0o755); err != nil {
				mu.Lock()
				failures = append(failures, id+": "+err.Error())
				mu.Unlock()
				return
			}
			childMD := filepath.Join(inputDir, "chapter.md")
			if err := os.WriteFile(childMD, []byte(chapterSubrange(lines, chapter)), 0o644); err != nil {
				mu.Lock()
				failures = append(failures, id+": "+err.Error())
				mu.Unlock()
				return
			}
			childBase := parentBase + "_" + id
			childOut := filepath.Join(nodeRoot, "result")
			childNode := chapterNodeConfig{
				Base: childBase, Depth: parent.Depth + 1, InputMount: "input", InputRef: "chapter.md",
				NodeRoot:       nodeRoot,
				TranscriptPath: filepath.Join(nodeRoot, "sessions", "convert_"+childBase+".jsonl"),
				SubmitRoot:     childOut,
				ReportPath:     filepath.Join(nodeRoot, "reports", "conversion.md"),
				StrictChecker:  true,
			}
			if err := r.convertChapterNode(proj, clsName, manualPath, childMD, workDir, idx, tid, false, childNode); err != nil {
				mu.Lock()
				failures = append(failures, id+": "+err.Error())
				mu.Unlock()
				return
			}
			if err := copyFile(filepath.Join(childOut, childBase+".tex"), filepath.Join(parentParts, childBase+".tex")); err != nil {
				mu.Lock()
				failures = append(failures, id+": "+err.Error())
				mu.Unlock()
				return
			}
			if fileExists(filepath.Join(childOut, childBase)) {
				if err := copyDir(filepath.Join(childOut, childBase), filepath.Join(parentParts, childBase)); err != nil {
					mu.Lock()
					failures = append(failures, id+": "+err.Error())
					mu.Unlock()
					return
				}
			}
			// 问题继承（T166）：把子节点 store 里**未解决**的条目收上来，
			// 由父节点提交时合并进它自己的 store。父会话不需要读这些也能
			// 保证它们继续向上（见 chapterissues.go）。
			childIssues := openIssues(readIssueStore(filepath.Join(nodeRoot, "work", ".issues", "issues.json")))
			mu.Lock()
			inherited = append(inherited, childIssues...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("子章节转换或 checker 失败；父章节禁止提交: %s", strings.Join(failures, "; "))
	}
	// 父节点的继承清单：去重后落盘，整合会话与 submit 都从这里取。
	if err := writeIssueStore(filepath.Join(parentWork, "_subdivision", "issues-inherited.json"), openIssues(inherited)); err != nil {
		return fmt.Errorf("写入子章节问题继承清单失败: %w", err)
	}
	if n := len(openIssues(inherited)); n > 0 {
		r.log.LogInfo(tid, "[convert]", parentBase, "继承", strconv.Itoa(n), "条子章节未解决问题（未修的会原样向上）")
	}
	return nil
}

func writeChapterChildrenManifest(work, base string, children []ChapterSubdivision) error {
	var b strings.Builder
	b.WriteString("# 已核验子章节\n\n")
	b.WriteString("每个子章节已经过独立转换 + 独立 checker（明确 pass 才会出现在这里）。\n\n")
	for i, child := range children {
		id := fmt.Sprintf("child_%02d", i+1)
		artifactBase := base + "_" + id
		fmt.Fprintf(&b, "%d. %s（Markdown 行 %d-%d）\n   - TeX: %s/children/%s.tex\n   - Parts: %s/children/%s/\n   - Conversion report: %s/_subdivision/%s/node/reports/conversion.md\n   - Conversion transcript: %s/_subdivision/%s/node/sessions/convert.jsonl\n   - Checker transcript: %s/_subdivision/%s/node/sessions/checker.jsonl\n", i+1, child.Title, child.StartLine, child.EndLine, base, artifactBase, base, artifactBase, base, id, base, id, base, id)
	}
	if err := os.WriteFile(filepath.Join(work, "_subdivision", "children-manifest.md"), []byte(b.String()), 0o644); err != nil {
		return err
	}
	return writeChildrenSummary(work, base, children)
}

// writeChildrenSummary 把每个子章节**自己交的汇报**（结论/问题/建议）原样
// 汇总成一个文件，交给父章节的整合会话（T165）。
//
// 为什么要这一步：子章节的 checker 只能判"通过/不通过"，它给不出"这一章
// 哪里没还原"的细节；细节在子会话 submit 的 report 里（有章节明确写了
// "enumerate/proof 结构对不上、没能干净编译"）。以前这些报告只是躺在
// `_subdivision/<child>/node/reports/` 下没人读——父会话的整合指令只点名
// 读 .tex 与 parts/，于是父章节带着同样的毛病提交，问题一路上浮到最后的
// 终审才炸。现在汇总成一份必读文件，并在文首与结尾各强调一次。
func writeChildrenSummary(work, base string, children []ChapterSubdivision) error {
	var b strings.Builder
	b.WriteString("# 子章节汇报汇总（整合前必读）\n\n")
	b.WriteString("下面是每个子章节转换会话在 submit 时交的 `report` 原文。它说的是\"这一章向原书还原时哪里没做到 / 哪里绕过了手册\"，" +
		"**checker 只看结构与文本一致性，不会替你判断这些**。\n\n" +
		"**不要求你修复这些问题**——多数是样式/类文件层面的，后面有专门的风格修复大循环统一处理。" +
		"你要做的是：能顺手修的就修（修了在 submit 的 `report.resolved` 里写上它的 ID）；" +
		"没修的**保持原样**，提交时合并机制会自动把它们带进你的报告继续向上传递，你不需要抄一遍。" +
		"你自己发现的**新问题**写进 `report.issues`（推荐字符串数组，一个问题一条），它们会和继承条目一起往上走。\n\n")
	problems := 0
	for i, child := range children {
		id := fmt.Sprintf("child_%02d", i+1)
		reportPath := filepath.Join(work, "_subdivision", id, "node", "reports", "conversion.md")
		raw, err := os.ReadFile(reportPath)
		fmt.Fprintf(&b, "## %d. %s（行 %d-%d，%s）\n\n", i+1, child.Title, child.StartLine, child.EndLine, id)
		if err != nil {
			b.WriteString("_（这个子章节没有留下汇报文件）_\n\n")
			continue
		}
		text := strings.TrimSpace(string(raw))
		if strings.Contains(text, "结论: issues") || strings.Contains(text, "结论:issues") {
			problems++
		}
		if len(text) > 6000 {
			text = text[:6000] + "\n…（截断，原文见 " + reportPath + "）"
		}
		b.WriteString(text + "\n\n")
	}
	b.WriteString("## 你要做的\n\n")
	b.WriteString("1. 读完上面每一段与每个子 `.tex`/`parts/`，按原书顺序整合成父章节；\n")
	b.WriteString("2. 顺手能修的修（缺段、错层级、丢公式、编号错位都算），修掉的把 ID 写进 `report.resolved`；\n")
	b.WriteString("3. 修不了的不必勉强：不写 resolved，合并机制会把它原样带上；你自己的新问题写进 `report.issues`；\n")
	b.WriteString("4. 改完必须重新 compile，并用 view_pdf 看过编译出的 PDF（至少第 1 页与最后一页），否则 submit 会被拒绝。\n")
	if problems > 0 {
		fmt.Fprintf(&b, "\n⚠ 有 %d 个子章节自己报了 `结论: issues`——它们的问题很可能还在父章节里，别当成已经解决。\n", problems)
	}
	// 机制合并的清单：这些条目会在你 submit 时自动进入报告（不需要你抄）。
	if inherited := openIssues(readIssueStore(filepath.Join(work, "_subdivision", "issues-inherited.json"))); len(inherited) > 0 {
		fmt.Fprintf(&b, "\n## 已继承的问题（%d 条，机制合并）\n\n", len(inherited))
		b.WriteString("这些是子章节提交时记录的问题，**没修就会原样向上**。修掉了哪条，把它的 ID 填进 `report.resolved`。\n\n")
		for _, it := range inherited {
			fmt.Fprintf(&b, "- `%s`（来源 %s）：%s\n", it.ID, it.Source, strings.ReplaceAll(it.Text, "\n", " "))
		}
	}
	summaryPath := filepath.Join(work, "_subdivision", "children-summary.md")
	return os.WriteFile(summaryPath, []byte(b.String()), 0o644)
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
	node := chapterNodeConfig{
		InputMount: "project", InputRef: "chapters/" + base + ".md",
		NodeRoot:       filepath.Join(proj, "work", "temp", "conv_"+base),
		TranscriptPath: filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"),
		SubmitRoot:     filepath.Join(workDir, "chapters"),
		ReportPath:     filepath.Join(workDir, "reports", base+".md"),
	}
	return r.convertChapterNode(proj, clsName, manualPath, chapPath, workDir, idx, tid, retry, node)
}

type chapterNodeConfig struct {
	Base            string
	Depth           int
	InputMount      string
	InputRef        string
	NodeRoot        string
	TranscriptPath  string
	SubmitRoot      string
	FinalSubmitRoot string
	ReportPath      string
	StrictChecker   bool
	// Attempt 是**本次运行**内的「第几次尝试」（1 起）：checker 连续不通过就换
	// 一个全新会话重来，预算按它算（checker_rounds，按层递减）。
	Attempt int
	// AttemptNum 是写进转录文件名的尝试号（1 起），跨运行单调递增：0 = 由磁盘
	// 上已有的转录推导。两者分开是因为"这次运行还有几次机会"与"这一章历史上
	// 第几次尝试"不是一回事——以前只有一个号，重跑时会拿已经用过的
	// convert_<章>.try2.jsonl 继续追加，两段对话混进同一条转录（T171）。
	AttemptNum int
}

func (r *Runner) convertChapterNode(proj, clsName, manualPath, chapPath, workDir string, idx, tid int, retry bool, node chapterNodeConfig) error {
	base := node.Base
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(chapPath), filepath.Ext(chapPath))
	}
	if node.TranscriptPath == "" {
		node.TranscriptPath = filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
	}
	if node.NodeRoot == "" {
		node.NodeRoot = filepath.Join(proj, "work", "temp", "conv_"+base)
	}
	if node.SubmitRoot == "" {
		node.SubmitRoot = filepath.Join(workDir, "chapters")
	}
	if node.ReportPath == "" {
		node.ReportPath = filepath.Join(workDir, "reports", base+".md")
	}
	if node.FinalSubmitRoot == "" {
		node.FinalSubmitRoot = node.SubmitRoot
	}
	if node.Attempt < 1 {
		node.Attempt = 1
	}
	// T171：每次尝试都从**原始**转录名派生。重试是递归调用同一个函数，而
	// `next := node` 会带上已经被改写的 TranscriptPath——过去就是拿它再套一层
	// 后缀，跑出 convert_<章>.try2.try3.jsonl 这种套娃名，预览页把同一次尝试
	// 拆成两个节点（一个 chapter_001 一个 chapter_001.try2）。
	transcriptBase := node.TranscriptPath
	mdData, readErr := os.ReadFile(chapPath)
	if readErr != nil {
		return fmt.Errorf("读取章节 Markdown 失败: %w", readErr)
	}
	lineCount := len(strings.Split(string(mdData), "\n"))
	splitTool := &RequestChapterSplitTool{Root: filepath.Join(node.NodeRoot, "work", "_subdivision"), TotalLine: lineCount, Depth: node.Depth}
	if err := splitTool.Restore(); err != nil {
		return err
	}
	if splitTool.Restored {
		node.SubmitRoot = filepath.Join(node.NodeRoot, "integrated-stage")
		node.StrictChecker = true
	}
	if node.AttemptNum < 1 {
		node.AttemptNum = lastAttemptNumber(transcriptBase)
	}
	node.TranscriptPath = attemptTranscriptPath(transcriptBase, node.AttemptNum)
	texPath := filepath.Join(node.SubmitRoot, base+".tex")
	if node.Depth == 0 && fileExists(filepath.Join(node.FinalSubmitRoot, base+".tex")) {
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
	trPath := node.TranscriptPath
	workRoot := filepath.Join(node.NodeRoot, "work")
	resuming := node.Attempt == 1 && fileExists(trPath) && fileExists(workRoot)
	if !resuming {
		// T174：开新尝试只清这一层的工作区，**不动 `sessions/`**——递归子章节
		// 的转录挂在这棵子树里，删了预览页就看不到它们的历史。上一轮的东西
		// 由换会话那一步先挪进 `work/attempts/`，这里删的是已经挪空的位置。
		_ = os.RemoveAll(workRoot)
		_ = os.RemoveAll(filepath.Join(node.NodeRoot, "result"))
		_ = os.RemoveAll(filepath.Join(node.NodeRoot, "integrated-stage"))
		_ = os.RemoveAll(filepath.Join(node.NodeRoot, "checker-view"))
	}
	work, cleanWork, err := r.chapterWorkTreeAt(proj, clsName, base, node.NodeRoot, true, "")
	if err != nil {
		return err
	}
	defer func() { cleanWork() }()
	if node.Depth > 0 || splitTool.Restored {
		cleanWork = func() {}
	}

	goal := NewGoalTool(filepath.Join(work, "plan.md"))
	gate := r.submitGate(work, base, chapPath)
	submit := &SubmitChapterTool{
		GateRef:       gate,
		WorkRoot:      work,
		SubmitRoot:    node.SubmitRoot,
		Base:          base,
		ReportPath:    node.ReportPath,
		Depth:         node.Depth,
		IssuesPath:    filepath.Join(work, ".issues", "issues.json"),
		InheritedPath: filepath.Join(work, "_subdivision", "issues-inherited.json"),
		Gate: func() string {
			if msg := goalGate(goal); msg != "" {
				return msg
			}
			// 章节太大就先拆（T169）：这条与"看过原书/内容覆盖率"同样属于
			// 提交前必须满足的条件，而不是建议。
			if msg := chapterSizeReject(lineCount, r.cfg.Latex.MaxChapterLines, node.Depth, splitTool.Requested); msg != "" {
				return msg
			}
			if gate == nil {
				return ""
			}
			return gate.check()
		},
	}
	// 挂载表：work = 本章临时工作区（唯一可写），project/source = 只读。
	convertMounts := r.sessionMounts(kindConvert, work)
	if node.InputMount == "input" {
		convertMounts = append(convertMounts, Mount{Name: "input", Dir: filepath.Dir(chapPath)})
	}
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
		&ViewPDFTool{Mounts: convertMounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio(), Gate: gate},
		&CompileChapterTool{Comp: r.comp, Dir: work, MainFile: base + ".tex", WrapperFile: base + "_wrapper.tex", Gate: gate, Log: r.log, Tid: 1},
		&WorkBashTool{Mounts: convertMounts, Root: work, TmpDir: bashTmp,
			MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		goal,
		submit,
	}
	tools = append(tools, splitTool)
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

	// T93：首条消息不再内联章节前 2000 字符——该 md 本就是会话第一步
	// 必读的文件，内联只会跟着每轮请求重发。
	initial := prompts.Render(prompts.ConvertUser, map[string]string{
		"CHAPTER_FILE": "chapter markdown path: " + node.InputMount + ":" + node.InputRef,
		"TEX_PATH":     "Write it as work:" + base + ".tex (extra \\input parts go in the folder work:" + base + "/), compile until clean, then submit those TWO paths.",
	})
	// 章节规模当面说清（T169）：几千行的章节一个会话既慢又容易漏内容，先按语义
	// 拆开比事后打回便宜得多。
	if note := chapterSizeNote(lineCount, r.cfg.Latex.MaxChapterLines, node.Depth); note != "" {
		initial += "\n\n" + note
	}

	if r.cfg.Latex.RemoveWatermark {
		initial += "\n\n" + r.watermarkGuidance("WATERMARK: exclude watermark artifacts from the .tex output (repeated decorative overlay text such as institution marks, faint background strings). Skip such content entirely - do not typeset it.")
	}

	if resumed && !resumeSkip {
		// T29：历史以 tool 回执收尾就原样续行（不插内容、不重发任务提示）；
		// 只有模型自己停了才发一句最小推动。
		initial = resumeUserText(resumeMsgs, "The session was interrupted earlier. Continue from where you left off: read your last written .tex state (read_file), finish the conversion, compile until clean, then submit.")
	}
	if !resumeSkip && !splitTool.Requested {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "queued"})
		release := r.acquireConvertSlot()
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
		_, runErr := sess.Run(session.RunOptions{UserText: initial})
		release()
		if runErr != nil {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
			return fmt.Errorf("会话失败: %w", runErr)
		}
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "checking_result"})
	}
	if splitTool.Requested && submit.Submitted && !splitTool.Restored {
		_ = os.Remove(texPath)
		_ = os.RemoveAll(filepath.Join(node.SubmitRoot, base))
		return fmt.Errorf("父章节不能在请求细分的同一轮提交；子章节和 checker 尚未完成")
	}
	promoteSubmitRoot := ""
	if splitTool.Restored {
		promoteSubmitRoot = node.FinalSubmitRoot
	}
	if splitTool.Requested && !submit.Submitted {
		cleanWork = func() {}
		promoteSubmitRoot = node.FinalSubmitRoot
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "waiting_children", "children": len(splitTool.Children)})
		stageRoot := filepath.Join(node.NodeRoot, "integrated-stage")
		_ = os.RemoveAll(stageRoot)
		if err := os.MkdirAll(stageRoot, 0o755); err != nil {
			return err
		}
		node.SubmitRoot = stageRoot
		node.StrictChecker = true
		submit.SubmitRoot = stageRoot
		texPath = filepath.Join(stageRoot, base+".tex")
		if err := r.convertChapterChildren(proj, clsName, manualPath, chapPath, base, work, workDir, idx, tid, node, splitTool.Children); err != nil {
			return err
		}
		if err := writeChapterChildrenManifest(work, base, splitTool.Children); err != nil {
			return err
		}
		integrationText := "All child chapters have passed their individual checkers. FIRST read work:_subdivision/children-summary.md — it contains each child's own report (what it could not restore, what it worked around) and the list of issues already inherited for you. " +
			"You are NOT required to fix those issues (most are class/manual level and are handled later by the style-fix pass); fix the ones you can while integrating, put their IDs in report.resolved, and leave the rest untouched — the merge mechanism carries them into your own report automatically. " +
			"Then read work:_subdivision/children-manifest.md and every listed child .tex/parts file. " +
			"Integrate them in source order into work:" + base + ".tex and work:" + base + "/, preserving all content and references. " +
			"Then compile the parent wrapper, LOOK at the resulting PDF with view_pdf (page 1 and the last page at least), AND compare it with the original book pages (view_pdf source:<part>.pdf): submit requires you to have looked at a few original pages. " +
			"submit also compares your PDF text with the chapter markdown and rejects when too much content is missing, so do not wrap the markdown into LaTeX with a script. " +
			"Put any NEW problems you found into report.issues (as a list); unresolved inherited ones go up by themselves. " +
			"Do not submit child files as independent book chapters."
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
		release := r.acquireConvertSlot()
		_, runErr := sess.Run(session.RunOptions{UserText: integrationText})
		release()
		if runErr != nil {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
			return fmt.Errorf("父章节整合会话失败: %w", runErr)
		}
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "checking_result"})
	}
	if splitTool.Requested {
		if !submit.Submitted {
			return fmt.Errorf("细分父章节整合会话未提交")
		}
	} else if !r.remindSubmit(sess, tid, "[convert]", base, "submit", func() bool { return submit.Submitted }) {
		// 必须显式提交（用户口径）：提前收尾漏了 submit 就提醒续跑（最多两次），
		// 仍不提交判失败——不再静默采纳"编译通过但没提交"的产物
		// （chapter_003 事故：模型漏调 submit，章节被悄悄收下，界面看不出）。
		return fmt.Errorf("会话未提交（已提醒两次）")
	}
	if !fileExists(texPath) {
		return fmt.Errorf("会话已提交但没有写出 .tex")
	}
	// 内容覆盖率（T168）：submit 已经用它挡过一道（低于阈值直接打回），这里把
	// 数字留在日志里——"这一章到底和原稿对不对得上"是可以事后查的。
	if gate != nil {
		if cov, ok := gate.lastCoverage(); ok {
			r.log.LogInfo(tid, "[convert]", base, fmt.Sprintf("内容覆盖率 %.1f%%（md 正文 → PDF 文本）", cov*100))
			if cov < 0.92 {
				r.log.LogWarning(tid, "[convert]", base, "内容覆盖率偏低，建议再与原文对照检查是否漏段")
			}
		}
	}
	// Checker pass: a small dedicated session (read_file/grep/submit only,
	// read-only view holding the chapter markdown + the submitted .tex +
	// its parts folder) verifies the chapter against the markdown. Hard
	// problems are fed straight back to the SAME conversion session (its
	// context is still live, so it is the cheapest and most accurate
	// fixer), up to `rounds` (按层递减) rounds; only then the chapter is
	// discarded and re-converted with a fresh session.
	checkerTranscript := ""
	checkerView := ""
	if node.Depth > 0 || promoteSubmitRoot != "" {
		checkerTranscript = attemptTranscriptPath(filepath.Join(node.NodeRoot, "sessions", "checker_"+base+".jsonl"), node.AttemptNum)
		checkerView = filepath.Join(node.NodeRoot, "checker-view")
	}
	rounds := r.checkerAttempts(node.Depth)
	partsPath := filepath.Join(node.SubmitRoot, base)
	hints := func() checkHints {
		h := checkHints{Report: node.ReportPath, Notes: submit.Notes}
		if gate != nil { // 门禁关掉时没有编译记录，就没有产物页可给
			if pdf, _, ok := gate.compiledPDF(); ok {
				h.PDF = pdf
			}
		}
		return h
	}
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints(), tid, checkerTranscript, checkerView, node.StrictChecker)
	if !ok && isSessionAPIError(issues) {
		return fmt.Errorf("checker 会话失败（换 checker 模型后可续跑）: %s", issues)
	}
	if !ok && node.StrictChecker {
		// 递归子节点与顶层同策略：先在本会话里打回，再由上面的换会话逻辑兜底。
		r.log.LogWarning(tid, "[checker]", base, "递归节点首次核对未过:", issues)
	}
	for round := 0; !ok && round < rounds; round++ {
		r.log.LogWarning(tid, "[checker]", base, "第"+strconv.Itoa(round+1)+"轮反馈:", issues)
		r.phaseNote()("[checker] %s 第 %d 轮反馈", base, round+1)
		msg := "The checker reviewed your submitted chapter and found problems that must be fixed:\n" + issues +
			"\n\nFix them in your workspace (edit_file/write_file), compile until clean, then submit the two paths again." +
			"\n\nAn item is not automatically true: the checker reads the markdown, your compiled PDF and the original pages. " +
			"If the marked spot is NOT a real problem (the markdown is OCR noise, the formula/table/figure comes from the original page and your .tex reproduces it), do not damage correct content to satisfy it — put the evidence (file and line, or the source page number) in submit.notes and submit again; it will be re-checked against the page."
		release := r.acquireConvertSlot()
		_, runErr := sess.Run(session.RunOptions{UserText: msg})
		release()
		if runErr != nil {
			r.log.LogWarning(tid, "[checker]", base, "反馈轮失败:", runErr)
			break
		}
		ok, issues = r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints(), tid, checkerTranscript, checkerView, node.StrictChecker)
		if !ok && isSessionAPIError(issues) {
			return fmt.Errorf("checker 会话失败（换 checker 模型后可续跑）: %s", issues)
		}
	}
	if ok {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "pass"})
		r.log.Log(tid, "[checker]", base, "通过")
		_ = os.Remove(texPath + ".checker")
		if promoteSubmitRoot != "" {
			if _, err := placeChapterFiles(node.SubmitRoot, promoteSubmitRoot, base); err != nil {
				return fmt.Errorf("父章节 checker 通过但提升产物失败: %w", err)
			}
		}
	} else {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "issues"})
		_ = os.WriteFile(texPath+".checker", []byte(issues), 0o644)
		// 同一个会话被 checker 打回 N 轮还不过，说明它的理解已经跑偏（实测：
		// 一条子会话在只读报错后把"环境有问题"当成前提，反复重写同一个文件、
		// 甚至去 kill 别的进程）。这时**换一个全新会话**比继续劝它便宜也可靠，
		// 与顶层章节同一策略、次数同样按层递减。每次尝试一条独立转录，预览页
		// 里能看到「第几次尝试」。
		if node.Attempt < r.checkerAttempts(node.Depth) {
			r.log.LogWarning(tid, "[checker]", base, "第", strconv.Itoa(node.Attempt), "次尝试反馈",
				strconv.Itoa(rounds), "轮仍未通过 — 作废产物，换全新会话重转换")
			r.phaseNote()("[checker] %s 第 %d 次尝试未过 — 换新会话", base, node.Attempt)
			// T174：作废产物 ≠ 删光证据。上一轮交的 .tex/分片、工作区、编译
			// 产物先整棵挪进 `work/attempts/<章>/try<N>/`，再开新会话——否则
			// 「历史」里只剩对话，看不到那一轮到底交了什么东西。
			r.archiveChapterAttempt(proj, base, node, texPath, issues)
			_ = os.Remove(texPath)
			_ = os.RemoveAll(filepath.Join(node.SubmitRoot, base))
			next := node
			next.Attempt = node.Attempt + 1
			// 新号在这里显式给：只有"checker 跑满轮次仍不过"才是新尝试，
			// 进程被杀/断连的重跑一律沿用磁盘上最近的那一次。
			next.AttemptNum = node.AttemptNum + 1
			next.TranscriptPath = transcriptBase
			return r.convertChapterNode(proj, clsName, manualPath, chapPath, workDir, idx, tid, true, next)
		}
		// 次数用尽：**判失败**，不再"记一笔就放过"（旧行为让一个 checker 从
		// 未给出结论的章节也进了书）。
		return fmt.Errorf("%s 共 %d 次尝试仍未通过 checker: %s", base, node.Attempt, issues)
	}
	return nil
}

// attemptTranscriptPath 给「第 N 次尝试」一条独立转录：第一次沿用原名，
// 之后是 `<名>.try2.jsonl`、`<名>.try3.jsonl`…… 这样几次尝试的完整对话都
// 留在磁盘上（预览页的「历史 N」就是按这个后缀聚合的），而不是像以前那样
// 换会话时把上一份转录覆盖掉——被覆盖的那份恰好是"它当时为什么跑偏"的证据。
func attemptTranscriptPath(path string, attempt int) string {
	if attempt <= 1 || path == "" {
		return path
	}
	ext := filepath.Ext(path)
	stem := trimAttemptSuffix(strings.TrimSuffix(path, ext))
	return fmt.Sprintf("%s.try%d%s", stem, attempt, ext)
}

// attemptNumberOnDisk 查这一章在磁盘上已有的最大尝试号，并告诉最后一次是否
// 已经正常收尾（转录里有 run_end）。0 表示还没有任何尝试。收尾与否只用于诊断
// 与测试：重跑一律沿用最大号（T174），新号只由换会话那一步显式分配。
// lastAttemptNumber 决定这一次往哪条转录里写：**中断不算新尝试**。
//
// 用户口径（T174）：只有 checker 连续打回 `latex.checker_rounds` 轮仍不过、
// 代码主动作废产物换全新会话时，才分配新号（那一步显式写 `next.AttemptNum`）。
// 进程被杀、网络断连、窗口关掉之后重跑，都是"同一轮没做完"，接着磁盘上最近
// 那条转录继续——否则每按一次 Ctrl-C 就多出一条空尝试，历史里全是"第 2 次"
// "第 3 次"，而真正干活的第 1 次永远停在半路。
func lastAttemptNumber(transcriptBase string) int {
	if n, _ := attemptNumberOnDisk(transcriptBase); n >= 1 {
		return n
	}
	return 1
}

// archiveChapterAttempt 把一次被作废的尝试整棵挪进
// `<proj>/work/attempts/<章>/try<N>/`（T174）。挪的位置在 `work/` 下而不是
// 章节的 `work/temp/conv_<章>/` 里：后者会在章节收尾时整棵清掉，留档就白留了。
// 内容 = 上一轮的工作区（含 `_subdivision/` 子树）、提交的 `.tex` 与分片、
// `result/`、`integrated-stage/`，外加 checker 给的问题清单。移动失败只警告，
// 绝不因为留档失败而挡住重试。
func (r *Runner) archiveChapterAttempt(proj, base string, node chapterNodeConfig, texPath, issues string) {
	dst := filepath.Join(proj, "work", "attempts", base, fmt.Sprintf("try%d", node.AttemptNum))
	if err := os.MkdirAll(dst, 0o755); err != nil {
		r.log.LogWarning(0, "[convert]", base, "留档目录创建失败（继续重试）:", err)
		return
	}
	move := func(src, name string) {
		if src == "" || !fileExists(src) {
			return
		}
		if err := movePath(src, filepath.Join(dst, name)); err != nil {
			r.log.LogWarning(0, "[convert]", base, "留档失败（继续重试）:", filepath.Base(src), err)
		}
	}
	move(filepath.Join(node.NodeRoot, "work"), "work")
	move(filepath.Join(node.NodeRoot, "result"), "result")
	move(filepath.Join(node.NodeRoot, "integrated-stage"), "integrated-stage")
	move(texPath, base+".tex")
	move(filepath.Join(node.SubmitRoot, base), base)
	if issues != "" {
		_ = os.WriteFile(filepath.Join(dst, "checker-issues.md"), []byte(issues), 0o644)
	}
}

// movePath 把 src 整个挪到 dst（同盘 rename；跨盘/失败退回复制后删除）。
func movePath(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return err
		}
	} else if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func attemptNumberOnDisk(path string) (int, bool) {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	stem := trimAttemptSuffix(strings.TrimSuffix(filepath.Base(path), ext))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	maxNum, maxName := 0, ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ext) {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		var n int
		switch {
		case base == stem:
			n = 1
		case strings.HasPrefix(base, stem+".try"):
			if v, err := strconv.Atoi(strings.TrimPrefix(base, stem+".try")); err == nil {
				n = v
			}
		}
		if n > maxNum || (n == maxNum && name > maxName) {
			maxNum, maxName = n, name
		}
	}
	if maxNum == 0 {
		return 0, false
	}
	return maxNum, transcriptEnded(filepath.Join(dir, maxName))
}

// transcriptEnded 看转录末尾有没有 run_end（会话层每次 Run 结束都会写）。
// 它区分"这一段对话是否已经收尾"，用于诊断；尝试号的分配见 lastAttemptNumber。
func transcriptEnded(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	const tail = 64 << 10
	st, err := f.Stat()
	if err != nil {
		return false
	}
	off := st.Size() - tail
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err.Error() != "EOF" {
		return false
	}
	return strings.Contains(string(buf), `"kind":"run_end"`)
}

// trimAttemptSuffix 去掉名字末尾的 `.tryN`（可能要剥好几层）——这是写入方向
// 的保险：派生新文件名前先剥干净旧后缀，就不会再产出 `.try2.try3` 这种套娃名。
func trimAttemptSuffix(stem string) string {
	for {
		i := strings.LastIndex(stem, ".try")
		if i < 0 || i == 0 {
			return stem
		}
		digits := stem[i+len(".try"):]
		if digits == "" {
			return stem
		}
		ok := true
		for _, r := range digits {
			if r < '0' || r > '9' {
				ok = false
				break
			}
		}
		if !ok {
			return stem
		}
		stem = stem[:i]
	}
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
		if view := r.projectView(); view != "" {
			mounts = append(mounts, Mount{Name: "project", Dir: view})
		}
	}
	mounts = append(mounts, r.pdfView.Mounts()...)
	return mounts
}

func (r *Runner) projectView() string {
	r.cfgMu.Lock()
	defer r.cfgMu.Unlock()
	if r.projView == "" && r.projDir != "" {
		r.projView = ensureProjectView(r.projDir)
	}
	return r.projView
}

// projectRoot is the directory a session reads as "the project": the
// narrow view when available, otherwise the project root (degraded).
func (r *Runner) projectRoot() string {
	if view := r.projectView(); view != "" {
		return view
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
	return r.chapterWorkTreeAt(proj, clsName, base, filepath.Join(proj, "work", "temp", "conv_"+base), resume, seed)
}

func (r *Runner) chapterWorkTreeAt(proj, clsName, base, root string, resume bool, seed string) (string, func(), error) {
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
	defer func() { cleanWork() }()
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
// checkerAttemptBudget 是「第 depth 层」一个节点可以尝试几次：顶层（depth 0）
// 默认 6，往下每层 -1，但**不低于 3**——浅层内容少、一两次就收敛；深层也不
// 能低于 3，否则短子章节一次误判就整支失败。基数可由 `latex.checker_rounds`
// 调整（未写 = 6）。
func checkerAttemptBudget(base, depth int) int {
	if base <= 0 {
		base = defaultCheckerRounds
	}
	n := base - depth
	if n < 3 {
		n = 3
	}
	return n
}

// defaultCheckerRounds 是顶层节点的默认尝试次数。
const defaultCheckerRounds = 6

func (r *Runner) checkerAttempts(depth int) int {
	return checkerAttemptBudget(r.cfg.Latex.CheckerRounds, depth)
}

// checkerBudgetWarning 在「顶层次数 - 最大层数 < 3」时给出启动告警：顶层的
// 次数要和递归层数匹配，否则深层永远只吃下限 3，调大顶层等于白调。
func checkerBudgetWarning(base int) string {
	if base <= 0 {
		return ""
	}
	if base-maxChapterSplitDepth >= 3 {
		return ""
	}
	return fmt.Sprintf("latex.checker_rounds=%d 与最大递归层数 %d 不匹配（%d-%d=%d < 3）：第 %d 层只能吃到下限 3 次。建议至少设成 %d（= 3 + 最大递归层数）。",
		base, maxChapterSplitDepth, base, maxChapterSplitDepth, base-maxChapterSplitDepth, maxChapterSplitDepth, 3+maxChapterSplitDepth)
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

// remindSubmit 统一"必须显式提交"语义（用户口径，T94 扩展到全部会话）：
// 会话提前收尾没提交 → 提醒续跑最多两次 → 仍未提交返回 false（调用方判失败）。
// 提醒轮在同一会话里续跑——前面写好的文件都还在，补上 submit 即可。
func (r *Runner) remindSubmit(sess *session.Session, tid int, tag, label, submitTool string, submitted func() bool) bool {
	for remind := 0; remind < 2 && !submitted(); remind++ {
		r.log.LogWarning(tid, tag, label, "会话结束但未提交，发送提交提醒")
		release := r.acquireChapterSlot()
		_, runErr := sess.Run(session.RunOptions{UserText: "You have NOT submitted yet — the result only counts after an explicit " + submitTool + " call. Call " + submitTool + " now (fix and compile first if something is still broken)."})
		release()
		if runErr != nil {
			r.log.LogWarning(tid, tag, label, "提交提醒轮失败:", runErr)
			return false
		}
	}
	return submitted()
}
