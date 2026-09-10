package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestListSourcePagesTool pins the source-page contract: the origin
// PDFs are reported under their READ-ONLY mount (so they are viewed
// with the ordinary view_pdf), section starts are derived from the OCR
// layout, and page detail lists that page's text and extracted images.
func TestListSourcePagesTool(t *testing.T) {
	dir := t.TempDir()
	mineru := filepath.Join(dir, "mineru_output")
	part := filepath.Join(mineru, "book_part1")
	if err := os.MkdirAll(part, 0o755); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join(part, "book_part1_origin.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The view exposes clean names; the real pdf paths stay internal.
	viewDir := filepath.Join(dir, "pdfview")
	if err := os.MkdirAll(viewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(pdf, filepath.Join(viewDir, "book_part1.pdf")); err != nil {
		t.Fatal(err)
	}
	view := &pdfView{Dir: viewDir, total: 12,
		Files: []pdfViewFile{{Name: "book_part1.pdf", PDF: pdf, First: 1, Count: 12}}}
	doc := &DocIndex{Entries: []DocEntry{
		{Global: 3, Type: "text", Text: "第二章 随机变量"},
		{Global: 3, Type: "text", Text: "这是一段普通正文，带标点符号，不应当被当成标题。"},
		{Global: 4, Type: "image", Img: "book_part1/images/abc.jpg", Text: "图 2.1 分布函数"},
	}}
	tool := &ListSourcePagesTool{View: view, Index: doc, Mount: "source"}

	res, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"source:book_part1.pdf",
		"pages 1..12",
		"TOTAL 12 original pages",
		"g3  第二章 随机变量",
	} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("list output missing %q:\n%s", want, res.Text)
		}
	}
	if strings.Contains(res.Text, "普通正文") {
		t.Errorf("body text must not be listed as a section start:\n%s", res.Text)
	}

	det, err := tool.Execute(`{"page":4}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"local page 4", "abc.jpg", "view_pdf", "分布函数"} {
		if !strings.Contains(det.Text, want) {
			t.Errorf("page detail missing %q:\n%s", want, det.Text)
		}
	}

	out, err := tool.Execute(`{"page":99}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "OUT OF RANGE") {
		t.Errorf("out-of-range page must be reported: %s", out.Text)
	}
}

// TestViewPDFToolMountDescription: the origin PDFs are reachable with
// the SAME viewer as compiled PDFs, and the tool says so.
func TestViewPDFToolMountDescription(t *testing.T) {
	tool := &ViewPDFTool{
		Mounts: []Mount{
			{Name: "work", Dir: t.TempDir(), Writable: true},
			{Name: "source", Dir: t.TempDir()},
		},
	}
	desc, _ := tool.Definition()["function"].(map[string]any)["description"].(string)
	if !strings.Contains(desc, "source") || !strings.Contains(desc, "ORIGINAL") {
		t.Fatalf("description must mention the read-only source mount:\n%s", desc)
	}
}
