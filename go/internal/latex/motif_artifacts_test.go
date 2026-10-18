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
	applied := []motifAppliedGroup{{Key: a, Members: []*motifCandidate{a, b}, Class: ClassVector}}
	r.writeMotifArtifacts(outDir, leaders, copies, applied, 1)

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
	applied := []motifAppliedGroup{{Key: a, Members: []*motifCandidate{a, b}, Class: ClassVector}}
	r.writeMotifArtifacts(outDir, leaders, copies, applied, 1)

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
	// 核验标记被清掉（比如有人重跑了 classify）→ 结论不再可信。
	prog[c.task.key()].DedupeChecked = false
	if _, ok := r.motifReuseDedupe(outDir, prog, hashed); ok {
		t.Fatal("结论被清掉必须重扫")
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
