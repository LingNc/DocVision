package latex

import (
	"image"
	"image/color"
	"testing"

	"mineru-tools/internal/config"
)

// drawDiamond 在 img 上画一枚"菱形 + Z + 竖线底座"的标记，笔画粗细与半径成
// 比例——真实书里的同标记图片是重采样出来的，笔画宽度跟着缩放；固定 1px 的
// 线条在 2× 缩放下相对粗细会翻倍，那是不真实的人造差异。
func drawDiamond(img *image.RGBA, cx, cy, r int) {
	stroke := r / 5
	if stroke < 1 {
		stroke = 1
	}
	in := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < img.Bounds().Dx() && y < img.Bounds().Dy()
	}
	ink := func(x, y int) {
		if in(x, y) {
			img.Set(x, y, color.RGBA{A: 255})
		}
	}
	// 菱形边框：|dx|+|dy| ≈ r。
	for y := cy - r - stroke; y <= cy+r+stroke; y++ {
		for x := cx - r - stroke; x <= cx+r+stroke; x++ {
			d := abs(x-cx) + abs(y-cy)
			if abs(d-r) <= stroke/2 {
				ink(x, y)
			}
		}
	}
	// 内部的 Z：上下两条横杠 + 一条斜杠。
	bar := func(yline int) {
		for y := yline - stroke/2; y <= yline+stroke/2; y++ {
			for x := cx - r/2; x <= cx+r/2; x++ {
				ink(x, y)
			}
		}
	}
	bar(cy - r/2)
	bar(cy + r/2)
	d0 := r / 2
	for t := 0; t <= 2*d0; t++ {
		x := cx + d0 - t
		y := cy - d0 + t
		for yy := y - stroke/2; yy <= y+stroke/2; yy++ {
			for xx := x - stroke/2; xx <= x+stroke/2; xx++ {
				ink(xx, yy)
			}
		}
	}
	// 底座：竖线 + 一横。
	for y := cy + r; y <= cy+2*r; y++ {
		for x := cx - stroke/2; x <= cx+stroke/2; x++ {
			ink(x, y)
		}
	}
	for x := cx - r/2; x <= cx+r/2; x++ {
		for y := cy + 2*r - stroke/2; y <= cy+2*r; y++ {
			ink(x, y)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func whiteCanvas(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return img
}

// 同一枚标记缩小 + 挪位置 + 换画布尺寸，内容框归一化之后必须仍然重合得上：
// 这正是旧的逐对相似度做不到、而"居中重合"要解决的事。
func TestMotifAlignCoverageNormalizesScaleAndPosition(t *testing.T) {
	big := whiteCanvas(120, 140)
	drawDiamond(big, 60, 60, 22)

	small := whiteCanvas(90, 100)
	drawDiamond(small, 30, 25, 11)

	cb, ok := motifAlignCoverage(big)
	if !ok {
		t.Fatal("big canvas must have coverage")
	}
	cs, ok := motifAlignCoverage(small)
	if !ok {
		t.Fatal("small canvas must have coverage")
	}
	if v := cb.overlap(cs, motifAlignShiftCells(0.10)); v < 0.75 {
		t.Fatalf("same mark at half scale shifted must overlap, got %.3f", v)
	}
	// 完全不搜平移时，位置差会明显拉低重合度——证明搜索不是装饰。
	if v := cb.overlap(cs, 0); v >= 0.95 {
		t.Fatalf("no-shift comparison should be worse, got %.3f", v)
	}
}

// 结构不同的图（实心方块 vs 菱形标记）必须被拒。
func TestMotifAlignOverlapRejectsDifferentShape(t *testing.T) {
	mark := whiteCanvas(80, 100)
	drawDiamond(mark, 40, 30, 18)

	block := whiteCanvas(80, 100)
	for y := 20; y < 70; y++ {
		for x := 30; x < 55; x++ {
			block.Set(x, y, color.RGBA{A: 255})
		}
	}
	cm, ok := motifAlignCoverage(mark)
	if !ok {
		t.Fatal("mark needs coverage")
	}
	cbk, ok := motifAlignCoverage(block)
	if !ok {
		t.Fatal("block needs coverage")
	}
	if v := cm.overlap(cbk, motifAlignShiftCells(0.10)); v >= 0.60 {
		t.Fatalf("different shape must stay below the default threshold, got %.3f", v)
	}
}

// 平移上限按内容框边长的比例配置，夹在 0..16 格。
func TestMotifAlignShiftCells(t *testing.T) {
	cases := map[float64]int{0: 0, 0.01: 0, 0.10: 3, 0.25: 8, 0.50: 16, 0.90: 16}
	for pct, want := range cases {
		if got := motifAlignShiftCells(pct); got != want {
			t.Errorf("motifAlignShiftCells(%v)=%d want %d", pct, got, want)
		}
	}
}

// 组内闸门：模型把"同一枚标记的缩放/移位版"并进组里，闸门要认；把异形图
// 并进来，闸门要拒；模型的 key 达标时保留它。
func TestMotifAlignFilterKeepsAlignedDropsOthers(t *testing.T) {
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{DedupeAlignMin: 0.60, DedupeAlignShift: 0.10}}}

	keyImg := whiteCanvas(60, 80)
	drawDiamond(keyImg, 30, 22, 14)

	nearImg := whiteCanvas(72, 90)
	drawDiamond(nearImg, 36, 26, 15) // 稍大一点、位置不同

	otherImg := whiteCanvas(60, 80)
	for y := 18; y < 60; y++ {
		for x := 18; x < 42; x++ {
			otherImg.Set(x, y, color.RGBA{A: 255})
		}
	}

	key := &motifCandidate{image: keyImg}
	near := &motifCandidate{image: nearImg}
	other := &motifCandidate{image: otherImg}

	safe, chosen, dropped, worst, aligned := r.motifAlignFilter([]*motifCandidate{key, near, other}, key)
	if !aligned {
		t.Fatal("small icon-like images must take the align path")
	}
	if !motifContains(safe, key) || !motifContains(safe, near) {
		t.Fatalf("aligned variant must be kept, safe=%v", len(safe))
	}
	if motifContains(safe, other) || dropped != 1 {
		t.Fatalf("different shape must be dropped, dropped=%d", dropped)
	}
	// 代表图取合格成员里内容框最大的一张（"覆盖了必要内容"），模型选的 key
	// 只有在面积够全时才被保留——这里 near 略大，所以代表图应当是 near。
	if chosen != near {
		t.Fatalf("representative must be the most complete member, got %p", chosen)
	}
	if worst < 0.60 {
		t.Fatalf("worst kept member must be above the threshold, got %.3f", worst)
	}
}

