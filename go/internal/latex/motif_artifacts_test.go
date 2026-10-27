package latex

import (
	"encoding/json"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// dedupe 的最终结论必须落成一棵可复查的目录（T133）：以前只有每个会话自己的
// groups.json 和每图 progress 里的 equivalent_to，人想核对"并了哪几组、代表图
// 是谁、组内最差分是多少"只能翻转录。
func TestWriteMotifArtifactsRecordsFinalGroups(t *testing.T) {
	outDir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{
		DedupeAlignMin: 0.60, DedupeAlignShift: 0.10, DedupeIconAuto: 0.75,
	}}}
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	near := whiteCanvas(72, 98)
	drawDiamond(near, 36, 30, 19)
	other := whiteCanvas(90, 120)
	for y := 20; y < 90; y++ {
		for x := 30; x < 60; x++ {
			other.Set(x, y, color.RGBA{A: 255})
		}
	}
	a := &motifCandidate{task: &task{mdName: "b.md", imgPath: "images/b/a.jpg"}, progress: &imageProgress{Class: ClassVector}, image: base, sourceHash: "hash-a"}
	b := &motifCandidate{task: &task{mdName: "b.md", imgPath: "images/b/b.jpg"}, progress: &imageProgress{Class: ClassVector}, image: near, sourceHash: "hash-b"}
	b2 := &motifCandidate{task: &task{mdName: "b.md", imgPath: "images/b/b2.jpg"}, progress: &imageProgress{Class: ClassVector}, image: near, sourceHash: "hash-b"}
	c := &motifCandidate{task: &task{mdName: "b.md", imgPath: "images/b/c.jpg"}, progress: &imageProgress{Class: ClassVector}, image: other, sourceHash: "hash-c"}
	leaders := []*motifCandidate{a, b, c}
	copies := map[*motifCandidate][]*motifCandidate{b: {b2}}
	applied := []motifAppliedGroup{{Key: a, Members: []*motifCandidate{a, b}, Class: ClassVector, Source: "auto"}}
	r.writeMotifArtifacts(outDir, leaders, copies, applied, nil, nil)

	dir := filepath.Join(outDir, "dedupe")
	data, err := os.ReadFile(filepath.Join(dir, "groups.json"))
	if err != nil {
		t.Fatalf("必须写出 groups.json: %v", err)
	}
	var art motifArtifact
	if err := json.Unmarshal(data, &art); err != nil {
		t.Fatalf("groups.json 不是合法 JSON: %v", err)
	}
	if art.Refs != 4 || art.Distinct != 3 {
		t.Fatalf("引用/不同内容计数不对: refs=%d distinct=%d", art.Refs, art.Distinct)
	}
	if len(art.Groups) != 1 || art.Groups[0].Source != "auto" {
		t.Fatalf("应当记录 1 个本地自动定组（source=auto）: %+v", art.Groups)
	}
	if got := len(art.Groups[0].Members); got != 2 {
		t.Fatalf("组成员应为 2，got %d", got)
	}
	var dup int
	for _, m := range art.Groups[0].Members {
		if m.Hash == "hash-b" && len(m.Duplicates) == 1 {
			dup++
		}
		if m.Hash == "hash-b" && m.AlignScore <= 0 {
			t.Fatalf("成员必须带与代表图的重合度: %+v", m)
		}
	}
	if dup != 1 {
		t.Fatalf("逐字节副本必须挂在成员下面: %+v", art.Groups[0].Members)
	}
	if len(art.Independent) != 1 || art.Independent[0].Hash != "hash-c" {
		t.Fatalf("独立份应当只有 c: %+v", art.Independent)
	}
	if art.Fingerprint == "" {
		t.Fatal("必须带输入指纹")
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatalf("必须写出 report.md: %v", err)
	}
	for _, want := range []string{"不同内容 3 份", "本地自动", "指纹"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report.md 缺少 %q:\n%s", want, report)
		}
	}
}

