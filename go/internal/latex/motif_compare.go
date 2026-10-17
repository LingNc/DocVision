package latex

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"mineru-tools/internal/session"
)

// motifCompareTool renders 2-4 images of the current session side by side in ONE
// picture, all inside the same box with the aspect ratio preserved.
//
// Why it exists: a model that only sees one image per call has to remember the
// previous drawing to judge "same logo or not", and the hardest call of the whole
// dedupe phase — one mark redrawn at different scales and crops — is exactly the
// one that looks different when you look at the pictures one at a time (T126).
// A side-by-side view makes the shared structure visible; the numbers in the
// receipt (pixel size, aspect) make the scale/crop differences explicit.
//
// It is deliberately an AID, not evidence: this tool has no access to the `seen`
// set, so looking at a comparison can never satisfy the "every member must be
// inspected individually and uncropped" gate on submit. Nor is it a contact
// sheet: it takes at most 4 ids the model itself names (never a whole bucket),
// so it cannot be used to "inspect" hundreds of images with a handful of calls.
type motifCompareTool struct {
	// root is the session workspace (the same tree view_image sees).
	root string
	// files maps 1-based session id to a workspace-relative path: members
	// first (originals/NNNNNN.ext), then keys (keys/NNNNNN.ext), so the ids
	// are the very ids used by motif_groups and by the submit gate.
	files []string
}

const (
	motifCompareMax    = 4   // more than four panes and it becomes a contact sheet
	motifCompareBoxW   = 470 // per-pane drawing box, px
	motifCompareBoxH   = 420 //
	motifCompareGap    = 10  // white margin between panes, px
	motifCompareLabelH = 24  // id strip above each pane, px
	motifCompareLabelX = 3   // label glyph scale (basicfont is 7x13)
)

func (t *motifCompareTool) Name() string { return "compare_images" }

func (t *motifCompareTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "compare_images",
		"description": "Render 2-4 images of this session side by side in one picture (one box each, aspect preserved, no distortion) to compare a detail that is hard to judge one image at a time. ids are the same 1-based ids motif_groups uses (keys/000004 counts as its member id). This is a comparison aid only: it does not count as inspecting anything, so every member still needs its own uncropped view_image before it can be merged.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}, "description": "2-4 session ids to place side by side, left to right then top to bottom"},
		}, "required": []string{"ids"}},
	}}
}

