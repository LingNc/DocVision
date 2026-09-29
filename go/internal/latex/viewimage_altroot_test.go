package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T104：view_image 的 AltRoots——Root 找不到时按原样路径去 project 根找
// 原书位图（source/images/<书>/<file>），终审/修复会话不必翻原书页。
func TestViewImageAltRoots(t *testing.T) {
	build := t.TempDir()
	proj := t.TempDir()
	img := filepath.Join(proj, "source", "images", "书A", "fig1.png")
	if err := os.MkdirAll(filepath.Dir(img), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: build, AltRoots: []AltRoot{{Label: "project", Dir: proj}}, BareSearch: true}

	// 三种写法都要命中同一文件。
	for _, arg := range []string{
		"source/images/书A/fig1.png", // 项目相对原样路径
		"images/书A/fig1.png",        // md 引用形态
		"fig1.png",                  // 裸名唯一匹配
	} {
		got, err := tool.resolve(arg)
		if err != nil {
			t.Fatalf("resolve(%q) 应命中 AltRoot: %v", arg, err)
		}
		if got != img {
			t.Fatalf("resolve(%q) = %q, want %q", arg, got, img)
		}
	}
	// Root 优先：build 树里有同名文件时不用 AltRoot。
	local := filepath.Join(build, "fig1.png")
	if err := os.WriteFile(local, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := tool.resolve("fig1.png"); err != nil || got != local {
		t.Fatalf("Root 必须优先于 AltRoot: got %q err %v", got, err)
	}
	// 越界拒绝：AltRoot 外的路径不能借道逃出。
	if _, err := tool.resolve("../../../etc/passwd"); err == nil {
		t.Fatal("越界路径必须拒绝")
	}
}
