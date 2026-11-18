package dedupe

import (
	"image/color"
	"testing"
)

func nrgbaBlack() color.NRGBA { return color.NRGBA{A: 255} }

func TestContentLeaders(t *testing.T) {
	items := []struct {
		Key, Hash string
	}{
		{"a", "h1"}, {"b", "h2"}, {"c", "h1"}, {"d", "h3"}, {"e", "h2"},
	}
	leaders, leaderOf := ContentLeaders(items)
	if len(leaders) != 3 {
		t.Fatalf("应三张不同内容: %+v", leaders)
	}
	if leaderOf["c"] != "a" || leaderOf["e"] != "b" {
		t.Fatalf("副本应挂到各自代表: %v", leaderOf)
	}
	// 首现者当代表（确定性）
	if leaders[0].Key != "a" || leaders[0].Copies[0] != "c" {
		t.Fatalf("首现者应为 a 且 c 为其副本: %+v", leaders[0])
	}
}

func TestPickRepresentative(t *testing.T) {
	// key 是双块图标（内容 24×10）；big 是同一图标加一条 1×10 的细边
	// （重合 240/250=0.96 ≥ 0.95 仍是典型成员，但内容框更大）→ 代表图应取 big。
	key, _ := AlignCoverage(iconPair("k", 0).Image)
	bigImg := blankImage(80, 60)
	for _, bx := range []int{10, 24} {
		for y := 25; y < 35; y++ {
			for x := bx; x < bx+10; x++ {
				bigImg.SetNRGBA(x, y, nrgbaBlack())
			}
		}
	}
	for y := 25; y < 35; y++ {
		bigImg.SetNRGBA(34, y, nrgbaBlack())
	}
	mBig, _ := AlignCoverage(bigImg)
	rep, ok := PickRepresentative(key, []AlignImage{key, mBig}, 0.95, ShiftCells(0.10))
	if !ok || rep.Area <= key.Area {
		t.Fatalf("代表图应取内容框更大的典型成员: key=%d rep=%d ok=%v", key.Area, rep.Area, ok)
	}
}

func TestFoldGroupsSameBucketOnly(t *testing.T) {
	score := func(a, b string) (float64, bool) { return 0.9, true }
	okAll := func(a, b string) bool { return true }
	applied := []AppliedGroup{
		{Key: "k1", Members: []string{"a", "b"}, Bucket: 1},
		{Key: "k2", Members: []string{"c", "d"}, Bucket: 1},
		{Key: "k3", Members: []string{"e", "f"}, Bucket: 2},
	}
	bucketOf := map[string]int{"k1": 1, "k2": 1, "k3": 2}
	out, folded, _ := FoldGroups(applied, bucketOf, score, okAll, ShiftCells(0.10))
	if folded != 1 || len(out) != 2 {
		t.Fatalf("同桶两组应折一，跨桶不动: folded=%d out=%+v", folded, out)
	}
	var foldedGroup *AppliedGroup
	for i := range out {
		if len(out[i].Members) == 4 {
			foldedGroup = &out[i]
		}
	}
	if foldedGroup == nil {
		t.Fatalf("折后组应有 4 成员: %+v", out)
	}
}

func TestAutoIconGroups(t *testing.T) {
	a := iconPair("a", 0)
	b := iconPair("b", 30) // 平移副本：内容等价，重合 ~1
	d := iconPairWide("d")
	covs := map[string]AlignImage{}
	for _, s := range []Sample{a, b, d} {
		if c, ok := AlignCoverage(s.Image); ok {
			covs[s.Key] = c
		}
	}
	okAll := func(x, y string) bool { return true }
	groups, remaining := AutoIconGroups([]string{"a", "b", "d"}, covs, 0.75, okAll, ShiftCells(0.10))
	if len(groups) != 1 || groups[0][0] != "a" || groups[0][1] != "b" {
		t.Fatalf("a+b 应自动定组: %+v", groups)
	}
	if len(remaining) != 1 || remaining[0] != "d" {
		t.Fatalf("d 应剩余: %v", remaining)
	}
}

func TestAttachLeftovers(t *testing.T) {
	a := iconPair("a", 0)
	covA, _ := AlignCoverage(a.Image)
	loose := iconPair("loose", 30) // 平移副本 → 重合 ~1，达标
	covL, _ := AlignCoverage(loose.Image)
	other := iconPairWide("other") // 内容不同 → 不挂
	covO, _ := AlignCoverage(other.Image)
	covs := map[string]AlignImage{"loose": covL, "other": covO}
	anchors := map[string]AlignImage{"a": covA}
	grouped := map[string]bool{}
	attached := AttachLeftovers([]string{"loose", "other"}, covs, anchors, grouped, 0.60, ShiftCells(0.10))
	if attached["loose"] != "a" {
		t.Fatalf("loose 应挂到 a: %v", attached)
	}
	if _, bad := attached["other"]; bad {
		t.Fatalf("other 不应挂靠: %v", attached)
	}
	// 已 grouped 的跳过
	grouped["loose"] = true
	attached = AttachLeftovers([]string{"loose"}, covs, anchors, grouped, 0.60, ShiftCells(0.10))
	if len(attached) != 0 {
		t.Fatalf("已确认图不应再挂: %v", attached)
	}
}
