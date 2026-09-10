package latex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// TestReadFileToolLineWindow covers the unified read_file tool: whole
// file, numbered line window and read-only extra roots.
func TestReadFileToolLineWindow(t *testing.T) {
	root := t.TempDir()
	alt := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "own.tex"), []byte("a\nb\nc\nd\ne\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alt, "manual.md"), []byte("# manual\nrule 1\nrule 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFileTool{Root: root, AltRoots: []AltRoot{{Label: "project", Dir: alt}}}

	res, err := tool.Execute(`{"path":"own.tex"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "a\nb\nc") || !strings.Contains(res.Text, "10 bytes") {
		t.Errorf("whole-file read = %q", res.Text)
	}

	res, err = tool.Execute(`{"path":"own.tex","start_line":2,"end_line":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "2: b") || !strings.Contains(res.Text, "3: c") || strings.Contains(res.Text, "4: d") {
		t.Errorf("line window = %q", res.Text)
	}

	res, err = tool.Execute(`{"path":"manual.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "project/manual.md") || !strings.Contains(res.Text, "rule 1") {
		t.Errorf("alt-root read = %q", res.Text)
	}

	if _, err := tool.Execute(`{"path":"../escape.tex"}`); err == nil {
		t.Error("path escape must be rejected")
	}
	if _, err := tool.Execute(`{"path":"missing.tex"}`); err == nil {
		t.Error("missing file must be an error")
	}
}

// TestNeedsAuxPass checks the second-pass detection (toc/refs/bib),
// including markers inside \input'ed files.
func TestNeedsAuxPass(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("plain.tex", "\\begin{document}hello\\end{document}\n")
	write("toc.tex", "\\begin{document}\\tableofcontents\\end{document}\n")
	write("child.tex", "see \\ref{sec:one}\n")
	write("parent.tex", "\\begin{document}\\input{child}\\end{document}\n")

	if needsAuxPass(dir, "plain.tex") {
		t.Error("plain document must not need a second pass")
	}
	if !needsAuxPass(dir, "toc.tex") {
		t.Error("\\tableofcontents must need a second pass")
	}
	if !needsAuxPass(dir, "parent.tex") {
		t.Error("a \\ref inside an \\input'ed file must need a second pass")
	}
}

// TestCompileTexToolRejectsMissingFile: compile only accepts a path to
// an existing .tex (never inline code).
func TestCompileTexToolRejectsMissingFile(t *testing.T) {
	root := t.TempDir()
	tool := &CompileTexTool{Comp: NewCompiler(config.LatexCompileConfig{Engine: "xelatex"}), Root: root, MainFile: "main.tex"}
	res, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "NOT FOUND") {
		t.Errorf("missing main file = %q", res.Text)
	}
	res, err = tool.Execute(`{"path":"other.pdf"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "只接受 .tex") {
		t.Errorf("non-tex path = %q", res.Text)
	}
}

// TestCompileTexToolStyleWorkspace mirrors the style session: the class
// written into the workspace is picked up when compiling example.tex
// from the same directory.
func TestCompileTexToolStyleWorkspace(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("bookstyle.cls", "\\NeedsTeXFormat{LaTeX2e}\n\\ProvidesClass{bookstyle}[2026/01/01 test]\n\\LoadClass{article}\n\\newcommand{\\househeading}[1]{\\section{#1}}\n")
	write("example.tex", "\\documentclass{bookstyle}\n\\begin{document}\n\\househeading{Hello}\nBody text.\n\\end{document}\n")

	tool := &CompileTexTool{
		Comp:     NewCompiler(config.LatexCompileConfig{Engine: "xelatex"}),
		Root:     root,
		MainFile: "example.tex",
		Tag:      "style",
	}
	res, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "COMPILE OK.") || !strings.Contains(res.Text, "example.pdf") {
		t.Fatalf("style workspace compile = %q", res.Text)
	}
}

// TestCompileTexToolMultiFile compiles a real two-file project with a
// table of contents and a cross-reference, exercising the multi-pass
// path and the product report (PDF name + page count).
func TestCompileTexToolMultiFile(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("child.tex", "\\section{Child}\\label{sec:child}\nSee \\ref{sec:child}.\n")
	write("main.tex", "\\documentclass{article}\n\\begin{document}\n\\tableofcontents\n\\input{child}\n\\end{document}\n")

	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	log.SetQuiet(true)
	defer log.Close()

	tool := &CompileTexTool{
		Comp:     NewCompiler(config.LatexCompileConfig{Engine: "xelatex", Timeout: 120}),
		Root:     root,
		MainFile: "main.tex",
		Tag:      "book",
		Log:      log,
	}
	res, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "COMPILE OK.") {
		t.Fatalf("multi-file compile = %q", res.Text)
	}
	if !strings.Contains(res.Text, "main.pdf") || !strings.Contains(res.Text, "pages") {
		t.Errorf("product report missing PDF name/pages: %q", res.Text)
	}
	if !tool.LastOK || tool.LastPDF == "" || tool.Pages < 1 {
		t.Errorf("state not recorded: ok=%v pdf=%q pages=%d", tool.LastOK, tool.LastPDF, tool.Pages)
	}
}

// TestEditWorkFilePrefixes: a convert session may edit its OWN chapter
// file and asset folder, and is refused on any sibling chapter.
func TestEditWorkFilePrefixes(t *testing.T) {
	work := t.TempDir()
	for _, f := range []string{"chapters/chapter_01.tex", "chapters/chapter_02.tex", "chapters/chapter_01/table1.tex", "reports/chapter_01.md"} {
		full := filepath.Join(work, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("OLD\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tool := &EditWorkFileTool{Root: work, Prefixes: []string{"chapters/chapter_01.tex", "chapters/chapter_01/"}}

	res, err := tool.Execute(`{"path":"chapters/chapter_01.tex","find":"OLD","replace":"NEW"}`)
	if err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("own chapter must be editable: %v %s", err, res.Text)
	}
	res, err = tool.Execute(`{"path":"chapters/chapter_01/table1.tex","find":"OLD","replace":"NEW"}`)
	if err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("own asset folder must be editable: %v %s", err, res.Text)
	}
	for _, bad := range []string{"chapters/chapter_02.tex", "reports/chapter_01.md", "../outside.tex"} {
		res, _ := tool.Execute(`{"path":"` + bad + `","find":"OLD","replace":"NEW"}`)
		if !strings.Contains(res.Text, "REJECTED") && !strings.Contains(res.Text, "越界") {
			t.Errorf("%s must be refused, got: %s", bad, res.Text)
		}
	}
	data, _ := os.ReadFile(filepath.Join(work, "chapters", "chapter_02.tex"))
	if string(data) != "OLD\n" {
		t.Errorf("sibling chapter was modified: %q", data)
	}
}
