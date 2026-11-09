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
	if err := r.stylePilotPhase(proj); err != nil { // 无 source md：静默返回，不写 marker
		t.Fatalf("没有 markdown 不算失败: %v", err)
	}
	if fileExists(pilotMarkerPath(proj)) {
		t.Fatal("没有 markdown 时不应写 marker")
	}
	if err := os.WriteFile(pilotMarkerPath(proj), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.stylePilotPhase(proj); err != nil { // marker 存在：直接返回（无 md 也不报错）
		t.Fatalf("marker 存在应当直接跳过: %v", err)
	}
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

// T108：pilot_spec 工具——操作即校验（md 文件必须存在、行段/页码合法）、
// 批量原子、转换回 pilotSpec 两数组。
func TestPilotSpecTool(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "book.md"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewPilotSpecTool(filepath.Join(dir, "spec.json"), src)

	// 不存在的 md 文件当场拒。
	res, _ := tool.Execute(`{"op":"add","kind":"md","file":"missing.md","start":1,"end":5}`)
	if !strings.HasPrefix(res.Text, "REJECTED") {
		t.Fatalf("不存在的 md 必须拒: %s", res.Text)
	}
	// 批量：一条 md + 一条 pdf。
	res, _ = tool.Execute(`{"op":"add","items":[{"kind":"md","file":"book.md","start":1,"end":2},{"kind":"pdf","file":"book.pdf","pages":[3,1]}]}`)
	if !strings.HasPrefix(res.Text, "OK") {
		t.Fatalf("批量 add 失败: %s", res.Text)
	}
	spec := specFromTool(tool.loadLocked())
	if len(spec.MDRanges) != 1 || spec.MDRanges[0].End != 2 {
		t.Fatalf("md_ranges 不对: %+v", spec.MDRanges)
	}
	if len(spec.PDFPages) != 1 || spec.PDFPages[0].Pages[0] != 1 {
		t.Fatalf("pdf_pages 不对（页码应排序）: %+v", spec.PDFPages)
	}
	// delete 越界整批不动。
	res, _ = tool.Execute(`{"op":"delete","ns":[1,9]}`)
	if !strings.HasPrefix(res.Text, "REJECTED") || len(tool.loadLocked()) != 2 {
		t.Fatalf("越界 delete 必须整批不动: %s", res.Text)
	}
}
