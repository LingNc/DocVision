package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestCollectStyleMaterials(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	seedClassifyWithVerdict(t, q, root, "figs", `{"class":"styled_text","extracted":"第一行题字"}`)
	seedClassifyWithVerdict(t, q, root, "figv", `{"class":"vector","reason":"x"}`)
	rep, err := CollectStyleMaterials(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Collected != 1 || len(rep.Materials) != 1 || rep.Materials[0] != "cls_figs.md" {
		t.Fatalf("只应收 styled_text: %+v", rep)
	}
	body, err := os.ReadFile(filepath.Join(root, "style", "styled_text", "cls_figs.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "第一行题字") || !strings.Contains(s, "cls_figs") {
		t.Fatalf("素材内容/溯源头错: %.120s", s)
	}
}

func seedClassifyWithVerdict(t *testing.T, q *queue.Queue, root, stem, verdict string) {
	t.Helper()
	sandbox := "tasks/cls_" + stem
	if err := os.MkdirAll(filepath.Join(root, sandbox, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, sandbox, "out", "verdict.json"), []byte(verdict), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, sandbox, "inputs"), 0o755); err != nil {
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
