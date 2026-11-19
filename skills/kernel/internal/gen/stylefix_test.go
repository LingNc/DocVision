package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestGenStylefixAssessGateAndIdempotent(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	q.Enqueue(&task.Task{ID: "cv1", Type: task.TypeConvert, Status: task.StatusPending})
	if _, err := GenStylefixAssess(q); err == nil {
		t.Fatal("有未竟 convert 应拒绝")
	}
	q.Tasks["cv1"].Status = task.StatusDone
	q.Enqueue(&task.Task{ID: "ck1", Type: task.TypeCheck, Status: task.StatusDone})
	rep, err := GenStylefixAssess(q)
	if err != nil || rep.AssessAdded != 1 {
		t.Fatalf("应铺 1 个 assess: %+v %v", rep, err)
	}
	rep2, err := GenStylefixAssess(q)
	if err != nil || rep2.AssessAdded != 0 || rep2.AssessSkipped == "" {
		t.Fatalf("幂等: %+v %v", rep2, err)
	}
}

func TestGenStylefixBlocksFromAssess(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	q.Enqueue(&task.Task{ID: "cv1", Type: task.TypeConvert, Status: task.StatusDone})
	rep, err := GenStylefixAssess(q)
	if err != nil || rep.AssessAdded != 1 {
		t.Fatal(err)
	}
	if _, err := GenStylefixBlocks(q, root); err == nil {
		t.Fatal("assess 未 done 应拒绝")
	}
	sb := filepath.Join(root, "tasks", "sfx_assess", "out")
	os.MkdirAll(sb, 0o755)
	os.WriteFile(filepath.Join(sb, "blocks.json"),
		[]byte(`[{"block_id":"b1","problems":["页边距"],"chapters":["chapter_001"]},{"block_id":"b2","problems":["标题字号"],"chapters":["chapter_002"]}]`), 0o644)
	q.Tasks["sfx_assess"].Status = task.StatusDone
	rep2, err := GenStylefixBlocks(q, root)
	if err != nil || rep2.BlocksAdded != 2 {
		t.Fatalf("应铺 2 块: %+v %v", rep2, err)
	}
	rep3, err := GenStylefixBlocks(q, root)
	if err != nil || rep3.BlocksAdded != 0 || len(rep3.Skipped) != 2 {
		t.Fatalf("幂等: %+v %v", rep3, err)
	}
}
