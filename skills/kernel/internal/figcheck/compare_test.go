package figcheck

import (
	"image"
	"image/color"
	"testing"
)

func rectImage(w, h, rw, rh int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := (h - rh) / 2; y < (h-rh)/2+rh; y++ {
		for x := (w - rw) / 2; x < (w-rw)/2+rw; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	return img
}

func TestIdenticalHighSimilarity(t *testing.T) {
	ref := rectImage(200, 200, 100, 100)
	cand := rectImage(200, 200, 100, 100)
	r, _, err := CompareImages(ref, cand)
	if err != nil {
		t.Fatal(err)
	}
	if r.Similarity < 0.9 {
		t.Fatalf("完全相同的图应高度相似: %f", r.Similarity)
	}
}

func TestMMScaleDeviation(t *testing.T) {
	// 原图 40mm 宽，候选同宽画布但墨迹只有一半宽 → ScaleX≈0.5 被 mm 判据抓住（T312）
	ref := rectImage(200, 200, 100, 100)
	cand := rectImage(200, 200, 50, 100)
	mm := CanvasMM{RefW: 40, RefH: 40, CandW: 40, CandH: 40}
	r, _, err := CompareImagesMM(ref, cand, mm)
	if err != nil {
		t.Fatal(err)
	}
	if r.SizeUnit != "mm" {
		t.Fatalf("有物理尺寸时应走 mm 口径: %s", r.SizeUnit)
	}
	if r.ScaleX > 0.7 || r.ScaleDev < 0.4 {
		t.Fatalf("宽度差 50%% 应被抓: scaleX=%f dev=%f", r.ScaleX, r.ScaleDev)
	}
	if r.ShareRefW <= r.ShareCandW {
		t.Fatalf("画布占比应体现宽度差: ref=%f cand=%f", r.ShareRefW, r.ShareCandW)
	}
}

func TestAspectFallbackWithoutMM(t *testing.T) {
	// 无物理尺寸时退化为宽高比判据：方 vs 扁
	ref := rectImage(200, 200, 100, 100)
	cand := rectImage(200, 200, 160, 60)
	r, _, err := CompareImages(ref, cand)
	if err != nil {
		t.Fatal(err)
	}
	if r.SizeUnit != "aspect" {
		t.Fatalf("无 mm 时应走 aspect 口径: %s", r.SizeUnit)
	}
	if r.AspectDev < 0.5 {
		t.Fatalf("宽高比差应被抓: %f", r.AspectDev)
	}
}

func TestEmptyImageRejected(t *testing.T) {
	empty := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	ref := rectImage(100, 100, 50, 50)
	if _, _, err := CompareImages(ref, empty); err == nil {
		t.Fatal("空图应报错")
	}
}

func TestDecodeImageMissing(t *testing.T) {
	if _, err := DecodeImage("/nonexistent/x.png"); err == nil {
		t.Fatal("缺文件应报错")
	}
}
