package sessionview

import (
	"os"
	"path/filepath"
	"testing"
)

// T210：/api/retry 的「重新核对」前置检查与 latex 侧共用同一个判据——拆分父节点
// 的产物在暂存区等核对，这一份必须认。
func TestChapterTexInFindsStagedParentProduct(t *testing.T) {
	proj := t.TempDir()
	stage := filepath.Join(proj, "work", "temp", "conv_chapter_004", "integrated-stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(stage, "chapter_004.tex")
	if err := os.WriteFile(tex, []byte("\\input{chapter_004/children/chapter_004_child_01.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := chapterTexIn(proj, "chapter_004"); got != tex {
		t.Fatalf("chapterTexIn = %q, want %q", got, tex)
	}
}

// 目录在、文件不在时不能凭空认产物。
func TestChapterTexInIgnoresEmptyStagingDir(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "work", "temp", "conv_chapter_004", "integrated-stage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := chapterTexIn(proj, "chapter_004"); got != "" {
		t.Fatalf("chapterTexIn = %q, want 空", got)
	}
}
