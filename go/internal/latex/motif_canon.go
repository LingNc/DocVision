package latex

import (
	"image"
	"math"
	"sort"
)

// 居中重合判据（T138 定稿）：软墨量、S 自适应、不缩放、逐级搜索，
// 判定分 = 逐级搜索最终（最细）那一级的结果。
const (
	motifCanonInkLevel = 0.2  // 内容框阈值：软墨量 > 0.2 视为有墨
	motifCanonMinS     = 24   // 画布边长下限
	motifCanonMaxS     = 512  // 画布边长上限
	motifCanonPct      = 0.90 // 池内取 P90
	motifCanonMinG     = 32   // 最粗一级
	motifCanonRefine   = 2    // 细层搜索窗口（格）
	motifCanonMaxG     = 1024 // 最细一级上限
)

// motifCanonImage 是一张图的软墨量内容框：三通道交错存储
// （下标 (y*w+x)*3+c），w/h 是**内容框**尺寸，不含画布。
type motifCanonImage struct {
	px   []float32
	w, h int
}

// maxSide 是内容框长边。
func (m motifCanonImage) maxSide() int {
	if m.w > m.h {
		return m.w
	}
	return m.h
}

// motifCanonFromImage 取软墨量内容框（不二值化、不缩放）。
// 第二返回值 false 表示整张图没有墨（空白图）。
func motifCanonFromImage(img image.Image) (motifCanonImage, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return motifCanonImage{}, false
	}
	raw := make([]float32, w*h*3)
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			cw := [3]float32{255 - float32(r>>8), 255 - float32(g>>8), 255 - float32(bl>>8)}
			i := (y*w + x) * 3
			raw[i], raw[i+1], raw[i+2] = cw[0]/255, cw[1]/255, cw[2]/255
			ink := cw[0]
			if cw[1] > ink {
				ink = cw[1]
			}
			if cw[2] > ink {
				ink = cw[2]
			}
			if ink/255 > motifCanonInkLevel {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < 0 {
		return motifCanonImage{}, false
	}
	cw, ch := maxX-minX+1, maxY-minY+1
	px := make([]float32, cw*ch*3)
	for y := 0; y < ch; y++ {
		src := ((minY+y)*w + minX) * 3
		copy(px[y*cw*3:(y+1)*cw*3], raw[src:src+cw*3])
	}
	return motifCanonImage{px: px, w: cw, h: ch}, true
}

