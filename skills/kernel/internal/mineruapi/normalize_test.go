package mineruapi

import (
	"os"
	"path/filepath"
	"testing"
)

// 新契约：<part>/auto/<data_id>/* → 摊平到 <part>/auto/*（内层空壳删除，不整目录搬）。
func TestNormalizeLayoutFlatten(t *testing.T) {
	root := t.TempDir()
	// 模拟中毒解压：多套一层 <part>/auto/<data_id>/
	inner := filepath.Join(root, "auto", "abc123")
	for _, d := range []string{filepath.Join(inner, "images")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(inner, "x_content_list.json"): "[]",
		filepath.Join(inner, "layout.json"):         "{}",
		filepath.Join(inner, "images", "a.jpg"):     "img",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	auto, err := NormalizeLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if auto != filepath.Join(root, "auto") {
		t.Fatalf("应归位到 <part>/auto: %s", auto)
	}
	// 摊平：内容直接在 <part>/auto/ 下，不套 <data_id>/
	for _, p := range []string{"x_content_list.json", "layout.json", "images/a.jpg"} {
		if _, err := os.Stat(filepath.Join(root, "auto", p)); err != nil {
			t.Fatalf("摊平后丢失 %s: %v", p, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "auto")); len(entries) != 3 {
		t.Fatalf("auto 下应只有 3 项（不残留 data_id 壳）: %d", len(entries))
	}
	// 幂等：标准布局再来一次
	auto2, err := NormalizeLayout(root)
	if err != nil || auto2 != auto {
		t.Fatalf("幂等: %s %v", auto2, err)
	}
}

// 上游直出 part 根布局（content_list 直接在 part 根、无 auto 层）：normalize 原样返回 part 根，
// 不得归位（归位会把产物搬进 auto/ 造成错位）。cmd 层特判之外，函数自身也必须安全。
func TestNormalizeLayoutPartRootUntouched(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x_content_list.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeLayout(root)
	if err != nil || got != root {
		t.Fatalf("part 根布局应原样返回: %s %v", got, err)
	}
}

// 残留中毒 auto/（内容不全）+ part 根有完整 content_list：取最浅命中，part 根胜出，不动。
func TestNormalizeLayoutShallowestWins(t *testing.T) {
	root := t.TempDir()
	// part 根完整产物
	os.MkdirAll(filepath.Join(root, "images"), 0o755)
	os.WriteFile(filepath.Join(root, "y_content_list.json"), []byte("[]"), 0o644)
	os.WriteFile(filepath.Join(root, "images", "b.jpg"), []byte("img"), 0o644)
	// 残留中毒 auto/（内层缺 images）
	inner := filepath.Join(root, "auto", "abc123")
	os.MkdirAll(inner, 0o755)
	os.WriteFile(filepath.Join(inner, "y_content_list.json"), []byte("[]"), 0o644)
	got, err := NormalizeLayout(root)
	if err != nil || got != root {
		t.Fatalf("应取 part 根（最浅命中）: %s %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "images", "b.jpg")); err != nil {
		t.Fatalf("part 根产物被误动: %v", err)
	}
}

func TestNormalizeLayoutMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := NormalizeLayout(root); err == nil {
		t.Fatal("无 content_list 应报错")
	}
}
