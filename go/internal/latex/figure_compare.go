package latex

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	"mineru-tools/internal/session"
)

// compareFigureTool returns visual evidence for the latest successful TikZ
// compile. It does not change tikzState or the submit path.
type compareFigureTool struct {
	State        *tikzState
	Comp         *Compiler
	OriginalPath string
}

func (t *compareFigureTool) Name() string { return "compare_figure" }

func (t *compareFigureTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "compare_figure",
		"description": "Compare the original image with a compiled figure. Returns a four-panel overlay (gray/R/G/B) or one selected panel; supports targeted crop and zoom for detailed re-checks. This is visual evidence, not semantic proof.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"pdf_path":     map[string]any{"type": "string", "description": "Workspace-relative compiled PDF path; defaults to the latest successful compile."},
			"detail":       map[string]any{"type": "string", "enum": []string{"all", "gray", "r", "g", "b", "rg", "rb", "gb"}, "description": "all returns a 2x2 gray/R/G/B board; a channel or channel combination returns that panel."},
			"worst_region": map[string]any{"type": "boolean", "description": "Return only the automatically located weakest comparison region instead of the full figure."},
			"left":         map[string]any{"type": "number", "description": "Crop left percentage, 0-100."},
			"top":          map[string]any{"type": "number", "description": "Crop top percentage, 0-100."},
			"right":        map[string]any{"type": "number", "description": "Crop right percentage, 0-100."},
			"bottom":       map[string]any{"type": "number", "description": "Crop bottom percentage, 0-100."},
			"zoom":         map[string]any{"type": "integer", "description": "Target output width in pixels; 0 keeps the cropped native width."},
		}},
	}}
}

func (t *compareFigureTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	hasExplicitPDF := false
	if raw, ok := args["pdf_path"].(string); ok && strings.TrimSpace(raw) != "" {
		hasExplicitPDF = true
	}
	if t.State == nil || (!hasExplicitPDF && (!t.State.compileOK || strings.TrimSpace(t.State.lastPDF) == "")) {
		return session.ToolResult{Text: "Comparison unavailable: compile a figure successfully first, or provide pdf_path."}, nil
	}
	pdfPath := t.State.lastPDF
	if raw, ok := args["pdf_path"].(string); ok && strings.TrimSpace(raw) != "" {
		candidate := raw
		if filepath.IsAbs(candidate) {
			return session.ToolResult{Text: "Comparison unavailable: pdf_path must be workspace-relative."}, nil
		}
		pdfPath = filepath.Join(t.State.workDir, filepath.Clean(candidate))
		if rel, err := filepath.Rel(t.State.workDir, pdfPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return session.ToolResult{Text: "Comparison unavailable: pdf_path escapes the figure workspace."}, nil
		}
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return session.ToolResult{Text: "Comparison unavailable: PDF not found: " + err.Error()}, nil
	}
	if t.Comp == nil || strings.TrimSpace(t.OriginalPath) == "" {
		return session.ToolResult{Text: "Comparison unavailable: the compiler or original image is not configured."}, nil
	}
	base := filepath.Join(t.State.workDir, "compare_figure_candidate")
	if err := t.Comp.Rasterize(pdfPath, base); err != nil {
		return session.ToolResult{Text: "Comparison unavailable: " + err.Error()}, nil
	}
	candidatePath := base + ".png"
	original, err := decodeFigureImage(t.OriginalPath)
	if err != nil {
		return session.ToolResult{Text: "Comparison unavailable: cannot read original image: " + err.Error()}, nil
	}
	candidate, err := decodeFigureImage(candidatePath)
	if err != nil {
		return session.ToolResult{Text: "Comparison unavailable: cannot read compiled preview: " + err.Error()}, nil
	}
	result, _, err := compareFigureImages(original, candidate)
	if err != nil {
		return session.ToolResult{Text: "Comparison unavailable: " + err.Error()}, nil
	}
	detail := stringArg(args, "detail", "all")
	if !validFigureDetail(detail) {
		return session.ToolResult{Text: "Comparison unavailable: detail must be all, gray, r, g, b, rg, rb, or gb."}, nil
	}
	left, top := numberArg(args, "left", 0), numberArg(args, "top", 0)
	right, bottom := numberArg(args, "right", 100), numberArg(args, "bottom", 100)
	if left < 0 || top < 0 || right > 100 || bottom > 100 || right <= left || bottom <= top {
		return session.ToolResult{Text: "Comparison unavailable: crop must satisfy 0 <= left < right <= 100 and 0 <= top < bottom <= 100."}, nil
	}
	zoom := int(numberArg(args, "zoom", 0))
	if zoom < 0 {
		zoom = 0
	}
	if zoom > 6000 {
		zoom = 6000
	}
	worstOnly := false
	if v, ok := args["worst_region"].(bool); ok {
		worstOnly = v
	}
	if worstOnly {
		if _, hasCrop := args["left"]; !hasCrop {
			left, top, right, bottom = worstFigureRegion(original, candidate)
		}
	}
	overlay := figureOverlayDetail(original, candidate, result.DX, result.DY, detail)
	overlay = cropFigureImage(overlay, left, top, right, bottom)
	if zoom > 0 && overlay.Bounds().Dx() != zoom {
		h := max(1, figureRound(float64(overlay.Bounds().Dy())*float64(zoom)/float64(overlay.Bounds().Dx())))
		resized := image.NewNRGBA(image.Rect(0, 0, zoom, h))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), overlay, overlay.Bounds(), draw.Src, nil)
		overlay = resized
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, overlay); err != nil {
		return session.ToolResult{Text: "Comparison unavailable: cannot encode comparison overlay: " + err.Error()}, nil
	}
	text := fmt.Sprintf("Soft-ink overlap: %.2f%% (R %.2f%%, G %.2f%%, B %.2f%%, gray %.2f%%). Estimated candidate translation applied: dx=%d px, dy=%d px. Alignment starts: %s. Detail=%s, crop=%.1f%%,%.1f%%-%.1f%%,%.1f%%, output=%dx%dpx. Overlay key: white=shared ink, magenta=original-only ink, green=candidate-only ink, black=background.", result.Similarity*100, result.Channels[0]*100, result.Channels[1]*100, result.Channels[2]*100, result.Gray*100, result.DX, result.DY, formatAlignmentRuns(result.Runs), detail, left, top, right, bottom, overlay.Bounds().Dx(), overlay.Bounds().Dy())
	t.State.compareCalled = true
	return session.ToolResult{Text: text, ImageBase64: base64.StdEncoding.EncodeToString(encoded.Bytes()), ImageMIME: "image/png"}, nil
}

