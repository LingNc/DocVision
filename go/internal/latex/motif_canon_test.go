package latex

import (
	"image"
	"image/color"
	"testing"
)

// 画一张"交换图"式截图片：深色边框 + 两条斜线 + 中间一行方块。
func canonTestImage(w, h int, variant int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{20, 20, 20, 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, white)
		}
	}
	// 外框
	for x := 10; x < w-10; x++ {
		for t := 0; t < 2; t++ {
			img.Set(x, 10+t, black)
			img.Set(x, h-11-t, black)
		}
	}
	for y := 10; y < h-10; y++ {
		for t := 0; t < 2; t++ {
			img.Set(10+t, y, black)
			img.Set(w-11-t, y, black)
		}
	}
	// 中间一行符号：variant=1 时第三个符号从实心块换成"叉"（真实的符号差异）
	cx := 24
	for i := 0; i < 3; i++ {
		if variant == 1 && i == 2 {
			for t := 0; t < 14; t++ {
				for d := 0; d < 2; d++ {
					img.Set(cx+t, 40+t+d, black)
					img.Set(cx+13-t, 40+t+d, black)
				}
			}
			cx += 30
			continue
		}
		for y := 40; y < 52; y++ {
			for x := cx; x < cx+14; x++ {
				img.Set(x, y, black)
			}
		}
		cx += 30
	}
	return img
}

func TestMotifCanonScoreSynthetic(t *testing.T) {
	a, ok := motifCanonFromImage(canonTestImage(120, 160, 0))
	if !ok {
		t.Fatal("a 没有内容")
	}
	same, _ := motifCanonFromImage(canonTestImage(120, 160, 0))
	other, _ := motifCanonFromImage(canonTestImage(120, 160, 1))
	canvas := motifCanonCanvas([]motifCanonImage{a, same, other})
	s := canvas * 95 / 100
	if s1, _, _, g := motifCanonScore(a, same, canvas, s); s1 < 0.999 || g < motifCanonMinG {
		t.Errorf("同一张图自比 = %.3f（g=%d），应 ≈1", s1, g)
	}
	sc, dy, dx, g := motifCanonScore(a, other, canvas, s)
	if sc >= 0.95 {
		t.Errorf("符号不同的图重合度 %.3f 太高（g=%d, shift=%d,%d）", sc, g, dy, dx)
	}
	if g < 64 {
		t.Errorf("判定层只到 g=%d，逐级细搜没生效", g)
	}
	// 白边 + 平移：内容框被居中后不受影响
	padded := image.NewRGBA(image.Rect(0, 0, 160, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 160; x++ {
			padded.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	src := canonTestImage(120, 160, 0)
	for y := 0; y < 160; y++ {
		for x := 0; x < 120; x++ {
			padded.Set(x+20, y+20, src.At(x, y))
		}
	}
	pv, _ := motifCanonFromImage(padded)
	if pv.w != a.w || pv.h != a.h {
		t.Fatalf("白边改变了内容框：%dx%d vs %dx%d", pv.w, pv.h, a.w, a.h)
	}
	if s2, _, _, _ := motifCanonScore(a, pv, canvas, s); s2 < 0.99 {
		t.Errorf("白边平移后重合度 %.3f，应 ≈1（居中贴入失效）", s2)
	}
}

func TestMotifCanonCanvas(t *testing.T) {
	imgs := []motifCanonImage{{w: 30, h: 40}, {w: 70, h: 60}, {w: 200, h: 100}}
	if got := motifCanonCanvas(imgs); got != 200 {
		t.Errorf("P90 = %d，期望 200", got)
	}
	if got := motifCanonCanvas([]motifCanonImage{{w: 5, h: 5}}); got != motifCanonMinS {
		t.Errorf("太小应夹到 %d，得到 %d", motifCanonMinS, got)
	}
	if got := motifCanonCanvas([]motifCanonImage{{w: 4000, h: 900}}); got != motifCanonMaxS {
		t.Errorf("太大应夹到 %d，得到 %d", motifCanonMaxS, got)
	}
}

func TestMotifCanonLevels(t *testing.T) {
	cases := []struct {
		canvas, finest int
		want           []int
	}{
		{390, 371, []int{32, 64, 128, 256}},
		{70, 66, []int{32, 64}},
		{38, 36, []int{32}},
		{130, 123, []int{32, 64}},
	}
	for _, c := range cases {
		got := motifCanonLevels(c.canvas, c.finest)
		if len(got) != len(c.want) {
			t.Errorf("levels(%d,%d)=%v，期望 %v", c.canvas, c.finest, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("levels(%d,%d)=%v，期望 %v", c.canvas, c.finest, got, c.want)
				break
			}
		}
	}
}
