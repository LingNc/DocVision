package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T281：分块单位是**顶层章节**。细分父章节的子节名混进块里会让分块会话去找
// 一份不存在的产物、把那一章悄悄跳过。
func TestBlocksRejectSubChapterNames(t *testing.T) {
	dir := t.TempDir()
	known := []string{"chapter_010", "chapter_012"}
	blocks := NewBlocksTool(filepath.Join(dir, "blocks.json"), known...)

	res, err := blocks.Execute(`{"op":"add","title":"行距","problem_ids":["P1"],"chapters":["chapter_010_child_03"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "不是可分的章节名") || !strings.Contains(res.Text, "chapter_010") {
		t.Fatalf("子节名应当被拒并指出可用名: %q", res.Text)
	}
	res, err = blocks.Execute(`{"op":"add","title":"行距","problem_ids":["P1"],"chapters":["chapter_010"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "REJECTED") || strings.Contains(res.Text, "不是可分") {
		t.Fatalf("父章节名应当被接受: %q", res.Text)
	}

	// 直接编辑 blocks.json 再提交的路径也要过同一道门。
	sub := &SubmitBlocksTool{Workspace: dir, Known: known}
	if err := os.WriteFile(filepath.Join(dir, "blocks.json"),
		[]byte(`{"blocks":[{"title":"x","problem_ids":["P1"],"chapters":["chapter_012_child_02"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := sub.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "REJECTED") || sub.Set {
		t.Fatalf("提交时应当拒绝子节名: %q set=%v", out.Text, sub.Set)
	}
}

// 细分父章节看得出来：<章>/children/ 在就是。
func TestSubdividedChaptersDetection(t *testing.T) {
	proj := t.TempDir()
	chap := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(filepath.Join(chap, "chapter_010", "children"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(chap, "chapter_011", "q1"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"chapter_010", "chapter_011"} {
		if err := os.WriteFile(filepath.Join(chap, b+".tex"), []byte("\\input{x}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := subdividedChapters(proj, []string{"chapter_010", "chapter_011", "chapter_012"})
	if len(got) != 1 || got[0] != "chapter_010" {
		t.Fatalf("只有带 children/ 的才算细分父章节: %v", got)
	}
	if note := subdividedBlockNote(proj, []string{"chapter_011"}); note != "" {
		t.Fatalf("没有细分章节时不该加这句: %q", note)
	}
	note := subdividedBlockNote(proj, []string{"chapter_010"})
	if !strings.Contains(note, "chapters/chapter_010/children/") {
		t.Fatalf("应当点名正文目录: %q", note)
	}
}