func stringArg(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return def
}
func numberArg(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}
func validFigureDetail(s string) bool {
	switch s {
	case "all", "gray", "r", "g", "b", "rg", "rb", "gb":
		return true
	}
	return false
}

// The image decoder registry includes PNG and JPEG. Keep format selection
// explicit here so malformed/unsupported files return a useful tool receipt.
func decodeFigureImage(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return nil, fmt.Errorf("unsupported image format %q", format)
	}
	return flattenOnWhite(img), nil
}

func flattenOnWhite(src image.Image) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			a := uint32(c.A)
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8((uint32(c.R)*a + 255*(255-a)) / 255),
				G: uint8((uint32(c.G)*a + 255*(255-a)) / 255),
				B: uint8((uint32(c.B)*a + 255*(255-a)) / 255), A: 255,
			})
		}
	}
	return out
}

type softInkImage struct {
	W, H int
	Pix  [][3]float64
}

func (s softInkImage) at(x, y int) [3]float64 { return s.Pix[y*s.W+x] }

type figureAlignmentRun struct {
	Name     string
	Score    float64
	DX, DY   int
	Channels [3]float64
}

type figureComparison struct {
	Similarity float64
	Channels   [3]float64
	Gray       float64
	DX, DY     int
	Runs       []figureAlignmentRun
	Canvas     int
	Grid       int
}

func formatAlignmentRuns(runs []figureAlignmentRun) string {
	parts := make([]string, 0, len(runs))
	for _, run := range runs {
		parts = append(parts, fmt.Sprintf("%s %.2f%% @(%d,%d)", run.Name, run.Score*100, run.DX, run.DY))
	}
	return strings.Join(parts, "; ")
}

