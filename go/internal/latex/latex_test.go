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

func TestParseClassification(t *testing.T) {
	cases := []struct {
		name, reply string
		want        string
		wantErr     bool
	}{
		{"plain", `{"kind":"vector","confidence":0.9,"label":"函数图像","reason":"坐标曲线"}`, "vector", false},
		{"fenced", "好的，结果如下：\n\n```json\n{\"kind\":\"text\",\"confidence\":0.8}\n```\n", "text", false},
		{"prose-wrapped", "The answer is {\"kind\":\"raster\",\"confidence\":0.5} thanks", "raster", false},
		{"invalid kind", `{"kind":"photo"}`, "", true},
		{"no json", "I think it is a vector image.", "", true},
		{"nested braces", `{"kind":"vector","reason":"has {curly} braces"}`, "vector", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseClassification(c.reply)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != c.want {
				t.Fatalf("kind = %q, want %q", got.Kind, c.want)
			}
		})
	}
}

func TestStripImgTypeHeader(t *testing.T) {
	in := "[IMG_TYPE: Flowchart]\n```mermaid\ngraph TD\n```"
	got := stripImgTypeHeader(in)
	if strings.Contains(got, "IMG_TYPE") {
		t.Fatalf("header not stripped: %q", got)
	}
	if !strings.HasPrefix(got, "```mermaid") {
		t.Fatalf("content changed: %q", got)
	}
	if got := stripImgTypeHeader("plain content"); got != "plain content" {
		t.Fatalf("plain content changed: %q", got)
	}
}

func TestValidateSplit(t *testing.T) {
	ok := []ChapterRange{
		{Title: "a", StartLine: 1, EndLine: 10},
		{Title: "b", StartLine: 11, EndLine: 20},
	}
	if err := validateSplit(ok, 20); err != nil {
		t.Fatalf("valid split rejected: %v", err)
	}
	gap := []ChapterRange{
		{Title: "a", StartLine: 1, EndLine: 10},
		{Title: "b", StartLine: 12, EndLine: 20},
	}
	if err := validateSplit(gap, 20); err == nil {
		t.Fatal("gap not detected")
	}
	overflow := []ChapterRange{{Title: "a", StartLine: 1, EndLine: 25}}
	if err := validateSplit(overflow, 20); err == nil {
		t.Fatal("overflow not detected")
	}
	notCovered := []ChapterRange{{Title: "a", StartLine: 1, EndLine: 19}}
	if err := validateSplit(notCovered, 20); err == nil {
		t.Fatal("uncovered tail not detected")
	}
}