func (t *motifCompareTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rawIDs, _ := args["ids"].([]any)
	ids := make([]int, 0, len(rawIDs))
	for _, v := range rawIDs {
		f, ok := v.(float64)
		if !ok || f != math.Trunc(f) {
			return session.ToolResult{Text: "REJECTED: ids 必须是整数。"}, nil
		}
		ids = append(ids, int(f))
	}
	if len(ids) < 2 || len(ids) > motifCompareMax {
		return session.ToolResult{Text: fmt.Sprintf(
			"REJECTED: compare_images 一次要比 2-%d 张（给了 %d 个）。本会话 id 范围 1..%d；张数更多就退化成联系图了，那正是 dedupe 不要的东西。",
			motifCompareMax, len(ids), len(t.files))}, nil
	}
	used := map[int]bool{}
	for _, id := range ids {
		if id < 1 || id > len(t.files) {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: id %d 超出本会话 1..%d。", id, len(t.files))}, nil
		}
		if used[id] {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: id %d 重复；同一张图比两次没有意义。", id)}, nil
		}
		used[id] = true
	}

	type pane struct {
		id   int
		img  image.Image
		w, h int
	}
	panes := make([]pane, 0, len(ids))
	for _, id := range ids {
		img, err := decodeImage(filepath.Join(t.root, filepath.FromSlash(t.files[id-1])))
		if err != nil {
			return session.ToolResult{}, fmt.Errorf("compare_images: id %d (%s): %w", id, t.files[id-1], err)
		}
		b := img.Bounds()
		panes = append(panes, pane{id: id, img: img, w: b.Dx(), h: b.Dy()})
	}

	cols := 2
	rows := (len(panes) + cols - 1) / cols
	canvasW := motifCompareGap + cols*(motifCompareBoxW+motifCompareGap)
	canvasH := motifCompareGap + rows*(motifCompareBoxH+motifCompareLabelH+motifCompareGap)
	canvas := image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	var where strings.Builder
	for i, p := range panes {
		col, row := i%cols, i/cols
		left := motifCompareGap + col*(motifCompareBoxW+motifCompareGap)
		top := motifCompareGap + row*(motifCompareBoxH+motifCompareLabelH+motifCompareGap)
		box := image.Rect(left, top+motifCompareLabelH, left+motifCompareBoxW, top+motifCompareLabelH+motifCompareBoxH)
		drawn := fitBox(box, p.w, p.h)
		// CatmullRom both ways: most motifs here are tiny bitmaps that get
		// blown up, and nearest-neighbour blockiness hides thin strokes.
		xdraw.CatmullRom.Scale(canvas, drawn, p.img, p.img.Bounds(), draw.Src, nil)
		strokeRect(canvas, drawn.Inset(-2), color.RGBA{120, 120, 120, 255})
		drawCompareLabel(canvas, left, top+2, fmt.Sprintf("%d", p.id))
		fmt.Fprintf(&where, "%s%d: %d×%dpx aspect %.2f", sep(i), p.id, p.w, p.h, aspect(p.w, p.h))
	}

	b64, err := encodeJPEGBase64(canvas)
	if err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{
		Text: fmt.Sprintf(
			"Compared %d images in one picture (%s). Each pane is %d×%d px, aspect preserved and no distortion; every image was scaled to fill its box, so on-picture size is NOT comparable — the numbers above are. This is a comparison aid, not evidence: it does not count as inspecting anything, and every member still needs its own uncropped view_image before it can be merged.",
			len(panes), where.String(), motifCompareBoxW, motifCompareBoxH),
		ImageBase64: b64,
		ImageMIME:   "image/jpeg",
	}, nil
}

func sep(i int) string {
	if i == 0 {
		return "left→right, top→bottom — "
	}
	return "; "
}

func aspect(w, h int) float64 {
	if h <= 0 {
		return 0
	}
	return float64(w) / float64(h)
}

// fitBox centres a w×h drawing inside box at the largest scale that fits,
// preserving the aspect ratio.
func fitBox(box image.Rectangle, w, h int) image.Rectangle {
	if w <= 0 || h <= 0 || box.Dx() <= 0 || box.Dy() <= 0 {
		return image.Rectangle{}
	}
	scale := math.Min(float64(box.Dx())/float64(w), float64(box.Dy())/float64(h))
	nw := int(math.Round(float64(w) * scale))
	nh := int(math.Round(float64(h) * scale))
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	x := box.Min.X + (box.Dx()-nw)/2
	y := box.Min.Y + (box.Dy()-nh)/2
	return image.Rect(x, y, x+nw, y+nh)
}

// strokeRect draws a 1px rectangle, clipped to the image.
func strokeRect(dst draw.Image, r image.Rectangle, c color.Color) {
	r = r.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}
	fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), c)
	fillRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), c)
	fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), c)
	fillRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), c)
}

func fillRect(dst draw.Image, r image.Rectangle, c color.Color) {
	r = r.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(dst, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// drawCompareLabel writes the session id above a pane, scaled up so it is still
// readable after the provider downscales the composite.
func drawCompareLabel(dst *image.RGBA, x, y int, text string) {
	face := basicfont.Face7x13
	m := face.Metrics()
	asc, h := m.Ascent.Ceil(), m.Height.Ceil()
	w := font.MeasureString(face, text).Ceil()
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(tmp, tmp.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	d := &font.Drawer{Dst: tmp, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(0, asc)}
	d.DrawString(text)
	box := image.Rect(x, y, x+w*motifCompareLabelX, y+h*motifCompareLabelX)
	strokeRect(dst, box.Inset(-2), color.RGBA{120, 120, 120, 255})
	xdraw.NearestNeighbor.Scale(dst, box, tmp, tmp.Bounds(), draw.Src, nil)
}
