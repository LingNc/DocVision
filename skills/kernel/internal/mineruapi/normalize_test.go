package mineruapi

import (
	"os"
	"path/filepath"
	"testing"
)

// 契约：一切形态收敛到 part 根 <part>/{content_list, images/}，无 auto 层。
// ①上游 zip 带顶层文件夹（<part>/<uuid>/{content_list, images}）→ 上移合并进 part 根。
func TestNormalizeConvergeNested(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "auto", "abc123")
	os.MkdirAll(filepath.Join(inner, "images"), 0o755)
	os.WriteFile(filepath.Join(inner, "x_content_list.json"), []byte("[]"), 0o644)
	os.WriteFile(filepath.Join(inner, "layout.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(inner, "images", "a.jpg"), []byte("img"), 0o644)
	got, err := NormalizeLayout(root)
	if err != nil || got != root {
		t.Fatalf("应返回 part 根: %s %v", got, err)
	}
	for _, p := range []string{"x_content_list.json", "layout.json", "images/a.jpg"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Fatalf("part 根缺 %s: %v", p, err)
		}
	}
	if dirExists(filepath.Join(root, "auto")) {
		t.Fatal("auto 壳应已删除")
	}
}

// ②直出 part 根：幂等，不建 auto。
func TestNormalizeConvergeIdempotent(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "y_content_list.json"), []byte("[]"), 0o644)
	got, err := NormalizeLayout(root)
	if err != nil || got != root {
		t.Fatalf("幂等应原样: %s %v", got, err)
	}
	if dirExists(filepath.Join(root, "auto")) {
		t.Fatal("不应创建 auto 层")
	}
}

// ③混合残留：part 根已有完整产物 + 半截 auto/（缺 images）→ part 根优先，auto 壳清除。
func TestNormalizeConvergeMixed(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "images"), 0o755)
	os.WriteFile(filepath.Join(root, "z_content_list.json"), []byte("[]"), 0o644)
	os.WriteFile(filepath.Join(root, "images", "keep.jpg"), []byte("img"), 0o644)
	inner := filepath.Join(root, "auto", "abc123")
	os.MkdirAll(inner, 0o755)
	os.WriteFile(filepath.Join(inner, "z_content_list.json"), []byte("stale"), 0o644)
	got, err := NormalizeLayout(root)
	if err != nil || got != root {
		t.Fatalf("应返回 part 根: %s %v", got, err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "z_content_list.json")); string(data) != "[]" {
		t.Fatal("part 根既有产物被覆盖")
	}
	if dirExists(filepath.Join(root, "auto")) {
		t.Fatal("auto 壳应已删除")
	}
}

// ④无 content_list → 报错。
func TestNormalizeMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := NormalizeLayout(root); err == nil {
		t.Fatal("无 content_list 应报错")
	}
}
