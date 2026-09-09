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
	// Figures and raster assets from the level-2 source pass.
	for _, asset := range []string{"figures", "images"} {
		src := filepath.Join(proj, "source", asset)
		if fileExists(src) {
			if err := copyDir(src, filepath.Join(buildDir, asset)); err != nil {
				return err
			}
		}
	}
	// Chapter .tex files.
	texs, _ := filepath.Glob(filepath.Join(proj, "work", "chapters", "chapter_*.tex"))
	if len(texs) == 0 {
		return fmt.Errorf("没有已转换的章节 .tex")
	}
	sort.Strings(texs)
	for _, f := range texs {
		if err := copyFile(f, filepath.Join(buildDir, "chapters", filepath.Base(f))); err != nil {
			return err
		}
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
		if err := r.fixSession(proj, buildDir, res.Err); err != nil {
			return err
		}
	}

	// Final artefacts.
	pdf := filepath.Join(buildDir, "main.pdf")
	if !fileExists(pdf) {
		return fmt.Errorf("全书编译未产出 PDF")
	}
	if err := copyFile(pdf, filepath.Join(outDir, "book.pdf")); err != nil {
		return err
	}
	copyFile(filepath.Join(buildDir, "main.tex"), filepath.Join(outDir, "main.tex"))
	copyFile(filepath.Join(buildDir, clsName+".cls"), filepath.Join(outDir, clsName+".cls"))
	copyDir(filepath.Join(buildDir, "chapters"), filepath.Join(outDir, "chapters"))
	if fileExists(filepath.Join(buildDir, "figures")) {
		copyDir(filepath.Join(buildDir, "figures"), filepath.Join(outDir, "figures"))
	}

	// standalone.tex: single file with all chapters inlined (no compile
	// needed; easy for AI consumption and archival).
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
	sess := session.NewSession(client, modelCfg, tuning, fixSystemPrompt, []session.Tool{
		&ReadFileTool{Root: buildDir},
		&WriteWorkFileTool{Root: buildDir, AnyExt: true},
		&EditWorkFileTool{Root: buildDir},
		&GrepTool{Root: buildDir},
		&WorkBashTool{Dir: buildDir, MaxOutput: r.cfg.Latex.BashMaxOutput},
		compile,
		&ViewPDFTool{Root: buildDir, Comp: r.comp},
		&ViewImageTool{Root: buildDir},
		&ListFontsTool{FontsDir: r.cfg.Paths.Fonts},
		submit,
	}, r.log, 1, "fix")

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
