package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
)

// T85：中断态与完成态是两份指纹——部分态不触发"已完成跳过"，
// 完成态覆盖后部分态作废。
func TestFinalReviewPartialState(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	mk := func(dir, name, body string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// work/chapters 与 build 树
	mk(filepath.Join(proj, "work", "chapters"), "ch1.tex", "chapter one")
	mk(filepath.Join(build, "chapters"), "ch1.tex", "chapter one EDITED")
	mk(build, "main.tex", "\\documentclass{book}")

	// 中断态：写入 partial 指纹
	if err := persistFinalReviewPartial(proj, build); err != nil {
		t.Fatalf("persist partial: %v", err)
	}
	// 完成态不应成立（state.sha256 不存在）
	if finalReviewStateCurrent(proj) {
		t.Fatal("部分态不应触发完成态跳过")
	}
	// 部分态成立（work/chapters 已被回写为 EDITED，指纹一致）
	if !finalReviewPartialCurrent(proj) {
		t.Fatal("回写后部分态指纹应当一致")
	}
	// 章节被重转换（内容变了）→ 部分态失效
	mk(filepath.Join(proj, "work", "chapters"), "ch1.tex", "chapter one reconverted")
	if finalReviewPartialCurrent(proj) {
		t.Fatal("章节变化后部分态应当失效")
	}
	// 完成态写入会清掉部分态
	mk(filepath.Join(proj, "work", "chapters"), "ch1.tex", "final")
	mk(filepath.Join(build, "chapters"), "ch1.tex", "final")
	if err := persistFinalReviewPartial(proj, build); err != nil {
		t.Fatal(err)
	}
	if err := persistFinalReviewState(proj, build); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, "work", "final_review", "state.partial.sha256")); !os.IsNotExist(err) {
		t.Fatal("完成态写入后部分态指纹应当被清除")
	}
	if !finalReviewStateCurrent(proj) {
		t.Fatal("完成态写入后 finalReviewStateCurrent 应当为真")
	}
}

// T85 续：progress.json 标 assemble done 但终审中断时，assembleNeedsResume
// 应放行重跑；完成态/无状态/终审关闭时不受影响。
func TestAssembleNeedsResume(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	mk := func(dir, name, body string) {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(proj, "work", "chapters"), "ch1.tex", "v1")
	mk(filepath.Join(build, "chapters"), "ch1.tex", "v1 edited")
	mk(build, "main.tex", "main")

	r := &Runner{cfg: &config.Config{}}
	// 无任何状态 → 不需要续跑（老书、T39 之前跑完的）
	if r.assembleNeedsResume(proj) {
		t.Fatal("无状态不应触发续跑")
	}
	// 中断态 → 需要续跑
	if err := persistFinalReviewPartial(proj, build); err != nil {
		t.Fatal(err)
	}
	if !r.assembleNeedsResume(proj) {
		t.Fatal("中断态应触发续跑")
	}
	// 完成态 → 不需要
	if err := persistFinalReviewState(proj, build); err != nil {
		t.Fatal(err)
	}
	if r.assembleNeedsResume(proj) {
		t.Fatal("完成态不应触发续跑")
	}
	// 终审关闭 → 不需要（先回到中断态场景）
	_ = os.Remove(filepath.Join(proj, "work", "final_review", stateHashFile))
	if err := persistFinalReviewPartial(proj, build); err != nil {
		t.Fatal(err)
	}
	off := false
	r2 := &Runner{cfg: &config.Config{}}
	r2.cfg.Latex.Compile.FinalReview = &off
	if r2.assembleNeedsResume(proj) {
		t.Fatal("终审关闭不应触发续跑")
	}
}
