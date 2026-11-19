package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// mkDoneClassify 造一个 done 且判 vector 的 classify 任务（沙箱 inputs 有图、out 有 verdict.json）。
func mkDoneClassify(root, stem string, q *queue.Queue) {
	id := "cls_" + stem
	sb := filepath.Join("tasks", id)
	os.MkdirAll(filepath.Join(root, sb, "inputs"), 0o755)
	os.MkdirAll(filepath.Join(root, sb, "out"), 0o755)
	os.WriteFile(filepath.Join(root, sb, "inputs", stem+".png"), []byte("img"), 0o644)
	os.WriteFile(filepath.Join(root, sb, "out", "verdict.json"), []byte(`{"class":"vector","confidence":0.9}`), 0o644)
	q.Enqueue(&task.Task{ID: id, Type: task.TypeClassify, Status: task.StatusDone, Sandbox: sb,
		Inputs: []string{"inputs/" + stem + ".png"}})
}

// 按章按需铺：allowedSHA 集合只铺集合内的 vector 图；nil=全书（行为同旧）。
func TestGenDrawChapterFilter(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	mkDoneClassify(root, "aaa111", q)
	mkDoneClassify(root, "bbb222", q)
	if err := q.Save(); err != nil {
		t.Fatal(err)
	}

	rep, err := GenDraw(q, root, map[string]bool{"aaa111": true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 1 || len(rep.VectorStems) != 1 || rep.VectorStems[0] != "aaa111" {
		t.Fatalf("应只铺章内 aaa111: %+v", rep)
	}

	// nil=全书：bbb222 也铺（幂等：aaa111 不重复）
	rep2, err := GenDraw(q, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 1 || len(rep2.VectorStems) != 2 {
		t.Fatalf("nil 集合应补铺 bbb222 共 2 矢量图: %+v", rep2)
	}
}
