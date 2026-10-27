package latex

import (
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
	if overlay == nil || overlay.Bounds() != ref.Bounds() {
		t.Fatalf("overlay bounds = %v, want %v", overlay.Bounds(), ref.Bounds())
	}
	if result.Similarity < 0.75 {
		t.Fatalf("similarity = %.3f, want at least 0.75", result.Similarity)
	}
	if len(result.Runs) < 1 {
		t.Fatalf("alignment runs = %d, want at least one", len(result.Runs))
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
