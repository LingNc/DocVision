package gen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestCollectImg2Text(t *testing.T) {
	root := t.TempDir()
	idx := &docindex.DocIndex{Sizes: map[string][2]float64{}}
	idx.Entries = []docindex.DocEntry{
		{Seq: 7, Part: "p", Page: 0, Global: 3, Type: "image", Img: "images/x.jpg"},
	}
	writeIndex(t, root, idx)
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	// done 的 img2text 任务（entry_seq=7）
	outDir := filepath.Join(root, "tasks/img2txt_7/out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "description.md"), []byte("流程图：开始→判断→结束"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(&task.Task{
		ID: "img2txt_7", Type: task.TypeImg2Text, Sandbox: "tasks/img2txt_7",
		Accept: map[string]any{"entry_seq": float64(7)},
		Status: task.StatusDone, Verdicts: []task.Verdict{{Pass: true}},
	}); err != nil {
		t.Fatal(err)
	}
	n, err := CollectImg2Text(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应回填 1: %d", n)
	}
	// 重载验证 + 幂等（已有 Content 不覆盖）
	loaded, err := docindex.Load(filepath.Join(root, "doc_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Entries[0].Content != "流程图：开始→判断→结束" {
		t.Fatalf("Content 应回填: %q", loaded.Entries[0].Content)
	}
	n2, err := CollectImg2Text(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("已有 Content 应幂等跳过: %d", n2)
	}
}
