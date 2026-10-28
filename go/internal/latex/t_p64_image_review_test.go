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

// P64/T262：全书盘点分"作业项"与"跟随项"两层，文字/位图不许算成"矢量画好"。
// 现场口径：一本 388 条引用的书里，379 条矢量只有 23 张是代表图，其余 356 条
// 跟随代表图；把它们算进"未处理"会显示成"还剩 361 张"，看着像什么都没做。
func TestCountImageStatusesSplitsWorkAndFollowers(t *testing.T) {
	prog := map[string]*imageProgress{
		"a": {Class: ClassVector, Status: ImageStatusDone},
		"b": {Class: ClassRaster, Status: ImageStatusDone},
		"c": {Class: ClassVector, Status: ImageStatusFallback, FallbackReason: "TikZ 重画没有通过校验"},
		"d": {Class: ClassVector, Status: ImageStatusClassified},
		"e": {Class: ClassVector, Status: ImageStatusError},
		"f": {Class: ClassVector},
		// 跟随项：矢量 + 有 equivalent_to，不单独跑，也不算作业项。
		"g": {Class: ClassVector, Status: ImageStatusClassified, EquivalentTo: "a"},
		"h": {Class: ClassVector, Status: ImageStatusDone, EquivalentTo: "a"},
		"i": {Class: ClassText, Status: ImageStatusDone},
	}
	c := countImageStatuses(prog)
	if c.Total != 9 || c.Work != 7 || c.Followers != 2 {
		t.Fatalf("作业项/跟随项分错了: %+v", c)
	}
	if c.VectorOK != 1 || c.TextRaster != 2 || c.Fallback != 1 || c.Error != 1 || c.Pending != 2 {
		t.Fatalf("分栏不对: %+v", c)
	}
}
