package latex

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"mineru-tools/internal/config"
)

// The 2026-09-11 run had 87/87 chapter compiles fail because the tool
// compiled the bare `\input` fragment (no \documentclass) instead of the
// wrapper that loads the book class. This pins the wrapper contract.
func TestCompileChapterToolUsesWrapper(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	// 会话的工作树就是编译目录：类、wrapper、章节片段都在同一棵树里，
	// 编译不再从别处拷贝任何文件。
	work := t.TempDir()

	if err := os.WriteFile(filepath.Join(work, "mybook.cls"), []byte(
		"\\NeedsTeXFormat{LaTeX2e}\n\\ProvidesClass{mybook}\n\\LoadClass{article}\n"+
			"\\newcommand{\\gnote}[1]{\\par\\noindent\\textbf{Note:} #1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapper := "\\documentclass{mybook}\n\\begin{document}\n\\input{chapter_007.tex}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(work, "chapter_007_wrapper.tex"), []byte(wrapper), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "chapter_007.tex"), []byte("\\section{A section}\n\\gnote{a note}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &CompileChapterTool{
		Comp: NewCompiler(config.LatexCompileConfig{Engine: "xelatex", Timeout: 120}),
		Dir:  work, MainFile: "chapter_007.tex", WrapperFile: "chapter_007_wrapper.tex",
	}
	res, err := tool.Execute("{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "COMPILE OK") {
		t.Fatalf("wrapper compile must succeed, got: %s", res.Text)
	}
	// 回执只报事实，但要看产物得告诉模型用 view_pdf（用户明确要求"简单说一下"）。
	if !strings.Contains(res.Text, "view_pdf") || !strings.Contains(res.Text, "chapter_007_wrapper.pdf") {
		t.Fatalf("chapter compile receipt must name the PDF and point at view_pdf, got: %s", res.Text)
	}
	// The fragment alone (the old behaviour) cannot compile: no class.
	plain := &CompileChapterTool{
		Comp: NewCompiler(config.LatexCompileConfig{Engine: "xelatex", Timeout: 120}),
		Dir:  work, MainFile: "chapter_007.tex",
	}
	if got, err := plain.Execute("{}"); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(got.Text, "COMPILE FAILED") {
		t.Fatalf("compiling the bare fragment must fail (that was the bug), got: %s", got.Text)
	}

	// {path} compiles any other .tex of the same tree in the same wrapper.
	probe := filepath.Join(work, "chapter_007", "probe.tex")
	if err := os.MkdirAll(filepath.Dir(probe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(probe, []byte("\\gnote{probe}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "chapter_007/probe.tex"})
	got, err := tool.Execute(string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Text, "COMPILE OK") {
		t.Fatalf("probe compile must work, got: %s", got.Text)
	}
}

// read_file/grep/write_file must share ONE namespace: the convert
// session's first call used to be `project:chapters/…` against a tool
// whose only mount was called "work" and pointed at the project view.
func TestSessionToolsShareMountTable(t *testing.T) {
	work := t.TempDir() // private chapter view
	proj := t.TempDir() // project view
	if err := os.MkdirAll(filepath.Join(work, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "chapters", "chapter_001.tex"), []byte("\\section{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "style"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_001.md"), []byte("# Chapter\n\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "style", "manual.md"), []byte("## Commands\n\\gnote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounts := []Mount{{Name: "work", Dir: work, Writable: true}, {Name: "project", Dir: proj}}

	read := &ReadFileTool{Mounts: mounts}
	got, err := read.Execute(`{"path":"project:chapters/chapter_001.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "正文") {
		t.Fatalf("project: path must resolve, got: %s", got.Text)
	}
	if got, _ := read.Execute(`{"path":"work:chapters/chapter_001.tex"}`); !strings.Contains(got.Text, "\\section") {
		t.Fatalf("work: path must resolve, got: %s", got.Text)
	}
	if got, _ := read.Execute(`{"path":"chapters/chapter_001.tex"}`); !strings.Contains(got.Text, "\\section") {
		t.Fatalf("bare path must default to the writable mount, got: %s", got.Text)
	}

	grep := &GrepTool{Mounts: mounts}
	// Mount-prefixed paths used to be concatenated into a filename
	// ("…/project/project:style: no such file or directory").
	if got, _ := grep.Execute(`{"pattern":"gnote","path":"project:style"}`); !strings.Contains(got.Text, "gnote") {
		t.Fatalf("grep project:style must find the manual, got: %s", got.Text)
	}
	// A bare path searches every mount (the prompt promises a project-wide grep).
	if got, _ := grep.Execute(`{"pattern":"section","path":"."}`); !strings.Contains(got.Text, "chapter_001.tex") {
		t.Fatalf("project-wide grep must see the work mount, got: %s", got.Text)
	}
	if got, _ := grep.Execute(`{"pattern":"正文","path":"."}`); !strings.Contains(got.Text, "chapter_001.md") {
		t.Fatalf("project-wide grep must see the project mount, got: %s", got.Text)
	}
}

// The sandbox /tmp must be a real per-session directory (persisting
// between bash calls) when one is configured, and only fall back to a
// per-call tmpfs otherwise.
func TestSandboxTmpIsPersistentWhenConfigured(t *testing.T) {
	work := t.TempDir()
	tmp := t.TempDir()
	base := &WorkBashTool{Root: work, Mounts: []Mount{{Name: "work", Dir: work, Writable: true}}}
	args := strings.Join(base.sandboxArgs("true", false), " ")
	if !strings.Contains(args, "--tmpfs /tmp") {
		t.Fatalf("without TmpDir the sandbox keeps a private tmpfs, got: %s", args)
	}
	withTmp := &WorkBashTool{Root: work, Mounts: []Mount{{Name: "work", Dir: work, Writable: true}}, TmpDir: tmp}
	args = strings.Join(withTmp.sandboxArgs("true", false), " ")
	if strings.Contains(args, "--tmpfs /tmp") {
		t.Fatalf("TmpDir must replace the tmpfs, got: %s", args)
	}
	if !strings.Contains(args, "--bind "+tmp+" /tmp") {
		t.Fatalf("TmpDir must be bound at /tmp, got: %s", args)
	}
	if !strings.Contains(args, "--setenv TMPDIR /tmp") {
		t.Fatalf("TMPDIR must point at the persistent scratch, got: %s", args)
	}
}

// submit_style takes workspace PATHS (plus optional extras and a report)
// instead of making the model re-emit ~47KB of content it already wrote.
func TestSubmitStyleByPathWithExtras(t *testing.T) {
	ws := t.TempDir()
	cls := "\\NeedsTeXFormat{LaTeX2e}\n\\ProvidesClass{mybook}\n\\LoadClass{article}\n\\RequirePackage{theme}\n"
	files := map[string]string{
		"mybook.cls":       cls,
		"manual.md":        "## Commands\n\\gnote\n",
		"example.tex":      "\\documentclass{mybook}\n\\begin{document}hi\\end{document}\n",
		"theme.sty":        "\\ProvidesPackage{theme}\n",
		"tikz/mindmap.tex": "\\tikzstyle{root}=[]\n",
	}
	for name, body := range files {
		full := filepath.Join(ws, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tool := &SubmitStyleTool{Workspace: ws}
	args, _ := json.Marshal(map[string]any{
		"cls": "mybook.cls", "manual": "manual.md", "example": "example.tex",
		"extra": []string{"theme.sty", "tikz/mindmap.tex"}, "report": "Fandol 字体缺失：FandolSong-Bold.otf",
	})
	res, err := tool.Execute(string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "SUBMITTED") {
		t.Fatalf("path submission must be accepted, got: %s", res.Text)
	}
	if tool.Cls != cls {
		t.Fatalf("cls content must be read from disk, got %q", tool.Cls)
	}
	if len(tool.Extra) != 2 || !strings.Contains(tool.Extra["tikz/mindmap.tex"], "tikzstyle") {
		t.Fatalf("extra files must carry content, got %#v", tool.Extra)
	}
	if !strings.Contains(tool.Reported, "Fandol") {
		t.Fatalf("report must be captured, got %q", tool.Reported)
	}
	// A path that does not exist is an error, never silently submitted as
	// content (that would ship the string "mybook.cls" as the class).
	bad := &SubmitStyleTool{Workspace: ws}
	badArgs, _ := json.Marshal(map[string]any{"cls": "nope.cls", "manual": "manual.md", "example": "example.tex"})
	if got, _ := bad.Execute(string(badArgs)); !strings.HasPrefix(got.Text, "REJECTED") {
		t.Fatalf("missing path must be rejected, got: %s", got.Text)
	}
	// No inline fallback any more (用户: 别给 AI 保留它不知道的旧形态):
	// 内容式的参数一律拒绝，并明确指向"先 write_file 再交路径"。
	inline := &SubmitStyleTool{Workspace: ws}
	inlineArgs, _ := json.Marshal(map[string]any{"cls": cls, "manual": "## x", "example": files["example.tex"]})
	if got, _ := inline.Execute(string(inlineArgs)); !strings.HasPrefix(got.Text, "REJECTED") {
		t.Fatalf("inline content must be rejected, got: %s", got.Text)
	} else if !strings.Contains(got.Text, "只收工作区路径") {
		t.Fatalf("rejection must tell the model to submit a path: %s", got.Text)
	}
}

// A 21,658-char Chinese chapter was handed to the model as a 1,527-char
// stub because truncation counted BYTES while the prompt said "chars".
func TestChapterPreviewCountsRunes(t *testing.T) {
	s := strings.Repeat("概率论与数理统计", 3000) // 8 runes / 24 bytes each
	got := truncateRunes(s, 2000)
	if n := len([]rune(strings.TrimSuffix(got, "..."))); n != 2000 {
		t.Fatalf("preview must hold 2000 characters, got %d", n)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatal("truncated preview must be marked")
	}
	// Byte-based truncation must never split a rune.
	cut := truncateStr(s, 2000)
	if !utf8.ValidString(cut) {
		t.Fatalf("truncateStr produced invalid UTF-8: %q", cut[:20])
	}
	if len(cut) > 2003 {
		t.Fatalf("truncateStr must respect the byte cap, got %d", len(cut))
	}
}