// 全是大图/点阵拿不到覆盖率时退回逐对相似度：行为与旧闸门一致。
func TestMotifAlignFilterFallsBackWithoutCoverage(t *testing.T) {
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{DedupeAlignMin: 0.60, DedupeAlignShift: 0.10}}}
	dot := image.NewRGBA(image.Rect(0, 0, 1, 1))
	dot.Set(0, 0, color.RGBA{A: 255})
	a := &motifCandidate{image: dot}
	b := &motifCandidate{image: dot}
	safe, _, dropped, _, aligned := r.motifAlignFilter([]*motifCandidate{a, b}, a)
	if aligned {
		t.Fatal("1x1 images have no coverage, must fall back")
	}
	if len(safe) != 2 || dropped != 0 {
		t.Fatalf("identical dots must pass the fallback gate, safe=%d dropped=%d", len(safe), dropped)
	}
}

// squashX 把图按水平比例压扁：形状还是那个形状，长宽比变了——旧的逐对判据
// 用 |Δaspect| ≤ 0.18 硬门槛，压扁 28% 就会被拒；居中重合判据只看重合度。
func squashX(src image.Image, factor float64) *image.RGBA {
	b := src.Bounds()
	w := int(float64(b.Dx()) * factor)
	if w < 8 {
		w = 8
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < w; x++ {
			sx := b.Min.X + int(float64(x)/factor)
			if sx >= b.Max.X {
				sx = b.Max.X - 1
			}
			dst.Set(x, y, src.At(sx, b.Min.Y+y))
		}
	}
	return dst
}

