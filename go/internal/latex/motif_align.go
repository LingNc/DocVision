package latex

import (
	"image"
	"math"
)

// 图片"居中重合"判据（T129）。动机：本地的逐对相似度（MotifSimilarTo）在
// 小图上过于脆弱——该书那 90 张菱形 Z 标记只有 28×38 像素，内容框居中之后
// 中心只要差半个像素，硬 mask 的 IoU 就从 0.9 掉到 0.3，于是同一枚标记被逐对
// 判据分成 20 张"像"和 69 张"不像"。
//
// 现在的判据分两步：
//  1. 每张图按前景外接框归一化到 32×32 网格，格值是"该格里前景面积的占比"
//     （软覆盖，不做二值化——二值化本身会在这么小的图上制造假差异）；
//  2. 比较两张图时允许整体平移有限格数（配置项，默认内容框边长的 10%），取
//     平移搜索下最大的 Σmin/Σmax 作为重合度。
//
// 实测（该书 130 份不同内容）：90 张会话内 + 10 张会话外的同标记图片构成一个
// 100 张的连通片，组内最差 0.647、中位 0.807，组外 30 张与组中心的最高分只有
// 0.494——0.60 的门槛两侧都留了余量。平移上限从 10% 放大到 50% 结果完全一样
// （只是比较次数多 28 倍），不搜平移（0%）则最差跌到 0.439。

const (
	// motifAlignGrid 是归一化网格边长。32 格足够区分笔画结构，又让一次
	// 比较只有 1024 个格值。
	motifAlignGrid = 32
	// motifAlignDefaultMin 是"与组中心重合度达标"的默认门槛。
	motifAlignDefaultMin = 0.60
	// motifAlignDefaultShift 是默认平移上限（占内容框边长的比例）。
	// 实测 10% 已经饱和：再放大只增加耗时，不改变任何一对的结论。
	motifAlignDefaultShift = 0.10
	// motifAlignMaxPixels 限制参与该判据的图：一张 4M 像素的书页扫描件没有
	// 必要走"图标重合"，而且逐像素取 mask 的内存代价不值得。超限的图退回
	// 旧的逐对相似度判据。
	motifAlignMaxPixels = 4_000_000
	// motifAlignMaxArea 限制前景外接框面积。大图即使总像素不多（细长条）
	// 也不适合拿来做图标重合。
	motifAlignMaxArea = 200_000
	// motifAlignInkSum 是"算墨"的阈值（R+G+B 之和）。这里比 foregroundMask
	// 的 720 更紧一点：JPEG 纸面噪声（灰阶 236-239）会把内容框撑到整张图，
	// 归一化随即失真——实测同一批图，阈值 240 时组内最差重合度从 0.647 掉到
	// 0.548。
	motifAlignInkSum = 690
)

// motifAlignImage 是一张图归一化后的软覆盖网格。
type motifAlignImage struct {
	grid []float64
	area int // 前景外接框的像素面积，用来挑"内容最全"的代表图
}

