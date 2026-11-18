package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
)

func TestGenClassify(t *testing.T) {
	root := t.TempDir()
	imgDir := filepath.Join(root, "source", "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a.png", "b.jpg", "c.png"} {
		if err := os.WriteFile(filepath.Join(imgDir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	rep, err := GenClassify(q, root, "source/images")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 3 || rep.TasksTotal != 3 {
		t.Fatalf("应新入队 3: %+v", rep)
	}
	// 确定性：任务 id 按文件名排序生成，claim 顺序可复现
	got := q.Claim("x", 5, 60)
	if len(got) != 3 || got[0].ID != "cls_a" || got[1].ID != "cls_b" || got[2].ID != "cls_c" {
		t.Fatalf("派发顺序应确定: %+v", got)
	}
	// 幂等：再生成一次 Added=0
	q2, _ := queue.Load(filepath.Join(root, "q.json")) // 重新加载（claim 已改状态，但任务仍在）
	rep2, err := GenClassify(q2, root, "source/images")
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 0 || rep2.TasksTotal != 3 {
		t.Fatalf("重复生成应幂等: %+v", rep2)
	}
}

func TestGenClassifyMissingDir(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	rep, err := GenClassify(q, root, "nope/images")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 {
		t.Fatalf("缺目录应零新增: %+v", rep)
	}
}