func compareFigureImages(reference, candidate image.Image) (figureComparison, *image.NRGBA, error) {
	ref := flattenOnWhite(reference)
	cand := flattenOnWhite(candidate)
	if ref.Bounds().Dx() < 1 || ref.Bounds().Dy() < 1 || cand.Bounds().Dx() < 1 || cand.Bounds().Dy() < 1 {
		return figureComparison{}, nil, fmt.Errorf("empty input image")
	}
	ref, cand = fitFigureCanvases(ref, cand)
	rgbRef, originRef := cropFigureInk(ref, false)
	rgbCand, originCand := cropFigureInk(cand, false)
	canvas := figureMax4(rgbRef.W, rgbRef.H, rgbCand.W, rgbCand.H)
	canvas = max(24, min(512, canvas))
	levels := figureGridLevels(canvas)
	coarse := levels[0]
	centroidRefX, centroidRefY := inkCentroid(rgbRef)
	centroidCandX, centroidCandY := inkCentroid(rgbCand)
	centroidStart := [2]int{
		figureRound((centroidRefY - centroidCandY) * float64(coarse) / float64(canvas)),
		figureRound((centroidRefX - centroidCandX) * float64(coarse) / float64(canvas)),
	}
	best, err := progressiveFigureOverlap(rgbRef, rgbCand, levels, canvas, centroidStart)
	if err != nil {
		return figureComparison{}, nil, err
	}
	dx := figureRound(float64(best.DX*canvas)/float64(levels[len(levels)-1])) + originRef.X - originCand.X
	dy := figureRound(float64(best.DY*canvas)/float64(levels[len(levels)-1])) + originRef.Y - originCand.Y

	grayRef, grayOriginRef := cropFigureInk(ref, true)
	grayCand, grayOriginCand := cropFigureInk(cand, true)
	grayDX := dx - (grayOriginRef.X - grayOriginCand.X)
	grayDY := dy - (grayOriginRef.Y - grayOriginCand.Y)
	grid := levels[len(levels)-1]
	grayA, err := figureToGrid(grayRef, canvas, grid)
	if err != nil {
		return figureComparison{}, nil, err
	}
	grayB, err := figureToGrid(grayCand, canvas, grid)
	if err != nil {
		return figureComparison{}, nil, err
	}
	grayScore := figureOverlapScore(grayA, grayB, figureRound(float64(grayDY*grid)/float64(canvas)), figureRound(float64(grayDX*grid)/float64(canvas))).Mean()

	comparison := figureComparison{Similarity: best.Similarity, Channels: best.Channels, Gray: grayScore,
		DX: dx, DY: dy, Runs: best.Runs, Canvas: canvas, Grid: grid}
	return comparison, figureOverlay(ref, cand, dx, dy), nil
}

