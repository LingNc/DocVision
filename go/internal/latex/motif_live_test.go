package latex

import (
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Set DOCVISION_MOTIF_CASE to the extracted images directory to run the
// read-only live-book fixture; ordinary CI has no private source book.
func TestMotifLiveBookCandidateRecall(t *testing.T) {
	root := os.Getenv("DOCVISION_MOTIF_CASE")
	if root == "" {
		t.Skip("read-only optional private-book fixture")
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	var samples []VisualMotifSample
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		im, _, decodeErr := image.Decode(f)
		_ = f.Close()
		if decodeErr != nil {
			continue
		}
		if im.Bounds().Dx() > 70 || im.Bounds().Dy() > 70 {
			continue
		}
		samples = append(samples, VisualMotifSample{Key: filepath.Base(p), Image: im})
	}
	groups := FindVisualMotifCandidates(samples)
	largest := 0
	var blueGrouped bool
	for _, g := range groups {
		if len(g.Members) > largest {
			largest = len(g.Members)
		}
		for _, name := range g.Members {
			if strings.HasPrefix(name, "61cb8ba4") || strings.HasPrefix(name, "1fd979c3") || strings.HasPrefix(name, "acca0bd6") || strings.HasPrefix(name, "88d28590") {
				blueGrouped = true
			}
		}
	}
	t.Logf("small=%d groups=%d largest=%d", len(samples), len(groups), largest)
	if largest < 100 {
		t.Fatalf("real repeated emblem not nominated (largest=%d)", largest)
	}
	if blueGrouped {
		t.Fatal("blue problem number grouped with pure emblem")
	}
}
