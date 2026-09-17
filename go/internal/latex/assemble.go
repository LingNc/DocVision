package latex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// assemblePhase builds the full book: clean build dir, main.tex + cls
// + figures + chapters, compiles (2 passes for refs), spawns a fix
// session on failures, then copies the final artefacts to out/ and
// writes the single-file standalone.tex.
func (r *Runner) assemblePhase(proj string) error {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	if clsName == "" {
		return fmt.Errorf("未找到样式 cls")
	}
	buildDir := filepath.Join(proj, "build")
	outDir := filepath.Join(proj, "out")

	// Clean build dir.
	os.RemoveAll(buildDir)
	if err := os.MkdirAll(filepath.Join(buildDir, "chapters"), 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(proj, "style", clsName+".cls"), filepath.Join(buildDir, clsName+".cls")); err != nil {
		return err
	}
	// 整个样式包（cls/sty/manual/example）作为构建树的基础：终审会话
	// 需要它来整理全书、核对章节用法。
	if err := copyDir(filepath.Join(proj, "style"), buildDir); err != nil {
		return err
	}
	// Figures and raster assets from the level-2 source pass.
	//
	// 先自愈：上一次运行可能在项目 source 树里留下了一批断链（相对目标
	// 的软链），而 filepath.Walk 会走进断链并让 copyFile 以
	// "open …/source/images/<书>/<sha>.jpg: no such file or directory" 整段
	// 中止——一条取不到的图不该毁掉跑了一刻钟的 assemble。
	if n := r.repairProjectImageLinks(proj); n > 0 {
		r.log.Log(0, "[images] assemble 前修复断链:", strconv.Itoa(n), "个")
	}
	for _, asset := range []string{"figures", "images"} {
		src := filepath.Join(proj, "source", asset)
		if fileExists(src) {
			skipped, err := copyDirReport(src, filepath.Join(buildDir, asset))
			if err != nil {
				return err
			}
			for _, sk := range skipped {
				r.log.LogWarning(0, "[assemble] 跳过取不到的插图（不中止）:", sk)
			}
		}
	}
	// Chapter .tex files: the WHOLE chapters tree is copied so a chapter
	// that submitted extra files (chapters/<base>/…) keeps its relative
	// structure. Only top-level .tex files become \input targets.
	texs, _ := filepath.Glob(filepath.Join(proj, "work", "chapters", "*.tex"))
	if len(texs) == 0 {
		return fmt.Errorf("没有已转换的章节 .tex")
	}
	sort.Strings(texs)
	if err := copyDir(filepath.Join(proj, "work", "chapters"), filepath.Join(buildDir, "chapters")); err != nil {
		return err
	}

	// T44：assemble 资源盘点——编译前列出 build 树就位了什么、缺什么：
	// cls/figures/images/章节的文件数，以及每章 .tex 里的 \includegraphics
	// 引用是否都能在 build 树解析到（缺失逐条告警，不中止——缺图不该毁掉
	// 跑了一刻钟的 assemble，但要让人一眼看出结构对不对）。
	r.preflightAssemble(buildDir, clsName)

	// main.tex.
	var in strings.Builder
	for _, f := range texs {
		fmt.Fprintf(&in, "\\input{chapters/%s}\n", filepath.Base(f))
	}
	// 续跑短路（T39）：上次终审成功时已把终审产物（修订后的章节 +
	// main.tex/frontmatter 等顶层文件）回写 work/final_review/ 与
	// work/chapters/。若这些文件自上次终审后没变（convert 没重跑），
	// 重建出的树与上次终审后的一致，直接沿用持久化状态、跳过终审会话。
	frCurrent := finalReviewStateCurrent(proj)

	mainTex := "\\documentclass{" + clsName + "}\n" +
		"\\graphicspath{{figures/}}\n" +
		"\\begin{document}\n" + in.String() + "\\end{document}\n"
	if err := os.WriteFile(filepath.Join(buildDir, "main.tex"), []byte(mainTex), 0o644); err != nil {
		return err
	}
	if frCurrent {
		// 用回写的终审版 main.tex/顶层文件覆盖刚生成的骨架。
		if err := applyFinalReviewState(proj, buildDir); err != nil {
			r.log.LogWarning(0, "[final-review] 恢复终审产物失败，改为重跑终审:", err)
			frCurrent = false
		} else {
			r.log.Log(0, "[final-review] 章节未变且上次终审已回写——沿用终审产物，跳过终审会话")
		}
	}

	// T49：成品 PDF 要可导航（书签 + 跳转点）。cls 通常不加载 hyperref
	// （kyexam.cls 实测就没有），骨架与终审版 main.tex 都可能是——编译前
	// 幂等地补上 hyperref 与逐章 \pdfbookmark（章节标题取划分阶段 md 的
	// 首个标题行，缺失退 base 名）。
	if err := ensurePDFBookmarks(proj, buildDir); err != nil {
		r.log.LogWarning(0, "[assemble] PDF 书签注入失败（不影响编译）:", err)
	}

	// Compile (2 passes for TOC/refs).
	start := time.Now()
	// 全书是多文件工程（main.tex + chapters/*.tex + cls + figures）：
	// 有 latexmk 就整轮构建（多遍 + 参考文献），否则退回两遍编译。
	res := r.comp.CompileFull(buildDir, "main.tex")
	LogCompileResult(r.log, 1, "book", res, time.Since(start))
	if !res.OK {
		// 编译失败是正常的工作状态（会话需要编译才能看到报错），
		// 不是阶段失败：交给修复会话在构建树里迭代。修复会话最终
		// 也没能让它编译通过时才告警，产物照样交付。
		if err := r.fixSession(proj, buildDir, res.Err); err != nil {
			r.log.LogWarning(0, "[assemble] 修复会话未能在上限内让全书编译通过:", err)
			r.phaseNote()("[assemble] 修复未完成，交付现有产物")
		}
	}
	// 汇总/终审会话：核对成品 PDF（封面/目录/顺序/页码/图表/版面）、
	// 整理目录结构，并重新构建 —— 它以 submit 作为"全书定稿"的标记。
	if r.cfg.Latex.Compile.FinalReviewEnabled() && !frCurrent && fileExists(filepath.Join(buildDir, "main.pdf")) {
		if err := r.finalReview(proj, buildDir, texs); err != nil {
			r.log.LogWarning(0, "[final-review] 终审会话未通过（保留已编译全书）:", err)
			r.phaseNote()("[final-review] 终审未通过（保留已编译全书）")
		}
	} else if r.cfg.Latex.Compile.FinalReviewEnabled() {
		r.log.LogWarning(0, "[final-review] 没有可用的全书 PDF，跳过终审")
	}

	// 交付：整棵构建树 → out/（去掉编译中间文件），结构以终审会话
	// 实际产出的目录为准（书不同结构可以不同），book.pdf 为成品别名。
	if err := deliverBook(buildDir, outDir); err != nil {
		return err
	}
	pdf := filepath.Join(buildDir, "main.pdf")
	if !fileExists(pdf) {
		return fmt.Errorf("全书编译未产出 PDF（源码树已交付到 %s，可查看 build 日志）", outDir)
	}
	if err := copyFile(pdf, filepath.Join(outDir, "book.pdf")); err != nil {
		return err
	}

	// standalone.tex: single file with all chapters inlined (no compile
	// needed; easy for AI consumption and archival). The file list is
	// re-globbed because the final review session may have reorganised
	// or renamed chapter files.
	if reglob, gerr := filepath.Glob(filepath.Join(buildDir, "chapters", "*.tex")); gerr == nil && len(reglob) > 0 {
		texs = reglob
		sort.Strings(texs)
	}
	standalone, err := buildStandaloneTex(filepath.Join(buildDir, "main.tex"), texs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(proj, "standalone.tex"), []byte(standalone), 0o644); err != nil {
		return err
	}
	_ = copyFile(filepath.Join(proj, "standalone.tex"), filepath.Join(outDir, "standalone.tex"))

	// T42：最终 PDF 同时复制到项目根外侧（<latex_project>/<项目名>.pdf），
	// 不用翻进工作区找成品；失败只告警（out/ 里的交付不受影响）。
	outer := filepath.Join(filepath.Dir(proj), filepath.Base(proj)+".pdf")
	if err := copyFile(filepath.Join(outDir, "book.pdf"), outer); err != nil {
		r.log.LogWarning(0, "[assemble] 复制最终 PDF 到项目根外侧失败:", err)
	} else {
		r.log.Log(0, "[assemble] 最终 PDF 已复制到:", outer)
	}

	r.log.Log(0, "[assemble] 全书编译完成:", filepath.Join(outDir, "book.pdf"))
	r.log.Log(0, "[assemble] 单文件版:", filepath.Join(proj, "standalone.tex"))
	return nil
}

