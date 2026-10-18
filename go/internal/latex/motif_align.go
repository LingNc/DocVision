package latex

import (
	"image"
	"math"
	"strings"

	"mineru-tools/internal/config"
)

// 图片"居中重合"判据（T129 起、T137/T138 定稿）。
//
// 动机：本地的逐对相似度（MotifSimilarTo）在小图上过于脆弱——该书那 90 张
// 菱形 Z 标记只有 28×38 像素，中心差半个像素，硬 mask 的 IoU 就从 0.9 掉到
// 0.3，于是同一枚标记被分成 20 张"像"和 69 张"不像"。
//
// 现在的判据（T138 定稿，逐条对应 issue.md）：
//  1. **非二值化**：每像素取软墨量 w = 1 − 通道/255（三通道各自算），
//     内容框 = 软墨量 > 0.2 的外接框；
//  2. **S 自适应**：画布边长 S = clamp(max(this, that) 的内容框长边, 24, 512)，
//     **按每一对**算（不按池子取 P90——同一池子里混着 28×38 的小图标和整页
//     截图时，P90 会让小图标在画布里只占一角，重合度被系统性压低）；
//  3. **不缩放**：内容框居中贴进 S×S，保持原有尺寸与比例；
//  4. **逐级搜索**：G = 32, 64, … 到最细一级；0 级全窗 ±10%，之后以上一层
//     的解（换算成本层格号，即 2×）为中心 ±2 格，不再扩窗（T143）；
//  5. **判定分 = 最终（最细）那一级的分数**，不额外锁定某一层。
//
// 实测（A/B 交换图，仅 f→β、X→W 两处不同，内容框 171×390）：S=395、
// 逐级到 G=256 时异符号 0.378、同符号自比 1.000；人工抖动变体（非对称白边 +
// 噪声）同符号 0.52–0.71、异符号 0.25–0.42，两侧仍有明显间隔。

const (
	// motifAlignGrid 是旧判据固定的网格边长，现在只有"平移格数换算"还用
	// 得到它（配置里的平移比例按旧 32 格的刻度表达，保持兼容）。
	motifAlignGrid = 32
	// motifAlignDefaultMin 是"与组中心重合度达标"的默认门槛。
	motifAlignDefaultMin = 0.60
	// motifAlignDefaultShift 是默认平移上限（占内容框边长的比例）。
	motifAlignDefaultShift = 0.10
	// motifAlignTypical 是"典型成员"的默认门槛：与组中心重合度达到它的成员
	// 才算"大多数图都长这样"。代表图只从这一批里挑（T131）。
	motifAlignTypical = 0.95
	// motifAlignMaxPixels 限制参与该判据的图：一张 4M 像素的书页扫描件没有
	// 必要走"图标重合"。
	motifAlignMaxPixels = 4_000_000
	// 预筛阈值：墨量占比差 > 0.12 或长宽比差 > 0.35 的两张小图标，平移搜完也
	// 不可能重合（T130 起用于桶扫描，T151 起逐对明细同一口径）。
	motifAlignInkFracMax = 0.12
	motifAlignAspectMax  = 0.35
	// motifAutoIconMaxArea 是"自动归组"允许的最大内容框面积（约 141×141）：
	// 自动合并意味着不再有人看原图，只留给真正的小图标。
	motifAutoIconMaxArea = 20_000
)

// 三个由 dedupe 入口从配置注入的判据参数（未被注入时取这里的默认值）。
var (
	// motifAlignGridSpec 是判据最细一级的格数规格（latex.dedupe_align_grid，
	// 写入 "32" 或 "0.95S"）。
	motifAlignGridSpec = "0.95S"
	// motifAlignIconMaxArea 是"小图标"的内容框面积上限，超过它的图不参与
	// 居中重合判据（大图/照片仍只走逐对相似度）。
	motifAlignIconMaxArea = 200_000
	// motifAlignBucketMax 是桶里最多多少张小图标参与两两比较。
	motifAlignBucketMax = 400
)

// applyMotifMetricConfig 把配置里的判据参数注入包级状态（dedupe 入口调用一次）。
// 这几个参数没有随 Runner 逐次传递，是因为 motifAlignCoverage/overlap 是
// 纯函数、被候选扫描与产物落盘共用；一次 dedupe 内它们不变。
func applyMotifMetricConfig(r *Runner) {
	if r == nil || r.cfg == nil {
		return
	}
	if v := strings.TrimSpace(r.cfg.Latex.DedupeAlignGrid); v != "" {
		motifAlignGridSpec = v
	}
	motifAlignIconMaxArea = r.motifIconMaxArea()
	motifAlignBucketMax = r.motifBucketMax()
}

