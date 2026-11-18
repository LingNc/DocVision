// Package figcheck 是 T2 作图验收的软墨迹对齐算法（compareFigure 核心）。
// 从旧线 go/internal/latex/figure_compare.go 逐字移植（T307/T312 判据：
// 多分辨率渐进对齐、质心起点、尺寸按物理 mm 在归一化前量取、scale_dev 抓"看着对上了其实窄了一截"）。
// 剥掉的只是会话工具壳（Tool 接口/base64 回执），比较核心一字未改。
package figcheck

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// DecodeImage 读图并压白底（旧线 decodeFigureImage）。
func DecodeImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	return flattenOnWhite(img), nil
}

func flattenOnWhite(src image.Image) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			a := uint32(c.A)
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8((uint32(c.R)*a + 255*(255-a)) / 255),
				G: uint8((uint32(c.G)*a + 255*(255-a)) / 255),
				B: uint8((uint32(c.B)*a + 255*(255-a)) / 255), A: 255,
			})
		}
	}
	return out
}

type softInkImage struct {
	W, H int
	Pix  [][3]float64
}

func (s softInkImage) at(x, y int) [3]float64 { return s.Pix[y*s.W+x] }

// AlignmentRun 一次对齐尝试（origin / centroid 起点）的成绩。
type AlignmentRun struct {
	Name     string
	Score    float64
	DX, DY   int
	Channels [3]float64
}

// Comparison 是比较结果。
type Comparison struct {
	Similarity float64
	Channels   [3]float64
	Gray       float64
	DX, DY     int
	Runs       []AlignmentRun
	Canvas     int
	Grid       int
	// 尺寸全部按物理尺寸（mm）量（T312）：墨迹占各自画布的比例 × 画布真实尺寸。
	// ScaleX/ScaleY = 候选墨迹宽/高 ÷ 原图墨迹宽/高（1 = 一样大），ScaleDev 取两者偏得最多的那个。
	// 必须在 fitCanvases 之前量——那个函数会按原图宽度把候选等比缩放，宽度差就此被抹掉。
	ScaleX, ScaleY float64
	ScaleDev       float64
	SizeUnit       string
	AspectRef      float64
	AspectCand     float64
	AspectDev      float64
	InkRefW        float64
	InkRefH        float64
	InkCandW       float64
	InkCandH       float64
	ShareRefW      float64
	ShareRefH      float64
	ShareCandW     float64
	ShareCandH     float64
}

// CanvasMM 是两张图各自的画布真实尺寸（mm）。任一为 0 = 不知道。
type CanvasMM struct {
	RefW, RefH   float64
	CandW, CandH float64
}

// measureFigureSize 量"这张图到底画多大"（旧线 measureFigureSize 逐字）。
func measureFigureSize(ref, cand image.Image, mm CanvasMM) (scaleX, scaleY, dev float64,
	aspectRef, aspectCand, aspectDev float64, unit string,
	inkRefW, inkRefH, inkCandW, inkCandH float64,
	shareRefW, shareRefH, shareCandW, shareCandH float64) {
	rb, cb := ref.Bounds(), cand.Bounds()
	pxRefW, pxRefH := inkBox(ref)
	pxCandW, pxCandH := inkBox(cand)
	if rb.Dx() > 0 && rb.Dy() > 0 && cb.Dx() > 0 && cb.Dy() > 0 {
		shareRefW = float64(pxRefW) / float64(rb.Dx())
		shareRefH = float64(pxRefH) / float64(rb.Dy())
		shareCandW = float64(pxCandW) / float64(cb.Dx())
		shareCandH = float64(pxCandH) / float64(cb.Dy())
	}
	if mm.RefW > 0 && mm.RefH > 0 && mm.CandW > 0 && mm.CandH > 0 {
		unit = "mm"
		inkRefW, inkRefH = shareRefW*mm.RefW, shareRefH*mm.RefH
		inkCandW, inkCandH = shareCandW*mm.CandW, shareCandH*mm.CandH
		scaleX, scaleY = 1, 1
		if inkRefW > 0 {
			scaleX = inkCandW / inkRefW
		}
		if inkRefH > 0 {
			scaleY = inkCandH / inkRefH
		}
		dev = math.Max(math.Abs(1-scaleX), math.Abs(1-scaleY))
	} else {
		unit = "aspect"
		inkRefW, inkRefH = float64(pxRefW), float64(pxRefH)
		inkCandW, inkCandH = float64(pxCandW), float64(pxCandH)
	}
	if inkRefW > 0 && inkRefH > 0 && inkCandW > 0 && inkCandH > 0 {
		aspectRef = inkRefW / inkRefH
		aspectCand = inkCandW / inkCandH
		if aspectRef > 0 {
			aspectDev = math.Abs(aspectCand/aspectRef - 1)
		}
	}
	if unit == "aspect" {
		dev = aspectDev
	}
	return
}

