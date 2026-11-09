package latex

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

func testFigureImage(dx, dy int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 80))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	ink := color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	for y := 12; y < 62; y++ {
		for x := 18; x < 72; x++ {
			if (x-18)%11 < 2 || (y-12)%13 < 2 || (x-46)*(x-46)+(y-37)*(y-37) < 80 {
				px, py := x+dx, y+dy
				if image.Pt(px, py).In(img.Bounds()) {
					img.SetNRGBA(px, py, ink)
				}
			}
		}
	}
	return img
}

func TestCompareFigureImagesFindsTranslation(t *testing.T) {
	ref := testFigureImage(0, 0)
	candidate := testFigureImage(4, -3)
	result, overlay, err := compareFigureImages(ref, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if overlay == nil {
		t.Fatal("legacy overlay is nil")
	}
	if result.Similarity < 0.75 {
		t.Fatalf("similarity = %.3f, want at least 0.75", result.Similarity)
	}
	if len(result.Runs) < 1 {
		t.Fatalf("alignment runs = %d, want at least one", len(result.Runs))
	}
}

func TestFigureOverlayDetailFourPanelsAndCrop(t *testing.T) {
	ref, cand := testFigureImage(0, 0), testFigureImage(2, 1)
	board := figureOverlayDetail(ref, cand, 0, 0, "all")
	if got, want := board.Bounds().Size(), (image.Point{X: 192, Y: 160}); got != want {
		t.Fatalf("four-panel bounds = %v, want %v", got, want)
	}
	gray := figureOverlayDetail(ref, cand, 0, 0, "gray")
	if got, want := gray.Bounds().Size(), ref.Bounds().Size(); got != want {
		t.Fatalf("single-channel bounds = %v, want %v", got, want)
	}
	crop := cropFigureImage(gray, 25, 25, 75, 75)
	if got, want := crop.Bounds().Size(), (image.Point{X: 48, Y: 40}); got != want {
		t.Fatalf("crop bounds = %v, want %v", got, want)
	}
}

func TestCompareFigureDefinitionIncludesReviewParameters(t *testing.T) {
	definition := (&compareFigureTool{}).Definition()
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"pdf_path", "detail", "worst_region", "left", "top", "right", "bottom", "zoom"} {
		if !strings.Contains(string(encoded), `"`+key+`"`) {
			t.Errorf("tool schema missing %q", key)
		}
	}
}

func TestCompareFigureToolIsOptionalBeforeCompile(t *testing.T) {
	state := &tikzState{}
	tool := &compareFigureTool{State: state}
	result, err := tool.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "compile a figure successfully first") {
		t.Fatalf("unexpected receipt: %q", result.Text)
	}
	if state.submitted || state.compileOK {
		t.Fatal("compare must not change compile or submit state")
	}
}