func TestBuildStandaloneHoisting(t *testing.T) {
	body := "\\usetikzlibrary{arrows.meta}\n\\usepackage{xcolor}\n\\begin{tikzpicture}\n\\draw (0,0) -- (1,1);\n\\end{tikzpicture}"
	out := buildStandalone(body, true)
	for _, want := range []string{
		"\\documentclass[border=6pt]{standalone}",
		"\\usepackage{ctex}",
		"\\usetikzlibrary{arrows.meta}",
		"\\usepackage{xcolor}",
		"\\begin{tikzpicture}",
		"\\end{document}",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	// Hoisted lines must appear BEFORE \begin{document}.
	if idx := strings.Index(out, "\\begin{document}"); idx < 0 || strings.Index(out, "\\usetikzlibrary") > idx {
		t.Fatal("library lines not hoisted into preamble")
	}
}

func TestSubmitSplitToolValidation(t *testing.T) {
	tool := &SubmitSplitTool{}
	res, err := tool.Execute(`{"chapters":[{"title":"a","start_line":1,"end_line":10},{"title":"b","start_line":5,"end_line":20}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Set {
		t.Fatal("overlapping split must be rejected")
	}
	if !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("expected rejection message, got: %s", res.Text)
	}
	res, err = tool.Execute(`{"chapters":[{"title":"a","start_line":1,"end_line":10},{"title":"b","start_line":11,"end_line":20}]}`)
	if err != nil || !tool.Set || len(tool.Chapters) != 2 {
		t.Fatalf("valid split rejected: err=%v set=%v n=%d", err, tool.Set, len(tool.Chapters))
	}
	if !strings.Contains(res.Text, "SUBMITTED") {
		t.Fatalf("expected submit message: %s", res.Text)
	}
}

func TestResolveInside(t *testing.T) {
	full, err := resolveInside("/tmp/proj", "chapters/a.md")
	if err != nil || !strings.HasSuffix(full, "chapters/a.md") {
		t.Fatalf("normal path failed: %v %v", full, err)
	}
	if _, err := resolveInside("/tmp/proj", "../../etc/passwd"); err == nil {
		t.Fatal("escape not detected")
	}
	if _, err := resolveInside("/tmp/proj", "/etc/passwd"); err == nil {
		t.Fatal("absolute escape not detected")
	}
}

func TestSanitizeName(t *testing.T) {
	if got := sanitizeName("2.1 函数图像.png"); strings.ContainsAny(got, " /\\") {
		t.Fatalf("unsafe name: %q", got)
	}
	if got := sanitizeName("!!!"); got != "img" {
		t.Fatalf("empty fallback = %q", got)
	}
	if got := sanitizeName("图像1"); got != "图像1" {
		t.Fatalf("CJK stripped: %q", got)
	}
}

func TestFindOriginPDFs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"书_part1", "书_part2", "书_part10", "书", "其他_part1", "status"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	make := func(rel, file string) {
		if err := os.WriteFile(filepath.Join(root, rel, file), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	make("书_part1", "uuid1_origin.pdf")
	make("书_part2", "uuid2_origin.pdf")
	make("书_part10", "uuid10_origin.pdf")
	make("书", "uuid0_origin.pdf")
	make("其他_part1", "other_origin.pdf")

	got := findOriginPDFs(root, "书")
	if len(got) != 4 {
		t.Fatalf("want 4 origin pdfs, got %d: %v", len(got), got)
	}
	// 顺序：无编号书目录在前，之后 part1, part2, part10（自然序）。
	if !strings.Contains(got[0], filepath.Join("书", "uuid0_origin.pdf")) {
		t.Fatalf("partless dir must come first: %v", got[0])
	}
	if !strings.Contains(got[3], "part10") {
		t.Fatalf("part10 must sort after part2: %v", got[3])
	}
}

func TestRenderOriginPages(t *testing.T) {
	// 用真实 LaTeX 生成两页 PDF 来验证渲染与缓存。
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex 不可用")
	}
	dir := t.TempDir()
	tex := filepath.Join(dir, "t.tex")
	content := strings.Join([]string{
		"\\documentclass{article}",
		"\\usepackage[paperheight=10cm,paperwidth=8cm,margin=1cm]{geometry}",
		"\\begin{document}",
		"Page one\\newpage",
		"Page two",
		"\\end{document}",
		"",
	}, "\n")
	if err := os.WriteFile(tex, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	comp := NewCompiler(config.LatexCompileConfig{Engine: "pdflatex", Timeout: 60, RasterCommand: "pdftoppm", RasterDPI: 60})
	if res := comp.Compile(dir, "t.tex"); !res.OK {
		t.Fatalf("compile failed: %s", res.Err)
	}
	cfg := &config.Config{Paths: config.PathsConfig{MineruOutput: t.TempDir()}}
	log, err := logger.NewLogger(filepath.Join(dir, "t.log"), filepath.Join(dir, "t.err.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	r := &Runner{cfg: cfg, comp: comp, log: log}
	outDir := filepath.Join(dir, "pages")
	n, err := r.renderOriginPages([]string{filepath.Join(dir, "t.pdf")}, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 pages, got %d", n)
	}
	// 再跑一次：缓存生效，页数不变。
	n2, err := r.renderOriginPages([]string{filepath.Join(dir, "t.pdf")}, outDir)
	if err != nil || n2 != 2 {
		t.Fatalf("cache re-run: n=%d err=%v", n2, err)
	}
}
