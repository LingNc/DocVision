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
	runPhase := func(name string, fn func() error) error {
		if prog2Field(prog, name) == "done" && !opts.Restart && opts.Step == "" {
			r.log.Log(0, "[book] phase", name, "already done, skipping")
			return nil
		}
		if opts.Step != "" && opts.Step != name {
			return nil
		}
		r.log.Log(0, "[book] === phase:", name, "===")
		if err := fn(); err != nil {
			return fmt.Errorf("phase %s: %w", name, err)
		}
		setProgField(&prog, name, "done")
		saveProg()
		return nil
	}

	// Phase: images (level-2 pass on the source markdown).
	err := runPhase("images", func() error {
		return r.RunImages(ImagesOptions{
			TestMode: opts.TestMode, Number: opts.Number, Seed: opts.Seed,
			SourceDir: opts.SourceDir, Files: opts.Files,
			OutDir: filepath.Join(proj, "source"),
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
	err = runPhase("convert", func() error { return r.convertPhase(proj) })
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

	client := r.clientFor(r.cfg.Latex.StyleModel)
	modelCfg := r.models[r.cfg.Latex.StyleModel]
	tuning := r.cfg.LatexSession("style")
	tuning.MaxTokens = 32768 // cls+manual+example are big

	comp := r.comp
	if err := comp.Available(); err != nil {
		return fmt.Errorf("样式阶段需要 LaTeX 工具链: %w", err)
	}

	submit := &SubmitStyleTool{}
	tools := []session.Tool{
		&ListImagesTool{ImagesDir: filepath.Join(sourceDir, "images")},
		&ViewImageTool{Root: sourceDir},
		&ReadMDTool{Path: mainMD},
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

	prompt := styleSystemPrompt
	if pageIdx != nil {
		prompt += "\n\nIMPORTANT: this document HAS original page renders (list_pages -> p001.png...). They show the TRUE typography and layout — inspect them FIRST (chapter title pages, section headings, body text, headers/footers) before looking at extracted images."
	}
	sess := session.NewSession(client, modelCfg, tuning, prompt, tools, r.log, 1, "style")

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
		res := comp.Compile(scratch, "example.tex")
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

	initial := fmt.Sprintf(
		"Split the markdown into chapter files.\nThe file has %d lines total. Map the heading structure with grep, verify boundaries, then submit_split.",
		totalLines)

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
// phase: convert (concurrent per-chapter sessions)
// ------------------------------------------------------------------

func (r *Runner) convertPhase(proj string) error {
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

	for idx, chapPath := range chapters {
		wg.Add(1)
		tid := <-tidPool
		go func(i int, chap string, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			if err := r.convertOneChapter(proj, clsName, manualPath, chap, workDir, i, tid); err != nil {
				r.log.LogError(tid, "[convert] 章节失败:", filepath.Base(chap), err)
				failsMu.Lock()
				failures = append(failures, filepath.Base(chap)+": "+err.Error())
				failsMu.Unlock()
			}
		}(idx, chapPath, tid)
	}
	wg.Wait()
	if len(failures) > 0 {
		return fmt.Errorf("%d 个章节转换失败: %s", len(failures), strings.Join(failures, "; "))
	}
	r.log.Log(0, "[convert] 全部", strconv.Itoa(len(chapters)), "章转换完成")
	return nil
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
	submit := &SubmitDoneTool{Label: "chapter " + base}
	sess := session.NewSession(client, modelCfg, tuning,
		strings.ReplaceAll(convertSystemPrompt, "{MAX_ROUNDS}", strconv.Itoa(session.EffectiveToolRounds(tuning))),
		[]session.Tool{
			&ReadFileTool{Root: proj},
			write,
			&CompileChapterTool{Comp: r.comp, Scratch: scratch, MainFile: base + ".tex", SourcePath: texPath},
			submit,
		}, r.log, tid, "convert:"+base)

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

	if _, err := sess.Run(session.RunOptions{UserText: initial}); err != nil {
		return fmt.Errorf("会话失败: %w", err)
	}
	if !submit.Submitted {
		// Accept a written file that compiles clean even without an
		// explicit submit (rounds may have run out).
		if !fileExists(texPath) {
			return fmt.Errorf("会话未提交且未写出 .tex")
		}
		data, _ := os.ReadFile(texPath)
		_ = os.WriteFile(filepath.Join(scratch, base+".tex"), data, 0o644)
		res := r.comp.Compile(scratch, base+"_wrapper.tex")
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
