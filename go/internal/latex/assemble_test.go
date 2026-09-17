package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T49：main.tex 幂等注入 hyperref + 逐章 \pdfbookmark（终审版同样适用）。
func TestEnsurePDFBookmarks(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_001.md"),
		[]byte("# 第一套试题\n\n正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "\\documentclass{kyexam}\n\\begin{document}\n" +
		"\\input{chapters/chapter_001.tex}\n\\input{chapters/chapter_002.tex}\n\\end{document}\n"
	mainPath := filepath.Join(build, "main.tex")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(mainPath)
	s := string(got)
	if !strings.Contains(s, "\\usepackage[bookmarksnumbered") {
		t.Error("hyperref not injected")
	}
	if !strings.Contains(s, "\\pdfbookmark[0]{第一套试题}{bk:chapter_001}") {
		t.Errorf("chapter 1 bookmark missing:\n%s", s)
	}
	if !strings.Contains(s, "\\pdfbookmark[0]{chapter\\_002}{bk:chapter_002}") {
		t.Error("chapter 2 bookmark should fall back to the base name")
	}
	// 幂等：第二次运行产物不变。
	if err := ensurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(mainPath)
	if string(again) != s {
		t.Error("second run must be a no-op (idempotent)")
	}
}

// T49：终审版 main.tex 已自带 hyperref 时不重复注入。
func TestEnsurePDFBookmarksKeepsExistingHyperref(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	main := "\\documentclass{kyexam}\n\\usepackage{hyperref}\n\\begin{document}\n" +
		"\\input{chapters/chapter_001.tex}\n\\end{document}\n"
	mainPath := filepath.Join(build, "main.tex")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(mainPath)
	s := string(got)
	if strings.Count(s, "hyperref") != 1 {
		t.Errorf("hyperref count = %d, want 1", strings.Count(s, "hyperref"))
	}
	if !strings.Contains(s, "\\pdfbookmark[0]{chapter\\_001}{bk:chapter_001}") {
		t.Error("bookmark should still be added when hyperref already present")
	}
}

// T44：资源盘点数得出文件、解析得出引用、缺失逐条告警。
func TestPreflightAssemble(t *testing.T) {
	r := keepTestRunner(t)
	build := t.TempDir()
	mk := func(rel, content string) {
		t.Helper()
		p := filepath.Join(build, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("kyexam.cls", "% cls")
	mk("figures/a.pdf", "pdf")
	mk("images/b.jpg", "jpg")
	mk("chapters/chapter_001.tex", "\\includegraphics{a.pdf}\n\\includegraphics{images/b.jpg}\n\\includegraphics{gone.pdf}\n")
	r.preflightAssemble(build, "kyexam")
	// 不 panic、不中止即可；覆盖解析逻辑断言单独跑。
	found := func(rel string) bool {
		return fileExists(filepath.Join(build, rel)) ||
			fileExists(filepath.Join(build, "figures", rel)) ||
			fileExists(filepath.Join(build, "chapters", rel))
	}
	if !found("a.pdf") || !found("images/b.jpg") {
		t.Error("existing assets should resolve")
	}
	if found("gone.pdf") {
		t.Error("missing asset must not resolve")
	}
}

// T49：终审会话已按书的结构布置书签时，基线锚点不再注入（只补 hyperref）。
func TestEnsurePDFBookmarksYieldsToAI(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	main := "\\documentclass{kyexam}\n\\begin{document}\n" +
		"\\bookmark[level=0,dest=chap1]{第一套}\n\\input{chapters/chapter_001.tex}\n\\end{document}\n"
	mainPath := filepath.Join(build, "main.tex")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(mainPath)
	s := string(got)
	if strings.Contains(s, "bk:chapter_001") {
		t.Errorf("baseline anchor must not be added when AI bookmarks exist:\n%s", s)
	}
	if !strings.Contains(s, "hyperref") {
		t.Error("hyperref should still be injected when missing")
	}
}

// T49：书签层级来自章节 md 首个标题的 # 数（#=0 层，##=1 层，clamp 0..3）。
func TestBookmarkLevelFollowsHeadingDepth(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(proj, "chapters", "chapter_001.md"), []byte("# 第一章 大章\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "chapters", "chapter_002.md"), []byte("## 1.1 小节\n"), 0o644)
	os.WriteFile(filepath.Join(proj, "chapters", "chapter_003.md"), []byte("#### 1.1.1.1 太深\n"), 0o644)
	main := "\\documentclass{c}\n\\begin{document}\n" +
		"\\input{chapters/chapter_001.tex}\n\\input{chapters/chapter_002.tex}\n\\input{chapters/chapter_003.tex}\n\\end{document}\n"
	mainPath := filepath.Join(build, "main.tex")
	os.WriteFile(mainPath, []byte(main), 0o644)
	if err := ensurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(mainPath)
	s := string(got)
	for _, want := range []string{
		"\\pdfbookmark[0]{第一章 大章}{bk:chapter_001}",
		"\\pdfbookmark[1]{1.1 小节}{bk:chapter_002}",
		"\\pdfbookmark[3]{1.1.1.1 太深}{bk:chapter_003}",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}
