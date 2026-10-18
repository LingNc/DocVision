package latex

import (
	"fmt"
	"image"
	"image/color"
	"strings"
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

// 同一枚标记放在不同画布、位置差上一两个像素，软覆盖 + 逐级平移搜索必须仍然
// 认得出来（这正是旧版逐对相似度做不到、而"居中重合"要解决的事）。
// 注意：T137/T138 定稿后判据**不缩放**——尺寸差一倍的图不再走这条判据，交给
// 会话挂靠（T135）；这里钉的是"同尺寸、位置偏移"这一档。
func TestMotifAlignCoverageToleratessPositionOffset(t *testing.T) {
	big := whiteCanvas(120, 140)
	drawDiamond(big, 60, 60, 18)

	shifted := whiteCanvas(120, 140)
	drawDiamond(shifted, 62, 61, 18) // 内容框尺寸相同，只挪了 (2,1)

	cb, ok := motifAlignCoverage(big)
	if !ok {
		t.Fatal("big canvas must have coverage")
	}
	cs, ok := motifAlignCoverage(shifted)
	if !ok {
		t.Fatal("shifted canvas must have coverage")
	}
	if cb.area != cs.area {
		t.Fatalf("夹具不合适：内容框面积 %d vs %d（应相同）", cb.area, cs.area)
	}
	// 内容框裁掉画布上的位置差：同一枚标记换位置后应当几乎完全重合。
	if v := cb.overlap(cs, motifAlignShiftCells(0.10)); v < 0.99 {
		t.Fatalf("同尺寸、只挪位置的图必须完全重合，got %.3f", v)
	}
	// 平移搜索真的在起作用：内容框尺寸差 1px 的一对，搜平移不会比不搜差。
	bigger := whiteCanvas(120, 140)
	drawDiamond(bigger, 60, 60, 19)
	cg, ok := motifAlignCoverage(bigger)
	if !ok {
		t.Fatal("bigger canvas must have coverage")
	}
	withSearch := cb.overlap(cg, motifAlignShiftCells(0.10))
	if noSearch := cb.overlap(cg, 0); withSearch < noSearch-1e-9 {
		t.Fatalf("搜平移不该比不搜差：%.4f < %.4f", withSearch, noSearch)
	}
	// 尺寸差一倍（缩放）不吃这条判据：不缩放是有意的（T137）。
	half := whiteCanvas(90, 100)
	drawDiamond(half, 30, 25, 9)
	ch, ok := motifAlignCoverage(half)
	if !ok {
		t.Fatal("half canvas must have coverage")
	}
	if v := cb.overlap(ch, motifAlignShiftCells(0.10)); v >= 0.90 {
		t.Fatalf("半尺寸的图不该靠这条判据合并（不缩放），got %.3f", v)
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

// squashX 把图按水平比例压扁：形状还是那个形状，长宽比变了。
func squashX(src image.Image, factor float64) *image.RGBA {
	b := src.Bounds()
	w := int(float64(b.Dx()) * factor)
	if w < 1 {
		w = 1
	}
	out := whiteCanvas(w, b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < w; x++ {
			sx := int(float64(x) / factor)
			if sx >= b.Dx() {
				sx = b.Dx() - 1
			}
			out.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+y))
		}
	}
	return out
}

// 桶判据也吃"居中重合"（T130）：同尺寸、位置差几像素的同一枚标记必须并进同
// 一个桶，结构不同的图和**缩放过**的图不能被并进来（T137：判据不缩放，
// 跨尺寸交给会话挂靠）。
func TestFindVisualMotifCandidatesAlignedLinksShiftedDuplicates(t *testing.T) {
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)

	moved := whiteCanvas(70, 96)
	drawDiamond(moved, 36, 31, 18) // 同一枚，只挪了位置

	squashed := squashX(base, 0.72)

	block := whiteCanvas(70, 96)
	for y := 24; y < 70; y++ {
		for x := 24; x < 46; x++ {
			block.Set(x, y, color.RGBA{A: 255})
		}
	}

	samples := []VisualMotifSample{
		{Key: "a-base", Image: base},
		{Key: "b-moved", Image: moved},
		{Key: "c-squashed", Image: squashed},
		{Key: "d-block", Image: block},
	}
	groups, merges := FindVisualMotifCandidatesAligned(samples, 0.60, 0.10)
	if merges < 1 {
		t.Fatalf("居中重合应当并进至少一对，got %d", merges)
	}
	joined := false
	for _, g := range groups {
		if containsKey(g.Members, "a-base") && containsKey(g.Members, "b-moved") {
			joined = true
		}
		for _, bad := range []string{"c-squashed", "d-block"} {
			if containsKey(g.Members, bad) && containsKey(g.Members, "a-base") {
				t.Fatalf("%s 不该和基准同桶: %v", bad, g.Members)
			}
		}
	}
	if !joined {
		t.Fatalf("同一枚标记（位置差 1px）必须同桶，got %v", groups)
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

// 像素完全相同的副本只参与一次扫描（T140），且"是否达标"并行算、union 串行
// 做——重跑必须给出逐字一致的分组（并行不能引入顺序抖动）。
func TestFindVisualMotifCandidatesAlignedCopiesStableAcrossRuns(t *testing.T) {
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	moved := whiteCanvas(70, 96)
	drawDiamond(moved, 36, 31, 18)
	block := whiteCanvas(70, 96)
	for y := 24; y < 70; y++ {
		for x := 24; x < 46; x++ {
			block.Set(x, y, color.RGBA{A: 255})
		}
	}
	var samples []VisualMotifSample
	for i := 0; i < 3; i++ {
		samples = append(samples,
			VisualMotifSample{Key: fmt.Sprintf("a-copy-%d", i), Image: base},
			VisualMotifSample{Key: fmt.Sprintf("b-copy-%d", i), Image: moved},
			VisualMotifSample{Key: fmt.Sprintf("c-copy-%d", i), Image: block},
		)
	}
	var first []string
	for run := 0; run < 5; run++ {
		groups, merges := FindVisualMotifCandidatesAligned(samples, 0.60, 0.10)
		if merges < 1 {
			t.Fatalf("run %d: 居中重合应当并进至少一对，got %d", run, merges)
		}
		var got []string
		for _, g := range groups {
			got = append(got, strings.Join(g.Members, ","))
		}
		if run == 0 {
			first = got
			continue
		}
		if strings.Join(got, "|") != strings.Join(first, "|") {
			t.Fatalf("并行扫描的分组必须稳定: run %d = %v, run 0 = %v", run, got, first)
		}
	}
	// 同一枚标记的三份副本 + 挪了 1px 的那份都在同一个组里。
	joined := false
	for _, g := range first {
		if strings.Contains(g, "a-copy-0") && strings.Contains(g, "b-copy-0") && strings.Contains(g, "a-copy-2") {
			joined = true
		}
		if strings.Contains(g, "c-copy-0") && strings.Contains(g, "a-copy-0") {
			t.Fatalf("不同形状的图不该并进同一组: %v", first)
		}
	}
	if !joined {
		t.Fatalf("副本与原图必须同组，got %v", first)
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
	near := whiteCanvas(70, 96)
	drawDiamond(near, 36, 31, 18) // 同一枚，只挪了位置（不缩放）

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
	// 结构完全不同的小图标：重合度远低于门槛，必须留在池子里交会话。
	far := whiteCanvas(70, 96)
	for y := 24; y < 70; y++ {
		for x := 24; x < 46; x++ {
			far.Set(x, y, color.RGBA{A: 255})
		}
	}
	cf, ok := motifAlignCoverage(far)
	if !ok {
		t.Fatal("夹具必须是小图标")
	}
	if v := cb.overlap(cf, shift); v >= 0.60 {
		t.Fatalf("夹具不合适：不同结构的图标对基准有 %.3f（应 <0.60）", v)
	}

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

// T135：没进会话、也没有等价结论的图，若与已确认组的代表图重合度 ≥
// dedupe_attach_min，就挂靠过去（source=attached）；会话看过并判过独立的图
// 不改判，结构不同的图也不挂。
func TestMotifAttachLeftoversOnlyUnseenImages(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{
		DedupeAlignMin: 0.60, DedupeAlignShift: 0.10, DedupeAttachMin: 0.60, DedupeIconMaxArea: 200000,
	}}}
	mk := func(name string, img image.Image, class string) *motifCandidate {
		tk := &task{mdName: "book.md", imgPath: "images/" + name}
		p := &imageProgress{Key: tk.key(), MDName: tk.mdName, ImgPath: tk.imgPath, Class: class, Status: "classified"}
		return &motifCandidate{task: tk, progress: p, image: img, sourceHash: "hash-" + name}
	}
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	dup := whiteCanvas(70, 96)
	drawDiamond(dup, 35, 30, 18) // 同一枚（内容完全一致，重合度 1.00）
	block := whiteCanvas(70, 96)
	for y := 24; y < 70; y++ {
		for x := 24; x < 46; x++ {
			block.Set(x, y, color.RGBA{A: 255})
		}
	}
	key := mk("k.jpg", base, ClassVector)
	unseen := mk("u.jpg", dup, ClassVector)
	other := mk("o.jpg", block, ClassVector)
	seen := mk("s.jpg", dup, ClassRaster)
	seen.progress.DedupeChecked = true // 会话看过：不改判

	applied := []motifAppliedGroup{{Key: key, Members: []*motifCandidate{key}, Class: ClassVector}}
	n, extra := r.motifAttachLeftovers([]*motifCandidate{key, unseen, other, seen}, applied, dir)
	if n != 1 || len(extra) != 1 {
		t.Fatalf("应当只挂靠 1 张，got n=%d extra=%d", n, len(extra))
	}
	if unseen.progress.EquivalentTo != key.task.key() || !unseen.progress.DedupeChecked {
		t.Fatalf("挂靠后必须指向代表图并标记已核验: %q checked=%v", unseen.progress.EquivalentTo, unseen.progress.DedupeChecked)
	}
	if extra[0].Source != "attached" || extra[0].Key != key {
		t.Fatalf("挂靠组必须记 source=attached 且锚点是已确认组: %+v", extra[0])
	}
	if other.progress.EquivalentTo != "" {
		t.Fatal("结构不同的图不能挂靠")
	}
	if seen.progress.EquivalentTo != "" {
		t.Fatal("会话判过的图不能被挂靠改判")
	}
	// 门槛可配：抬到 1.01（夹到 1.0）时，只有完全一致的图能挂上。
	r.cfg.Latex.DedupeAttachMin = 1.0
	unseen.progress.EquivalentTo = ""
	unseen.progress.DedupeChecked = false
	if n, _ := r.motifAttachLeftovers([]*motifCandidate{key, unseen}, applied, dir); n != 1 {
		t.Fatalf("完全一致（1.00）在门槛 1.0 下仍应挂靠，got %d", n)
	}
}