// SizeSentence 把尺寸那一项说成人能对着改的一句话（T312）。
func SizeSentence(r Comparison) string {
	if r.InkRefW <= 0 || r.InkCandW <= 0 {
		return "Ink size: not measurable (no ink in one of the images)."
	}
	if r.SizeUnit == "mm" {
		return fmt.Sprintf("Ink size: canvas share w %.0f%%/%.0f%%, h %.0f%%/%.0f%% (original/candidate); physical ink %.1fmm x %.1fmm vs %.1fmm x %.1fmm; width x%.3f, height x%.3f, max deviation %.1f%% (measured in mm BEFORE the comparison rescales the candidate, so it does catch a wrong printed size; fix the coordinates/canvas rather than the stroke width). Ink aspect ratio %.2f:1 vs %.2f:1.",
			r.ShareRefW*100, r.ShareCandW*100, r.ShareRefH*100, r.ShareCandH*100,
			r.InkRefW, r.InkRefH, r.InkCandW, r.InkCandH, r.ScaleX, r.ScaleY, r.ScaleDev*100, r.AspectRef, r.AspectCand)
	}
	return fmt.Sprintf("Ink size: canvas share w %.0f%%/%.0f%%, h %.0f%%/%.0f%% (original/candidate); physical size unknown (image or PDF page size not measurable), so only the aspect ratio is checked: %.2f:1 vs %.2f:1, deviation %.1f%% >? limit. Ink bbox %dx%dpx vs %dx%dpx is NOT comparable (render DPI differs) — do not aim at it. Fix the coordinates/canvas rather than the stroke width.",
		r.ShareRefW*100, r.ShareCandW*100, r.ShareRefH*100, r.ShareCandH*100,
		r.AspectRef, r.AspectCand, r.AspectDev*100, int(r.InkRefW), int(r.InkRefH), int(r.InkCandW), int(r.InkCandH))
}

// FormatRuns 输出各对齐起点的成绩（供验收错误清单引用）。
func FormatRuns(runs []AlignmentRun) string {
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		parts = append(parts, fmt.Sprintf("%s %.2f%% @(%d,%d)", run.Name, run.Score*100, run.DX, run.DY))
	}
	return strings.Join(parts, "; ")
}

// CompareImages 是比较入口（旧线 compareFigureImages）。
func CompareImages(reference, candidate image.Image) (Comparison, *image.NRGBA, error) {
	return CompareImagesMM(reference, candidate, CanvasMM{})
}