// motifAlignCoverage 按前景外接框把图压缩成 32×32 的软覆盖网格。
// 拿不到覆盖率的图（没有前景、太大）返回 false，调用方退回逐对相似度。
func motifAlignCoverage(img image.Image) (motifAlignImage, bool) {
	if img == nil {
		return motifAlignImage{}, false
	}
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 || b.Dx()*b.Dy() > motifAlignMaxPixels {
		return motifAlignImage{}, false
	}
	mask := make([]bool, b.Dx()*b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := rgbaAt(img, b.Min.X+x, b.Min.Y+y)
			mask[y*b.Dx()+x] = c.A > 16 && int(c.R)+int(c.G)+int(c.B) < motifAlignInkSum
		}
	}
	n, minX, minY, maxX, maxY := foregroundComponents(mask, b.Dx(), b.Dy())
	if n == 0 || maxX < minX || maxY < minY {
		return motifAlignImage{}, false
	}
	w, h := maxX-minX+1, maxY-minY+1
	if w*h > motifAlignMaxArea {
		return motifAlignImage{}, false
	}
	out := motifAlignImage{grid: make([]float64, motifAlignGrid*motifAlignGrid), area: w * h}
	for gy := 0; gy < motifAlignGrid; gy++ {
		y0 := minY + gy*h/motifAlignGrid
		y1 := minY + (gy+1)*h/motifAlignGrid
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for gx := 0; gx < motifAlignGrid; gx++ {
			x0 := minX + gx*w/motifAlignGrid
			x1 := minX + (gx+1)*w/motifAlignGrid
			if x1 <= x0 {
				x1 = x0 + 1
			}
			ink, total := 0, 0
			for y := y0; y < y1 && y < b.Dy(); y++ {
				for x := x0; x < x1 && x < b.Dx(); x++ {
					total++
					if mask[y*b.Dx()+x] {
						ink++
					}
				}
			}
			if total > 0 {
				out.grid[gy*motifAlignGrid+gx] = float64(ink) / float64(total)
			}
		}
	}
	return out, true
}

// overlap 计算两张图在允许平移 shift 格前提下的最大软重合度
// Σmin / Σmax（值域 0..1）。
func (a motifAlignImage) overlap(b motifAlignImage, shift int) float64 {
	if len(a.grid) != len(b.grid) {
		return 0
	}
	if shift < 0 {
		shift = 0
	}
	if max := motifAlignGrid / 2; shift > max {
		shift = max
	}
	best := 0.0
	for dx := -shift; dx <= shift; dx++ {
		for dy := -shift; dy <= shift; dy++ {
			inter, union := 0.0, 0.0
			for gy := 0; gy < motifAlignGrid; gy++ {
				sy := gy - dy
				for gx := 0; gx < motifAlignGrid; gx++ {
					va := a.grid[gy*motifAlignGrid+gx]
					vb := 0.0
					sx := gx - dx
					if sy >= 0 && sy < motifAlignGrid && sx >= 0 && sx < motifAlignGrid {
						vb = b.grid[sy*motifAlignGrid+sx]
					}
					if va < vb {
						inter += va
					} else {
						inter += vb
					}
					if va > vb {
						union += va
					} else {
						union += vb
					}
				}
			}
			if union > 0 {
				if v := inter / union; v > best {
					best = v
				}
			}
		}
	}
	return best
}

// motifAlignShiftCells 把配置比例换算成平移格数（0..16）。
func motifAlignShiftCells(pct float64) int {
	if pct <= 0 {
		return 0
	}
	shift := int(math.Round(pct * motifAlignGrid))
	if max := motifAlignGrid / 2; shift > max {
		shift = max
	}
	return shift
}

// motifAlignMin 是当前配置的重合度门槛（未配置时用默认 0.60）。
func (r *Runner) motifAlignMin() float64 {
	v := r.cfg.Latex.DedupeAlignMin
	if v <= 0 {
		v = motifAlignDefaultMin
	}
	if v > 1 {
		v = 1
	}
	return v
}

// motifAlignShift 是当前配置的平移格数。
func (r *Runner) motifAlignShift() int {
	pct := r.cfg.Latex.DedupeAlignShift
	if pct <= 0 {
		pct = motifAlignDefaultShift
	}
	return motifAlignShiftCells(pct)
}

// motifMergeAllowed 是"这两张能不能共用一张成品"的判据：先用居中重合度，
// 拿不到覆盖率（大图/无前景）时退回逐对相似度。两个判据都是保守的，任一
// 通过即可合并。
func (r *Runner) motifMergeAllowed(a, b *motifCandidate) bool {
	if a == nil || b == nil {
		return false
	}
	if ca, ok := motifAlignCoverage(a.image); ok {
		if cb, ok := motifAlignCoverage(b.image); ok {
			if ca.overlap(cb, r.motifAlignShift()) >= r.motifAlignMin() {
				return true
			}
		}
	}
	return MotifSimilarTo(a.image, b.image)
}