// 桶也走"居中重合"判据：压扁过的同一标记会被逐对相似度漏掉（长宽比门槛），
// 但重合度达标，所以应当并进同一个桶；结构不同的图仍然分开。
func TestFindVisualMotifCandidatesAlignedLinksSquashedVariant(t *testing.T) {
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	squashed := squashX(base, 0.72)

	block := whiteCanvas(70, 96)
	for y := 24; y < 70; y++ {
		for x := 24; x < 46; x++ {
			block.Set(x, y, color.RGBA{A: 255})
		}
	}

	samples := []VisualMotifSample{
		{Key: "a-base", Image: base},
		{Key: "b-squashed", Image: squashed},
		{Key: "c-block", Image: block},
	}
	// 前提：逐对相似度确实漏掉了压扁版（否则这个测试证明不了桶判据的改动）。
	legacy := FindVisualMotifCandidates(samples)
	for _, g := range legacy {
		if containsKey(g.Members, "a-base") && containsKey(g.Members, "b-squashed") {
			t.Skip("夹具里的压扁版仍然过逐对判据（阈值可能被调过），本测试不适用")
		}
	}

	groups, merges := FindVisualMotifCandidatesAligned(samples, 0.60, 0.10)
	if merges < 1 {
		t.Fatalf("居中重合应当并进至少一对，got %d", merges)
	}
	joined := false
	for _, g := range groups {
		if containsKey(g.Members, "a-base") && containsKey(g.Members, "b-squashed") {
			joined = true
		}
		if containsKey(g.Members, "c-block") {
			t.Fatalf("结构不同的图被并进桶: %v", g.Members)
		}
	}
	if !joined {
		t.Fatalf("压扁版必须与基准同桶，got %v", groups)
	}
}

// alignMin<=0 时必须与旧入口完全一致（配置关掉这条判据的退路）。
func TestFindVisualMotifCandidatesAlignedDisabledMatchesLegacy(t *testing.T) {
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	squashed := squashX(base, 0.72)
	samples := []VisualMotifSample{
		{Key: "a", Image: base},
		{Key: "b", Image: squashed},
	}
	legacy := FindVisualMotifCandidates(samples)
	got, merges := FindVisualMotifCandidatesAligned(samples, 0, 0)
	if merges != 0 || len(got) != len(legacy) {
		t.Fatalf("alignMin=0 必须退回旧行为: got %d groups / merges=%d, want %d", len(got), merges, len(legacy))
	}
}

func containsKey(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// 小图标本地自动归组（T131）：≥0.75 的图标不进会话，0.60–0.75 的仍留在池子里
// 交会话；非矢量图永远不自动合并；门槛 0 = 全部交会话。
func TestMotifAutoIconGroupsIgnoresClass(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{
		DedupeAlignMin: 0.60, DedupeAlignShift: 0.10, DedupeIconAuto: 0.75,
	}}}
	mk := func(name string, img image.Image, class string) *motifCandidate {
		tk := &task{mdName: "book.md", imgPath: "images/" + name}
		p := &imageProgress{Key: tk.key(), MDName: tk.mdName, ImgPath: tk.imgPath, Class: class, Status: "classified"}
		return &motifCandidate{task: tk, progress: p, image: img, sourceHash: "hash-" + name}
	}
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	near := whiteCanvas(72, 98)
	drawDiamond(near, 36, 30, 19) // 同一枚，略大一点

	rBase := &Runner{cfg: r.cfg}
	cb, okB := motifAlignCoverage(base)
	cn, okN := motifAlignCoverage(near)
	if !okB || !okN {
		t.Fatal("夹具必须是小图标")
	}
	shift := rBase.motifAlignShift()
	if v := cb.overlap(cn, shift); v < 0.75 {
		t.Fatalf("夹具不合适：略大那枚对基准只有 %.3f（应 ≥0.75）", v)
	}
	// 找一个"压扁到 0.60–0.75 之间"的版本：这类图正好是用户口径里"仍要送
	// 会话核验"的那一批，自动归组不能把它们吞掉。具体压多少交给度量自己决定，
	// 免得夹具和阈值脱钩。
	var far image.Image
	farScore, farFactor := 0.0, 0.0
	for _, f := range []float64{0.62, 0.58, 0.54, 0.50, 0.46, 0.42, 0.38} {
		cand := squashX(base, f)
		cov, ok := motifAlignCoverage(cand)
		if !ok {
			continue
		}
		v := cb.overlap(cov, shift)
		if v >= 0.60 && v < 0.75 {
			far, farScore, farFactor = cand, v, f
			break
		}
	}
	if far == nil {
		t.Skip("没有找到落在 0.60–0.75 区间的压扁版本（夹具不适用）")
	}
	t.Logf("中间档夹具：压扁系数 %.2f、重合度 %.3f（应留在池子里交会话）", farFactor, farScore)

	a := mk("a.jpg", base, ClassVector)
	b := mk("b.jpg", near, ClassVector)
	c := mk("c.jpg", far, ClassVector)
	raster := mk("d.jpg", near, ClassRaster) // 和基准几乎一致，但 class 不是矢量图
	big := mk("e.jpg", whiteCanvas(400, 500), ClassVector)

	applied, remaining, merged := r.motifAutoIconGroups([]*motifCandidate{a, b, c, raster, big}, dir)
	if len(applied) != 1 {
		t.Fatalf("应当只有一组自动合并，got %d", len(applied))
	}
	g := applied[0]
	// T134：class 不再是合并条件。raster 那张和基准几乎一致，必须一起并进去，
	// 组的类别（后面取占比最大的 class）不影响这一步。
	if len(g.Members) != 3 || !motifContains(g.Members, a) || !motifContains(g.Members, b) || !motifContains(g.Members, raster) {
		t.Fatalf("自动合并的成员不对（class 不该拦合并）: %d", len(g.Members))
	}
	if merged != 3 {
		t.Fatalf("merged=%d want 3", merged)
	}
	if g.Key == nil {
		t.Fatal("必须选出代表图")
	}
	// 0.60–0.75 那张和大图仍必须留在池子里交会话。
	for _, keep := range []*motifCandidate{c, big} {
		if !motifContains(remaining, keep) {
			t.Fatalf("%s 不该被自动合并（应留在池子里）", keep.task.imgPath)
		}
	}
	// 0 = 关闭：什么都不自动合并。
	r0 := &Runner{cfg: &config.Config{Latex: config.LatexConfig{DedupeIconAuto: 0}}}
	if applied0, remaining0, merged0 := r0.motifAutoIconGroups([]*motifCandidate{a, b}, dir); len(applied0) != 0 || merged0 != 0 || len(remaining0) != 2 {
		t.Fatalf("DedupeIconAuto=0 必须全部交会话: applied=%d merged=%d remaining=%d", len(applied0), merged0, len(remaining0))
	}
}

