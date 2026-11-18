package assemble

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneInlinesRecursively(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tex", "\\documentclass{book}\n\\input{chapters/ch1}\n")
	write("chapters/ch1.tex", "\\chapter{一}\n\\input{sec_a}\n")
	write("chapters/sec_a.tex", "\\section{节}\n正文\n")
	out, err := BuildStandaloneTex(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "正文") || !strings.Contains(out, "% ==== chapters/sec_a.tex ====") {
		t.Fatalf("应递归内联到最深层: %.200s", out)
	}
}

func TestStandaloneInputPathDeclaration(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tex", "\\documentclass{book}\n\\input{ch1}\n")
	write("chapters/ch1.tex", "\\chapter{一}\n")
	write("shared/frag.tex", "碎片内容\n")
	// 声明与使用须在同一个文档里（inliner 每次构建新建）
	write("main2.tex", "\\documentclass{book}\n\\input@path{{chapters/}{shared/}}\n\\input{frag}\n")
	out, err := BuildStandaloneTex(filepath.Join(root, "main2.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "碎片内容") {
		t.Fatalf("\\input@path 声明的目录应参与解析: %.200s", out)
	}
}

func TestStandaloneRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.tex"), []byte("\\input{../evil}"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := BuildStandaloneTex(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "../evil") {
		t.Fatal("越界 \\input 应原样保留（不内联）")
	}
}

func TestChapterInputBase(t *testing.T) {
	if got := ChapterInputBase("\\input{chapters/chapter_003.tex}"); got != "chapter_003" {
		t.Fatalf("base 解析错: %q", got)
	}
	for _, bad := range []string{"\\input{chapters/a/b}", "\\input{other/x}"} {
		if ChapterInputBase(bad) != "" {
			t.Fatalf("应拒: %s", bad)
		}
	}
}

func TestEnsurePDFBookmarks(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	proj := filepath.Join(root, "proj")
	for _, d := range []string{build, filepath.Join(proj, "chapters")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	main := "\\documentclass{book}\n\\begin{document}\n\\input{chapters/ch1}\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(build, "main.tex"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "ch1.md"), []byte("## 第二章 模型评估\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(build, "main.tex"))
	s := string(got)
	if !strings.Contains(s, "hyperref") || !strings.Contains(s, "\\pdfbookmark[1]") || !strings.Contains(s, "bk:ch1") {
		t.Fatalf("应注入 hyperref + 层级书签(##→level 1): %.300s", s)
	}
	// 幂等：再跑一次不重复
	if err := EnsurePDFBookmarks(proj, build); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(filepath.Join(build, "main.tex"))
	if strings.Count(string(got2), "pdfbookmark[1]") != 1 {
		t.Fatal("书签注入应幂等")
	}
}

func TestEnsurePDFBookmarksRespectsAI(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	main := "\\documentclass{book}\n\\pdfbookmark[0]{AI 目录}{ai}\n\\begin{document}\nx\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(build, "main.tex"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePDFBookmarks(root, build); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(build, "main.tex"))
	if strings.Count(string(got), "pdfbookmark") != 1 {
		t.Fatalf("AI 已接手时只补 hyperref 不动书签: %.200s", got)
	}
	if !strings.Contains(string(got), "hyperref") {
		t.Fatal("hyperref 仍应补上")
	}
}

func TestEscapeBookmark(t *testing.T) {
	if got := escapeBookmark("A_B%C"); got != "A\\_B\\%C" {
		t.Fatalf("转义错: %q", got)
	}
}

func TestPreflightAssemble(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("book.cls")
	mk("figures/f1.pdf")
	mk("images/i1.png")
	mk("chapters/ch1.tex")
	if err := os.WriteFile(filepath.Join(root, "chapters/ch1.tex"),
		[]byte("\\includegraphics{f1.pdf}\n\\includegraphics[width=3cm]{nope.png}\n\\includegraphics{images/i1.png}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := PreflightAssemble(root, "book")
	if !rep.ClsOK || rep.Chapters != 1 || rep.Figures != 1 || rep.Images != 1 {
		t.Fatalf("计数错: %+v", rep)
	}
	if rep.RefsTotal != 3 || len(rep.RefsMissing) != 1 || !strings.Contains(rep.RefsMissing[0], "nope.png") {
		t.Fatalf("引用盘点错: %+v", rep)
	}
}

func TestEnsureInputPath(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main.tex")
	if err := os.WriteFile(main, []byte("\\documentclass{book}\n\\begin{document}\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureInputPath(main); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(main)
	if !strings.Contains(string(got), "\\input@path") {
		t.Fatal("应注入 \\input@path")
	}
	// 幂等
	if err := EnsureInputPath(main); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(main)
	if strings.Count(string(got2), "\\input@path") != 1 {
		t.Fatal("注入应幂等")
	}
}
