package latex

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"testing"
)

func syntheticIcon(w, h, shift int, blueMark bool, edge bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	cx, cy := w/2+shift, h/2
	for y := -7; y <= 7; y++ {
		for x := -7; x <= 7; x++ {
			if absInt(x)+absInt(y) <= 7 {
				if edge {
					img.Set(0, y+cy, color.RGBA{220, 30, 30, 255})
				} else {
					img.Set(cx+x, cy+y, color.RGBA{220, 30, 30, 255})
				}
			}
		}
	}
	if blueMark {
		for y := 3; y < 8; y++ {
			img.Set(w-5, y, color.RGBA{20, 70, 210, 255})
		}
	}
	return img
}

func TestFindVisualMotifCandidatesExactContent(t *testing.T) {
	a := syntheticIcon(43, 43, 0, false, false)
	got := FindVisualMotifCandidates([]VisualMotifSample{{Key: "b", Image: a}, {Key: "a", Image: a}})
	if len(got) != 1 || !got[0].Exact || len(got[0].Members) != 2 {
		t.Fatalf("exact groups = %#v", got)
	}
	if got[0].Members[0] != "a" {
		t.Fatalf("members must be deterministic: %#v", got[0].Members)
	}
}

func TestFindVisualMotifCandidatesSimilarTinyIcons(t *testing.T) {
	got := FindVisualMotifCandidates([]VisualMotifSample{
		{Key: "first", Image: syntheticIcon(43, 43, 0, false, false)},
		{Key: "second", Image: syntheticIcon(46, 45, 1, false, false)},
	})
	if len(got) != 1 || got[0].Exact {
		t.Fatalf("similar groups = %#v", got)
	}
}

func TestFindVisualMotifCandidatesRejectsBlueLocalNumber(t *testing.T) {
	got := FindVisualMotifCandidates([]VisualMotifSample{
		{Key: "plain", Image: syntheticIcon(43, 43, 0, false, false)},
		{Key: "numbered", Image: syntheticIcon(43, 43, 0, true, false)},
	})
	if len(got) != 0 {
		t.Fatalf("blue local annotation must split candidate: %#v", got)
	}
}

func TestFindVisualMotifCandidatesRejectsPartialAndLarge(t *testing.T) {
	partial := syntheticIcon(43, 43, 0, false, true)
	large := syntheticIcon(71, 43, 0, false, false)
	plain := syntheticIcon(43, 43, 0, false, false)
	got := FindVisualMotifCandidates([]VisualMotifSample{
		{Key: "partial", Image: partial}, {Key: "large", Image: large}, {Key: "plain", Image: plain},
	})
	if len(got) != 0 {
		t.Fatalf("unsafe partial/large candidates = %#v", got)
	}
}

func TestFindVisualMotifCandidatesIgnoresInvalidSamples(t *testing.T) {
	got := FindVisualMotifCandidates([]VisualMotifSample{{Key: "", Image: nil}, {Key: "ok", Image: image.NewRGBA(image.Rect(0, 0, 2, 2))}})
	if len(got) != 0 {
		t.Fatalf("invalid samples = %#v", got)
	}
}

