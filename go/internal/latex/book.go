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

	"mineru-tools/internal/session"
)

// BookOptions drives the level-1 full-book pipeline.
type BookOptions struct {
	// Step limits the run to one phase: "" (all missing), "style",
	// "chapters", "convert", "assemble".
	Step string
	// SourceDir overrides paths.output_dir.
	SourceDir string
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
	proj := cfg.Paths.LatexProject
	for _, d := range []string{
		proj, filepath.Join(proj, "source"), filepath.Join(proj, "style"),
		filepath.Join(proj, "chapters"), filepath.Join(proj, "work"),
		filepath.Join(proj, "build"), filepath.Join(proj, "out"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
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
			fmt.Fprintf(os.Stdout, format+"\n", a...)
		}
	}

	runPhase := func(name string, fn func() error) error {
		if prog2Field(prog, name) == "done" && !opts.Restart && opts.Step == "" {
			note("[book] phase %s already done, skipping", name)
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

	// Phase: images (level-2 pass on the source markdown).
	err := runPhase("images", func() error {
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
	err = runPhase("style", func() error { return r.stylePhase(proj) })
	if err != nil {
		return err
	}
	err = runPhase("chapters", func() error { return r.chaptersPhase(proj) })
	if err != nil {
		return err
	}
	// 原始文档索引：把 MinerU 中间产物（content_list/layout/origin.pdf）
	// 加工为只读检索索引，转换会话可 doc_search 定位片段对应的原 PDF 页。
	r.buildDocIndexQuiet(proj, opts.SourceDir, opts.Files)

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
	modelCfg := r.models[r.cfg.Latex.StyleModel]
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
	tools := []session.Tool{
		&WriteWorkFileTool{Root: workDir},
		&ListImagesTool{ImagesDir: filepath.Join(sourceDir, "images")},
		&ViewImageTool{Root: sourceDir, Subject: "images"},
		&ReadMDTool{Path: mainMD},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		&InstallFontTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}

	// Original scanned pages: build the page index over the
	// MinerU-preserved origin PDFs. Pages render ON DEMAND when the
	// analyst calls view_page (cached afterwards).
	pagesDir := filepath.Join(proj, "pages")
	pageIdx, pageErr := buildPageIndex(r.cfg.Paths.MineruOutput, subjectOf(filepath.Base(mainMD)))
	if pageErr != nil {
		r.log.LogWarning(1, "[style] 未找到 MinerU 保留的原始 PDF（mineru_output/<主题>_part*/*_origin.pdf），将仅基于提取图片与 md 分析样式")
	} else {
		r.log.Log(1, "[style] 原始页面索引就绪:", strconv.Itoa(pageIdx.total), "页（view_page 按需渲染）")
		tools = append(tools, &ListPagesTool{Idx: pageIdx}, &ViewPageTool{Idx: pageIdx, PagesDir: pagesDir, Runner: r})
	}

	prompt := styleSystemPrompt + "\n\nYou have a persistent WORKSPACE: write_file stores class.cls / manual.md / example.tex as real files; submit_style can then reference them by file name instead of full inline contents. Check list_fonts before referencing fonts; install_font can add missing font files (record substitutions in the manual when a font cannot be provided)."
	if r.cfg.Latex.RemoveWatermark {
		prompt += "\n\n" + r.watermarkGuidance("WATERMARK: the source may carry watermark artifacts (repeated decorative overlay text such as institution/library marks, faint background strings). Identify the watermark pattern in the manual and instruct conversion to EXCLUDE it entirely - watermark text/graphics must NOT be typeset in the LaTeX output.")
	}
	if pageIdx != nil {
		prompt += "\n\nIMPORTANT: this document HAS original page renders (list_pages -> p001.png...). They show the TRUE typography and layout — inspect them FIRST (chapter title pages, section headings, body text, headers/footers) before looking at extracted images."
	}
	sess := session.NewSession(client, modelCfg, tuning, prompt, tools, r.log, 1, "style")
	// 会话上下文实时持久化：样式反馈回路直接复用这个上下文打回
	//（不开新会话，避免丢失信息）。成功/失败路径都会保存最新状态。
	ctxPath := filepath.Join(proj, "work", "style_session.json")
	defer func() {
		if err := saveSessionContext(sess, ctxPath); err != nil {
			r.log.LogWarning(1, "[style] 会话上下文保存失败:", err)
		}
	}()

	initial := strings.Join([]string{
		"Analyse the style of this book and produce the LaTeX class package.",
		"",
		"- Organized markdown (high-quality text): " + filepath.Base(mainMD),
		"- Extracted images live under images/ (use list_images + view_image).",
	}, "\n")
	if pageIdx != nil {
		initial += fmt.Sprintf("\n- ORIGINAL pages (%d total) are available via list_pages + view_page — use them for typography/layout (rendered on demand).", pageIdx.total)
	}
	initial += "\nStart by mapping the structure (list_pages / list_images / read_md), inspect representative pages (crop/zoom title pages, headings, figures), then submit_style."

	scratch, err := os.MkdirTemp("", "dsv-style-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	var lastExampleCompile string
	for attempt := 0; attempt <= r.cfg.Latex.Compile.MaxFixRounds; attempt++ {
		userText := initial
		if attempt > 0 {
			userText = "The example failed to compile:\n" + lastExampleCompile +
				"\n\nFix the cls/example and call submit_style again with the corrected package."
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("样式会话失败: %w", err)
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

		os.RemoveAll(scratch)
		scratch, _ = os.MkdirTemp("", "dsv-style-")
		copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls"))
		exFile := filepath.Join(scratch, "example.tex")
		copyFile(filepath.Join(styleDir, "example.tex"), exFile)
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

	// Sandbox with ONLY the source file, as book.md.
	sandbox, err := os.MkdirTemp("", "dsv-chap-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(sandbox)
	if err := copyFile(mainMD, filepath.Join(sandbox, "book.md")); err != nil {
		return err
	}

	client := r.clientFor(r.cfg.Latex.ChapterModel)
	modelCfg := r.models[r.cfg.Latex.ChapterModel]
	tuning := r.cfg.LatexSession("chapter")
	submit := &SubmitSplitTool{}
	sess := session.NewSession(client, modelCfg, tuning, chapterSystemPrompt, []session.Tool{
		&GrepMDTool{Path: mainMD},
		&ReadLinesTool{Path: mainMD},
		&SandboxBashTool{Dir: sandbox},
		submit,
	}, r.log, 1, "chapters")

	granularity := r.cfg.Latex.ChapterGranularity
	if granularity == "" {
		granularity = "small"
	}
	gran := "SMALL granularity (default): one chapter = one SECTION. Split at the finest heading level that yields coherent, self-contained units (a top-level chapter containing several sections becomes several files). Never split mid-section."
	if granularity == "large" {
		gran = "LARGE granularity: one chapter = one TOP-LEVEL chapter of the book. Never split a top-level chapter into pieces; if a chapter is huge, it stays one file (the converter handles it)."
	}
	initial := fmt.Sprintf(
		"Split the markdown into chapter files.\nThe file has %d lines total. %s\nMap the heading structure with grep, verify boundaries, then submit_split.",
		totalLines, gran)

	for attempt := 0; attempt < 3; attempt++ {
		userText := initial
		if attempt > 0 {
			userText = "Your split was rejected: " + r.lastSplitError + "\nFix the ranges and call submit_split again."
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("章节划分会话失败: %w", err)
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
// buildDocIndexQuiet compiles the read-only original-document index
// from the MinerU intermediate output. Failures are non-fatal: the
// convert sessions simply run without doc_search/view_page.
func (r *Runner) buildDocIndexQuiet(proj, sourceDir string, files []string) {
	mds := files
	if len(mds) == 0 && sourceDir != "" {
		matches, _ := filepath.Glob(filepath.Join(sourceDir, "*.md"))
		mds = matches
	}
	if len(mds) == 0 || r.cfg.Paths.MineruOutput == "" {
		return
	}
	outPath := filepath.Join(proj, "doc_index", "doc_index.json")
	idx, pageIdx, err := buildDocIndex(r.cfg.Paths.MineruOutput, mds, outPath)
	if err != nil {
		r.log.Log(0, "[docindex] 原始文档索引不可用（doc_search/view_page 关闭）:", err)
		return
	}
	r.docIndex = idx
	r.docPages = pageIdx
	r.log.Log(0, "[docindex] 原始文档索引就绪:", strconv.Itoa(len(idx.Entries)), "个块 /", strconv.Itoa(len(idx.Parts)), "个 part ->", outPath)
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
	var done, failed int
	var progMu sync.Mutex
	total := len(chapters)
	progress := func() {
		if verbose || total == 0 {
			return
		}
		pct := float64(done) * 100.0 / float64(total)
		fmt.Fprintf(os.Stdout, "\r%s %d/%d] %.2f%% (done: %d, errors: %d)          ",
			label, done, total, pct, done-failed, failed)
	}

	for idx, chapPath := range chapters {
		wg.Add(1)
		tid := <-tidPool
		if verbose {
			r.log.Log(0, label, "start", filepath.Base(chapPath),
				fmt.Sprintf("(%d/%d)", idx+1, total))
		}
		go func(i int, chap string, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			err := r.convertOneChapter(proj, clsName, manualPath, chap, workDir, i, tid)
			progMu.Lock()
			done++
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
	if !verbose && total > 0 {
		fmt.Fprintln(os.Stdout)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d 个章节转换失败: %s", len(failures), strings.Join(failures, "; "))
	}
	r.log.Log(0, label, "全部", strconv.Itoa(len(chapters)), "章转换完成")
	// 样式反馈回路：多数章节汇报 cls/手册问题 → 打回原样式会话修正
	// 后，用全新上下文重新并发转换。
	return r.styleFeedbackLoop(proj, fbRound)
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

func (r *Runner) convertOneChapter(proj, clsName, manualPath, chapPath, workDir string, idx, tid int) error {
	base := strings.TrimSuffix(filepath.Base(chapPath), filepath.Ext(chapPath))
	texRel := "chapters/" + base + ".tex"
	texPath := filepath.Join(workDir, texRel)
	if fileExists(texPath) {
		r.log.Log(tid, "[convert]", base, "已有产物，跳过")
		return nil
	}

	client := r.clientFor(r.cfg.Latex.ConvertModel)
	modelCfg := r.models[r.cfg.Latex.ConvertModel]
	tuning := r.cfg.LatexSession("convert")
	manual, _ := os.ReadFile(manualPath)

	// Scratch for per-chapter compile checks.
	scratch, err := os.MkdirTemp("", "dsv-conv-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	copyFile(filepath.Join(proj, "style", clsName+".cls"), filepath.Join(scratch, clsName+".cls"))
	wrapper := "\\documentclass{" + clsName + "}\n" +
		"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
		"\\graphicspath{{figures/}}\n" +
		"\\begin{document}\n\\input{" + base + ".tex}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(scratch, base+"_wrapper.tex"), []byte(wrapper), 0o644); err != nil {
		return err
	}

	write := &WriteFileTool{Root: workDir, AllowedRel: texRel}
	submit := &SubmitDoneTool{
		Label:      "chapter " + base,
		ReportPath: filepath.Join(workDir, "reports", base+".md"), // 工作汇报（实时落盘）
	}
	tools := []session.Tool{
		&ReadFileTool{Root: proj},
		write,
		&CompileChapterTool{Comp: r.comp, Scratch: scratch, MainFile: base + ".tex", SourcePath: texPath, Log: r.log, Tid: 1},
		submit,
	}
	if r.docIndex != nil && r.docPages != nil {
		// 原始文档只读工具：片段→原 PDF 页定位（doc_search），
		// 页面渲染检视复用 style 阶段的 view_page （全局页号 + 缓存）。
		tools = append(tools,
			&DocSearchTool{Index: r.docIndex},
			&ViewPageTool{Idx: r.docPages, PagesDir: filepath.Join(proj, "pages"), Runner: r})
	}
	sess := session.NewSession(client, modelCfg, tuning,
		strings.ReplaceAll(convertSystemPrompt, "{MAX_ROUNDS}", strconv.Itoa(session.EffectiveToolRounds(tuning))),
		tools, r.log, tid, "convert:"+base)

	// Pre-place the compiled wrapper input target: the wrapper inputs
	// base.tex, so the scratch MainFile is base.tex (copied by the tool).
	chapData, err := os.ReadFile(chapPath)
	if err != nil {
		return err
	}
	initial := strings.Join([]string{
		"Convert the chapter file chapters/" + base + ".md to LaTeX.",
		"",
		"## The usage manual (authoritative):",
		"```",
		truncateStr(string(manual), 24000),
		"```",
		"",
		"Read the chapter via read_file, then write_file your .tex and compile until clean, then submit.",
		"Chapter markdown preview (first 2000 chars):",
		truncateStr(string(chapData), 2000),
	}, "\n")

	if r.cfg.Latex.RemoveWatermark {
		initial += "\n\n" + r.watermarkGuidance("WATERMARK: exclude watermark artifacts from the .tex output (repeated decorative overlay text such as institution marks, faint background strings). Skip such content entirely - do not typeset it.")
	}

	if _, err := sess.Run(session.RunOptions{UserText: initial}); err != nil {
		return fmt.Errorf("会话失败: %w", err)
	}
	// Checker pass: a small text model verifies the chapter output
	// against the chapter markdown. On issues the converter gets one
	// feedback round; repeated failure is accepted with a warning.
	if fileExists(texPath) {
		if ok, issues := r.checkChapter(base, chapPath, texPath); !ok {
			fixed := false
			if submit.Submitted {
				fmt2 := "Your submitted chapter was reviewed and has issues:\n" + issues +
					"\n\nFix the .tex (write_file + compile) and submit again."
				if _, err := sess.Run(session.RunOptions{UserText: fmt2}); err == nil {
					if ok2, _ := r.checkChapter(base, chapPath, texPath); ok2 {
						fixed = true
					}
				}
			}
			if !fixed {
				r.log.LogWarning(tid, "[checker]", base, "核对仍有问题（已记录，供终审处理）:", issues)
				_ = os.WriteFile(texPath+".checker", []byte(issues), 0o644)
			} else {
				r.log.Log(tid, "[checker]", base, "复核通过")
			}
		} else {
			r.log.Log(tid, "[checker]", base, "通过")
		}
	}
	if !submit.Submitted {
		// Accept a written file that compiles clean even without an
		// explicit submit (rounds may have run out).
		if !fileExists(texPath) {
			return fmt.Errorf("会话未提交且未写出 .tex")
		}
		data, _ := os.ReadFile(texPath)
		_ = os.WriteFile(filepath.Join(scratch, base+".tex"), data, 0o644)
		start := time.Now()
		res := r.comp.Compile(scratch, base+"_wrapper.tex")
		LogCompileResult(r.log, tid, "convert-check", res, time.Since(start))
		if !res.OK {
			return fmt.Errorf("会话未提交且编译失败: %s", res.Err)
		}
		r.log.LogWarning(tid, "[convert]", base, "未显式提交，但编译通过，予以采纳")
		return nil
	}
	if !fileExists(texPath) {
		return fmt.Errorf("会话已提交但没有写出 .tex")
	}
	return nil
}

// ------------------------------------------------------------------
// phase: style feedback loop (cls/手册 打回)
// ------------------------------------------------------------------

// maxStyleFeedbackRounds caps how many times conversion results can be
// sent back to the original style session.
const maxStyleFeedbackRounds = 2

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

	ctxPath := filepath.Join(proj, "work", "style_session.json")
	msgs, err := loadSessionContext(ctxPath)
	if err != nil {
		r.log.LogWarning(0, "[style-feedback] 样式会话上下文不可用，跳过打回:", err)
		return nil
	}
	sourceDir := filepath.Join(proj, "source")
	mainMD := mainSourceMD(sourceDir)
	styleDir := filepath.Join(proj, "style")
	workDir := filepath.Join(proj, "work", "style")

	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.models[r.cfg.Latex.StyleModel]
	tuning := r.cfg.LatexSession("style")

	// 工具集与原样式会话一致（历史消息中引用过这些工具名）。
	submit := &SubmitStyleTool{Workspace: workDir}
	tools := []session.Tool{
		&WriteWorkFileTool{Root: workDir},
		&ListImagesTool{ImagesDir: filepath.Join(sourceDir, "images")},
		&ViewImageTool{Root: sourceDir, Subject: "images"},
		&ReadMDTool{Path: mainMD},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		&InstallFontTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}
	if pageIdx, perr := buildPageIndex(r.cfg.Paths.MineruOutput, subjectOf(filepath.Base(mainMD))); perr == nil {
		tools = append(tools,
			&ListPagesTool{Idx: pageIdx},
			&ViewPageTool{Idx: pageIdx, PagesDir: filepath.Join(proj, "pages"), Runner: r})
	}

	// 复用原样式会话：系统提示已在持久化消息里，不开新上下文。
	sess := session.NewSession(client, modelCfg, tuning, "", tools, r.log, 1, "style-feedback")
	sess.SetMessages(msgs)

	feedback := "The conversion phase finished: the MAJORITY of chapter conversion agents reported that the class/manual did NOT satisfy the book's real formatting." +
		" Their work reports follow (固定格式，结论: 存在问题 = issues):" + b.String() +
		"\n\nRe-inspect the relevant original pages (view_page), fix the cls/manual/example so these problems cannot recur, then submit_style with the corrected package."
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

	scratch, err := os.MkdirTemp("", "dsv-stylefb-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	if err := copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex")); err != nil {
		return err
	}
	start := time.Now()
	res := r.comp.Compile(scratch, "example.tex")
	LogCompileResult(r.log, 1, "style-feedback", res, time.Since(start))
	if !res.OK {
		return fmt.Errorf("样式反馈 example 编译失败: %s", res.Err)
	}
	r.log.Log(1, "[style-feedback] 更新后的 example 编译通过:", clsName+".cls")

	// 章节产物全部作废（转换必须用全新上下文，不复用旧会话）。
	chapWork := filepath.Join(proj, "work", "chapters")
	if matches, gerr := filepath.Glob(filepath.Join(chapWork, "*")); gerr == nil {
		for _, m := range matches {
			_ = os.RemoveAll(m)
		}
	}
	_ = os.RemoveAll(reportsDir)
	r.log.Log(0, "[style-feedback] 样式包已更新，丢弃全部章节 .tex，使用全新会话重新并发转换")
	return r.convertPhase(proj, round+1)
}

// saveSessionContext persists the full conversation of a session.
func saveSessionContext(sess *session.Session, path string) error {
	msgs := sess.Messages()
	if len(msgs) == 0 {
		return fmt.Errorf("空会话")
	}
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// loadSessionContext restores a persisted conversation.
func loadSessionContext(path string) ([]session.ChatMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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
