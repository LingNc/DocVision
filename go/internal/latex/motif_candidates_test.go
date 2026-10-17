package latex

import (
	"image"
	"image/color"
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
