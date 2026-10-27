package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// P64：图片阶段的复查页要能直接核对单图——每张图给出 progress_items 里的状态、
// 判定记录（json）与转换产物入口；并单独列出"还在等转换的原图"与"退回原图"，
// 否则几百张缩略图平铺在一起，这两批根本找不出来。
func TestMotifReviewShowsImageStatusAndLooseSets(t *testing.T) {
	outDir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{DedupeAlignMin: 0.6, DedupeAlignShift: 0.1}}}

	img := whiteCanvas(70, 96)
	drawDiamond(img, 35, 30, 18)
	done := &motifCandidate{
		task:       &task{mdName: "book.md", imgPath: "images/book/a.jpg"},
		progress:   &imageProgress{Key: "book.md::images/book/a.jpg", MDName: "book.md", ImgPath: "images/book/a.jpg", Class: ClassVector, Status: ImageStatusDone, TikzCode: "\\draw (0,0)--(1,1);", FigPDF: "figures/a.pdf"},
		image:      img,
		sourceHash: "hash-a",
	}
	prog := map[string]*imageProgress{
		done.task.key():              done.progress,
		"book.md::images/book/b.jpg": {Key: "book.md::images/book/b.jpg", MDName: "book.md", ImgPath: "images/book/b.jpg", Class: ClassVector, Status: ImageStatusFallback, FallbackReason: "TikZ 重画没有通过校验"},
		"book.md::images/book/c.jpg": {Key: "book.md::images/book/c.jpg", MDName: "book.md", ImgPath: "images/book/c.jpg", Class: ClassVector, Status: ImageStatusClassified},
		"book.md::images/book/d.jpg": {Key: "book.md::images/book/d.jpg", MDName: "book.md", ImgPath: "images/book/d.jpg", Class: ClassVector, Status: ImageStatusError, Error: "api error"},
	}

	r.writeMotifArtifacts(outDir, []*motifCandidate{done}, nil, nil, nil, prog)

	page, err := os.ReadFile(filepath.Join(outDir, "dedupe", "review.html"))
	if err != nil {
		t.Fatalf("review.html 必须写出: %v", err)
	}
	html := string(page)
	for _, want := range []string{
		"已完成",
		"判定记录",
		"../progress_items/book.md/images_book_a.jpg.json",
		"../tikz/a.tex",
		"../sessions/vector_a.jsonl",
		"等待转换的原图（2 张）",
		"退回原图的（已收尾但没画出成品）（1 张）",
		"退回原图",
		"TikZ 重画没有通过校验",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("review.html 缺少 %q", want)
		}
	}
	// 已完成的图不该落进"等待转换"；退回原图的图也不该重复出现在等待段。
	if strings.Count(html, "images/book/c.jpg") == 0 {
		t.Fatal("等待转换段应列出未处理的图")
	}
}

// P64：全书盘点必须只看 status（raster/fallback 也算收尾），否则续跑时进度行
// 会永远显示"还剩一大半"。
func TestCountImageStatusesUsesStatusOnly(t *testing.T) {
	prog := map[string]*imageProgress{
		"a": {Class: ClassVector, Status: ImageStatusDone},
		"b": {Class: ClassRaster, Status: ImageStatusDone},
		"c": {Class: ClassVector, Status: ImageStatusFallback, FallbackReason: "TikZ 重画没有通过校验"},
		"d": {Class: ClassVector, Status: ImageStatusClassified},
		"e": {Class: ClassVector, Status: ImageStatusError},
		"f": {Class: ClassVector},
	}
	c := countImageStatuses(prog)
	if c.Total != 6 || c.Settled != 3 || c.OK != 2 || c.Fallback != 1 || c.Raster != 1 || c.Error != 1 || c.Pending != 2 {
		t.Fatalf("盘点不对: %+v", c)
	}
}
