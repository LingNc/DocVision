package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T210：拆分父节点的产物先落在自己工作区的暂存区（integrated-stage），核对通过
// 才晋级到 work/chapters。重新核对的判据必须认得这一份——活的核对会话正在看它，
// 手动重核却说"还没有提交产物"是同一份文件两套判据。
func TestChapterProductsFindsStagedParentProduct(t *testing.T) {
	proj := t.TempDir()
	stage := filepath.Join(proj, "work", "temp", "conv_chapter_004", "integrated-stage")
	if err := os.MkdirAll(filepath.Join(stage, "chapter_004", "children"), 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(stage, "chapter_004.tex")
	if err := os.WriteFile(tex, []byte("\\input{chapter_004/children/chapter_004_child_01.tex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotTex, gotParts := chapterProductsFor(proj, "chapter_004")
	if gotTex != tex {
		t.Fatalf("tex = %q, want %q", gotTex, tex)
	}
	if gotParts != filepath.Join(stage, "chapter_004") {
		t.Fatalf("parts = %q, want %q", gotParts, filepath.Join(stage, "chapter_004"))
	}
}

// 最终产物已经晋级时仍然优先用它。
func TestChapterProductsPrefersPromotedProduct(t *testing.T) {
	proj := t.TempDir()
	promoted := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(promoted, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(promoted, "chapter_004.tex")
	if err := os.WriteFile(want, []byte("\\section{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(proj, "work", "temp", "conv_chapter_004", "integrated-stage")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "chapter_004.tex"), []byte("\\input{stale}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := chapterProductsFor(proj, "chapter_004"); got != want {
		t.Fatalf("tex = %q, want 晋级的 %q", got, want)
	}
}
