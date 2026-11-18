package verify

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestCheckDrift(t *testing.T) {
	root := t.TempDir()
	// done 任务带哈希快照（构造等价于 verdict 验收时的快照）
	sandbox := "tasks/conv_a"
	outDir := filepath.Join(root, sandbox, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "a.tex"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用 Checker.Run 真实产出快照（figure_ref 无 → 快照照出）
	tk := &task.Task{ID: "conv_a", Sandbox: sandbox, Accept: map[string]any{}}
	c := &Checker{ProjectRoot: root}
	_, notes := c.Run(tk)
	tk.Status = task.StatusDone
	tk.Verdicts = []task.Verdict{{At: time.Now(), Pass: true, Notes: notes}}

	drifts := CheckDrift(map[string]*task.Task{"conv_a": tk}, root)
	if len(drifts) != 0 {
		t.Fatalf("未动过应零漂移: %+v", drifts)
	}
	// 篡改产物 → 漂移被抓
	if err := os.WriteFile(filepath.Join(outDir, "a.tex"), []byte("v2-changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifts = CheckDrift(map[string]*task.Task{"conv_a": tk}, root)
	if len(drifts) != 1 || len(drifts[0].Mismatched) != 1 || drifts[0].Mismatched[0] != "a.tex" {
		t.Fatalf("篡改应被抓: %+v", drifts)
	}
	// 删除产物 → missing
	if err := os.Remove(filepath.Join(outDir, "a.tex")); err != nil {
		t.Fatal(err)
	}
	drifts = CheckDrift(map[string]*task.Task{"conv_a": tk}, root)
	if len(drifts) != 1 || len(drifts[0].Missing) != 1 {
		t.Fatalf("删除应报 missing: %+v", drifts)
	}
	// 无快照 → NoSnapshot
	tk2 := &task.Task{ID: "conv_b", Sandbox: "tasks/conv_b", Status: task.StatusDone,
		Verdicts: []task.Verdict{{At: time.Now(), Pass: true, Notes: map[string]string{}}}}
	drifts = CheckDrift(map[string]*task.Task{"conv_b": tk2}, root)
	if len(drifts) != 1 || !drifts[0].NoSnapshot {
		t.Fatalf("无快照应标记 NoSnapshot: %+v", drifts)
	}
}
