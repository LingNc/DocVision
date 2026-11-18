package dedupe

import (
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
)

// candidates.go：候选桶检测（sha256 精确收缩 + 保守视觉相似 + 小图标居中重合
// 并桶），从旧线 motif_candidates.go 逐字移植。桶与组内闸门同一套判据（T130）。

const (
	motifMaxWidth  = 70
	motifMaxHeight = 70
	motifGrid      = 20
	// IconMaxArea 是"小图标"内容框面积上限，超过的图不参与居中重合判据（插图误并风险不对等）。
	IconMaxArea = 200_000
	// BucketMax 是桶里最多多少张小图标参与两两比较，超过退回逐对相似度。
	BucketMax = 400
)

// Sample 是一张待比较图（上游已选好，如已分类矢量图；本包不看分类与文档上下文）。
type Sample struct {
	Key   string
	Image image.Image
}

// Group 是一个等价候选组：Exact 仅当全部成员尺寸与像素一致。
type Group struct {
	Members []string
	Exact   bool
}

// FindCandidatesAligned 在逐对相似度之外，把小图标类里"居中重合度"达标的图
// 并进同一个桶（T130）。alignMin ≤ 0 时行为与 FindCandidates 完全一致。
// 第二返回值是居中重合并进来的对数；-1 表示图标过多退回旧判据。
func FindCandidatesAligned(samples []Sample, alignMin, shiftPct float64) ([]Group, int) {
	if alignMin <= 0 {
		groups, _ := findCandidates(samples, 0, 0)
		return groups, 0
	}
	return findCandidates(samples, alignMin, ShiftCells(shiftPct))
}

// FindCandidates 是"只走逐对相似度"的入口。
func FindCandidates(samples []Sample) []Group {
	groups, _ := findCandidates(samples, 0, 0)
	return groups
}

func findCandidates(samples []Sample, alignMin float64, shiftCells int) ([]Group, int) {
	valid := make([]Sample, 0, len(samples))
	seen := map[string]bool{}
	for _, s := range samples {
		if s.Key == "" || s.Image == nil || seen[s.Key] {
			continue
		}
		seen[s.Key] = true
		valid = append(valid, s)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Key < valid[j].Key })

	parent := make([]int, len(valid))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) bool {
		a, b = find(a), find(b)
		if a == b {
			return false
		}
		parent[b] = a
		return true
	}

	// 像素指纹与形状/颜色特征都是像素纯函数，每只图算一次（O(N²) 对循环里重建
	// 388 张图要 ~150k 次重建；结论与逐对算完全一致）。
	features := make([]motifFeatureSet, len(valid))
	for i, s := range valid {
		features[i] = computeMotifFeatureSet(s.Image)
	}
	exact := map[[32]byte]int{}
	for i := range valid {
		if !features[i].rgbaOK {
			continue
		}
		h := features[i].rgba
		if j, exists := exact[h]; exists {
			union(i, j)
		} else {
			exact[h] = i
		}
	}
	for i := range valid {
		for j := i + 1; j < len(valid); j++ {
			if motifFeatureSetSimilar(features[i], features[j]) {
				union(i, j)
			}
		}
	}
	alignMerges := 0
	if alignMin > 0 {
		// 像素完全相同的图只比一份（T140）：同一份内容重合度结论必然相同。
		uniq := make([]int, 0, len(valid))
		seenContent := map[[32]byte]bool{}
		for i := range valid {
			if features[i].rgbaOK {
				if seenContent[features[i].rgba] {
					continue
				}
				seenContent[features[i].rgba] = true
			}
			uniq = append(uniq, i)
		}
		alignMerges = findAlignedMotifPairs(valid, uniq, alignMin, shiftCells, find, union)
	}

	groups := map[int][]string{}
	rootRep := map[int]int{}
	for i, s := range valid {
		root := find(i)
		if _, ok := rootRep[root]; !ok {
			rootRep[root] = i
		}
		groups[root] = append(groups[root], s.Key)
	}
	out := make([]Group, 0, len(groups))
	for root, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		rep := features[rootRep[root]]
		isExact := rep.rgbaOK
		if isExact {
			for _, key := range members {
				for i, s := range valid {
					if s.Key != key {
						continue
					}
					if !features[i].rgbaOK || features[i].rgba != rep.rgba {
						isExact = false
					}
					break
				}
			}
		}
		out = append(out, Group{Members: members, Exact: isExact})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Members[0] < out[j].Members[0] })
	return out, alignMerges
}