func fitFigureCanvases(reference, candidate *image.NRGBA) (*image.NRGBA, *image.NRGBA) {
	w := reference.Bounds().Dx()
	cw, ch := candidate.Bounds().Dx(), candidate.Bounds().Dy()
	if cw != w {
		nh := max(1, figureRound(float64(ch*w)/float64(cw)))
		resized := image.NewNRGBA(image.Rect(0, 0, w, nh))
		xdraw.CatmullRom.Scale(resized, resized.Bounds(), candidate, candidate.Bounds(), draw.Src, nil)
		candidate = resized
		ch = nh
	}
	h := max(reference.Bounds().Dy(), ch)
	refCanvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	candCanvas := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(refCanvas, refCanvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(candCanvas, candCanvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(refCanvas, image.Rect(0, (h-reference.Bounds().Dy())/2, w, (h-reference.Bounds().Dy())/2+reference.Bounds().Dy()), reference, reference.Bounds().Min, draw.Src)
	draw.Draw(candCanvas, image.Rect(0, (h-candidate.Bounds().Dy())/2, w, (h-candidate.Bounds().Dy())/2+candidate.Bounds().Dy()), candidate, candidate.Bounds().Min, draw.Src)
	return refCanvas, candCanvas
}

func cropFigureInk(src image.Image, gray bool) (softInkImage, image.Point) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	ink := make([][3]float64, w*h)
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			var v [3]float64
			if gray {
				luma := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
				v = [3]float64{1 - luma, 1 - luma, 1 - luma}
			} else {
				v = [3]float64{1 - float64(c.R)/255, 1 - float64(c.G)/255, 1 - float64(c.B)/255}
			}
			ink[y*w+x] = v
			if figureMax3(v[0], v[1], v[2]) > 0.2 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX || maxY < minY {
		return softInkImage{W: 1, H: 1, Pix: [][3]float64{{0, 0, 0}}}, image.Point{}
	}
	out := softInkImage{W: maxX - minX + 1, H: maxY - minY + 1,
		Pix: make([][3]float64, (maxX-minX+1)*(maxY-minY+1))}
	for y := 0; y < out.H; y++ {
		copy(out.Pix[y*out.W:(y+1)*out.W], ink[(minY+y)*w+minX:(minY+y)*w+minX+out.W])
	}
	return out, image.Pt(minX, minY)
}

func inkCentroid(ink softInkImage) (float64, float64) {
	var total, sumX, sumY float64
	for y := 0; y < ink.H; y++ {
		for x := 0; x < ink.W; x++ {
			p := ink.at(x, y)
			mass := figureMax3(p[0], p[1], p[2])
			total += mass
			sumX += float64(x) * mass
			sumY += float64(y) * mass
		}
	}
	if total <= 0 {
		return 0, 0
	}
	return sumX / total, sumY / total
}

func figureGridLevels(canvas int) []int {
	finest := figureRound(0.95 * float64(canvas))
	finest = max(8, min(1024, finest))
	final := min(finest, canvas)
	if canvas < 32 {
		return []int{canvas}
	}
	var levels []int
	for g := 32; g <= final; g *= 2 {
		levels = append(levels, g)
	}
	if len(levels) == 0 {
		levels = []int{32}
	} else if levels[len(levels)-1] < final {
		levels = append(levels, final)
	}
	return levels
}

type figureGrid struct {
	N     int
	Cells [][3]float64
}

func figureToGrid(src softInkImage, canvas, n int) (figureGrid, error) {
	if n < 1 || canvas < 1 {
		return figureGrid{}, fmt.Errorf("invalid grid dimensions")
	}
	stride := src.W + 1
	prefix := make([][3]float64, (src.H+1)*stride)
	for y := 1; y <= src.H; y++ {
		var row [3]float64
		for x := 1; x <= src.W; x++ {
			p := src.at(x-1, y-1)
			for c := 0; c < 3; c++ {
				row[c] += p[c]
				prefix[y*stride+x][c] = prefix[(y-1)*stride+x][c] + row[c]
			}
		}
	}
	edges := make([]int, n+1)
	for i := range edges {
		edges[i] = i * canvas / n
	}
	offX, offY := (canvas-src.W)/2, (canvas-src.H)/2
	out := figureGrid{N: n, Cells: make([][3]float64, n*n)}
	for gy := 0; gy < n; gy++ {
		y0, y1 := figureClampInt(edges[gy]-offY, 0, src.H), figureClampInt(edges[gy+1]-offY, 0, src.H)
		for gx := 0; gx < n; gx++ {
			x0, x1 := figureClampInt(edges[gx]-offX, 0, src.W), figureClampInt(edges[gx+1]-offX, 0, src.W)
			area := (edges[gx+1] - edges[gx]) * (edges[gy+1] - edges[gy])
			if area <= 0 {
				continue
			}
			var sum [3]float64
			for c := 0; c < 3; c++ {
				sum[c] = prefix[y1*stride+x1][c] - prefix[y0*stride+x1][c] - prefix[y1*stride+x0][c] + prefix[y0*stride+x0][c]
				out.Cells[gy*n+gx][c] = sum[c] / float64(area)
			}
		}
	}
	return out, nil
}

type gridOverlap struct {
	Channels [3]float64
}

func (s gridOverlap) Mean() float64 { return (s.Channels[0] + s.Channels[1] + s.Channels[2]) / 3 }

func figureOverlapScore(a, b figureGrid, dy, dx int) gridOverlap {
	var inter, union [3]float64
	for y := 0; y < a.N; y++ {
		for x := 0; x < a.N; x++ {
			av := a.Cells[y*a.N+x]
			by, bx := y-dy, x-dx
			if by < 0 || by >= b.N || bx < 0 || bx >= b.N {
				for c := 0; c < 3; c++ {
					union[c] += av[c]
				}
				continue
			}
			bv := b.Cells[by*b.N+bx]
			for c := 0; c < 3; c++ {
				inter[c] += math.Min(av[c], bv[c])
				union[c] += math.Max(av[c], bv[c])
			}
		}
	}
	out := gridOverlap{}
	for c := range out.Channels {
		if union[c] > 1e-9 {
			out.Channels[c] = inter[c] / union[c]
		}
	}
	return out
}

func progressiveFigureOverlap(src, dst softInkImage, levels []int, canvas int, centroidStart [2]int) (figureComparison, error) {
	grids := make([][2]figureGrid, len(levels))
	for i, n := range levels {
		a, err := figureToGrid(src, canvas, n)
		if err != nil {
			return figureComparison{}, err
		}
		b, err := figureToGrid(dst, canvas, n)
		if err != nil {
			return figureComparison{}, err
		}
		grids[i] = [2]figureGrid{a, b}
	}
	shiftCells := min(16, max(0, figureRound(0.10*32)))
	shiftPercent := float64(shiftCells) / 32
	coarseWindow := max(1, figureRound(shiftPercent*float64(levels[0])))
	starts := []struct {
		name string
		dx   int
		dy   int
	}{{name: "origin"}}
	if centroidStart != [2]int{} {
		starts = append(starts, struct {
			name string
			dx   int
			dy   int
		}{name: "centroid", dx: centroidStart[1], dy: centroidStart[0]})
	}
	var runs []figureAlignmentRun
	var bestRun figureAlignmentRun
	var bestCellsX, bestCellsY int
	bestScore := -1.0
	for _, start := range starts {
		shiftX, shiftY := start.dx, start.dy
		var runScore gridOverlap
		for levelIndex, pair := range grids {
			grid := levels[levelIndex]
			centerX, centerY, window := start.dx, start.dy, coarseWindow
			if levelIndex > 0 {
				prev := levels[levelIndex-1]
				centerX = figureRound(float64(shiftX*grid) / float64(prev))
				centerY = figureRound(float64(shiftY*grid) / float64(prev))
				window = min(2, grid)
			}
			levelBest := -1.0
			levelX, levelY := centerX, centerY
			for dy := centerY - window; dy <= centerY+window; dy++ {
				for dx := centerX - window; dx <= centerX+window; dx++ {
					score := figureOverlapScore(pair[0], pair[1], dy, dx)
					value := score.Mean()
					if betterFigureScore(value, dx, dy, levelBest, levelX, levelY) {
						levelBest, levelX, levelY, runScore = value, dx, dy, score
					}
				}
			}
			shiftX, shiftY = levelX, levelY
		}
		dxPx := figureRound(float64(shiftX*canvas) / float64(levels[len(levels)-1]))
		dyPx := figureRound(float64(shiftY*canvas) / float64(levels[len(levels)-1]))
		run := figureAlignmentRun{Name: start.name, Score: runScore.Mean(), DX: dxPx, DY: dyPx, Channels: runScore.Channels}
		runs = append(runs, run)
		if betterFigureScore(run.Score, shiftX, shiftY, bestScore, bestCellsX, bestCellsY) {
			bestRun, bestScore, bestCellsX, bestCellsY = run, run.Score, shiftX, shiftY
		}
	}
	return figureComparison{Similarity: bestRun.Score, Channels: bestRun.Channels, DX: bestRun.DX, DY: bestRun.DY, Runs: runs}, nil
}

func betterFigureScore(score float64, dx, dy int, best float64, bestDX, bestDY int) bool {
	if score > best+1e-12 {
		return true
	}
	if math.Abs(score-best) <= 1e-12 {
		m, bm := figureAbsInt(dx)+figureAbsInt(dy), figureAbsInt(bestDX)+figureAbsInt(bestDY)
		if m < bm || (m == bm && (dy < bestDY || (dy == bestDY && dx < bestDX))) {
			return true
		}
	}
	return false
}

func figureOverlay(reference, candidate *image.NRGBA, dx, dy int) *image.NRGBA {
	w, h := reference.Bounds().Dx(), reference.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ref := reference.NRGBAAt(x, y)
			cx, cy := x-dx, y-dy
			cand := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if image.Pt(cx, cy).In(candidate.Bounds()) {
				cand = candidate.NRGBAAt(cx, cy)
			}
			ri := [3]int{255 - int(ref.R), 255 - int(ref.G), 255 - int(ref.B)}
			ci := [3]int{255 - int(cand.R), 255 - int(cand.G), 255 - int(cand.B)}
			srcInk, candInk, shared := figureMax3Int(ri[0], ri[1], ri[2]) > 51, figureMax3Int(ci[0], ci[1], ci[2]) > 51, false
			for c := range ri {
				shared = shared || min(ri[c], ci[c]) > 51
			}
			switch {
			case shared:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			case srcInk && !candInk:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			case candInk && !srcInk:
				out.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
			default:
				out.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	return out
}

func worstFigureRegion(reference, candidate *image.NRGBA) (float64, float64, float64, float64) {
	reference, candidate = fitFigureCanvases(reference, candidate)
	bestScore := math.Inf(1)
	best := [4]float64{0, 0, 100, 100}
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			left, top := float64(col*50), float64(row*50)
			refCrop := cropFigureImage(reference, left, top, left+50, top+50)
			candCrop := cropFigureImage(candidate, left, top, left+50, top+50)
			comparison, _, err := compareFigureImages(refCrop, candCrop)
			if err == nil && comparison.Similarity < bestScore {
				bestScore = comparison.Similarity
				best = [4]float64{left, top, left + 50, top + 50}
			}
		}
	}
	return best[0], best[1], best[2], best[3]
}
func figureOverlayDetail(reference, candidate *image.NRGBA, dx, dy int, detail string) *image.NRGBA {
	reference, candidate = fitFigureCanvases(reference, candidate)
	if detail == "all" {
		panels := []*image.NRGBA{figureOverlayChannel(reference, candidate, dx, dy, "gray"), figureOverlayChannel(reference, candidate, dx, dy, "r"), figureOverlayChannel(reference, candidate, dx, dy, "g"), figureOverlayChannel(reference, candidate, dx, dy, "b")}
		return figurePanelBoard(panels)
	}
	return figureOverlayChannel(reference, candidate, dx, dy, detail)
}

