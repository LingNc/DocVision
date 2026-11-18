package dedupe

import (
	"image"
	"math"
)

// 居中重合判据的阈值与预筛参数（旧线 motif_align.go 逐字）。
const (
	// AlignDefaultMin 是"与组中心重合度达标"的默认门槛。
	AlignDefaultMin = 0.60
	// AlignDefaultShift 是默认平移上限（占内容框边长的比例）。
	AlignDefaultShift = 0.10
	// AlignTypical 是"典型成员"的默认门槛：与组中心重合度达到它的成员
	// 才算"大多数图都长这样"。代表图只从这一批里挑（T131）。
	AlignTypical = 0.95
	// AlignMaxPixels 限制参与该判据的图：4M 像素的书页扫描件没必要走"图标重合"。
	AlignMaxPixels = 4_000_000
	// 预筛阈值：墨量占比差 > 0.12 或长宽比差 > 0.35 的两张小图标，
	// 平移搜完也不可能重合（T130 桶扫描、T151 逐对明细同一口径）。
	AlignInkFracMax = 0.12
	AlignAspectMax  = 0.35
)

// AlignImage 是一张图的内容框（软墨量）及其派生量。
type AlignImage struct {
	Canon   CanonImage
	Area    int     // 内容框像素面积（挑"内容最全"代表图用）
	Aspect  float64 // 内容框宽/高
	InkFrac float64 // 内容框里的平均软墨量（去重前预筛用）
}

// AlignCoverage 取内容框（软墨量，不二值化、不缩放）。
// 拿不到覆盖率的图（没有前景、太大、内容框太小、纸面噪声）返回 false，
// 调用方退回逐对相似度（旧线"拿不到就退回"判据）。
func AlignCoverage(img image.Image) (AlignImage, bool) {
	if img == nil {
		return AlignImage{}, false
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx()*b.Dy() > AlignMaxPixels {
		return AlignImage{}, false
	}
	canon, ok := CanonFromImage(img)
	if !ok || canon.W <= 0 || canon.H <= 0 {
		return AlignImage{}, false
	}
	// 内容框太小（1×1 点阵、分隔线残留）没有结构可比，退回逐对相似度。
	if canon.W < 3 || canon.H < 3 || canon.W*canon.H < 16 {
		return AlignImage{}, false
	}
	var sum float64
	for _, v := range canon.Px {
		sum += float64(v)
	}
	ink := sum / float64(len(canon.Px))
	if ink < 0.01 {
		// 只有纸面噪声（JPEG 灰阶 236 起）：内容框会撑到整张图，归一化失真。
		return AlignImage{}, false
	}
	return AlignImage{
		Canon:   canon,
		Area:    canon.W * canon.H,
		Aspect:  float64(canon.W) / float64(canon.H),
		InkFrac: ink,
	}, true
}

// AlignDetail 是一次居中重合度比较的完整过程量（T146 写 metrics 用）：
// 除了分数，还留下判定层、画布、逐级搜出来的平移量与级数——调阈值或怀疑
// "某一对为什么只有 0.6"时，光看分数无法判断是尺寸差异还是平移没搜对。
type AlignDetail struct {
	Score  float64
	Level  int // 判定层的格数 G
	Canvas int // 画布边长 S（这一对取两者内容框长边的较大者）
	Levels int // 一共搜了几级（32 → … → 判定层）
	ShiftY int // 判定层上的平移格数：内容框相对居中位置往下/右挪几格
	ShiftX int
}

// PairCanvas 是一对图共用的画布边长：取两者内容框长边的较大者（同一批图里
// 这就是池子的尺子），夹在 [CanonMinS, CanonMaxS]。
func PairCanvas(a, b AlignImage) int {
	canvas := a.Canon.MaxSide()
	if s := b.Canon.MaxSide(); s > canvas {
		canvas = s
	}
	if canvas < CanonMinS {
		canvas = CanonMinS
	}
	if canvas > CanonMaxS {
		canvas = CanonMaxS
	}
	return canvas
}

// WorthScore 是"这一对值得算一次居中重合"的廉价前筛：墨量占比差或
// 长宽比差太远的两张图，把平移搜完也不可能是同一枚标记。
func WorthScore(a, b AlignImage) bool {
	if absFloat(a.InkFrac-b.InkFrac) > AlignInkFracMax {
		return false
	}
	return absFloat(a.Aspect-b.Aspect) <= AlignAspectMax
}

// AlignCost 估一对图的打分代价（画布² × 级数），逐对明细按它记账。
func AlignCost(a, b AlignImage) int {
	canvas := PairCanvas(a, b)
	return canvas * canvas * len(CanonLevels(canvas, Finest(canvas)))
}

// OverlapDetail 同 Overlap，另外给出判定层、画布、平移量与级数（写 metrics 用）。
func (a AlignImage) OverlapDetail(b AlignImage, shiftCells int) AlignDetail {
	canvas := PairCanvas(a, b)
	finest := Finest(canvas)
	pct := float64(shiftCells) / float64(AlignGrid)
	score, dy, dx, g := CanonScoreShift(a.Canon, b.Canon, canvas, finest, pct)
	return AlignDetail{
		Score: score, Level: g, Canvas: canvas,
		Levels: len(CanonLevels(canvas, finest)),
		ShiftY: dy, ShiftX: dx,
	}
}

// Overlap 算两张图的居中重合度：S 取两者内容框长边的较大者，平移上限按
// shiftCells/32 的比例（0.10 → 9.4%）。
func (a AlignImage) Overlap(b AlignImage, shiftCells int) float64 {
	canvas := PairCanvas(a, b)
	pct := float64(shiftCells) / float64(AlignGrid)
	score, _, _, _ := CanonScoreShift(a.Canon, b.Canon, canvas, Finest(canvas), pct)
	return score
}

// ShiftCells 把平移比例换算成平移格数（0..16）。
func ShiftCells(pct float64) int {
	if pct <= 0 {
		return 0
	}
	shift := int(math.Round(pct * float64(AlignGrid)))
	if max := AlignGrid / 2; shift > max {
		shift = max
	}
	return shift
}

// CarryOver 从"离组中心不足门槛"的成员里挑出仍可随组带入的那几张：只要
// 与 safe 里**原本就达标**的某个成员够分就带上。只跟安全集合的初始部分比、
// 不拿带进来的成员当跳板——0.60 这个门槛是可传递的（同一枚图换分辨率/长宽比
// 会掉分），任由接力会把整条相似链并成一张图（T151 判据）。
func CarryOver(below, safe []string, score func(a, b string) (float64, bool), min float64) (kept []string, carried int) {
	for _, c := range below {
		for _, s := range safe {
			if v, ok := score(c, s); ok && v >= min {
				kept = append(kept, c)
				carried++
				break
			}
		}
	}
	return kept, carried
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