// CompareImagesMM 是带画布物理尺寸的版本（旧线 compareFigureImagesMM 逐字）。
func CompareImagesMM(reference, candidate image.Image, mm CanvasMM) (Comparison, *image.NRGBA, error) {
	ref := flattenOnWhite(reference)
	cand := flattenOnWhite(candidate)
	if ref.Bounds().Dx() < 1 || ref.Bounds().Dy() < 1 || cand.Bounds().Dx() < 1 || cand.Bounds().Dy() < 1 {
		return Comparison{}, nil, fmt.Errorf("empty input image")
	}
	// 尺寸差要在"对齐归一化"之前量。
	scaleX, scaleY, scaleDev, aspectRef, aspectCand, aspectDev, sizeUnit,
		inkRefW, inkRefH, inkCandW, inkCandH,
		shareRefW, shareRefH, shareCandW, shareCandH := measureFigureSize(ref, cand, mm)

	ref, cand = fitCanvases(ref, cand)
	rgbRef, originRef := cropInk(ref, false)
	rgbCand, originCand := cropInk(cand, false)
	canvas := max4(rgbRef.W, rgbRef.H, rgbCand.W, rgbCand.H)
	canvas = max(24, min(512, canvas))
	levels := gridLevels(canvas)
	coarse := levels[0]
	centroidRefX, centroidRefY := inkCentroid(rgbRef)
	centroidCandX, centroidCandY := inkCentroid(rgbCand)
	centroidStart := [2]int{
		round((centroidRefY - centroidCandY) * float64(coarse) / float64(canvas)),
		round((centroidRefX - centroidCandX) * float64(coarse) / float64(canvas)),
	}
	best, err := progressiveOverlap(rgbRef, rgbCand, levels, canvas, centroidStart)
	if err != nil {
		return Comparison{}, nil, err
	}
	dx := round(float64(best.DX*canvas)/float64(levels[len(levels)-1])) + originRef.X - originCand.X
	dy := round(float64(best.DY*canvas)/float64(levels[len(levels)-1])) + originRef.Y - originCand.Y

	grayRef, grayOriginRef := cropInk(ref, true)
	grayCand, grayOriginCand := cropInk(cand, true)
	grayDX := dx - (grayOriginRef.X - grayOriginCand.X)
	grayDY := dy - (grayOriginRef.Y - grayOriginCand.Y)
	grid := levels[len(levels)-1]
	grayA, err := toGrid(grayRef, canvas, grid)
	if err != nil {
		return Comparison{}, nil, err
	}
	grayB, err := toGrid(grayCand, canvas, grid)
	if err != nil {
		return Comparison{}, nil, err
	}
	grayScore := overlapScore(grayA, grayB, round(float64(grayDY*grid)/float64(canvas)), round(float64(grayDX*grid)/float64(canvas))).Mean()

	comparison := Comparison{Similarity: best.Similarity, Channels: best.Channels, Gray: grayScore,
		DX: dx, DY: dy, Runs: best.Runs, Canvas: canvas, Grid: grid,
		ScaleX: scaleX, ScaleY: scaleY, ScaleDev: scaleDev, SizeUnit: sizeUnit,
		AspectRef: aspectRef, AspectCand: aspectCand, AspectDev: aspectDev,
		InkRefW: inkRefW, InkRefH: inkRefH, InkCandW: inkCandW, InkCandH: inkCandH,
		ShareRefW: shareRefW, ShareRefH: shareRefH, ShareCandW: shareCandW, ShareCandH: shareCandH}
	return comparison, Overlay(ref, cand, dx, dy), nil
}

func fitCanvases(reference, candidate *image.NRGBA) (*image.NRGBA, *image.NRGBA) {
	w := reference.Bounds().Dx()
	cw, ch := candidate.Bounds().Dx(), candidate.Bounds().Dy()
	if cw != w {
		nh := max(1, round(float64(ch*w)/float64(cw)))
		resized := image.NewNRGBA(image.Rect(0, 0, w, nh))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), candidate, candidate.Bounds(), draw.Src, nil)
		candidate = resized
		ch = nh
	}
	h := max(reference.Bounds().Dy(), ch)
	refCanvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	candCanvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(refCanvas, refCanvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(candCanvas, candCanvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(refCanvas, image.Rect(0, (h-reference.Bounds().Dy())/2, w, (h-reference.Bounds().Dy())/2+reference.Bounds().Dy()), reference, reference.Bounds().Min, draw.Src)
	draw.Draw(candCanvas, image.Rect(0, (h-candidate.Bounds().Dy())/2, w, (h-candidate.Bounds().Dy())/2+candidate.Bounds().Dy()), candidate, candidate.Bounds().Min, draw.Src)
	return refCanvas, candCanvas
}

// inkBox 墨迹外接框宽高（阈值与 cropInk 一致：最暗通道墨量 > 0.2）。
func inkBox(src image.Image) (int, int) {
	b := src.Bounds()
	minX, minY, maxX, maxY := b.Dx(), b.Dy(), -1, -1
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			ink := max3(1-float64(c.R)/255, 1-float64(c.G)/255, 1-float64(c.B)/255)
			if ink > 0.2 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX || maxY < minY {
		return 0, 0
	}
	return maxX - minX + 1, maxY - minY + 1
}

func cropInk(src image.Image, gray bool) (softInkImage, image.Point) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	ink := make([][3]float64, w*h)
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			var v [3]float64
			if gray {
				luma := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
				v = [3]float64{1 - luma, 1 - luma, 1 - luma}
			} else {
				v = [3]float64{1 - float64(c.R)/255, 1 - float64(c.G)/255, 1 - float64(c.B)/255}
			}
			ink[y*w+x] = v
			if max3(v[0], v[1], v[2]) > 0.2 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX || maxY < minY {
		return softInkImage{W: 1, H: 1, Pix: [][3]float64{{0, 0, 0}}}, image.Point{}
	}
	out := softInkImage{W: maxX - minX + 1, H: maxY - minY + 1,
		Pix: make([][3]float64, (maxX-minX+1)*(maxY-minY+1))}
	for y := 0; y < out.H; y++ {
		copy(out.Pix[y*out.W:(y+1)*out.W], ink[(minY+y)*w+minX:(minY+y)*w+minX+out.W])
	}
	return out, image.Pt(minX, minY)
}

