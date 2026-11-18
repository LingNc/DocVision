package gen

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// 铺一个已 done 的 classify 任务（verdict.json 由调用方决定内容）。
func seedClassify(t *testing.T, q *queue.Queue, root, stem, verdictJSON string) {
	t.Helper()
	sandbox := "tasks/cls_" + stem
	outDir := filepath.Join(root, sandbox, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "verdict.json"), []byte(verdictJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	imgDir := filepath.Join(root, sandbox, "inputs")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, stem+".png"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := q.Enqueue(&task.Task{
		ID: "cls_" + stem, Type: task.TypeClassify, Stage: "images", Sandbox: sandbox,
		Inputs: []string{"inputs/" + stem + ".png"}, Accept: ClassifySpec, Status: task.StatusDone,
		Created: now, Updated: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGenDraw(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	seedClassify(t, q, root, "figa", `{"class":"vector","reason":"线条图可重绘"}`)
	seedClassify(t, q, root, "figb", `{"class":"styled_text","extracted":"图注文字"}`)
	seedClassify(t, q, root, "figc", `{"class":"raster"}`)
	// done 但 verdict 损坏
	seedClassify(t, q, root, "figd", `{broken`)

	rep, err := GenDraw(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 1 || rep.TasksTotal != 1 {
		t.Fatalf("只有 figa 应驱动出 draw 任务: %+v", rep)
	}
	if len(rep.VectorStems) != 1 || rep.VectorStems[0] != "figa" {
		t.Fatalf("vector 清单错: %+v", rep.VectorStems)
	}
	if rep.StyledText != 1 || rep.Raster != 1 || rep.NoVerdict != 1 {
		t.Fatalf("分类计数错: %+v", rep)
	}
	// 图文件拷进 draw 沙箱（记忆泡泡自包含）
	if _, err := os.Stat(filepath.Join(root, "tasks/drw_figa/inputs/figa.png")); err != nil {
		t.Fatal("draw 沙箱应含图文件: ", err)
	}
	// 幂等
	rep2, err := GenDraw(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 0 {
		t.Fatalf("重复生成应幂等: %+v", rep2)
	}
}

func TestGenDrawWaitsForDone(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	// pending 状态的 classify（未验收）不应驱动 draw
	sandbox := "tasks/cls_figx"
	if err := os.MkdirAll(filepath.Join(root, sandbox, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, sandbox, "out", "verdict.json"), []byte(`{"class":"vector"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := q.Enqueue(&task.Task{
		ID: "cls_figx", Type: task.TypeClassify, Stage: "images", Sandbox: sandbox,
		Inputs: []string{}, Accept: ClassifySpec, Status: task.StatusPending, Created: now, Updated: now,
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := GenDraw(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 {
		t.Fatalf("未验收的 classify 不应驱动 draw: %+v", rep)
	}
}
