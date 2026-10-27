package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T197：已提交产物的两个路径要认细分树——递归子章节的 .tex 在
// <父>/children/<子>.tex（层数不定），暂存目录 children.tmp 里的半成品不算。
func TestChapterProductsFor(t *testing.T) {
	proj := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(proj, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("work/chapters/chapter_002.tex")
	mk("work/chapters/chapter_014/children/chapter_014_child_04.tex")
	mk("work/chapters/chapter_014/children/chapter_014_child_04/children/chapter_014_child_04_child_01.tex")
	mk("work/chapters/chapter_014/children/children.tmp/chapter_014_child_09.tex")

	if tex, parts := chapterProductsFor(proj, "chapter_002"); tex != filepath.Join(proj, "work/chapters/chapter_002.tex") ||
		parts != filepath.Join(proj, "work/chapters/chapter_002") {
		t.Fatalf("顶层路径不对: %q %q", tex, parts)
	}
	if tex, parts := chapterProductsFor(proj, "chapter_014_child_04"); tex != filepath.Join(proj, "work/chapters/chapter_014/children/chapter_014_child_04.tex") ||
		parts != filepath.Join(proj, "work/chapters/chapter_014/children/chapter_014_child_04") {
		t.Fatalf("子章节路径不对: %q %q", tex, parts)
	}
	if tex, _ := chapterProductsFor(proj, "chapter_014_child_04_child_01"); tex != filepath.Join(proj, "work/chapters/chapter_014/children/chapter_014_child_04/children/chapter_014_child_04_child_01.tex") {
		t.Fatalf("三层子章节路径不对: %q", tex)
	}
	// 只有 children.tmp 里有同名文件 → 当作没有产物。
	if got, _ := chapterProductsFor(proj, "chapter_014_child_09"); got != filepath.Join(proj, "work/chapters/chapter_014_child_09.tex") {
		t.Fatalf("暂存产物不该算已提交: %q", got)
	}
}
