package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
)

func queueLoad(path string) (*queue.Queue, error) { return queue.Load(path) }

// TestGenClassifyMineruDiscovery：source/images 为空时自动发现 mineru/*/auto/images 的图。
func TestGenClassifyMineruDiscovery(t *testing.T) {
	root := t.TempDir()
	imgDir := filepath.Join(root, "mineru", "book_part1", "auto", "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.jpg", "b.png"} {
		if err := os.WriteFile(filepath.Join(imgDir, n), []byte("img-"+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := queueLoad(filepath.Join(root, "q.json"))
	rep, err := GenClassify(q, root, "source/images")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 2 {
		t.Fatalf("应从 mineru 直出图铺 2 任务: %+v", rep)
	}
	tk, _ := q.Get("cls_a")
	if tk.Inputs[0] != "inputs/a.jpg" {
		t.Fatalf("沙箱输入: %v", tk.Inputs)
	}
	// 沙箱里图已拷（自包含）
	if _, err := os.Stat(filepath.Join(root, tk.Sandbox, "inputs", "a.jpg")); err != nil {
		t.Fatal("图应拷进沙箱")
	}
}
