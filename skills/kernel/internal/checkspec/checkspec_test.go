package checkspec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsNoVerdict(t *testing.T) {
	for _, s := range []string{
		"核对始终没有给出结论：checker 跑了 3 轮没有提交",
		"checker API failure: 429",
		"checker 会话已退化（连续 8 轮无工具调用）",
		"checker 只读视图无法准备",
	} {
		if !IsNoVerdict(s) {
			t.Fatalf("应判为无结论: %q", s)
		}
	}
	for _, s := range []string{
		"第3节公式漏排（chapter_003.md 第3节 ↔ ch003.tex）",
		"封面页眉样式与 cls 不一致",
		"",
	} {
		if IsNoVerdict(s) {
			t.Fatalf("不应判为无结论: %q", s)
		}
	}
}

func TestRecheckPending(t *testing.T) {
	dir := t.TempDir()
	tex := filepath.Join(dir, "chapter_003.tex")

	// 无 .checker 文件 → 不重核
	if RecheckPending(tex) {
		t.Fatal("无结论文件时不应挂起重核")
	}
	// 有判定 → 不重核
	if err := WriteVerdict(tex, "第3节公式漏排（md 第3节 ↔ tex）"); err != nil {
		t.Fatal(err)
	}
	if RecheckPending(tex) {
		t.Fatal("有判定时不应挂起重核")
	}
	// 无结论文案 → 挂起重核（T222：不能静默采纳没被判定过的产物）
	if err := WriteVerdict(tex, "checker API failure: timeout"); err != nil {
		t.Fatal(err)
	}
	if !RecheckPending(tex) {
		t.Fatal("无结论产物应挂起重核")
	}
	// pass 约定：空串不算无结论
	if err := os.WriteFile(VerdictPath(tex), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if RecheckPending(tex) {
		t.Fatal("空结论（pass）不应挂起重核")
	}
}
