package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// 造两张相似 PNG + ids 登记 + classify verdict，跑 Build。
func TestViewerBuild(t *testing.T) {
	root := t.TempDir()
	imgDir := filepath.Join(root, "source", "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(imgDir, "a.png"))
	writeTestPNG(t, filepath.Join(imgDir, "b.png"))
	tbl, _ := ids.Load(root)
	tbl.Register(ids.KindImg, "shaA", "source/images/a.png", 1, [4]float64{}, "cls_a")
	tbl.Register(ids.KindImg, "shaB", "source/images/b.png", 1, [4]float64{}, "cls_b")
	if err := tbl.Save(); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	q.Enqueue(&task.Task{ID: "cls_a", Type: task.TypeClassify, Sandbox: "tasks/cls_a", Status: task.StatusDone,
		Verdicts: []task.Verdict{{Pass: true}}})
	q.Enqueue(&task.Task{ID: "cls_b", Type: task.TypeClassify, Sandbox: "tasks/cls_b", Status: task.StatusDone,
		Verdicts: []task.Verdict{{Pass: true}}})
	for _, stem := range []string{"a", "b"} {
		outDir := filepath.Join(root, "tasks/cls_"+stem, "out")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		cls := "vector"
		if stem == "b" {
			cls = "styled_text"
		}
		body := "{\"class\":\"" + cls + "\"}"
		if err := os.WriteFile(filepath.Join(outDir, "verdict.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Build(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Images) != 2 || rep.Progress.Classified != 2 || rep.Progress.Vector != 1 || rep.Progress.StyledText != 1 {
		t.Fatalf("图清单/进度错: %+v", rep.Progress)
	}
	data, err := os.ReadFile(rep.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"img001", "img002", "看图台", "regroup", "viewer-apply", "vector"} {
		if !contains(s, want) {
			t.Fatalf("HTML 缺 %s", want)
		}
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// 1x1 白点 PNG（最小合法文件）
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00,
		0x00, 0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