func rgbaFingerprint(img image.Image) ([32]byte, bool) {
	var zero [32]byte
	b := img.Bounds()
	if b.Empty() {
		return zero, false
	}
	h := sha256.New()
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[:4], uint32(b.Dx()))
	binary.BigEndian.PutUint32(buf[4:], uint32(b.Dy()))
	_, _ = h.Write(buf[:])
	var px [4]byte
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			px = [4]byte{byte(r >> 8), byte(g >> 8), byte(bl >> 8), byte(a >> 8)}
			_, _ = h.Write(px[:])
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, true
}

// motifMergeTolerance 是仍视为同一像素的单通道差，只吸收 JPEG 重压缩噪声。
const motifMergeTolerance = 2

// motifPixelIdentical 报告两图像素是否一致（容 JPEG 噪声）。
func motifPixelIdentical(a, b image.Image) bool {
	if a == nil || b == nil {
		return false
	}
	ba, bb := a.Bounds(), b.Bounds()
	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return false
	}
	for y := 0; y < ba.Dy(); y++ {
		for x := 0; x < ba.Dx(); x++ {
			ar, ag, ab, aa := a.At(ba.Min.X+x, ba.Min.Y+y).RGBA()
			br, bg, bb2, ba2 := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if channelDiff(ar, br) > motifMergeTolerance ||
				channelDiff(ag, bg) > motifMergeTolerance ||
				channelDiff(ab, bb2) > motifMergeTolerance ||
				channelDiff(aa, ba2) > motifMergeTolerance {
				return false
			}
		}
	}
	return true
}

func channelDiff(a, b uint32) int {
	if a > b {
		return int((a - b) >> 8)
	}
	return int((b - a) >> 8)
}

// SimilarTo 报告两张图之间是否允许合并——与建桶检测器同一套相似度：
// AI 可以拆桶、跨桶合并，但不可能把内容无关的两张图说成一张。
// 像素完全一致永远允许。漏掉重复只多画一张；误并两张不同图会删掉真内容，闸门保守。
func SimilarTo(a, b image.Image) bool {
	if a == nil || b == nil {
		return false
	}
	if motifPixelIdentical(a, b) {
		return true
	}
	fa, fb := computeMotifFeatureSet(a), computeMotifFeatureSet(b)
	if motifFeatureSetSimilar(fa, fb) {
		return true
	}
	// 闸门是检测器的超集：严格形状/颜色测试失败的对，仍可凭更宽的色度证据合并。
	return fa.chromOK && fb.chromOK && chromaticFeatureSimilar(fa.chrom, fb.chrom)
}

type motifFeatureSet struct {
	rgba     [32]byte
	rgbaOK   bool
	visual   motifFeatureData
	visualOK bool
	chrom    chromaticFeature
	chromOK  bool
}

func computeMotifFeatureSet(img image.Image) motifFeatureSet {
	if img == nil {
		return motifFeatureSet{}
	}
	var f motifFeatureSet
	f.rgba, f.rgbaOK = rgbaFingerprint(img)
	f.visual, f.visualOK = motifFeature(img)
	f.chrom, f.chromOK = chromaticMotifFeature(img)
	return f
}

func motifFeatureSetSimilar(a, b motifFeatureSet) bool {
	if a.visualOK && b.visualOK {
		return visualFeatureSimilar(a.visual, b.visual)
	}
	if !a.chromOK || !b.chromOK {
		return false
	}
	return chromaticFeatureSimilar(a.chrom, b.chrom)
}

func visualFeatureSimilar(ma, mb motifFeatureData) bool {
	if ma.aspect < 0.55 || ma.aspect > 1.8 || mb.aspect < 0.55 || mb.aspect > 1.8 {
		return false
	}
	if absFloat(ma.aspect-mb.aspect) > 0.18 {
		return false
	}
	intersect, union, colorError := 0, 0, 0.0
	for i := range ma.mask {
		if ma.mask[i] && mb.mask[i] {
			intersect++
			colorError += colorDistance(ma.colors[i], mb.colors[i])
		}
		if ma.mask[i] || mb.mask[i] {
			union++
		}
	}
	if union == 0 || float64(intersect)/float64(union) < 0.65 {
		return false
	}
	return colorError/float64(intersect) < 0.055
}

