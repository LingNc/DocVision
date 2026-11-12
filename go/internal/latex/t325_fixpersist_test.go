package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T325：全书修复会话成功之后必须把构建树回写工作区。否则停在终审后下次再
// 跑，assemble 一重建树，编译又失败、又开一条全新修复会话——同一本书连修
// 两轮的真实事故就是这么来的。
//
// 回写用的是"中断态"（state.partial.sha256）：它只解锁"恢复现场 + 从终审
// 接着跑"，绝不能把终审判成已完成。
func TestFixPersistSurvivesRebuild(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	mustWrite := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 转换产物（未修）+ 修复会话改过的构建树。
	mustWrite(filepath.Join(proj, "work", "chapters", "c1.tex"), "\\input{c1/child.tex}\n")
	mustWrite(filepath.Join(build, "chapters", "c1.tex"), "\\input{chapters/c1/child.tex}\n")
	mustWrite(filepath.Join(build, "main.tex"), "\\documentclass{book}\n\\begin{document}\\input{chapters/c1.tex}\\end{document}\n")

	if err := persistFinalReviewPartial(proj, build); err != nil {
		t.Fatal(err)
	}
	// 1) 修复过的章节回写到 work/chapters（下次重建自带）。
	got, _ := os.ReadFile(filepath.Join(proj, "work", "chapters", "c1.tex"))
	if string(got) != "\\input{chapters/c1/child.tex}\n" {
		t.Fatalf("章节修订没回写：%q", got)
	}
	// 2) 顶层 .tex（main.tex）进 work/final_review。
	if _, err := os.Stat(filepath.Join(proj, "work", "final_review", "main.tex")); err != nil {
		t.Fatalf("顶层 main.tex 没回写：%v", err)
	}
	// 3) 指纹能被认出来 = 下次可以恢复，不再重修。
	if !finalReviewPartialCurrent(proj) {
		t.Fatal("回写后中断态指纹应当匹配")
	}
	// 4) 但绝不能当成"终审已完成"，否则会跳过终审。
	if finalReviewStateCurrent(proj) {
		t.Fatal("修复产物不该被当成终审完成态")
	}
	// 5) 重建树：恢复出来的 main.tex 是修过的那份，章节来自 work/chapters。
	rebuild := t.TempDir()
	mustWrite(filepath.Join(rebuild, "main.tex"), "\\documentclass{book}\n\\begin{document}\\end{document}\n")
	if err := copyDir(filepath.Join(proj, "work", "chapters"), filepath.Join(rebuild, "chapters")); err != nil {
		t.Fatal(err)
	}
	if err := applyFinalReviewState(proj, rebuild); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(rebuild, "main.tex")); len(b) == 0 || string(b) == "\\documentclass{book}\n\\begin{document}\\end{document}\n" {
		t.Fatalf("重建树没应用修复版 main.tex：%q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(rebuild, "chapters", "c1.tex")); string(b) != "\\input{chapters/c1/child.tex}\n" {
		t.Fatalf("重建树的章节不是修复版：%q", b)
	}
}
