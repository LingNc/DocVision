package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T33：deliverBook 先 RemoveAll(outDir) 再整树拷贝。顶层文件按字典序
// 排在目录前面（REPORT.md 的大写 R < chapters 的小写 c），out/ 若不在
// 拷贝前重建，第一个顶层文件就以 ENOENT 中止整本书的最后一步交付。
func TestDeliverBookRecreatesOutDir(t *testing.T) {
	base := t.TempDir()
	build := filepath.Join(base, "build")
	out := filepath.Join(base, "out")
	// 布局复刻真实失败现场：顶层有 REPORT.md（大写开头）和 chapters/。
	for _, d := range []string{build, filepath.Join(build, "chapters"), out} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"REPORT.md":          "report",
		"main.tex":           "tex",
		"chapters/chap1.tex": "body",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(build, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := deliverBook(build, out); err != nil {
		t.Fatalf("deliverBook: %v", err)
	}
	for name, want := range files {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}
}
