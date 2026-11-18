package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func TestPdfPageSizeMM(t *testing.T) {
	info := "Title:          x\nPage size:      595 x 842 pts (A4)\n"
	w, h := pdfPageSizeMM(info)
	if w < 209 || w > 211 || h < 296 || h > 298 {
		t.Fatalf("A4 尺寸换算错: %f x %f mm", w, h)
	}
	if w2, h2 := pdfPageSizeMM("no size here"); w2 != 0 || h2 != 0 {
		t.Fatal("无尺寸行应返回 0")
	}
}

func TestCheckFigureSkipsWithoutSpec(t *testing.T) {
	c := &Checker{ProjectRoot: t.TempDir()}
	tk := &task.Task{ID: "t", Sandbox: "tasks/t", Accept: map[string]any{}}
	if err := os.MkdirAll(filepath.Join(c.ProjectRoot, "tasks/t/out"), 0o755); err != nil {
		t.Fatal(err)
	}
	errs, notes := c.checkFigure(tk, filepath.Join(c.ProjectRoot, "tasks/t"))
	if len(errs) != 0 || len(notes) != 0 {
		t.Fatalf("无 figure_ref 应静默跳过: %v %v", errs, notes)
	}
}

func TestCheckFigureMissingRef(t *testing.T) {
	root := t.TempDir()
	tk := &task.Task{ID: "t", Sandbox: "tasks/t", Accept: map[string]any{"figure_ref": "nope.png"}}
	if err := os.MkdirAll(filepath.Join(root, "tasks/t/out"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Checker{ProjectRoot: root}
	errs, _ := c.checkFigure(tk, filepath.Join(root, "tasks/t"))
	if len(errs) == 0 || !containsStr(errs, "原图不存在") {
		t.Fatalf("原图缺失应打回: %v", errs)
	}
}

func TestCheckFigureMissingPDF(t *testing.T) {
	root := t.TempDir()
	// 原图就位（白底 40×40 中央 20×20 黑块，PNG）
	img := newWhiteImage(40, 40)
	fillBlackRect(img, 10, 10, 20, 20)
	refPath := filepath.Join(root, "source", "images", "figa.png")
	if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := savePNG(img, refPath); err != nil {
		t.Fatal(err)
	}
	tk := &task.Task{ID: "t", Sandbox: "tasks/t", Accept: map[string]any{
		"figure_ref": "source/images/figa.png",
	}}
	if err := os.MkdirAll(filepath.Join(root, "tasks/t/out"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Checker{ProjectRoot: root}
	errs, _ := c.checkFigure(tk, filepath.Join(root, "tasks/t"))
	if len(errs) == 0 || !containsStr(errs, "figure.pdf") {
		t.Fatalf("缺 figure.pdf 应打回: %v", errs)
	}
	// 内核临时目录应被清理（不留 .kv）
	if _, err := os.Stat(filepath.Join(root, "tasks/t/.kv")); !os.IsNotExist(err) {
		t.Fatal(".kv 应已清理")
	}
}
