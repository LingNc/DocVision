package latex

import (
	"mineru-tools/internal/logger"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T97：pilot marker 存在即跳过（中断续跑不重复试点）；无 md 时静默返回。
func TestPilotMarkerSkips(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(pilotMarkerPath(proj)), 0o755); err != nil {
		t.Fatal(err)
	}
	lg, _ := logger.NewLogger("", "", 2)
	r := &Runner{log: lg}
	r.stylePilotPhase(proj) // 无 source md：静默返回，不写 marker
	if fileExists(pilotMarkerPath(proj)) {
		t.Fatal("没有 markdown 时不应写 marker")
	}
	if err := os.WriteFile(pilotMarkerPath(proj), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.stylePilotPhase(proj) // marker 存在：直接返回（无 md 也不报错）
}

// T97：buildPilotSample 拼合行段（1 起含端点）、裁越界、全无效时报错。
func TestBuildPilotSample(t *testing.T) {
	proj := t.TempDir()
	src := filepath.Join(proj, "source")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "l1\nl2\nl3\nl4\nl5\n"
	if err := os.WriteFile(filepath.Join(src, "book.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	lg, _ := logger.NewLogger("", "", 2)
	r := &Runner{log: lg}
	var spec pilotSpec
	spec.MDRanges = append(spec.MDRanges,
		struct {
			File  string `json:"file"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		}{"book.md", 2, 3},
		struct {
			File  string `json:"file"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		}{"book.md", 4, 99}) // 越界端点裁剪到文件尾
	spec.PDFPages = append(spec.PDFPages, struct {
		File  string `json:"file"`
		Pages []int  `json:"pages"`
	}{"book.pdf", []int{1, 5}})
	sample, pages, err := r.buildPilotSample(proj, &spec)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(sample)
	text := string(data)
	if !strings.Contains(text, "l2\nl3") || !strings.Contains(text, "l4\nl5") {
		t.Fatalf("行段拼合不对:\n%s", text)
	}
	if !strings.Contains(pages, "book.pdf") {
		t.Fatalf("页清单缺失: %q", pages)
	}
	// 全无效行段 → 报错。
	var bad pilotSpec
	bad.MDRanges = append(bad.MDRanges, struct {
		File  string `json:"file"`
		Start int    `json:"start"`
		End   int    `json:"end"`
	}{"missing.md", 1, 2})
	if _, _, err := r.buildPilotSample(proj, &bad); err == nil {
		t.Fatal("全无效行段必须报错")
	}
}