func chromaticFeatureSimilar(ma, mb chromaticFeature) bool {
	if ma.hue != mb.hue || absFloat(ma.aspect-mb.aspect) > 0.36 {
		return false
	}
	intersect, union := 0, 0
	for i, v := range ma.mask {
		if v && mb.mask[i] {
			intersect++
		}
		if v || mb.mask[i] {
			union++
		}
	}
	return union > 0 && float64(intersect)/float64(union) >= 0.53
}

type chromaticFeature struct {
	mask   []bool
	hue    int
	aspect float64
}

func chromaticMotifFeature(img image.Image) (chromaticFeature, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 8 || h < 8 || w > motifMaxWidth || h > motifMaxHeight {
		return chromaticFeature{}, false
	}
	colors := [3]int{}
	maskColors := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := rgbaAt(img, b.Min.X+x, b.Min.Y+y)
			channels := [3]int{int(c.R), int(c.G), int(c.B)}
			peak, second, which := channels[0], channels[1], 0
			if channels[1] > peak {
				peak, second, which = channels[1], peak, 1
			}
			if channels[2] > peak {
				second, peak, which = peak, channels[2], 2
			} else if channels[2] > second {
				second = channels[2]
			}
			if int(c.A) > 16 && peak >= 115 && peak-second >= 30 {
				maskColors[y*w+x] = which + 1
				colors[which]++
			}
		}
	}
	dominant := 0
	for i := 1; i < 3; i++ {
		if colors[i] > colors[dominant] {
			dominant = i
		}
	}
	if colors[dominant] < 20 || colors[dominant]*25 < w*h {
		return chromaticFeature{}, false
	}
	for i, n := range colors {
		if i != dominant && n >= 3 {
			return chromaticFeature{}, false
		}
	}
	x0, y0, x1, y1 := w, h, -1, -1
	for i, c := range maskColors {
		if c != dominant+1 {
			continue
		}
		x, y := i%w, i/w
		if x < x0 {
			x0 = x
		}
		if x > x1 {
			x1 = x
		}
		if y < y0 {
			y0 = y
		}
		if y > y1 {
			y1 = y
		}
	}
	bw, bh := x1-x0+1, y1-y0+1
	if bw < 5 || bh < 5 {
		return chromaticFeature{}, false
	}
	out := chromaticFeature{mask: make([]bool, motifGrid*motifGrid), hue: dominant, aspect: float64(bw) / float64(bh)}
	for gy := 0; gy < motifGrid; gy++ {
		for gx := 0; gx < motifGrid; gx++ {
			x := x0 + (2*gx+1)*bw/(2*motifGrid)
			y := y0 + (2*gy+1)*bh/(2*motifGrid)
			if x > x1 {
				x = x1
			}
			if y > y1 {
				y = y1
			}
			out.mask[gy*motifGrid+gx] = maskColors[y*w+x] == dominant+1
		}
	}
	return out, true
}

type motifFeatureData struct {
	mask   []bool
	colors []color.RGBA
	aspect float64
}

func motifFeature(img image.Image) (motifFeatureData, bool) {
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 || b.Dx() > motifMaxWidth || b.Dy() > motifMaxHeight {
		return motifFeatureData{}, false
	}
	mask := foregroundMask(img)
	count := 0
	for _, v := range mask {
		if v {
			count++
		}
	}
	if count == 0 || float64(count)/float64(len(mask)) > 0.28 || whiteFraction(img) < 0.82 {
		return motifFeatureData{}, false
	}
	components, minX, minY, maxX, maxY := foregroundComponents(mask, b.Dx(), b.Dy())
	// 旁边另起一个连通片（局部数字等标注）就放弃——保守判据。
	if components != 1 || minX == 0 || minY == 0 || maxX == b.Dx()-1 || maxY == b.Dy()-1 {
		return motifFeatureData{}, false
	}
	w, h := maxX-minX+1, maxY-minY+1
	out := motifFeatureData{mask: make([]bool, motifGrid*motifGrid), colors: make([]color.RGBA, motifGrid*motifGrid), aspect: float64(w) / float64(h)}
	for gy := 0; gy < motifGrid; gy++ {
		for gx := 0; gx < motifGrid; gx++ {
			x0 := minX + gx*w/motifGrid
			x1 := minX + (gx+1)*w/motifGrid
			y0 := minY + gy*h/motifGrid
			y1 := minY + (gy+1)*h/motifGrid
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			idx := gy*motifGrid + gx
			fg, total := 0, 0
			var sr, sg, sb int
			for y := y0; y < y1 && y < b.Dy(); y++ {
				for x := x0; x < x1 && x < b.Dx(); x++ {
					total++
					if mask[y*b.Dx()+x] {
						fg++
						c := rgbaAt(img, b.Min.X+x, b.Min.Y+y)
						sr += int(c.R)
						sg += int(c.G)
						sb += int(c.B)
					}
				}
			}
			out.mask[idx] = fg*2 >= total
			if fg > 0 {
				out.colors[idx] = color.RGBA{uint8(sr / fg), uint8(sg / fg), uint8(sb / fg), 255}
			}
		}
	}
	return out, true
}