// bruteForceMotifGroups is an independent, deliberately naive reference for
// FindVisualMotifCandidates: it re-derives the features for every pair (the way
// the detector used to) and rebuilds the union. The detector hoists feature
// extraction out of the O(N²) loop for speed (20s → 0.3s on a 388-image book);
// this test pins that the verdicts did not move.
func bruteForceMotifGroups(samples []VisualMotifSample) []VisualMotifGroup {
	var valid []VisualMotifSample
	for _, s := range samples {
		if s.Key != "" && s.Image != nil {
			valid = append(valid, s)
		}
	}
	parent := make([]int, len(valid))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(i, j int) {
		ri, rj := find(i), find(j)
		if ri != rj {
			parent[rj] = ri
		}
	}
	exact := map[[32]byte]int{}
	for i, s := range valid {
		if h, ok := rgbaFingerprint(s.Image); ok {
			if j, exists := exact[h]; exists {
				union(i, j)
			} else {
				exact[h] = i
			}
		}
	}
	for i := range valid {
		for j := i + 1; j < len(valid); j++ {
			ma, oka := motifFeature(valid[i].Image)
			mb, okb := motifFeature(valid[j].Image)
			same := false
			if oka && okb {
				if ma.aspect >= 0.55 && ma.aspect <= 1.8 && mb.aspect >= 0.55 && mb.aspect <= 1.8 && absFloat(ma.aspect-mb.aspect) <= 0.18 {
					intersect, unionN, colorError := 0, 0, 0.0
					for k := range ma.mask {
						if ma.mask[k] && mb.mask[k] {
							intersect++
							colorError += colorDistance(ma.colors[k], mb.colors[k])
						}
						if ma.mask[k] || mb.mask[k] {
							unionN++
						}
					}
					if unionN > 0 && float64(intersect)/float64(unionN) >= 0.65 && colorError/float64(intersect) < 0.055 {
						same = true
					}
				}
			} else {
				ca, coka := chromaticMotifFeature(valid[i].Image)
				cb, cokb := chromaticMotifFeature(valid[j].Image)
				if coka && cokb && ca.hue == cb.hue && absFloat(ca.aspect-cb.aspect) <= 0.36 {
					inter, uni := 0, 0
					for k, v := range ca.mask {
						if v && cb.mask[k] {
							inter++
						}
						if v || cb.mask[k] {
							uni++
						}
					}
					same = uni > 0 && float64(inter)/float64(uni) >= 0.53
				}
			}
			if same {
				union(i, j)
			}
		}
	}
	groups := map[int][]string{}
	for i, s := range valid {
		groups[find(i)] = append(groups[find(i)], s.Key)
	}
	var out []VisualMotifGroup
	for root, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		isExact := true
		h, ok := rgbaFingerprint(valid[root].Image)
		if !ok {
			isExact = false
		}
		for _, key := range members {
			for _, s := range valid {
				if s.Key != key {
					continue
				}
				sh, sok := rgbaFingerprint(s.Image)
				if !sok || sh != h {
					isExact = false
				}
				break
			}
		}
		out = append(out, VisualMotifGroup{Members: members, Exact: isExact})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Members[0] < out[j].Members[0] })
	return out
}

func TestFindVisualMotifCandidatesMatchesBruteForce(t *testing.T) {
	var samples []VisualMotifSample
	add := func(key string, img image.Image) { samples = append(samples, VisualMotifSample{Key: key, Image: img}) }
	for i := 0; i < 6; i++ {
		add(fmt.Sprintf("plain-%d", i), syntheticIcon(43+i%3, 44+i%2, i%3, false, false))
	}
	shared := syntheticIcon(43, 43, 0, false, false)
	add("dup-a", shared)
	add("dup-b", shared)
	add("dup-c", syntheticIcon(43, 43, 0, false, false)) // identical pixels, separate allocation
	for i := 0; i < 3; i++ {
		add(fmt.Sprintf("numbered-%d", i), syntheticIcon(44+i, 43, i, true, false))
	}
	add("large", syntheticIcon(80, 60, 0, false, false))
	add("edge", syntheticIcon(43, 43, 0, false, true))
	empty := image.NewRGBA(image.Rect(0, 0, 9, 9))
	add("blank", empty)
	add("blank2", empty)

	want := bruteForceMotifGroups(samples)
	got := FindVisualMotifCandidates(samples)
	if len(want) != len(got) {
		t.Fatalf("group count: brute force %d, detector %d\nwant=%v\ngot=%v", len(want), len(got), want, got)
	}
	for i := range want {
		if want[i].Exact != got[i].Exact || strings.Join(want[i].Members, ",") != strings.Join(got[i].Members, ",") {
			t.Fatalf("group %d differs: want %v exact=%v, got %v exact=%v", i, want[i].Members, want[i].Exact, got[i].Members, got[i].Exact)
		}
	}
}
