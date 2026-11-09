package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T265：单文件版必须把子会话、子子会话的产物一起摊进来。现场那份 6534 行的
// standalone.tex 里还留着 192 条 \input{chapters/<章>/…}（子章节分片）和 3 条
// \input{figures/motif_*.tex}，它们指向的正是子会话交给父会话的内容——文件本身
// 在，只是没被展开，"可以不编译直接分发"因此不成立。
func TestStandaloneInlinesChildrenAndGrandchildren(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tex", "\\documentclass{book}\n\\begin{document}\n\\input{chapters/chapter_001.tex}\n\\end{document}\n")
	write("chapters/chapter_001.tex", "\\chapter{一}\n\\input{chapters/chapter_001/children/chapter_001_child_01.tex}\n")
	// 子会话提交的分片：自己又 \input 子子会话的产物。
	write("chapters/chapter_001/children/chapter_001_child_01.tex", "第一节正文\n\\input{chapters/chapter_001/children/chapter_001_child_01/grand_01.tex}\n\\input{figures/motif_abc.tex}\n")
	write("chapters/chapter_001/children/chapter_001_child_01/grand_01.tex", "子子会话的正文\n")
	write("figures/motif_abc.tex", "\\begin{tikzpicture}\\draw (0,0)--(1,1);\\end{tikzpicture}\n")

	got, err := buildStandaloneTex(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\\chapter{一}", "第一节正文", "子子会话的正文", "\\begin{tikzpicture}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("单文件里缺 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "\\input{") {
		t.Fatalf("还有没展开的 \\input：\n%s", got)
	}
	// 内联处留一行出处注释，出问题时能对回原文件。
	if !strings.Contains(got, "% ==== chapters/chapter_001/children/chapter_001_child_01/grand_01.tex ====") {
		t.Fatalf("缺少出处注释：\n%s", got)
	}
}

// 同一份片段被多章引用要各展开一次（那是同一张图出现在不同位置），但自引用、
// 互相引用这种环必须停手：保留原样的 \input，不让内联无限展开。
func TestStandaloneInlineStopsAtCyclesAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tex", "\\input{chapters/a.tex}\n\\input{chapters/a.tex}\n\\input{chapters/missing.tex}\n")
	write("chapters/a.tex", "A 正文\n\\input{chapters/a.tex}\n")
	got, err := buildStandaloneTex(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "A 正文") != 2 {
		t.Fatalf("同一片段被两处引用应各展开一次，实际 %d 次：\n%s", strings.Count(got, "A 正文"), got)
	}
	if !strings.Contains(got, "\\input{chapters/a.tex}") {
		t.Fatalf("自引用要原样保留：\n%s", got)
	}
	if !strings.Contains(got, "\\input{chapters/missing.tex}") {
		t.Fatalf("找不到的目标要原样保留：\n%s", got)
	}
}

// 章节用 \input@path 把子章节的分片目录挂进搜索路径，里面的 \input 是裸相对名
// （\input{<子章>/part01.tex}、\input{part01.tex}）——不认这张表就一条也内联不了。
func TestStandaloneHonorsInputPath(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.tex", "\\input{chapters/chapter_001.tex}\n")
	write("chapters/chapter_001.tex",
		"\\makeatletter\\def\\input@path{{chapters/chapter_001/children/}{chapters/chapter_001/parts/}}\\makeatother\n"+
			"章正文\n\\input{chapter_001_child_01.tex}\n")
	write("chapters/chapter_001/children/chapter_001_child_01.tex", "子章节正文\n\\input{part01.tex}\n")
	write("chapters/chapter_001/parts/part01.tex", "分片正文\n")

	got, err := buildStandaloneTex(filepath.Join(root, "main.tex"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"章正文", "子章节正文", "分片正文"} {
		if !strings.Contains(got, want) {
			t.Fatalf("单文件里缺 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "\\input{") {
		t.Fatalf("还有没展开的 \\input：\n%s", got)
	}
}