// bookSessionTools is the toolset shared by the build doctor and the
// final review session: a full workspace on the assembled book tree
// (read/write/edit/grep/bash + compile + view_pdf/view_image + fonts)
// with the project (original markdown, chapters, style) readable
// read-only.
func (r *Runner) bookSessionTools(proj, buildDir string, compile *CompileTexTool, submit *SubmitDoneTool) []session.Tool {
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_build")
	defer cleanBashTmp()
	tools := []session.Tool{
		&ReadFileTool{Root: buildDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&WriteWorkFileTool{Root: buildDir, AnyExt: true},
		&EditWorkFileTool{Root: buildDir},
		&GrepTool{Root: buildDir, AltRoots: []AltRoot{{Label: "project", Dir: r.projectRoot()}}},
		&WorkBashTool{Root: buildDir, Mounts: r.sessionMounts(kindBook, buildDir), TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		compile,
		&ViewPDFTool{Mounts: r.sessionMounts(kindBook, buildDir), Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewImageTool{Root: buildDir, BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}
	// 原书页面：核对真实版面/图表来源（原书 PDF 走 view_pdf 的 source 挂载）。
	tools = append(tools, r.sourcePageTools()...)
	return tools
}

// fixSession spawns the build-doctor session. It is a full workspace
// session on the build tree: read/write/edit/grep/bash + compile +
// view_pdf/view_image + fonts, so it can restructure the assembled
// project (multi-file, resources) and verify the PDF itself.
func (r *Runner) fixSession(proj, buildDir, firstErr string) error {
	r.log.LogWarning(0, "[assemble] 全书编译失败，启动修复会话:", firstErr)
	client := r.clientFor(r.cfg.Latex.ConvertModel)
	modelCfg := r.modelOf(r.cfg.Latex.ConvertModel)
	tuning := r.cfg.LatexSession("convert")
	compile := &CompileTexTool{Comp: r.comp, Root: buildDir, MainFile: "main.tex", Tag: "book", Log: r.log, Tid: 1}
	submit := &SubmitDoneTool{Label: "the build fix"}
	tools := r.bookSessionTools(proj, buildDir, compile, submit)
	sess := session.NewSession(client, modelCfg, tuning, renderPrompt(prompts.Must(prompts.FixSystem), tuning, r.outputLang()), tools, r.log, 1, "fix")

	// 转录挂到 work/sessions/（T39）：修复会话在 WebUI 可见、进成本表。
	// 构建树每次 assemble 都重建，上一轮的 edit 无法在新树上续用——
	// 所以不回放历史、计数从头来，只把旧转录归档成 *_prev 保留可查。
	trPath := filepath.Join(proj, "work", "sessions", "book_fix.jsonl")
	archivePrevTranscript(trPath)
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	fixHook, fixClose := r.livePhaseRow("assemble/fix", "fix")
	sess.SetProgressHook(fixHook)
	defer fixClose()

	lastErr := firstErr
	for attempt := 0; attempt < r.cfg.Latex.Compile.MaxFixRounds; attempt++ {
		userText := "The full-book compile failed:\n\n" + truncateStr(lastErr, 8000) +
			"\n\nRead the failing file, apply a minimal edit_file, then compile {path: \"main.tex\"} (engine \"latexmk\" for a full multi-pass build) and check the result with view_pdf."
		if attempt > 0 {
			userText = "Still failing:\n\n" + truncateStr(lastErr, 8000)
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("修复会话失败: %w", err)
		}
		if compile.LastOK {
			return nil
		}
		start := time.Now()
		res := r.comp.CompileFull(buildDir, "main.tex")
		LogCompileResult(r.log, 1, "book-fix", res, time.Since(start))
		if res.OK {
			return nil
		}
		lastErr = res.Err
		r.log.LogWarning(1, "[assemble] 修复第", strconv.Itoa(attempt+1), "轮后仍失败")
	}
	return fmt.Errorf("全书编译在 %d 轮修复内未通过", r.cfg.Latex.Compile.MaxFixRounds)
}

// finalReview runs the book-doctor session on an ALREADY COMPILED book:
// it reads the finished PDF (and the original pages / chapter markdown)
// and does the final consolidation — front matter, TOC, chapter order,
// page numbering, figure placement, layout — then recompiles. Capped by
// latex.compile.max_fix_rounds.
func (r *Runner) finalReview(proj, buildDir string, texs []string) error {
	rounds := r.cfg.Latex.Compile.MaxFixRounds
	if rounds <= 0 {
		rounds = 1
	}
	r.log.Log(0, "[final-review] 启动终审会话（上限", strconv.Itoa(rounds), "轮）")
	compile := &CompileTexTool{Comp: r.comp, Root: buildDir, MainFile: "main.tex", Tag: "final-review", Log: r.log, Tid: 1}
	submit := &SubmitDoneTool{Label: "the final review"}
	sess := session.NewSession(r.clientFor(r.cfg.Latex.ConvertModel), r.modelOf(r.cfg.Latex.ConvertModel),
		r.cfg.LatexSession("convert"), renderPrompt(prompts.Must(prompts.FinalReviewSystem), r.cfg.LatexSession("convert"), r.outputLang()),
		r.bookSessionTools(proj, buildDir, compile, submit), r.log, 1, "final-review")
	// 转录挂到 work/sessions/final_review.jsonl（T39）：终审会话在 WebUI
	// 可见、进成本表。构建树每次 assemble 都重建，上一轮终审的编辑无法
	// 直接续用——成功后会回写 work/（见 persistFinalReviewState），续跑时
	// 章节未变则整段跳过；章节变了才重开一场（旧转录归档成 *_prev）。
	trPath := filepath.Join(proj, "work", "sessions", "final_review.jsonl")
	archivePrevTranscript(trPath)
	frStart := time.Now()
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRowAt("assemble/final-review", "final-review", frStart)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	var chaps strings.Builder
	for _, f := range texs {
		fmt.Fprintf(&chaps, "- chapters/%s\n", filepath.Base(f))
	}
	pages, _ := pdfPageCount(filepath.Join(buildDir, "main.pdf"))
	first := prompts.Render(prompts.FinalReviewUser, map[string]string{
		"CHAPTERS":   chaps.String(),
		"PAGES_LINE": fmt.Sprintf("Compiled book.pdf: %d page(s).", pages),
	})

	lastErr := ""
	for attempt := 0; attempt < rounds; attempt++ {
		userText := first
		if attempt > 0 {
			userText = "The book still does not compile after your last edits:\n\n" + truncateStr(lastErr, 8000) +
				"\n\nFix it (edit_file / bash), compile {path:\"main.tex\", engine:\"latexmk\"}, verify with view_pdf, then submit."
		}
		if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
			return fmt.Errorf("终审会话失败: %w", err)
		}
		start := time.Now()
		res := r.comp.CompileFull(buildDir, "main.tex")
		LogCompileResult(r.log, 1, "final-review", res, time.Since(start))
		if res.OK {
			if !submit.Submitted {
				r.log.LogWarning(0, "[final-review] 会话未显式提交，但全书编译通过，予以采纳")
			}
			// 终审对构建树的整理（章节修订/frontmatter/main.tex 钩子）
			// 回写 work/：assemble 每次重建构建树，不回写的话续跑只能把
			// 几十轮终审从头再烧一遍（T39 的真实事故：进程死在终审完成
			// 后的收尾路上，下一跑全书重审 72→63 轮）。
			if err := persistFinalReviewState(proj, buildDir); err != nil {
				r.log.LogWarning(0, "[final-review] 回写终审产物失败（不影响交付，但续跑会重审）:", err)
			}
			finalPages, _ := pdfPageCount(filepath.Join(buildDir, "main.pdf"))
			r.log.Log(0, "[final-review] 终审完成，全书页数:", strconv.Itoa(finalPages))
			return nil
		}
		lastErr = res.Err
	}
	return fmt.Errorf("终审 %d 轮后全书仍编译失败: %s", rounds, lastErr)
}

// deliverBook copies the WHOLE build tree into out/ so the delivered
// structure is exactly what the sessions produced (extra front-matter
// folders, renamed or split chapters, added resources) — not a fixed
// file list. LaTeX intermediate files are skipped.
func deliverBook(buildDir, outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	// RemoveAll 连目录本身一起删掉，必须立即重建：Walk 只在"遇到目录项"
	// 时才 MkdirAll，而顶层文件按字典序排在目录前面时（REPORT.md 大写 R
	// < chapters 小写 c），copyFile 会写进一个不存在的 out/ 直接 ENOENT——
	// 整本书在最后一步交付时中止（T33）。
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	skipExt := map[string]bool{
		".aux": true, ".log": true, ".out": true, ".toc": true, ".fls": true,
		".fdb_latexmk": true, ".bbl": true, ".blg": true, ".nav": true,
		".snm": true, ".vrb": true, ".idx": true, ".ilg": true, ".ind": true,
		".synctex": true, ".lof": true, ".lot": true,
	}
	skipName := func(name string) bool {
		low := strings.ToLower(name)
		return strings.HasSuffix(low, ".synctex.gz")
	}
	return filepath.Walk(buildDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(buildDir, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(outDir, rel), 0o755)
		}
		if skipExt[strings.ToLower(filepath.Ext(p))] || skipName(info.Name()) {
			return nil
		}
		return copyFile(p, filepath.Join(outDir, rel))
	})
}

// buildStandaloneTex inlines every \input{chapters/...} of main.tex.
func buildStandaloneTex(mainTexPath string, texs []string) (string, error) {
	data, err := os.ReadFile(mainTexPath)
	if err != nil {
		return "", err
	}
	main := string(data)
	bodies := map[string]string{}
	for _, f := range texs {
		d, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		bodies[filepath.Base(f)] = string(d)
	}
	re := regexpInputChapters()
	out := re.ReplaceAllStringFunc(main, func(m string) string {
		sub := re.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if body, ok := bodies[sub[1]]; ok {
			return "% ==== " + sub[1] + " ====\n" + body
		}
		return m
	})
	header := "% standalone.tex — merged single-file version of the book project\n" +
		"% Generated by docvision latex; compile this file directly if needed.\n"
	return header + out, nil
}

// regexpInputChapters matches \input{chapters/xxx.tex} references.
func regexpInputChapters() *regexp.Regexp {
	return regexp.MustCompile(`\\input\{chapters/([^}]+)\}`)
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	_, err := copyDirReport(src, dst)
	return err
}

// copyDirReport 是 copyDir 的"别死在一张图上"版本：断链（软链自身存在、
// 目标不解析）记进 skipped 而不是让整个复制失败。其余 IO 错误照旧上报——
// 真正的磁盘问题必须响，不能悄悄少拷。
func copyDirReport(src, dst string) (skipped []string, err error) {
	err = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 && !pathExists(p) {
			skipped = append(skipped, p)
			return nil
		}
		if cerr := copyFile(p, target); cerr != nil {
			if info.Mode()&os.ModeSymlink != 0 && errors.Is(cerr, fs.ErrNotExist) {
				skipped = append(skipped, p)
				return nil
			}
			return cerr
		}
		return nil
	})
	return skipped, err
}

