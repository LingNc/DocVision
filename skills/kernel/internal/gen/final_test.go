package gen

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestGenFinal(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	// 无 convert done → 不触发
	rep, err := GenFinal(q)
	if err != nil || rep.Enqueued {
		t.Fatalf("无 convert 不应触发: %+v", rep)
	}
	// convert done 但 check 未完成 → 不触发
	now := time.Now()
	q.Enqueue(&task.Task{ID: "conv_a", Type: task.TypeConvert, Sandbox: "tasks/conv_a", Status: task.StatusDone, Created: now, Updated: now})
	rep, _ = GenFinal(q)
	if rep.Enqueued {
		t.Fatalf("check 未完不应触发: %+v", rep)
	}
	// check done → 触发，终审可领
	q.Enqueue(&task.Task{ID: "chk_a", Type: task.TypeCheck, Sandbox: "tasks/chk_a", Status: task.StatusDone, Created: now, Updated: now})
	rep, err = GenFinal(q)
	if err != nil || !rep.Enqueued {
		t.Fatalf("应触发终审: %+v %v", rep, err)
	}
	tk, err := q.Get("fin_book")
	if err != nil || tk.Type != task.TypeFinal || tk.Status != task.StatusPending {
		t.Fatalf("终审任务应直接可领: %+v %v", tk, err)
	}
	if tk.Accept["verdict_schema"] != "final" {
		t.Fatalf("schema 应为 final: %+v", tk.Accept)
	}
	// 幂等
	rep2, _ := GenFinal(q)
	if rep2.Enqueued {
		t.Fatal("重复生成应幂等")
	}
}

func mkTimeUnused() {}