// T150：分组结论要能在浏览器里手点一遍——review.html 把代表图、成员图、独立图
// 都排出来，缩略图内嵌（离线可看），点开是原图的相对链接。
func TestWriteMotifReviewLinksSelectedImages(t *testing.T) {
	outDir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{
		DedupeAlignMin: 0.60, DedupeAlignShift: 0.10, DedupeIconAuto: 0.75,
	}}}
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	near := whiteCanvas(72, 98)
	drawDiamond(near, 36, 30, 19)
	other := whiteCanvas(90, 120)
	for y := 20; y < 90; y++ {
		for x := 30; x < 60; x++ {
			other.Set(x, y, color.RGBA{A: 255})
		}
	}
	mk := func(name, hash string, img image.Image) *motifCandidate {
		return &motifCandidate{
			task:       &task{mdName: "b.md", imgPath: "images/b/" + name},
			progress:   &imageProgress{Class: ClassVector},
			image:      img,
			sourceHash: hash,
		}
	}
	a := mk("a.jpg", strings.Repeat("a", 64), base)
	b := mk("b.jpg", strings.Repeat("b", 64), near)
	c := mk("c.jpg", strings.Repeat("c", 64), other)
	applied := []motifAppliedGroup{{Key: a, Members: []*motifCandidate{a, b}, Class: ClassVector, Source: "auto"}}
	r.writeMotifArtifacts(outDir, []*motifCandidate{a, b, c}, nil, applied, nil, nil)

	data, err := os.ReadFile(filepath.Join(motifArtifactDir(outDir), "review.html"))
	if err != nil {
		t.Fatalf("必须写出 review.html: %v", err)
	}
	page := string(data)
	for _, want := range []string{
		"../images/b/a.jpg",        // 代表图：点开原图
		"../images/b/b.jpg",        // 成员图也要能点开
		"../images/b/c.jpg",        // 独立图也要能点开
		"thumbs/aaaaaaaaaaaa.jpg",  // 列表里用缩略图（T154：不再内嵌 base64）
		"id=\"lb\"",                // 页内灯箱
		"data-all=\"open\"",        // 全部展开
		"<details class=\"group\"", // 分组默认收起（没有 open 属性）
		"独立", "组内最低分",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("review.html 缺少 %q", want)
		}
	}
	if strings.Contains(page, "data:image/jpeg;base64,") {
		t.Fatal("review.html 不该再内嵌 base64 缩略图")
	}
	if strings.Contains(page, "<details class=\"group\" open") {
		t.Fatal("分组默认应当收起")
	}
	thumb := filepath.Join(motifArtifactDir(outDir), "thumbs", "aaaaaaaaaaaa.jpg")
	if st, err := os.Stat(thumb); err != nil || st.Size() == 0 {
		t.Fatalf("必须写出缩略图 %s: %v", thumb, err)
	}

	// T154：页面里的图片与缩略图都是链接（相对路径），一个 base64 都没有；
	// 会话预览页直接重发这份文件、只补一个 <base>，所以这里不再有第二份渲染。
	for _, want := range []string{"data-full=\"../images/b/a.jpg\"", "data-orig=\"../images/b/a.jpg\""} {
		if !strings.Contains(page, want) {
			t.Fatalf("review.html 缺少 %q", want)
		}
	}
	if strings.Contains(page, "<img src=\"data:") {
		t.Fatal("缩略图不该内嵌到页面里")
	}
	// report.md 也要能直接点开图（以前只有文件名）。
	report, err := os.ReadFile(filepath.Join(motifArtifactDir(outDir), "report.md"))
	if err != nil {
		t.Fatalf("必须写出 report.md: %v", err)
	}
	for _, want := range []string{"](../images/b/a.jpg)", "review.html"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("report.md 缺少 %q:\n%s", want, report)
		}
	}
}

// 同一输入 + 同一规则 → 同一指纹（重跑不会得到"另一张表"）。
func TestMotifArtifactFingerprintStable(t *testing.T) {
	rules := motifArtifactRules{AlignMin: 0.6, AlignShift: 0.1, IconAuto: 0.75}
	h1 := []string{"c", "a", "b"}
	h2 := []string{"b", "c", "a"}
	if motifArtifactFingerprint(rules, h1) != motifArtifactFingerprint(rules, h2) {
		t.Fatal("指纹必须与哈希顺序无关")
	}
	if motifArtifactFingerprint(rules, h1) == motifArtifactFingerprint(motifArtifactRules{AlignMin: 0.7, AlignShift: 0.1, IconAuto: 0.75}, h1) {
		t.Fatal("规则变了指纹必须变")
	}
}