// ---------- T49：PDF 书签 ----------

// ensurePDFBookmarks 让成品 PDF 可导航：向 build/main.tex 幂等注入
// hyperref（若整个文档还没加载）与逐章 \pdfbookmark（放在每个
// \input{chapters/…} 前面，锚点 bk:<base>）。骨架版与终审持久化版
// main.tex 都过这里——两边都没 hyperref 时都能补上，已有的不重复加。
func ensurePDFBookmarks(proj, buildDir string) error {
	mainPath := filepath.Join(buildDir, "main.tex")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}
	main := string(data)
	hasHyperref := strings.Contains(main, "hyperref")

	var b strings.Builder
	changed := false
	for _, line := range strings.SplitAfter(main, "\n") {
		trimmed := strings.TrimSpace(line)
		if !hasHyperref && strings.HasPrefix(trimmed, "\\documentclass") {
			b.WriteString(line)
			b.WriteString("\\usepackage[bookmarksnumbered,bookmarksopen,hidelinks]{hyperref}\n")
			hasHyperref = true
			changed = true
			continue
		}
		if base := chapterInputBase(trimmed); base != "" {
			anchor := "bk:" + base
			if !strings.Contains(main, "{"+anchor+"}") {
				fmt.Fprintf(&b, "\\pdfbookmark[0]{%s}{%s}\n", escapeBookmark(chapterBookmarkTitle(proj, base)), anchor)
				changed = true
			}
		}
		b.WriteString(line)
	}
	if !changed {
		return nil
	}
	return os.WriteFile(mainPath, []byte(b.String()), 0o644)
}

