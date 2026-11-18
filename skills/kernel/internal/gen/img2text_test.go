package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func writeIndex(t *testing.T, root string, idx *docindex.DocIndex) {
	t.Helper()
	b, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc_index.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenImg2Text(t *testing.T) {
	root := t.TempDir()
	// 造索引：3 个图条目（其中两个同 img 去重）+ 文字条目
	idx := &docindex.DocIndex{Sizes: map[string][2]float64{}}
	entries := []docindex.DocEntry{
		{Seq: 1, Part: "p", Page: 0, Global: 1, Type: "text", Text: "前文"},
		{Seq: 2, Part: "p", Page: 0, Global: 1, Type: "image", Img: "images/a.jpg", BBox: [4]float64{1, 2, 3, 4}},
		{Seq: 3, Part: "p", Page: 0, Global: 1, Type: "image", Img: "images/a.jpg"}, // 同图去重
		{Seq: 4, Part: "p", Page: 1, Global: 2, Type: "image", Img: "images/b.jpg"},
		{Seq: 5, Part: "p", Page: 1, Global: 2, Type: "text", Text: "后文"},
	}
	idx.Entries = entries
	writeIndex(t, root, idx)
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	rep, err := GenImg2Text(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 2 || rep.TasksTotal != 2 { // a.jpg 去重后 + b.jpg
		t.Fatalf("应铺 2 个任务: %+v", rep)
	}
	tk, _ := q.Get("img2txt_2")
	if tk.Accept["entry_seq"] != float64(2) || tk.Accept["global"] != float64(1) {
		t.Fatalf("任务包应带指针: %+v", tk.Accept)
	}
	if tk.Accept["img"] != "images/a.jpg" {
		t.Fatalf("图指针错: %+v", tk.Accept)
	}
	// 幂等
	rep2, err := GenImg2Text(q, root)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 0 {
		t.Fatalf("重复生成应幂等: %+v", rep2)
	}
}

func TestNeighborBlocks(t *testing.T) {
	// ctx-up/ctx-down 的数据源：按 seq 取邻块（实现内置于 dvagent，这里测纯逻辑）
	root := t.TempDir()
	idx := &docindex.DocIndex{Sizes: map[string][2]float64{}}
	idx.Entries = []docindex.DocEntry{
		{Seq: 10, Type: "text", Text: "前前", Global: 3},
		{Seq: 11, Type: "text", Text: "前一", Global: 3},
		{Seq: 12, Type: "image", Img: "images/x.jpg", Global: 3},
		{Seq: 13, Type: "text", Text: "后一", Global: 3},
	}
	writeIndex(t, root, idx)
	loaded, err := docindex.Load(filepath.Join(root, "doc_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	// up=seq 更小，down=seq 更大
	var ups, downs []docindex.DocEntry
	for _, e := range loaded.Entries {
		if e.Type == "image" {
			continue
		}
		if e.Seq < 12 {
			ups = append(ups, e)
		} else {
			downs = append(downs, e)
		}
	}
	if len(ups) != 2 || len(downs) != 1 {
		t.Fatalf("邻块切分错: up=%d down=%d", len(ups), len(downs))
	}
	_ = os.Getenv
	_ = task.TypeImg2Text
}
