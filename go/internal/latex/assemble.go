package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	for _, asset := range []string{"figures", "images"} {
		src := filepath.Join(proj, "source", asset)
		if fileExists(src) {
			if err := copyDir(src, filepath.Join(buildDir, asset)); err != nil {
				return err
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

	// main.tex.
	var in strings.Builder
	for _, f := range texs {
		fmt.Fprintf(&in, "\\input{chapters/%s}\n", filepath.Base(f))
	}
	mainTex := "\\documentclass{" + clsName + "}\n" +
		"\\graphicspath{{figures/}}\n" +
		"\\begin{document}\n" + in.String() + "\\end{document}\n"
	if err := os.WriteFile(filepath.Join(buildDir, "main.tex"), []byte(mainTex), 0o644); err != nil {
		return err
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
	if r.cfg.Latex.Compile.FinalReviewEnabled() && fileExists(filepath.Join(buildDir, "main.pdf")) {
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
	tools := []session.Tool{
		&ReadFileTool{Root: buildDir, AltRoots: []AltRoot{{Label: "project", Dir: proj}}},
		&WriteWorkFileTool{Root: buildDir, AnyExt: true},
		&EditWorkFileTool{Root: buildDir},
		&GrepTool{Root: buildDir, AltRoots: []AltRoot{{Label: "project", Dir: proj}}},
		&WorkBashTool{Dir: buildDir, MaxOutput: r.cfg.Latex.BashMaxOutput},
		compile,
		&ViewPDFTool{Root: buildDir, Comp: r.comp},
		&ViewImageTool{Root: buildDir},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}
	if r.docPages != nil {
		// 原书扫描页：核对真实版面/图表来源。
		tools = append(tools,
			&ListSourcePagesTool{Idx: r.docPages},
			&ViewSourcePageTool{Idx: r.docPages, PagesDir: filepath.Join(proj, "pages"), Runner: r})
	}
	return tools
}

// fixSession spawns the build-doctor session. It is a full workspace
// session on the build tree: read/write/edit/grep/bash + compile +
// view_pdf/view_image + fonts, so it can restructure the assembled
// project (multi-file, resources) and verify the PDF itself.
func (r *Runner) fixSession(proj, buildDir, firstErr string) error {
	r.log.LogWarning(0, "[assemble] 全书编译失败，启动修复会话:", firstErr)
	client := r.clientFor(r.cfg.Latex.ConvertModel)
	modelCfg := r.models[r.cfg.Latex.ConvertModel]
	tuning := r.cfg.LatexSession("convert")
	compile := &CompileTexTool{Comp: r.comp, Root: buildDir, MainFile: "main.tex", Tag: "book", Log: r.log, Tid: 1}
	submit := &SubmitDoneTool{Label: "the build fix"}
	tools := r.bookSessionTools(proj, buildDir, compile, submit)
	sess := session.NewSession(client, modelCfg, tuning, fixSystemPrompt, tools, r.log, 1, "fix")

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
	sess := session.NewSession(r.clientFor(r.cfg.Latex.ConvertModel), r.models[r.cfg.Latex.ConvertModel],
		r.cfg.LatexSession("convert"), finalReviewSystemPrompt,
		r.bookSessionTools(proj, buildDir, compile, submit), r.log, 1, "final-review")
	liveHook, liveClose := r.livePhaseLine("final-review")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	var chaps strings.Builder
	for _, f := range texs {
		fmt.Fprintf(&chaps, "- chapters/%s\n", filepath.Base(f))
	}
	pages, _ := pdfPageCount(filepath.Join(buildDir, "main.pdf"))
	first := strings.Join([]string{
		"The full book compiled successfully. This is the final consolidation pass.",
		"",
		"Book tree (your workspace): main.tex, chapters/*.tex, the class and manual.md/example.tex, figures/, images/.",
		fmt.Sprintf("Compiled book.pdf: %d page(s).", pages),
		"Chapters:",
		chaps.String(),
		"",
		"Read the finished PDF page by page (view_pdf) and compare against the original markdown (read_file \"project:<path>\") and the original book pages (list_source_pages/view_source_page).",
		"Fix everything a printed book needs: front matter/cover, table of contents, chapter order and completeness, page numbering and headers/footers, figure/table placement and sizing, orphan/blank pages, overfull boxes, duplicated or missing sections.",
		"Use edit_file for minimal fixes (never drop content), bash to reorganise files if needed, then compile {path:\"main.tex\", engine:\"latexmk\"} and verify with view_pdf.",
		"When the book is final, call submit.",
	}, "\n")

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
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}