// chapterInputBase 识别 "\input{chapters/<base>.tex}" 行，返回 base。
func chapterInputBase(line string) string {
	const pre = "\\input{chapters/"
	if !strings.HasPrefix(line, pre) || !strings.HasSuffix(line, "}") {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimPrefix(line, pre), "}")
	if strings.ContainsAny(name, "/\\{}") {
		return ""
	}
	return strings.TrimSuffix(name, ".tex")
}

// chapterBookmarkTitle 取划分阶段的章节 md 首个标题行作书签文字；
// 找不到时退回 base 名（chapter_003）。
func chapterBookmarkTitle(proj, base string) string {
	for _, dir := range []string{filepath.Join(proj, "chapters"), filepath.Join(proj, "work", "chapters")} {
		data, err := os.ReadFile(filepath.Join(dir, base+".md"))
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(data), "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "#") {
				return strings.TrimSpace(strings.TrimLeft(ln, "#"))
			}
			if ln != "" {
				break // 首个非空行不是标题就不再找（避免误取正文）
			}
		}
	}
	return base
}

// escapeBookmark 转义 pdfbookmark 参数里的 LaTeX 特殊字符。
func escapeBookmark(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	for _, c := range []string{"%", "&", "#", "_"} {
		s = strings.ReplaceAll(s, c, "\\"+c)
	}
	return s
}