// motifAlignFinest 按规格算最细一级的格数。
func motifAlignFinest(canvas int) int {
	g, err := config.ParseAlignGrid(motifAlignGridSpec, canvas)
	if err != nil || g <= 0 {
		return canvas * 95 / 100
	}
	return g
}

// motifAlignImage 是一张图的内容框（软墨量）及其派生量。
type motifAlignImage struct {
	canon   motifCanonImage
	area    int     // 内容框像素面积，用来挑"内容最全"的代表图
	aspect  float64 // 内容框宽/高
	inkFrac float64 // 内容框里的平均软墨量（去重前预筛用）
}

// motifAlignCoverage 取内容框（软墨量，不二值化、不缩放）。
// 拿不到覆盖率的图（没有前景、太大）返回 false，调用方退回逐对相似度。
func motifAlignCoverage(img image.Image) (motifAlignImage, bool) {
	if img == nil {
		return motifAlignImage{}, false
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx()*b.Dy() > motifAlignMaxPixels {
		return motifAlignImage{}, false
	}
	canon, ok := motifCanonFromImage(img)
	if !ok || canon.w <= 0 || canon.h <= 0 {
		return motifAlignImage{}, false
	}
	// 内容框太小（1×1 点阵、分隔线残留）没有结构可比，退回逐对相似度。
	if canon.w < 3 || canon.h < 3 || canon.w*canon.h < 16 {
		return motifAlignImage{}, false
	}
	var sum float64
	for _, v := range canon.px {
		sum += float64(v)
	}
	ink := sum / float64(len(canon.px))
	if ink < 0.01 {
		// 只有纸面噪声（JPEG 灰阶 236 起）：内容框会撑到整张图，归一化失真。
		return motifAlignImage{}, false
	}
	return motifAlignImage{
		canon:   canon,
		area:    canon.w * canon.h,
		aspect:  float64(canon.w) / float64(canon.h),
		inkFrac: ink,
	}, true
}

// motifAlignDetail 是一次居中重合度比较的完整过程量（T146 写进 metrics 用）：
// 除了分数，还留下判定层、画布、逐级搜出来的平移量与级数——调阈值或怀疑
// "某一对为什么只有 0.6"时，光看分数无法判断是尺寸差异还是平移没搜对。
type motifAlignDetail struct {
	Score  float64
	Level  int // 判定层的格数 G
	Canvas int // 画布边长 S（这一对取两者内容框长边的较大者）
	Levels int // 一共搜了几级（32 → … → 判定层）
	ShiftY int // 判定层上的平移格数：内容框相对居中位置往下/右挪几格
	ShiftX int
}

// motifPairCanvas 是一对图共用的画布边长：取两者内容框长边的较大者，
// 夹在 [motifCanonMinS, motifCanonMaxS]（同一批图里这就是池子的尺子）。
func motifPairCanvas(a, b motifAlignImage) int {
	canvas := a.canon.maxSide()
	if s := b.canon.maxSide(); s > canvas {
		canvas = s
	}
	if canvas < motifCanonMinS {
		canvas = motifCanonMinS
	}
	if canvas > motifCanonMaxS {
		canvas = motifCanonMaxS
	}
	return canvas
}

// motifAlignWorthScore 是"这一对值得算一次居中重合"的廉价前筛：墨量占比差或
// 长宽比差太远的两张图，把平移搜完也不可能是同一枚标记。桶扫描（T130）与逐对
// 明细（T151）共用同一口径，两处结果才解释得通。
func motifAlignWorthScore(a, b motifAlignImage) bool {
	if absFloat(a.inkFrac-b.inkFrac) > motifAlignInkFracMax {
		return false
	}
	return absFloat(a.aspect-b.aspect) <= motifAlignAspectMax
}

// motifAlignCost 估一对图的打分代价（画布² × 级数），逐对明细按它记账。
func motifAlignCost(a, b motifAlignImage) int {
	canvas := motifPairCanvas(a, b)
	return canvas * canvas * len(motifCanonLevels(canvas, motifAlignFinest(canvas)))
}

// overlapDetail 同 overlap，另外给出判定层、画布、平移量与级数（写 metrics 用）。
func (a motifAlignImage) overlapDetail(b motifAlignImage, shiftCells int) motifAlignDetail {
	canvas := motifPairCanvas(a, b)
	finest := motifAlignFinest(canvas)
	pct := float64(shiftCells) / float64(motifAlignGrid)
	score, dy, dx, g := motifCanonScoreShift(a.canon, b.canon, canvas, finest, pct)
	return motifAlignDetail{
		Score: score, Level: g, Canvas: canvas,
		Levels: len(motifCanonLevels(canvas, finest)),
		ShiftY: dy, ShiftX: dx,
	}
}

// overlap 算两张图的居中重合度：S 取两者内容框长边的较大者（同一批图里
// 就是池子的尺子），平移上限按 shiftCells/32 的比例（0.10 → 9.4%）。
func (a motifAlignImage) overlap(b motifAlignImage, shiftCells int) float64 {
	canvas := motifPairCanvas(a, b)
	pct := float64(shiftCells) / float64(motifAlignGrid)
	score, _, _, _ := motifCanonScoreShift(a.canon, b.canon, canvas, motifAlignFinest(canvas), pct)
	return score
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

// motifIconAutoMin 返回"小图标本地自动归组"的门槛；0 = 关闭自动归组，
// 全部交核验会话（T131）。默认值由配置加载填（0.75），Runner 零值即关闭。
func (r *Runner) motifIconAutoMin() float64 {
	if r == nil || r.cfg == nil {
		return 0
	}
	v := r.cfg.Latex.DedupeIconAuto
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// motifAlignTypical 是"典型成员"门槛（配置，未写时 0.95）。
func (r *Runner) motifAlignTypical() float64 {
	if r == nil || r.cfg == nil {
		return motifAlignTypical
	}
	v := r.cfg.Latex.DedupeAlignTypical
	if v <= 0 {
		v = motifAlignTypical
	}
	if v > 1 {
		v = 1
	}
	return v
}

// motifIconMaxArea 是能用居中重合判据的最大内容框面积（配置，未写时 200000
// ≈ 447×447；默认口径是"内容框 ≤200×200 像素"）。
func (r *Runner) motifIconMaxArea() int {
	if r == nil || r.cfg == nil {
		return 200_000
	}
	if v := r.cfg.Latex.DedupeIconMaxArea; v > 0 {
		return v
	}
	return 200_000
}

// motifBucketMax 是桶里最多多少张小图标参与重合度两两比较（配置，默认 400）。
func (r *Runner) motifBucketMax() int {
	if r == nil || r.cfg == nil {
		return 400
	}
	if v := r.cfg.Latex.DedupeBucketMax; v > 0 {
		return v
	}
	return 400
}

// motifAttachMin 是"未入桶的图挂靠已确认组"的门槛（配置，默认 0.60）。
func (r *Runner) motifAttachMin() float64 {
	if r == nil || r.cfg == nil {
		return 0.60
	}
	v := r.cfg.Latex.DedupeAttachMin
	if v <= 0 {
		return 0.60
	}
	if v > 1 {
		return 1
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
	if ca, ok := a.coverage(); ok {
		if cb, ok := b.coverage(); ok {
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
func (r *Runner) motifAlignFilter(cands []*motifCandidate, key *motifCandidate) (safe []*motifCandidate, chosen *motifCandidate, dropped int, worst float64, aligned bool, carried int) {
	min := r.motifAlignMin()
	shift := r.motifAlignShift()
	covered := map[*motifCandidate]motifAlignImage{}
	for _, c := range cands {
		if cov, ok := c.coverage(); ok {
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
		return safe, key, dropped, 0, false, 0
	}
	// 组中心 = 与其余成员平均重合度最高的那张。顺手把两两重合度留下来：
	// 下面"离中心不足、但组内成员之间达标"的那几张要复用它，不再重算一遍。
	center := (*motifCandidate)(nil)
	bestMean := -1.0
	scores := map[*motifCandidate]float64{}
	order := make([]*motifCandidate, 0, len(covered))
	for _, c := range cands {
		if _, ok := covered[c]; ok {
			order = append(order, c)
		}
	}
	at := make(map[*motifCandidate]int, len(order))
	for i, c := range order {
		at[c] = i
	}
	pairs := make([][]float64, len(order))
	for i := range pairs {
		pairs[i] = make([]float64, len(order))
		// 对角线是"自己跟自己"的重合度，与逐对判据一致取 1：组中心落在自己
		// 那一格上，若留 0 会把中心本身判成"离中心不足"。
		pairs[i][i] = 1
	}
	// 按 cands 的顺序遍历：并列时先出现的胜出，结果与 map 迭代顺序无关。
	for i, c := range order {
		cov := covered[c]
		sum := 0.0
		for j, o := range order {
			if o == c {
				continue
			}
			v := cov.overlap(covered[o], shift)
			pairs[i][j], pairs[j][i] = v, v
			sum += v
		}
		mean := 0.0
		if len(order) > 1 {
			mean = sum / float64(len(order)-1)
		}
		scores[c] = mean
		if mean > bestMean {
			bestMean, center = mean, c
		}
	}
	// 逐个成员与中心比；大图成员用逐对相似度对中心判定。
	var below []*motifCandidate
	for _, c := range cands {
		if _, ok := covered[c]; ok {
			v := pairs[at[c]][at[center]]
			scores[c] = v
			if v < min {
				below = append(below, c)
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
	// 会话把这张图并进了这一组、但它离组中心不足门槛时，别急着判它独立：同一
	// 张图换个分辨率/长宽比，与中心的分数会明显偏低，可它与组内**某个已经达标
	// 的成员**往往照样够分（实测：4 张离中心 0.52–0.64，与组内成员 0.686–0.812，
	// 四张确实是同一枚图）。带上来的成员不参与代表图挑选（它离中心不够，代表
	// 不了这一组）。
	carriedSet := map[*motifCandidate]bool{}
	if len(below) > 0 && len(safe) > 0 {
		score := func(a, b *motifCandidate) (float64, bool) {
			ia, ok := at[a]
			if !ok {
				return 0, false
			}
			ib, ok := at[b]
			if !ok {
				return 0, false
			}
			return pairs[ia][ib], true
		}
		kept, n := motifCarryOver(below, safe, score, min)
		for _, c := range kept {
			safe = append(safe, c)
			carriedSet[c] = true
		}
		carried = n
	}
	dropped += len(below) - carried
	// 代表图：先看"既典型又全"的那一批（T131 用户口径）——与组中心重合度
	// ≥ motifAlignTypical 的成员里挑内容框最大的，这样代表图既是大多数图都
	// 符合的样子，又不会因为某张被裁掉一角而让整组重画缺内容。没有这样的
	// 成员时退回旧顺序：模型点名的 key（达标且内容够全）→ 合格成员里内容框
	// 最大 → 组中心。
	chosen = nil
	bestArea, bestScore := -1, -1.0
	for _, c := range safe {
		cov, ok := covered[c]
		if !ok || carriedSet[c] || scores[c] < r.motifAlignTypical() {
			continue
		}
		if cov.area > bestArea || (cov.area == bestArea && scores[c] > bestScore) {
			bestArea, bestScore, chosen = cov.area, scores[c], c
		}
	}
	if chosen == nil && key != nil && motifContains(safe, key) && covered[key].canon.w > 0 {
		maxArea := 0
		for _, c := range safe {
			if cov, ok := covered[c]; ok && !carriedSet[c] && cov.area > maxArea {
				maxArea = cov.area
			}
		}
		if cov, ok := covered[key]; ok && float64(cov.area) >= 0.9*float64(maxArea) {
			chosen = key
		}
	}
	if chosen == nil {
		bestArea, bestScore = -1, -1.0
		for _, c := range safe {
			cov, ok := covered[c]
			if !ok || carriedSet[c] {
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
	return safe, chosen, dropped, worst, true, carried
}

// motifCarryOver 从"离组中心不足门槛"的成员里挑出仍可随组带入的那几张：只要
// 与 safe 里**原本就达标**的某个成员够分就带上（score 给两两重合度，拿不到就
// 当不够）。只跟安全集合的初始部分比、不拿带进来的成员当跳板——0.60 这个门槛
// 是可传递的（同一枚图换分辨率/长宽比会掉分），任由接力会把整条相似链并成一张
// 图。safe 只读不改，由调用方决定怎么并入。
func motifCarryOver(below, safe []*motifCandidate, score func(a, b *motifCandidate) (float64, bool), min float64) (kept []*motifCandidate, carried int) {
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
