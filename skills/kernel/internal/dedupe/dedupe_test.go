package dedupe

import (
	"image"
	"image/color"
	"testing"
)

// inkRect 生成白底黑块图（模拟一枚小标记）。
// 背景必须填不透明白：CanonFromImage 只看 RGB 不看 alpha，
// NRGBA 零值（透明黑）会被当成满墨——真实图片经解码器都是不透明像素。
func inkRect(w, h, rw, rh int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for y := (h - rh) / 2; y < (h-rh)/2+rh; y++ {
		for x := (w - rw) / 2; x < (w-rw)/2+rw; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	return img
}

// blankImage 不透明纯白图（真空白页）。
func blankImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return img
}

func TestSelfOverlapIsOne(t *testing.T) {
	img := inkRect(40, 40, 20, 20)
	a, ok := AlignCoverage(img)
	if !ok {
		t.Fatal("应有覆盖率")
	}
	if v := a.Overlap(a, ShiftCells(0.10)); v < 0.99 {
		t.Fatalf("自比应≈1: %f", v)
	}
	d := a.OverlapDetail(a, ShiftCells(0.10))
	// 画布 = 内容框长边 20，但会被抬到 CanonMinS=24
	if d.Score < 0.99 || d.Canvas != CanonMinS || d.Levels == 0 {
		t.Fatalf("detail 异常: %+v", d)
	}
}

func TestDifferentContentLowOverlap(t *testing.T) {
	a, _ := AlignCoverage(inkRect(40, 40, 30, 30))   // 大实心方块
	b, _ := AlignCoverage(inkRect(40, 40, 8, 30))    // 窄竖条
	if a.Overlap(b, ShiftCells(0.10)) >= 0.60 {
		t.Fatal("方块 vs 竖条不应达 0.60 组闸门")
	}
}

func TestWorthScorePrefilter(t *testing.T) {
	a, _ := AlignCoverage(inkRect(40, 40, 30, 30)) // aspect 1
	b, _ := AlignCoverage(inkRect(60, 30, 8, 28))   // aspect ≈0.29，差距 >0.35
	if WorthScore(a, b) {
		t.Fatal("长宽比差过大的对不应值得打分（预筛应拒）")
	}
	c, _ := AlignCoverage(inkRect(40, 40, 30, 30))
	if !WorthScore(a, c) {
		t.Fatal("同形同尺寸应对应通过预筛")
	}
}

func TestCoverageRejects(t *testing.T) {
	// 空白图（不透明纯白）
	blank := blankImage(40, 40)
	if _, ok := AlignCoverage(blank); ok {
		t.Fatal("空白图应拿不到覆盖率")
	}
	// 内容框太小
	tiny := inkRect(8, 8, 2, 2)
	if _, ok := AlignCoverage(tiny); ok {
		t.Fatal("2×2 内容框应退回逐对判据")
	}
	// nil
	if _, ok := AlignCoverage(nil); ok {
		t.Fatal("nil 图应拿不到覆盖率")
	}
}

func TestPairCanvasAndCost(t *testing.T) {
	a, _ := AlignCoverage(inkRect(40, 40, 20, 20))
	b, _ := AlignCoverage(inkRect(80, 80, 40, 40))
	if c := PairCanvas(a, b); c != 40 {
		t.Fatalf("画布应取内容框长边较大者 40: %d", c)
	}
	if AlignCost(a, b) <= 0 {
		t.Fatal("代价应为正")
	}
	// 极小内容框抬到画布下限
	small, _ := AlignCoverage(inkRect(10, 10, 5, 5))
	if c := PairCanvas(small, small); c != CanonMinS {
		t.Fatalf("小内容框画布应抬到 %d: %d", CanonMinS, c)
	}
}

func TestShiftCellsAndParse(t *testing.T) {
	if ShiftCells(0.10) != 3 {
		t.Fatalf("0.10×32 应 ≈3 格: %d", ShiftCells(0.10))
	}
	if g, err := ParseAlignGrid("0.95S", 100); err != nil || g != 95 {
		t.Fatalf("0.95S×100 应为 95: %d %v", g, err)
	}
	if g, err := ParseAlignGrid("64", 100); err != nil || g != 64 {
		t.Fatalf("固定格数 64 应解析为 64: %d %v", g, err)
	}
	if Finest(100) != 95 {
		t.Fatalf("Finest(100) 应为 95（默认 0.95S）: %d", Finest(100))
	}
}

func TestCarryOverNoRelay(t *testing.T) {
	// 模拟：below 里 c 与 safe 里 s 够分则带入；与 safe 外的不算
	score := func(a, b string) (float64, bool) {
		if (a == "c" && b == "s1") || (a == "c2" && b == "c") {
			return 0.7, true // c2 只与"被带入的 c"够分——不得当跳板
		}
		return 0.3, true
	}
	safe := []string{"s1"}
	below := []string{"c", "c2"}
	kept, n := CarryOver(below, safe, score, 0.60)
	if len(kept) != 1 || kept[0] != "c" || n != 1 {
		t.Fatalf("只应带入 c（c2 不得接力）: %v %d", kept, n)
	}
}

func TestCanonLevels(t *testing.T) {
	// 尾级规则：循环上界是 finest——尾级 = ≤finest 的最大 2 幂级
	// （g<=canvas 的放行只影响"已到达的级"；真实流 finest≈0.95S<canvas，上界恒为 finest）
	lv := CanonLevels(256, 243)
	if len(lv) == 0 || lv[len(lv)-1] != 128 {
		t.Fatalf("尾级应为 ≤finest 的最大级 128: %v", lv)
	}
	// 正常流：canvas 100 → 32,64（128>100 停）
	lv = CanonLevels(100, 95)
	if len(lv) != 2 || lv[1] != 64 {
		t.Fatalf("canvas=100 应为 [32 64]: %v", lv)
	}
	// finest 超上限被夹（CanonLevels 内部夹到 CanonMaxG）
	lv = CanonLevels(2000, 5000)
	for _, g := range lv {
		if g > CanonMaxG {
			t.Fatalf("级数超上限: %v", lv)
		}
	}
}