// 续跑不该重扫（T141）：输入与规则没变、每一条结论都还在 progress 里对得上，
// 就整段跳过；图片换掉、少一张、结论被清掉都必须重扫（宁可重算，不能不认账）。
func TestMotifReuseDedupeValidatesConclusions(t *testing.T) {
	outDir := t.TempDir()
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{
		DedupeAlignMin: 0.60, DedupeAlignShift: 0.10, DedupeIconAuto: 0.75,
	}}}
	base := whiteCanvas(70, 96)
	drawDiamond(base, 35, 30, 18)
	near := whiteCanvas(72, 98)
	drawDiamond(near, 36, 30, 19)
	other := whiteCanvas(90, 120)
	for y := 20; y < 90; y++ {
		for x := 30; x < 60; x++ {
			other.Set(x, y, color.RGBA{A: 255})
		}
	}
	mk := func(name, hash string, img image.Image) *motifCandidate {
		return &motifCandidate{
			task:       &task{mdName: "b.md", imgPath: "images/b/" + name},
			progress:   &imageProgress{Class: ClassVector},
			image:      img,
			sourceHash: hash,
		}
	}
	a := mk("a.jpg", "hash-a", base)
	b := mk("b.jpg", "hash-b", near)
	b2 := mk("b2.jpg", "hash-b", near)
	c := mk("c.jpg", "hash-c", other)
	leaders := []*motifCandidate{a, b, c}
	copies := map[*motifCandidate][]*motifCandidate{b: {b2}}
	applied := []motifAppliedGroup{{Key: a, Members: []*motifCandidate{a, b}, Class: ClassVector, Source: "auto"}}
	r.writeMotifArtifacts(outDir, leaders, copies, applied, nil, nil)

	// 复核 dedupe 收尾写进 progress 的结论（代表图自指为空、成员指向代表图）。
	settle := func(item *motifCandidate, leader *motifCandidate) {
		item.progress.DedupeChecked = true
		item.progress.DedupeVersion = motifDedupeVersion
		item.progress.DedupeSourceHash = item.sourceHash
		if leader == nil {
			item.progress.EquivalentTo = ""
			item.progress.DedupeLeaderHash = ""
			return
		}
		item.progress.EquivalentTo = leader.task.key()
		item.progress.DedupeLeaderHash = leader.sourceHash
	}
	settle(a, nil)
	settle(b, a)
	settle(c, nil)
	settle(b2, a)
	prog := map[string]*imageProgress{}
	hashed := []motifHashedDedupe{}
	for _, item := range []*motifCandidate{a, b, b2, c} {
		prog[item.task.key()] = item.progress
		hashed = append(hashed, motifHashedDedupe{Key: item.task.key(), Hash: item.sourceHash})
	}

	art, ok := r.motifReuseDedupe(outDir, prog, hashed)
	if !ok {
		t.Fatal("输入与规则都没变、结论齐全时必须能复用")
	}
	if art.Distinct != 3 || len(art.Groups) != 1 {
		t.Fatalf("复用的结论不对：distinct=%d groups=%d", art.Distinct, len(art.Groups))
	}

	// 图片内容变了（同一路径、新摘要）→ 指纹失配。
	changed := append([]motifHashedDedupe{}, hashed...)
	changed[3].Hash = "hash-z"
	if _, ok := r.motifReuseDedupe(outDir, prog, changed); ok {
		t.Fatal("输入变了必须重扫")
	}
	// 少了一张图 / 多了一张图 → 覆盖不全。
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed[:3]); ok {
		t.Fatal("图少了必须重扫")
	}
	extra := append(append([]motifHashedDedupe{}, hashed...), motifHashedDedupe{Key: "b.md|images/b/d.jpg", Hash: "hash-d"})
	if _, ok := r.motifReuseDedupe(outDir, prog, extra); ok {
		t.Fatal("多了图必须重扫")
	}
	// 结论被清掉（比如有人重跑了 classify）→ 结论不再可信。有跟随关系的图
	// 少一份标记就必须重扫；独立图没有关系可丢，只看摘要对不对（T148：以前
	// 连独立图也要求标记，而那些图根本没候选、永远拿不到标记，于是每次续跑
	// 都整池重算、把同样的会话再放一遍）。
	prog[a.task.key()].DedupeChecked = false
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed); ok {
		t.Fatal("结论被清掉必须重扫")
	}
	prog[a.task.key()].DedupeChecked = true
	prog[c.task.key()].DedupeChecked = false
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed); !ok {
		t.Fatal("独立图没有关系可丢，摘要对得上就该沿用")
	}
	prog[c.task.key()].DedupeChecked = true
	// 跟随关系丢了 → 同样重扫。
	prog[b.task.key()].EquivalentTo = ""
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed); ok {
		t.Fatal("跟随关系丢了必须重扫")
	}
	prog[b.task.key()].EquivalentTo = a.task.key()
	// 规则变了（阈值）→ 指纹失配。
	r.cfg.Latex.DedupeIconAuto = 0.80
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed); ok {
		t.Fatal("规则变了必须重扫")
	}
}

// 「组内最低分」可能低于入组门槛（成员是跟组内某个成员达标进来的、或组间收口
// 时随代表图带入），这一页得自己说清楚，不然看表的人会以为判据出了偏差。
func TestMotifArtifactReportExplainsBelowMinMember(t *testing.T) {
	art := motifArtifact{
		BuiltAt: "2026-01-01 00:00", Refs: 3, Distinct: 2, Fingerprint: "abc",
		Rules: motifArtifactRules{AlignMin: 0.60, AlignShift: 0.10, IconAuto: 0.75},
		Groups: []motifArtifactGroup{{
			Key: "images/b/a.jpg", KeyImage: "images/b/a.jpg", Class: ClassVector, Source: "session",
			Members: []motifArtifactRef{
				{Ref: "images/b/a.jpg", Image: "images/b/a.jpg", AlignScore: 1},
				{Ref: "images/b/b.jpg", Image: "images/b/b.jpg", AlignScore: 0.55},
			},
		}},
		Independent: []motifArtifactRef{{Ref: "images/b/c.jpg", Image: "images/b/c.jpg"}},
	}
	report := motifArtifactReport(art)
	if !strings.Contains(report, "0.550") {
		t.Fatalf("表里要给组内最低分：\n%s", report)
	}
	if !strings.Contains(report, "换个分辨率") {
		t.Fatalf("低于门槛时要有一句解释：\n%s", report)
	}
	art.Groups[0].Members[1].AlignScore = 0.90
	if off := motifArtifactReport(art); strings.Contains(off, "换个分辨率") {
		t.Fatalf("都达标时不该出现这句解释：\n%s", off)
	}
}
