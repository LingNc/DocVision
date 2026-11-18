package dedupe

import (
	"image/color"
	"testing"
)

// iconLike 造一枚"小图标"：60×60 白底中央 20×20 实块（白占比 0.89 ≥ 0.82 特征阈值、
// 单连通片、不贴边、内容框 20×20 满足居中重合下限；shift 为水平平移）。
func iconLike(key string, shift int) Sample {
	img := blankImage(60, 60)
	black := color.NRGBA{A: 255}
	for y := 20; y < 40; y++ {
		for x := 20 + shift; x < 40+shift; x++ {
			img.Set(x, y, black)
		}
	}
	return Sample{Key: key, Image: img}
}

func TestFindCandidatesExactGroup(t *testing.T) {
	a := iconLike("a", 0)
	b := iconLike("b", 0) // 像素完全相同 → sha256 精确收缩
	c := iconLike("c", 20) // 内容不同，不相似
	groups := FindCandidates([]Sample{a, b, c})
	if len(groups) != 1 {
		t.Fatalf("应只出一组（a+b 精确同内容）: %+v", groups)
	}
	if !groups[0].Exact || len(groups[0].Members) != 2 {
		t.Fatalf("a+b 应为精确组: %+v", groups[0])
	}
}

// iconPair 造一枚"双块小图标"：80×60 白底两个 10×10 实块（components=2 →
// 视觉/色度特征双双被拒，只能靠居中重合判据并桶——专门喂给 aligned 路径）。
// 注意：居中重合对内容框内的绝对平移天然不变（裁框即归一），所以平移副本
// 本来就应并桶；负例必须用"内容真不同"的图（块间距不同 → 长宽比不同）。
func iconPair(key string, shift int) Sample {
	img := blankImage(80, 60)
	black := color.NRGBA{A: 255}
	for _, bx := range []int{10, 24} {
		for y := 25; y < 35; y++ {
			for x := bx + shift; x < bx+10+shift; x++ {
				img.Set(x, y, black)
			}
		}
	}
	return Sample{Key: key, Image: img}
}

// iconPairWide 内容真不同的双块图标（块间距 20 → 长宽比差 >0.35，预筛拒）。
func iconPairWide(key string) Sample {
	img := blankImage(80, 60)
	black := color.NRGBA{A: 255}
	for _, bx := range []int{10, 34} {
		for y := 25; y < 35; y++ {
			for x := bx; x < bx+10; x++ {
				img.Set(x, y, black)
			}
		}
	}
	return Sample{Key: key, Image: img}
}

func TestFindCandidatesAlignedMerge(t *testing.T) {
	// b 是 a 的平移副本（特征路径全拒，只有居中重合判据能并桶）；
	// c 内容真不同（块间距变了），预筛/重合度都应拒。
	a := iconPair("a", 0)
	b := iconPair("b", 30) // 平移 30px：内容框归一化后与 a 等价，应并桶
	c := iconPairWide("c")
	groups, merges := FindCandidatesAligned([]Sample{a, b, c}, 0.60, 0.10)
	if merges < 1 {
		t.Fatalf("居中重合并桶应至少并入一对: merges=%d groups=%+v", merges, groups)
	}
	ab, withC := false, false
	for _, g := range groups {
		set := map[string]bool{}
		for _, m := range g.Members {
			set[m] = true
		}
		if set["a"] && set["b"] && !set["c"] {
			ab = true
		}
		if set["c"] && (set["a"] || set["b"]) {
			withC = true
		}
	}
	if !ab || withC {
		t.Fatalf("a+b 应同组且 c 独立: ab=%v withC=%v groups=%+v", ab, withC, groups)
	}
}

func TestSimilarToGate(t *testing.T) {
	a := iconLike("a", 0)
	b := iconLike("b", 1) // 1px 平移，像素不完全同
	if !SimilarTo(a.Image, b.Image) {
		t.Fatal("1px 平移的同一枚图标应允许合并（闸门保守但像素近同应过）")
	}
	c := blankImage(60, 60)
	if SimilarTo(a.Image, c) {
		t.Fatal("空白图与图标不应允许合并")
	}
}

func TestValidateGroups(t *testing.T) {
	// 合法：key 在组内、无跨组重复
	ok := []GroupIDs{{Key: 1, Members: []int{1, 2, 3}}, {Key: 5, Members: []int{5, 6}}}
	if err := ValidateGroups(ok, 10); err != nil {
		t.Fatalf("合法组应通过: %v", err)
	}
	// key 不在组内
	bad := []GroupIDs{{Key: 9, Members: []int{1, 2}}}
	if err := ValidateGroups(bad, 10); err == nil {
		t.Fatal("key 不在组内应拒")
	}
	// 跨组重复
	dup := []GroupIDs{{Key: 1, Members: []int{1, 2}}, {Key: 3, Members: []int{2, 3}}}
	if err := ValidateGroups(dup, 10); err == nil {
		t.Fatal("跨组重复应拒")
	}
	// 单张不成组
	single := []GroupIDs{{Key: 1, Members: []int{1}}}
	if err := ValidateGroups(single, 10); err == nil {
		t.Fatal("单张组应拒")
	}
}

func TestValidateGroupShape(t *testing.T) {
	if err := ValidateGroupShape([]GroupIDs{{Key: 1, Members: []int{1, 2}}}); err != nil {
		t.Fatalf("有 key 应通过: %v", err)
	}
	if err := ValidateGroupShape([]GroupIDs{{Members: []int{1, 2}}}); err == nil {
		t.Fatal("无 key 应拒（整池一组也必须有代表图）")
	}
}