// 代表图规则（T131）：在"典型成员"（与组中心 ≥0.95）里挑内容框最大的那张，
// 而不是无条件挑全场最大，也不是固定用模型点的 key。
func TestMotifAlignFilterPrefersTypicalLargestMember(t *testing.T) {
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{DedupeAlignMin: 0.60, DedupeAlignShift: 0.10}}}
	typicalSmall := whiteCanvas(60, 80)
	drawDiamond(typicalSmall, 30, 26, 14) // 典型：与中心完全重合
	typicalBig := whiteCanvas(66, 88)
	drawDiamond(typicalBig, 33, 28, 15) // 稍大一点，仍属"典型"
	oddBig := whiteCanvas(80, 96)
	for y := 14; y < 84; y++ { // 实心块：内容框最大，但形状和菱形完全不是一回事
		for x := 26; x < 54; x++ {
			oddBig.Set(x, y, color.RGBA{A: 255})
		}
	}

	safe, chosen, _, _, _ := r.motifAlignFilter([]*motifCandidate{
		{image: typicalSmall}, {image: typicalBig}, {image: oddBig},
	}, &motifCandidate{image: typicalBig})
	if chosen == nil {
		t.Fatal("必须有代表图")
	}
	cb, _ := motifAlignCoverage(typicalBig)
	cs, _ := motifAlignCoverage(typicalSmall)
	co, _ := motifAlignCoverage(oddBig)
	if chosen.image != typicalBig {
		t.Fatalf("代表图应当是典型成员里内容框最大的那张（typicalBig），实际选中了别的")
	}
	if chosen.image == oddBig {
		t.Fatal("内容框最大但形状不同的那张不该当代表图")
	}
	if len(safe) < 2 {
		t.Fatalf("典型成员必须都被保留，got %d", len(safe))
	}
	if cb.area <= cs.area {
		t.Fatalf("夹具不合适：typicalBig 的内容框应更大 (%d vs %d)", cb.area, cs.area)
	}
	if co.area <= cb.area {
		t.Fatalf("夹具不合适：oddBig 的内容框应最大 (%d vs %d)", co.area, cb.area)
	}
}
