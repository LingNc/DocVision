package gen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/viewer"
)

func TestGenDrawRespectsOverride(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	// 两张 vector 图（a=代表，b=组员）
	seedVectorClassify(t, q, root, "figa")
	seedVectorClassify(t, q, root, "figb")
	// 覆盖分组：img001 为代表，img002 跟随
	tbl, _ := ids.Load(root)
	_, _ = tbl.Register(ids.KindImg, "shaA", "source/images/figa.png", 1, [4]float64{}, "cls_figa")
	_, _ = tbl.Register(ids.KindImg, "shaB", "source/images/figb.png", 1, [4]float64{}, "cls_figb")
	if err := tbl.Save(); err != nil {
		t.Fatal(err)
	}
	ov := &viewer.OverrideGroups{Threshold: 0.99, Groups: []viewer.GroupCard{
		{Rep: "img001", Members: []string{"img001", "img002"}},
	}}
	if err := os.MkdirAll(filepath.Join(root, "work", "viewer"), 0o755); err != nil {
		t.Fatal(err)
	}
	ovPath := viewer.OverridePath(root)
	writeJSON(t, ovPath, ov)

	rep, err := GenDraw(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 1 || rep.Followers != 1 {
		t.Fatalf("只应为代表铺任务（组员跟随）: %+v", rep)
	}
	if _, err := q.Get("drw_figa"); err != nil {
		t.Fatalf("代表任务应有: %v", err)
	}
	if _, err := q.Get("drw_figb"); err == nil {
		t.Fatal("组员不应有独立任务")
	}
	// 无覆盖时：两图各铺
	os.Remove(ovPath)
	q2, _ := queue.Load(filepath.Join(root, "q.json"))
	rep2, err := GenDraw(q2, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 1 { // figa 已存在（幂等），只新增 figb
		t.Fatalf("无覆盖时应补齐组员任务: %+v", rep2)
	}
	if _, err := q2.Get("drw_figb"); err != nil {
		t.Fatalf("figb 任务应补上: %v", err)
	}
}

func seedVectorClassify(t *testing.T, q *queue.Queue, root, stem string) {
	t.Helper()
	sandbox := "tasks/cls_" + stem
	for _, d := range []string{"inputs", "out"} {
		if err := os.MkdirAll(filepath.Join(root, sandbox, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, sandbox, "out", "verdict.json"),
		[]byte("{\"class\":\"vector\",\"reason\":\"x\"}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, sandbox, "inputs", stem+".png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := q.Enqueue(&task.Task{
		ID: "cls_" + stem, Type: task.TypeClassify, Sandbox: sandbox,
		Inputs: []string{"inputs/" + stem + ".png"}, Accept: ClassifySpec,
		Status: task.StatusDone, Created: now, Updated: now,
		Verdicts: []task.Verdict{{Pass: true}},
	}); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := jsonMarshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
