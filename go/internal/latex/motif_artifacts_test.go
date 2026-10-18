package latex

import (
	"encoding/json"
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
	if len(art.Groups) != 1 || !art.Groups[0].Auto {
		t.Fatalf("应当记录 1 个本地自动定组: %+v", art.Groups)
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