func figureOverlayChannel(reference, candidate *image.NRGBA, dx, dy int, channel string) *image.NRGBA {
	w, h := reference.Bounds().Dx(), reference.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ref := reference.NRGBAAt(x, y)
			cx, cy := x-dx, y-dy
			cand := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
			if image.Pt(cx, cy).In(candidate.Bounds()) {
				cand = candidate.NRGBAAt(cx, cy)
			}
			ri, ci := figureChannelInk(ref, channel), figureChannelInk(cand, channel)
			switch {
			case ri > 51 && ci > 51:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			case ri > 51:
				out.SetNRGBA(x, y, color.NRGBA{R: 255, B: 255, A: 255})
			case ci > 51:
				out.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
			default:
				out.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	return out
}

func figureChannelInk(c color.NRGBA, channel string) int {
	switch channel {
	case "gray":
		return 255 - int(0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B))
	case "r":
		return 255 - int(c.R)
	case "g":
		return 255 - int(c.G)
	case "b":
		return 255 - int(c.B)
	case "rg":
		return max(255-int(c.R), 255-int(c.G))
	case "rb":
		return max(255-int(c.R), 255-int(c.B))
	case "gb":
		return max(255-int(c.G), 255-int(c.B))
	}
	return max(255-int(c.R), max(255-int(c.G), 255-int(c.B)))
}

