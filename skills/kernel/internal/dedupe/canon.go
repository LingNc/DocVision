// Package dedupe 是图片去重的本地算法包（居中重合判据核心）。
// 从旧线 go/internal/latex/motif_canon.go + motif_align.go 逐字移植
// （T129–T151 判据：非二值化软墨量、S 自适应按对计算、不缩放、逐级搜索
// 32→…→最细、判定分取最细一级、±10% 平移预算 / 细层 ±2 格不扩窗）。
// Runner/配置耦合的部分改为包级变量与参数（applyMotifMetricConfig 的对应物）。
package dedupe

import (
	"image"
	"math"
	"strconv"
	"strings"
)

// 居中重合判据常量（旧线 motif_canon.go 逐字）。
const (
	CanonInkLevel = 0.2  // 内容框阈值：软墨量 > 0.2 视为有墨
	CanonMinS     = 24   // 画布边长下限
	CanonMaxS     = 512  // 画布边长上限
	CanonMinG     = 32   // 最粗一级
	CanonRefine   = 2    // 细层搜索窗口（格）
	CanonMaxG     = 1024 // 最细一级上限
)

// 可注入的判据参数（旧线 applyMotifMetricConfig 的对应物；一次 dedupe 内不变）。
var (
	// AlignGridSpec 是最细一级的格数规格（"32" 或 "0.95S"）。
	AlignGridSpec = "0.95S"
	// AlignGrid 是旧判据固定的网格边长，只有"平移格数换算"用得到它。
	AlignGrid = 32
)

// Finest 按规格算最细一级的格数（旧线 motifAlignFinest）。
func Finest(canvas int) int {
	g, err := ParseAlignGrid(AlignGridSpec, canvas)
	if err != nil || g <= 0 {
		return canvas * 95 / 100
	}
	return g
}

// ParseAlignGrid 解析格数规格（旧线 config.ParseAlignGrid）：
// "32" = 固定格数；"0.95S" = 画布边长的 95%。
func ParseAlignGrid(spec string, canvas int) (int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, nil
	}
	if strings.HasSuffix(spec, "S") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(spec, "S"), 64)
		if err != nil {
			return 0, err
		}
		return int(math.Round(pct * float64(canvas))), nil
	}
	return strconv.Atoi(spec)
}

// CanonImage 是一张图的软墨量内容框：三通道交错存储
// （下标 (y*w+x)*3+c），w/h 是**内容框**尺寸，不含画布。
type CanonImage struct {
	Px   []float32
	W, H int
}

// MaxSide 是内容框长边。
func (m CanonImage) MaxSide() int {
	if m.W > m.H {
		return m.W
	}
	return m.H
}

// CanonFromImage 取软墨量内容框（不二值化、不缩放）。
// 第二返回值 false 表示整张图没有墨（空白图）。
func CanonFromImage(img image.Image) (CanonImage, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return CanonImage{}, false
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
			if ink/255 > CanonInkLevel {
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
		return CanonImage{}, false
	}
	cw, ch := maxX-minX+1, maxY-minY+1
	px := make([]float32, cw*ch*3)
	for y := 0; y < ch; y++ {
		src := ((minY+y)*w + minX) * 3
		copy(px[y*cw*3:(y+1)*cw*3], raw[src:src+cw*3])
	}
	return CanonImage{Px: px, W: cw, H: ch}, true
}

// CanonGrid 把内容框居中贴进 S×S 后按 G×G 分箱，格子取格内软墨量均值
// （三通道交错：下标 (y*G+x)*3+c）。内容框之外的格为 0。
func CanonGrid(im CanonImage, canvas, g int) []float32 {
	offX := (canvas - im.W) / 2
	offY := (canvas - im.H) / 2
	pw := im.W + 1
	pre := make([]float64, pw*(im.H+1)*3)
	for y := 0; y < im.H; y++ {
		for c := 0; c < 3; c++ {
			var rowSum float64
			for x := 0; x < im.W; x++ {
				rowSum += float64(im.Px[(y*im.W+x)*3+c])
				i := ((y+1)*pw + (x + 1)) * 3
				pre[i+c] = pre[((y)*pw+(x+1))*3+c] + rowSum
			}
		}
	}
	sum := func(x0, y0, x1, y1, c int) float64 {
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
		y0 := cy0 - offY
		y1 := cy1 - offY
		if y0 < 0 {
			y0 = 0
		}
		if y1 > im.H {
			y1 = im.H
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
			if x1 > im.W {
				x1 = im.W
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

// CanonOverlap 是两套网格在平移 (dy,dx) 下的三通道平均 Jaccard：
// b 整体平移后与 a 比，越界部分不参与。
func CanonOverlap(a, b []float32, g, dy, dx int) float64 {
	var inter, union [3]float64
	for y := 0; y < g; y++ {
		by := y - dy
		if by < 0 || by >= g {
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

// CanonLevels 给出逐级搜索的级数序列：从 32 起逐级翻倍，直到最细一级。
// 判定分取最后一层。
func CanonLevels(canvas, finest int) []int {
	if finest <= 0 {
		finest = canvas
	}
	if finest > CanonMaxG {
		finest = CanonMaxG
	}
	var out []int
	for g := CanonMinG; g <= finest; g *= 2 {
		if g <= canvas || len(out) == 0 {
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		out = append(out, CanonMinG)
	}
	return out
}

// CanonScore 算两张图的居中重合度：逐级由粗到细搜索平移，返回最终
// （最细）一级的分数与平移量、该级格数。
func CanonScore(a, b CanonImage, canvas, finest int) (score float64, dy, dx, g int) {
	return CanonScoreShift(a, b, canvas, finest, 0.10)
}

// CanonScoreShift 同 CanonScore，但可指定 0 级搜索窗（占格数的比例）。
// 逐级搜索窗口约定（T143）：0 级以 0 为中心开 ±shiftPct·G 的窗；此后每级把
// 上一级的解换算到本层格号（×2），只在中心周围 ±CanonRefine 格里找，不扩窗。
// 粗层只负责把平移找准，分数与平移都取最终（最细）一级的解。
func CanonScoreShift(a, b CanonImage, canvas, finest int, shiftPct float64) (score float64, dy, dx, g int) {
	if shiftPct <= 0 {
		shiftPct = 0.10
	}
	levels := CanonLevels(canvas, finest)
	shiftX, shiftY := 0, 0
	best := 0.0
	for li, gi := range levels {
		ga := CanonGrid(a, canvas, gi)
		gb := CanonGrid(b, canvas, gi)
		win := CanonRefine
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
				s := CanonOverlap(ga, gb, gi, y, x)
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
