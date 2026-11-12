package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T324：子章节之间按 `chapter_00X/children/...` 的相对路径互相 \input，主文件却
// 在构建根——不给 \input@path，全书就只能靠修复会话逐个改路径（而修复只在临时
// 构建树里生效，下次 assemble 又得重烧一遍）。
func TestEnsureInputPath(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.tex")
	body := "\\documentclass{book}\n\\graphicspath{{figures/}}\n\\begin{document}\n\\input{chapters/a.tex}\n\\end{document}\n"
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureInputPath(main); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(main)
	if !strings.Contains(string(got), "\\input@path{{chapters/}}") {
		t.Fatalf("没有注入 \\input@path：%s", got)
	}
	// 幂等：再跑一次不该叠第二行。
	if err := ensureInputPath(main); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(main)
	if strings.Count(string(again), "\\input@path") != 1 {
		t.Fatalf("重复注入：%s", again)
	}
	// 注入点在第一行之后（\documentclass 之后、\begin{document} 之前）。
	if i, j := strings.Index(string(again), "\\input@path"), strings.Index(string(again), "\\begin{document}"); i > j {
		t.Fatalf("注入点落在 \\begin{document} 之后")
	}
}

// T324：样式类已经加载 hyperref 时，再往 main.tex 塞一个 \usepackage[选项]{hyperref}
// 就是 "Option clash for package hyperref"——全书根本编不过去。
func TestClassLoadsHyperref(t *testing.T) {
	dir := t.TempDir()
	style := filepath.Join(dir, "style")
	_ = os.MkdirAll(style, 0o755)
	if classLoadsHyperref(dir) {
		t.Fatal("空 style 目录不该判成已加载")
	}
	_ = os.WriteFile(filepath.Join(style, "a.cls"), []byte("\\RequirePackage{tikz}\n"), 0o644)
	if classLoadsHyperref(dir) {
		t.Fatal("没加载 hyperref 却判成已加载")
	}
	_ = os.WriteFile(filepath.Join(style, "a.cls"), []byte("\\RequirePackage[bookmarks]{hyperref}\n"), 0o644)
	if !classLoadsHyperref(dir) {
		t.Fatal("类里加载了 hyperref 却没认出来")
	}
}