func inkCentroid(ink softInkImage) (float64, float64) {
	var total, sumX, sumY float64
	for y := 0; y < ink.H; y++ {
		for x := 0; x < ink.W; x++ {
			p := ink.at(x, y)
			mass := max3(p[0], p[1], p[2])
			total += mass
			sumX += float64(x) * mass
			sumY += float64(y) * mass
		}
	}
	if total <= 0 {
		return 0, 0
	}
	return sumX / total, sumY / total
}

func gridLevels(canvas int) []int {
	finest := round(0.95 * float64(canvas))
	finest = max(8, min(1024, finest))
	final := min(finest, canvas)
	if canvas < 32 {
		return []int{canvas}
	}
	var levels []int
	for g := 32; g <= final; g *= 2 {
		levels = append(levels, g)
	}
	if len(levels) == 0 {
		levels = []int{32}
	} else if levels[len(levels)-1] < final {
		levels = append(levels, final)
	}
	return levels
}

type grid struct {
	N     int
	Cells [][3]float64
}

func toGrid(src softInkImage, canvas, n int) (grid, error) {
	if n < 1 || canvas < 1 {
		return grid{}, fmt.Errorf("invalid grid dimensions")
	}
	stride := src.W + 1
	prefix := make([][3]float64, (src.H+1)*stride)
	for y := 1; y <= src.H; y++ {
		var row [3]float64
		for x := 1; x <= src.W; x++ {
			p := src.at(x-1, y-1)
			for c := 0; c < 3; c++ {
				row[c] += p[c]
				prefix[y*stride+x][c] = prefix[(y-1)*stride+x][c] + row[c]
			}
		}
	}
	edges := make([]int, n+1)
	for i := range edges {
		edges[i] = i * canvas / n
	}
	offX, offY := (canvas-src.W)/2, (canvas-src.H)/2
	out := grid{N: n, Cells: make([][3]float64, n*n)}
	for gy := 0; gy < n; gy++ {
		y0, y1 := clampInt(edges[gy]-offY, 0, src.H), clampInt(edges[gy+1]-offY, 0, src.H)
		for gx := 0; gx < n; gx++ {
			x0, x1 := clampInt(edges[gx]-offX, 0, src.W), clampInt(edges[gx+1]-offX, 0, src.W)
			area := (edges[gx+1] - edges[gx]) * (edges[gy+1] - edges[gy])
			if area <= 0 {
				continue
			}
			var sum [3]float64
			for c := 0; c < 3; c++ {
				sum[c] = prefix[y1*stride+x1][c] - prefix[y0*stride+x1][c] - prefix[y1*stride+x0][c] + prefix[y0*stride+x0][c]
				out.Cells[gy*n+gx][c] = sum[c] / float64(area)
			}
		}
	}
	return out, nil
}

type gridOverlap struct {
	Channels [3]float64
}

func (s gridOverlap) Mean() float64 { return (s.Channels[0] + s.Channels[1] + s.Channels[2]) / 3 }

func overlapScore(a, b grid, dy, dx int) gridOverlap {
	var inter, union [3]float64
	for y := 0; y < a.N; y++ {
		for x := 0; x < a.N; x++ {
			av := a.Cells[y*a.N+x]
			by, bx := y-dy, x-dx
			if by < 0 || by >= b.N || bx < 0 || bx >= b.N {
				for c := 0; c < 3; c++ {
					union[c] += av[c]
				}
				continue
			}
			bv := b.Cells[by*b.N+bx]
			for c := 0; c < 3; c++ {
				inter[c] += math.Min(av[c], bv[c])
				union[c] += math.Max(av[c], bv[c])
			}
		}
	}
	out := gridOverlap{}
	for c := range out.Channels {
		if union[c] > 1e-9 {
			out.Channels[c] = inter[c] / union[c]
		}
	}
	return out
}