func foregroundMask(img image.Image) []bool {
	b := img.Bounds()
	out := make([]bool, b.Dx()*b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := rgbaAt(img, b.Min.X+x, b.Min.Y+y)
			out[y*b.Dx()+x] = c.A > 16 && (int(c.R)+int(c.G)+int(c.B) < 720)
		}
	}
	return out
}

func foregroundComponents(mask []bool, w, h int) (n, minX, minY, maxX, maxY int) {
	minX, minY, maxX, maxY = w, h, -1, -1
	seen := make([]bool, len(mask))
	for i := range mask {
		if !mask[i] || seen[i] {
			continue
		}
		n++
		q := []int{i}
		seen[i] = true
		for len(q) > 0 {
			p := q[0]
			q = q[1:]
			x, y := p%w, p/w
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h || (dx == 0 && dy == 0) {
						continue
					}
					j := ny*w + nx
					if mask[j] && !seen[j] {
						seen[j] = true
						q = append(q, j)
					}
				}
			}
		}
	}
	return
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func whiteFraction(img image.Image) float64 {
	b := img.Bounds()
	white := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := rgbaAt(img, x, y)
			if c.A > 16 && c.R > 245 && c.G > 245 && c.B > 245 {
				white++
			}
		}
	}
	return float64(white) / float64(b.Dx()*b.Dy())
}

func colorDistance(a, b color.RGBA) float64 {
	return float64(absIntMotif(int(a.R)-int(b.R))+absIntMotif(int(a.G)-int(b.G))+absIntMotif(int(a.B)-int(b.B))) / (3.0 * 255.0)
}

func absIntMotif(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// motifAlignWorkers 是重合度扫描的并行度上限（纯函数比较，结果与串行一致）。
const motifAlignWorkers = 8

// findAlignedMotifPairs 把"小图标类"里居中重合度达标的图并在一个桶。
// 预筛（WorthScore）+ 同桶跳过 + 并行打分、串行 union（合并顺序与并发前一致，T140）。
// 覆盖数超 BucketMax 返回 -1（成本上限，调用方退回逐对相似度并告警）。
func findAlignedMotifPairs(valid []Sample, uniq []int, alignMin float64, shiftCells int, find func(int) int, union func(a, b int) bool) int {
	covs := make([]AlignImage, len(uniq))
	ok := make([]bool, len(uniq))
	covered := 0
	for p, i := range uniq {
		covs[p], ok[p] = AlignCoverage(valid[i].Image)
		if ok[p] {
			covered++
		}
	}
	if covered > BucketMax {
		return -1
	}
	type alignedPair struct{ a, b int32 }
	cand := make([]alignedPair, 0, 64)
	for i := range uniq {
		if !ok[i] {
			continue
		}
		if covs[i].Area > IconMaxArea {
			continue
		}
		for j := i + 1; j < len(uniq); j++ {
			if !ok[j] || covs[j].Area > IconMaxArea {
				continue
			}
			if !WorthScore(covs[i], covs[j]) {
				continue
			}
			if find(uniq[i]) == find(uniq[j]) {
				continue
			}
			cand = append(cand, alignedPair{int32(i), int32(j)})
		}
	}
	hits := make([]bool, len(cand))
	workers := runtime.NumCPU()
	if workers > motifAlignWorkers {
		workers = motifAlignWorkers
	}
	if workers > len(cand) {
		workers = len(cand)
	}
	var next int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(atomic.AddInt64(&next, 1)) - 1
				if k >= len(cand) {
					return
				}
				p := cand[k]
				hits[k] = covs[p.a].Overlap(covs[p.b], shiftCells) >= alignMin
			}
		}()
	}
	wg.Wait()
	merged := 0
	for k, p := range cand {
		if hits[k] && union(uniq[p.a], uniq[p.b]) {
			merged++
		}
	}
	return merged
}