// motifAlignFilter 是组内闸门：以组中心（平均重合度最高的成员）为基准，
// 重合度达标的成员合并，其余保持独立；拿不到覆盖率的成员退回旧的逐对判据。
// 代表图优先用模型指定的 key（它看过图），但要求它自己达标且内容框足够完整
// （≥ 合格成员最大面积的 90%），否则取合格成员里内容框最大的一张——那才是
// "覆盖了必要内容"的那张。
func (r *Runner) motifAlignFilter(cands []*motifCandidate, key *motifCandidate) (safe []*motifCandidate, chosen *motifCandidate, dropped int, worst float64, aligned bool) {
	min := r.motifAlignMin()
	shift := r.motifAlignShift()
	covered := map[*motifCandidate]motifAlignImage{}
	for _, c := range cands {
		if cov, ok := motifAlignCoverage(c.image); ok {
			covered[c] = cov
		}
	}
	if len(covered) < 2 {
		// 全是大图或者没有前景：保留原来的逐对判据。
		safe = make([]*motifCandidate, 0, len(cands))
		for _, c := range cands {
			if key != nil && c != key && !MotifSimilarTo(c.image, key.image) {
				dropped++
				continue
			}
			safe = append(safe, c)
		}
		return safe, key, dropped, 0, false
	}
	// 组中心 = 与其余成员平均重合度最高的那张。
	center := (*motifCandidate)(nil)
	bestMean := -1.0
	scores := map[*motifCandidate]float64{}
	// 按 cands 的顺序遍历：并列时先出现的胜出，结果与 map 迭代顺序无关。
	for _, c := range cands {
		cov, ok := covered[c]
		if !ok {
			continue
		}
		sum, n := 0.0, 0
		for o, oc := range covered {
			if o == c {
				continue
			}
			sum += cov.overlap(oc, shift)
			n++
		}
		mean := 0.0
		if n > 0 {
			mean = sum / float64(n)
		}
		scores[c] = mean
		if mean > bestMean {
			bestMean, center = mean, c
		}
	}
	// 逐个成员与中心比；大图成员用逐对相似度对中心判定。
	for _, c := range cands {
		if cov, ok := covered[c]; ok {
			v := cov.overlap(covered[center], shift)
			scores[c] = v
			if v < min {
				dropped++
				continue
			}
			safe = append(safe, c)
			if worst == 0 || v < worst {
				worst = v
			}
			continue
		}
		// 拿不到覆盖率的成员（大图）：模型点名的代表图与中心都放行，
		// 其余按逐对相似度对中心判定。
		if c == key || (center != nil && (c == center || MotifSimilarTo(c.image, center.image))) {
			safe = append(safe, c)
			continue
		}
		dropped++
	}
	// 代表图：模型的 key 达标且内容足够全就尊重它，否则挑内容最全的合格成员。
	if key != nil && motifContains(safe, key) && len(covered[key].grid) > 0 {
		maxArea := 0
		for _, c := range safe {
			if cov, ok := covered[c]; ok && cov.area > maxArea {
				maxArea = cov.area
			}
		}
		if cov, ok := covered[key]; ok && float64(cov.area) >= 0.9*float64(maxArea) {
			chosen = key
		}
	}
	if chosen == nil {
		bestArea, bestScore := -1, -1.0
		for _, c := range safe {
			cov, ok := covered[c]
			if !ok {
				continue
			}
			if cov.area > bestArea || (cov.area == bestArea && scores[c] > bestScore) {
				bestArea, bestScore, chosen = cov.area, scores[c], c
			}
		}
	}
	if chosen == nil {
		chosen = center
	}
	return safe, chosen, dropped, worst, true
}
