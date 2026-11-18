package prep

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
)

func TestFindPage(t *testing.T) {
	pages := &docindex.PageIndex{
		Srcs: []docindex.PageSrc{
			{PDF: "a.pdf", First: 1, Count: 3},
			{PDF: "b.pdf", First: 4, Count: 2},
		},
		Total: 5,
	}
	pdf, pg, err := findPage(pages, 4)
	if err != nil || pdf != "b.pdf" || pg != 1 {
		t.Fatalf("全书4应映射 b.pdf 第1页: %s %d %v", pdf, pg, err)
	}
	if _, _, err := findPage(pages, 6); err == nil {
		t.Fatal("越界页应报错")
	}
}

func TestRenderPageWithRealPoppler(t *testing.T) {
	if _, err := os.Stat("/usr/bin/pdftoppm"); err != nil {
		if _, err2 := findInPath("pdftoppm"); err2 != nil {
			t.Skip("无 pdftoppm")
		}
	}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "book.pdf")
	makeTestPDF(t, pdf, 3)
	pages := &docindex.PageIndex{
		Srcs: []docindex.PageSrc{{PDF: pdf, First: 1, Count: 3}},
		Total: 3,
	}
	outDir := filepath.Join(dir, "renders")
	// 整页渲染
	res, err := RenderPage(pages, 2, nil, outDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Image); err != nil {
		t.Fatal("PNG 应生成:", err)
	}
	if res.MMW < 200 || res.MMW > 220 {
		t.Fatalf("A4 150dpi 渲染宽约 210mm: %f", res.MMW)
	}
	// bbox 裁剪：左上半页 → 宽不变高减半（A4 595x842pt → crop 0,0,595,421）
	res2, err := RenderPage(pages, 1, &[4]float64{0, 0, 595, 421}, outDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res2.MMH >= res.MMH*0.9 || res2.MMH <= res.MMH*0.4 {
		t.Fatalf("裁剪后高度应约为整页一半: 整页 %f 裁剪 %f", res.MMH, res2.MMH)
	}
	// 越界 bbox 应报错而非乱裁
	if _, err := RenderPage(pages, 1, &[4]float64{0, 0, 99999, 99999}, outDir, nil); err == nil {
		t.Fatal("完全越界 bbox 应报错")
	}
}

func findInPath(name string) (string, error) {
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
