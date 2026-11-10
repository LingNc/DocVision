package latex

import (
	"image"
	"image/color"
	"testing"
)

// inkOn 造一张白底图，按比例画一个实心墨块（灰度 0）。
func inkOn(w, h int, fx0, fy0, fx1, fy1 float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	for y := int(float64(h) * fy0); y < int(float64(h)*fy1); y++ {
		for x := int(float64(w) * fx0); x < int(float64(w)*fx1); x++ {
			img.Set(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	return img
}

// T312：尺寸那一项以前比的是"两张图的墨迹像素宽高"，等于拿 PDF 渲染 DPI 除
// 原扫描分辨率——渲染参数一改数字就变，物理上毫无意义。现在按画布物理尺寸
// 换算成 mm 再比。
func TestFigureSizeUsesPhysicalMM(t *testing.T) {
	// 同一张图：原图 200x100 像素对应 20x10mm；候选渲染成 1000x500 像素，
	// 页仍是 20x10mm。墨迹都占画布一半 —— 物理尺寸完全一致。
	ref := inkOn(200, 100, 0.25, 0.25, 0.75, 0.75)
	cand := inkOn(1000, 500, 0.25, 0.25, 0.75, 0.75)
	mm := figureCanvasMM{RefW: 20, RefH: 10, CandW: 20, CandH: 10}
	sx, sy, dev, aspRef, aspCand, aspDev, unit, inkRefW, _, inkCandW, _, _, _, _, _ := measureFigureSize(ref, cand, mm)

	if unit != "mm" {
		t.Fatalf("单位应当走物理尺寸，得到 %q", unit)
	}
	if diff := sx - 1; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("宽度倍数应当 1.0（物理一样大），得到 %.4f", sx)
	}
	if diff := sy - 1; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("高度倍数应当 1.0，得到 %.4f", sy)
	}
	if dev != 0 {
		t.Errorf("尺寸偏差应当 0，得到 %.4f", dev)
	}
	if aspRef != aspCand || aspDev != 0 {
		t.Errorf("宽高比应当一致，得到 %.3f vs %.3f（偏差 %.3f）", aspRef, aspCand, aspDev)
	}
	if inkRefW != 10 || inkCandW != 10 {
		t.Errorf("墨迹物理宽应当都是 10mm，得到 %.2f / %.2f", inkRefW, inkCandW)
	}
	// 旧口径会把 1000/200=5 倍的"尺寸偏差 400%"报出来 —— 这就是被修掉的误判。
	if dev > 0.001 {
		t.Errorf("像素不同但物理相同，不该报尺寸偏差")
	}
}

// 物理尺寸真的放大一倍：必须报出来（这才是"画大了"）。
func TestFigureSizeCatchesRealScaleUp(t *testing.T) {
	ref := inkOn(200, 100, 0.25, 0.25, 0.75, 0.75)  // 墨迹 10x5mm
	cand := inkOn(400, 200, 0.25, 0.25, 0.75, 0.75) // 墨迹 20x10mm（页也是 40x20mm）
	_, _, dev, _, _, _, unit, _, _, _, _, _, _, _, _ := measureFigureSize(ref, cand,
		figureCanvasMM{RefW: 20, RefH: 10, CandW: 40, CandH: 20})
	if unit != "mm" {
		t.Fatalf("单位应当走物理尺寸，得到 %q", unit)
	}
	if dev < 0.99 || dev > 1.01 {
		t.Errorf("放大一倍应当报 ~100%% 偏差，得到 %.3f", dev)
	}
}

// 压扁：宽度没变、高度翻倍 -> 物理尺寸报 100%（高度那一项），宽高比也报出来。
func TestFigureSizeCatchesSquash(t *testing.T) {
	ref := inkOn(200, 100, 0.25, 0.25, 0.75, 0.75) // 墨迹 100x50px = 10x5mm，宽高比 2:1
	cand := inkOn(200, 100, 0.25, 0.0, 0.75, 1.0)  // 墨迹 100x100px = 10x10mm，宽高比 1:1
	_, _, dev, aspRef, aspCand, aspDev, _, _, _, _, _, _, _, _, _ := measureFigureSize(ref, cand,
		figureCanvasMM{RefW: 20, RefH: 10, CandW: 20, CandH: 10})
	if dev < 0.99 || dev > 1.01 {
		t.Errorf("高度翻倍应当报 ~100%% 偏差，得到 %.3f", dev)
	}
	if aspRef != 2 || aspCand != 1 {
		t.Errorf("宽高比应当 2:1 vs 1:1，得到 %.2f vs %.2f", aspRef, aspCand)
	}
	if aspDev < 0.49 || aspDev > 0.51 {
		t.Errorf("宽高比偏差应当 50%%，得到 %.3f", aspDev)
	}
}

// 两边都拿不到物理尺寸时退到宽高比：形状对了就放过，形状被压扁才打回。
func TestFigureSizeFallsBackToAspect(t *testing.T) {
	ref := inkOn(200, 100, 0.25, 0.25, 0.75, 0.75)
	sameShape := inkOn(400, 200, 0.25, 0.25, 0.75, 0.75)
	if _, _, dev, _, _, _, unit, _, _, _, _, _, _, _, _ := measureFigureSize(ref, sameShape, figureCanvasMM{}); unit != "aspect" || dev > 0.001 {
		t.Errorf("同形状应放过：unit=%q dev=%.4f", unit, dev)
	}
	squashed := inkOn(400, 200, 0.0, 0.0, 1.0, 0.5) // 宽高比 4:1（原图 2:1，翻了一倍）
	if _, _, dev, _, _, _, unit, _, _, _, _, _, _, _, _ := measureFigureSize(ref, squashed, figureCanvasMM{}); unit != "aspect" || dev < 0.99 || dev > 1.01 {
		t.Errorf("压扁到 4:1 应当报 ~100%%：unit=%q dev=%.3f", unit, dev)
	}
}