// ---------- T44：assemble 资源盘点 ----------

// preflightAssemble 在进入编译前盘点 build 树：cls/figures/images/章节
// 各就位多少文件，以及逐章 .tex 的 \includegraphics 引用能否在树内解析。
// 缺失逐条告警但不中止（缺图不该中止整本书，见 T33 同款理由）。
func (r *Runner) preflightAssemble(buildDir, clsName string) {
	countDir := func(rel string) int {
		n := 0
		root := filepath.Join(buildDir, rel)
		_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	figures := countDir("figures")
	images := countDir("images")
	texs, _ := filepath.Glob(filepath.Join(buildDir, "chapters", "*.tex"))
	clsOK := fileExists(filepath.Join(buildDir, clsName+".cls"))

	// \includegraphics（可带 [选项]）引用解析检查。
	total, missing := 0, []string{}
	for _, tex := range texs {
		data, err := os.ReadFile(tex)
		if err != nil {
			continue
		}
		for _, m := range includeGraphicsRe.FindAllStringSubmatch(string(data), -1) {
			total++
			rel := m[1]
			// 章节 .tex 在 buildDir/chapters/ 下；graphicspath 声明 figures/，
			// 引用也可能是相对 chapters/ 或树内绝对相对路径（images/…）。
			cands := []string{
				filepath.Join(buildDir, rel),
				filepath.Join(buildDir, "figures", rel),
				filepath.Join(filepath.Dir(tex), rel),
			}
			found := false
			for _, c := range cands {
				if fileExists(c) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, filepath.Base(tex)+": "+rel)
			}
		}
	}
	state := "OK"
	if !clsOK || len(missing) > 0 {
		state = "有问题"
	}
	r.log.Log(0, fmt.Sprintf("[assemble] 资源盘点: cls=%v · 章节 %d · figures %d 文件 · images %d 文件 · 插图引用 %d/%d 就位 [%s]",
		clsOK, len(texs), figures, images, total-len(missing), total, state))
	for _, m := range missing {
		r.log.LogWarning(0, "[assemble] 插图引用解析不到:", m)
	}
}

// includeGraphicsRe 抽 \includegraphics[…]{path} 的路径。
var includeGraphicsRe = regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)