func figurePanelBoard(panels []*image.NRGBA) *image.NRGBA {
	w, h := panels[0].Bounds().Dx(), panels[0].Bounds().Dy()
	board := image.NewNRGBA(image.Rect(0, 0, w*2, h*2))
	for i, p := range panels {
		x, y := (i%2)*w, (i/2)*h
		draw.Draw(board, image.Rect(x, y, x+w, y+h), p, image.Point{}, draw.Src)
	}
	return board
}

func cropFigureImage(src *image.NRGBA, left, top, right, bottom float64) *image.NRGBA {
	b := src.Bounds()
	x0 := b.Min.X + figureRound(left*float64(b.Dx())/100)
	y0 := b.Min.Y + figureRound(top*float64(b.Dy())/100)
	x1 := b.Min.X + figureRound(right*float64(b.Dx())/100)
	y1 := b.Min.Y + figureRound(bottom*float64(b.Dy())/100)
	x0 = max(b.Min.X, min(x0, b.Max.X-1))
	y0 = max(b.Min.Y, min(y0, b.Max.Y-1))
	x1 = max(x0+1, min(x1, b.Max.X))
	y1 = max(y0+1, min(y1, b.Max.Y))
	out := image.NewNRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	draw.Draw(out, out.Bounds(), src, image.Pt(x0, y0), draw.Src)
	return out
}

func figureMax3(a, b, c float64) float64 {
	return math.Max(a, math.Max(b, c))
}

func figureMax3Int(a, b, c int) int {
	return max(a, max(b, c))
}

func figureMax4(a, b, c, d int) int {
	return max(max(a, b), max(c, d))
}

func figureClampInt(v, low, high int) int {
	return max(low, min(high, v))
}

func figureRound(v float64) int {
	if v >= 0 {
		return int(math.Floor(v + 0.5))
	}
	return int(math.Ceil(v - 0.5))
}

func figureAbsInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