// motifCanonCanvas 按池子算画布边长 S：P90(内容框长边)，夹在 [24, 512]。
func motifCanonCanvas(imgs []motifCanonImage) int {
	if len(imgs) == 0 {
		return motifCanonMinS
	}
	sides := make([]int, len(imgs))
	for i, im := range imgs {
		sides[i] = im.maxSide()
	}
	sort.Ints(sides)
	idx := int(math.Ceil(motifCanonPct*float64(len(sides)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sides) {
		idx = len(sides) - 1
	}
	s := sides[idx]
	if s < motifCanonMinS {
		s = motifCanonMinS
	}
	if s > motifCanonMaxS {
		s = motifCanonMaxS
	}
	return s
}

// motifCanonGrid 把内容框居中贴进 S×S 后按 G×G 分箱，格子取格内软墨量均值
// （三通道交错：下标 (y*G+x)*3+c）。内容框之外的格为 0。
func motifCanonGrid(im motifCanonImage, canvas, g int) []float32 {
	// 内容框左上角在画布里的偏移（负偏移 = 内容超过画布，只取画布内部分，
	// 这种情况调用方已按"放不进 S"跳过，这里仍安全裁剪）。
	offX := (canvas - im.w) / 2
	offY := (canvas - im.h) / 2
	// 前缀和（三通道），用于格内求和。
	pw := im.w + 1
	pre := make([]float64, pw*(im.h+1)*3)
	for y := 0; y < im.h; y++ {
		for c := 0; c < 3; c++ {
			var rowSum float64
			for x := 0; x < im.w; x++ {
				rowSum += float64(im.px[(y*im.w+x)*3+c])
				i := ((y+1)*pw + (x + 1)) * 3
				pre[i+c] = pre[((y)*pw+(x+1))*3+c] + rowSum
			}
		}
	}
	sum := func(x0, y0, x1, y1, c int) float64 {
		// 矩形 [x0,x1)×[y0,y1) 的软墨量和（内容框坐标）。
		if x1 <= x0 || y1 <= y0 {
			return 0
		}
		a := pre[(y1*pw+x1)*3+c]
		b := pre[(y0*pw+x1)*3+c]
		d := pre[(y1*pw+x0)*3+c]
		e := pre[(y0*pw+x0)*3+c]
		return a - b - d + e
	}
	out := make([]float32, g*g*3)
	for gy := 0; gy < g; gy++ {
		cy0 := gy * canvas / g
		cy1 := (gy + 1) * canvas / g
		// 转成内容框坐标并裁到 [0, w/h]。
		y0 := cy0 - offY
		y1 := cy1 - offY
		if y0 < 0 {
			y0 = 0
		}
		if y1 > im.h {
			y1 = im.h
		}
		if y1 <= y0 {
			continue
		}
		for gx := 0; gx < g; gx++ {
			cx0 := gx * canvas / g
			cx1 := (gx + 1) * canvas / g
			x0 := cx0 - offX
			x1 := cx1 - offX
			if x0 < 0 {
				x0 = 0
			}
			if x1 > im.w {
				x1 = im.w
			}
			if x1 <= x0 {
				continue
			}
			area := float64((cx1 - cx0) * (cy1 - cy0))
			o := (gy*g + gx) * 3
			for c := 0; c < 3; c++ {
				out[o+c] = float32(sum(x0, y0, x1, y1, c) / area)
			}
		}
	}
	return out
}

// motifCanonOverlap 是两套网格在平移 (dy,dx) 下的三通道平均 Jaccard：
// b 整体平移后与 a 比，越界部分不参与。
func motifCanonOverlap(a, b []float32, g, dy, dx int) float64 {
	var inter, union [3]float64
	for y := 0; y < g; y++ {
		by := y - dy
		if by < 0 || by >= g {
			// b 越界：a 有墨、b 无墨 → 全进并集。
			for x := 0; x < g; x++ {
				o := (y*g + x) * 3
				for c := 0; c < 3; c++ {
					union[c] += float64(a[o+c])
				}
			}
			continue
		}
		for x := 0; x < g; x++ {
			bx := x - dx
			o := (y*g + x) * 3
			if bx < 0 || bx >= g {
				for c := 0; c < 3; c++ {
					union[c] += float64(a[o+c])
				}
				continue
			}
			bo := (by*g + bx) * 3
			for c := 0; c < 3; c++ {
				av := float64(a[o+c])
				bv := float64(b[bo+c])
				if av < bv {
					inter[c] += av
					union[c] += bv
				} else {
					inter[c] += bv
					union[c] += av
				}
			}
		}
	}
	var s float64
	cnt := 0
	for c := 0; c < 3; c++ {
		if union[c] <= 1e-9 {
			continue
		}
		s += inter[c] / union[c]
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return s / float64(cnt)
}

// motifCanonLevels 给出逐级搜索的级数序列：从 32 起逐级翻倍，直到放得进画布
// 的最细一级（也受 finest 上限约束）。判定分取最后一层。
func motifCanonLevels(canvas, finest int) []int {
	if finest <= 0 {
		finest = canvas
	}
	if finest > motifCanonMaxG {
		finest = motifCanonMaxG
	}
	var out []int
	for g := motifCanonMinG; g <= finest; g *= 2 {
		if g <= canvas || len(out) == 0 {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		out = append(out, motifCanonMinG)
	}
	return out
}

// motifCanonScore 算两张图的居中重合度：逐级由粗到细搜索平移，返回最终
// （最细）一级的分数与平移量、该级格数。
func motifCanonScore(a, b motifCanonImage, canvas, finest int) (score float64, dy, dx, g int) {
	return motifCanonScoreShift(a, b, canvas, finest, 0.10)
}

// motifCanonScoreShift 同 motifCanonScore，但可指定 0 级搜索窗（占格数的比例）。
//
// 逐级搜索的窗口约定（用户口径，T143）：0 级没有上一级可继承，以 0 为中心开
// ±shiftPct·G 的窗（平移预算）；此后每一级把上一级的解**换算到本层格号**
// （粗格 g 的一个格子 = 细格 2g 的 2×2 个格子，所以中心 = 2×上一层解），
// 只在这个中心周围 ±motifCanonRefine 格里找——不再有任何"顶到边缘就扩窗"。
// 粗层只负责把平移找准，分数与平移都取最终（最细）一级的解。
func motifCanonScoreShift(a, b motifCanonImage, canvas, finest int, shiftPct float64) (score float64, dy, dx, g int) {
	if shiftPct <= 0 {
		shiftPct = 0.10
	}
	levels := motifCanonLevels(canvas, finest)
	shiftX, shiftY := 0, 0
	best := 0.0
	for li, gi := range levels {
		ga := motifCanonGrid(a, canvas, gi)
		gb := motifCanonGrid(b, canvas, gi)
		win := motifCanonRefine
		cx, cy := shiftX*2, shiftY*2
		if li == 0 {
			win = int(math.Round(shiftPct * float64(gi)))
			if win < 1 {
				win = 1
			}
			cx, cy = 0, 0
		}
		if win > gi {
			win = gi
		}
		bs, by, bx := -1.0, cy, cx
		for y := cy - win; y <= cy+win; y++ {
			for x := cx - win; x <= cx+win; x++ {
				s := motifCanonOverlap(ga, gb, gi, y, x)
				if s > bs {
					bs, by, bx = s, y, x
				}
			}
		}
		best, shiftY, shiftX = bs, by, bx
		g = gi
	}
	return best, shiftY, shiftX, g
}
