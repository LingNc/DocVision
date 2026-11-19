package prep

import (
	"os"
	"path/filepath"
	"testing"
)

func seedPart(t *testing.T, mineruOut, name string, contentListJSON string) string {
	t.Helper()
	auto := filepath.Join(mineruOut, name, "auto")
	if err := os.MkdirAll(auto, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(auto, name+"_content_list.json"), []byte(contentListJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return auto
}

func TestOrganizePart(t *testing.T) {
	out := t.TempDir()
	cl := `[
	  {"type":"text","page_idx":0,"text":"第一章 引论"},
	  {"type":"image","page_idx":0,"img_path":"images/abc.jpg","image_caption":["图 1-1 决策树"]},
	  {"type":"equation","page_idx":0,"text":"E = mc^2"},
	  {"type":"interline_equation","page_idx":1,"text":"x^2+y^2=1"}
	]`
	auto := seedPart(t, out, "book_part1", cl)
	md, err := OrganizePart(auto)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"第一章 引论",
		"![图 1-1 决策树](images/abc.jpg)",
		"$$E = mc^2$$",
		"$$x^2+y^2=1$$",
	} {
		if !contains(md, want) {
			t.Fatalf("md 缺 %q:\n%s", want, md)
		}
	}
}

func TestOrganizeWholeBook(t *testing.T) {
	out := t.TempDir()
	auto1 := seedPart(t, out, "book_part1", `[{"type":"text","page_idx":0,"text":"第一 part"}]`)
	auto2 := seedPart(t, out, "book_part2", `[{"type":"text","page_idx":0,"text":"第二 part"}]`)
	mdDir := filepath.Join(out, "md")
	outs, err := Organize([]string{auto1, auto2}, mdDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 3 { // part1.md + part2.md + book.md
		t.Fatalf("应写出 3 个 md: %v", outs)
	}
	book, err := os.ReadFile(filepath.Join(mdDir, "book.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(book)
	i1, i2 := indexOf(s, "第一 part"), indexOf(s, "第二 part")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Fatalf("整本 md 应按 part 顺序拼接:\n%s", s)
	}
}

// SK-I17：part 根输入（无 auto 层）时命名应取 part 目录名，而非父目录 mineru。
func TestPartStemPartRoot(t *testing.T) {
	root := t.TempDir()
	part := filepath.Join(root, "mineru", "book_part1")
	os.MkdirAll(filepath.Join(part, "images"), 0o755)
	os.WriteFile(filepath.Join(part, "book_part1_content_list.json"), []byte("[]"), 0o644)
	mdDir := filepath.Join(root, "chapters")
	os.MkdirAll(mdDir, 0o755)
	mds, err := Organize([]string{part}, mdDir) // 直接传 part 根（模拟 auto 缺失回退）
	if err != nil {
		t.Fatal(err)
	}
	if !hasBase(mds, "book_part1.md") {
		t.Fatalf("part 根输入应命名 book_part1.md（SK-I17），实: %v", mds)
	}
	// 旧布局 <part>/auto 也应同名（main.go 发现逻辑：auto 存在传 <part>/auto）
	part2 := filepath.Join(root, "mineru", "book_part2", "auto")
	os.MkdirAll(filepath.Join(part2, "images"), 0o755)
	os.WriteFile(filepath.Join(part2, "book_part2_content_list.json"), []byte("[]"), 0o644)
	mds2, err := Organize([]string{part2}, mdDir)
	if err != nil {
		t.Fatal(err)
	}
	if !hasBase(mds2, "book_part2.md") {
		t.Fatalf("auto 旧布局应命名 book_part2.md，实: %v", mds2)
	}
}

func hasBase(paths []string, want string) bool {
	for _, p := range paths {
		if filepath.Base(p) == want {
			return true
		}
	}
	return false
}