func progressiveOverlap(src, dst softInkImage, levels []int, canvas int, centroidStart [2]int) (Comparison, error) {
	grids := make([][2]grid, len(levels))
	for i, n := range levels {
		a, err := toGrid(src, canvas, n)
		if err != nil {
			return Comparison{}, err
		}
		b, err := toGrid(dst, canvas, n)
		if err != nil {
			return Comparison{}, err
		}
		grids[i] = [2]grid{a, b}
	}
	shiftCells := min(16, max(0, round(0.10*32)))
	shiftPercent := float64(shiftCells) / 32
	coarseWindow := max(1, round(shiftPercent*float64(levels[0])))
	starts := []struct {
		name string
		dx   int
		dy   int
	}{{name: "origin"}}
	if centroidStart != [2]int{} {
		starts = append(starts, struct {
			name string
			dx   int
			dy   int
		}{name: "centroid", dx: centroidStart[1], dy: centroidStart[0]})
	}
	var runs []AlignmentRun
	var bestRun AlignmentRun
	var bestCellsX, bestCellsY int
	bestScore := -1.0
	for _, start := range starts {
		shiftX, shiftY := start.dx, start.dy
		var runScore gridOverlap
		for levelIndex, pair := range grids {
			grid := levels[levelIndex]
			centerX, centerY, window := start.dx, start.dy, coarseWindow
			if levelIndex > 0 {
				prev := levels[levelIndex-1]
				centerX = round(float64(shiftX*grid) / float64(prev))
				centerY = round(float64(shiftY*grid) / float64(prev))
				window = min(2, grid)
			}
			levelBest := -1.0
			levelX, levelY := centerX, centerY
			for dy := centerY - window; dy <= centerY+window; dy++ {
				for dx := centerX - window; dx <= centerX+window; dx++ {
					score := overlapScore(pair[0], pair[1], dy, dx)
					value := score.Mean()
					if betterScore(value, dx, dy, levelBest, levelX, levelY) {
						levelBest, levelX, levelY, runScore = value, dx, dy, score
					}
				}
			}
			shiftX, shiftY = levelX, levelY
		}
		dxPx := round(float64(shiftX*canvas) / float64(levels[len(levels)-1]))
		dyPx := round(float64(shiftY*canvas) / float64(levels[len(levels)-1]))
		run := AlignmentRun{Name: start.name, Score: runScore.Mean(), DX: dxPx, DY: dyPx, Channels: runScore.Channels}
		runs = append(runs, run)
		if betterScore(run.Score, shiftX, shiftY, bestScore, bestCellsX, bestCellsY) {
			bestRun, bestScore, bestCellsX, bestCellsY = run, run.Score, shiftX, shiftY
		}
	}
	return Comparison{Similarity: bestRun.Score, Channels: bestRun.Channels, DX: bestRun.DX, DY: bestRun.DY, Runs: runs}, nil
}

func betterScore(score float64, dx, dy int, best float64, bestDX, bestDY int) bool {
	if score > best+1e-12 {
		return true
	}
	if math.Abs(score-best) <= 1e-12 {
		m, bm := absInt(dx)+absInt(dy), absInt(bestDX)+absInt(bestDY)
		if m < bm || (m == bm && (dy < bestDY || (dy == bestDY && dx < bestDX))) {
			return true
		}
	}
	return false
}

// Overlay 生成对齐叠加证据图（白=共有墨迹，品红=原图独有，绿=候选独有）。
func Overlay(reference, candidate *image.NRGBA, dx, dy int) *image.NRGBA {
	w, h := reference.Bounds().Dx(), reference.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ref := reference.NRGBAAt(x, y)
			cx, cy := x-dx, y-dy
			cand := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if image.Pt(cx, cy).In(candidate.Bounds()) {
				cand = candidate.NRGBAAt(cx, cy)
			}
			ri := [3]int{255 - int(ref.R), 255 - int(ref.G), 255 - int(ref.B)}
			ci := [3]int{255 - int(cand.R), 255 - int(cand.G), 255 - int(cand.B)}
			srcInk, candInk, shared := max3Int(ri[0], ri[1], ri[2]) > 51, max3Int(ci[0], ci[1], ci[2]) > 51, false
			for c := range ri {
				shared = shared || min(ri[c], ci[c]) > 51
			}
			switch {
			case shared:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			case srcInk && !candInk:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			case candInk && !srcInk:
				out.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
			default:
				out.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	return out
}

// SavePNG 存证据图。
func SavePNG(img *image.NRGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func max3(a, b, c float64) float64 { return math.Max(a, math.Max(b, c)) }

func max3Int(a, b, c int) int { return max(a, max(b, c)) }

func max4(a, b, c, d int) int { return max(max(a, b), max(c, d)) }

func clampInt(v, low, high int) int { return max(low, min(high, v)) }

func round(v float64) int {
	if v >= 0 {
		return int(math.Floor(v + 0.5))
	}
	return int(math.Ceil(v - 0.5))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
