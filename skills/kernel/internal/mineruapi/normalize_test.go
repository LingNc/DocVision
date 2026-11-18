package mineruapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeLayoutNested(t *testing.T) {
	root := t.TempDir()
	// 模拟 zip 解压出的多层结构：<dst>/<stem>/auto/...
	inner := filepath.Join(root, "book_part1", "auto")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "book_part1_content_list.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "layout.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	auto, err := NormalizeLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if auto != filepath.Join(root, "auto") {
		t.Fatalf("应归位到 <dst>/auto: %s", auto)
	}
	if _, err := os.Stat(filepath.Join(root, "auto", "book_part1_content_list.json")); err != nil {
		t.Fatal("content_list 应在归位后目录")
	}
	// 幂等：标准布局再来一次
	auto2, err := NormalizeLayout(root)
	if err != nil || auto2 != auto {
		t.Fatalf("幂等: %s %v", auto2, err)
	}
}

func TestNormalizeLayoutMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := NormalizeLayout(root); err == nil {
		t.Fatal("无 content_list 应报错")
	}
}
