package latex

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/session"
	_ "image/gif"
	_ "image/png"
	_ "golang.org/x/image/webp"
)

// ListImagesTool enumerates the extracted document images so the style
// analyst can pick which ones to inspect.
type ListImagesTool struct {
	ImagesDir string
}

func (t *ListImagesTool) Name() string { return "list_images" }

func (t *ListImagesTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "list_images",
		"description": "List the extracted document images (path relative to the images root + pixel dimensions), up to 400 entries.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *ListImagesTool) Execute(_ string) (session.ToolResult, error) {
	var b strings.Builder
	count := 0
	_ = filepath.Walk(t.ImagesDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || count >= 400 {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
			rel, _ := filepath.Rel(t.ImagesDir, p)
			w, h := imageSize(p)
			fmt.Fprintf(&b, "%s (%dx%d)\n", rel, w, h)
			count++
		}
		return nil
	})
	if count == 0 {
		return session.ToolResult{Text: "(no images found)"}, nil
	}
	return session.ToolResult{Text: b.String()}, nil
}

// ViewImageTool returns an image to the model, optionally cropping a
// percentage-defined sub-region and scaling it up (zoom inspection).
type ViewImageTool struct {
	// Root is the base directory; path arguments are resolved inside
	// it (absolute paths outside Root are rejected).
	Root string
}

func (t *ViewImageTool) Name() string { return "view_image" }

func (t *ViewImageTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_image",
		"description": "View an image (relative to the document assets root). Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Image path relative to the assets root."},
				"left":   map[string]any{"type": "number", "description": "Crop left in percent (0-100), default 0."},
				"top":    map[string]any{"type": "number", "description": "Crop top in percent (0-100), default 0."},
				"right":  map[string]any{"type": "number", "description": "Crop right in percent (0-100), default 100."},
				"bottom": map[string]any{"type": "number", "description": "Crop bottom in percent (0-100), default 100."},
				"zoom":   map[string]any{"type": "integer", "description": "Target width in pixels for the returned image (e.g. 1400 for detail view). Default: fit to 1280."},
			},
			"required": []string{"path"},
		},
	}}
}

func (t *ViewImageTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	pathArg, _ := args["path"].(string)
	full, err := t.resolve(pathArg)
	if err != nil {
		return session.ToolResult{}, err
	}
	img, err := decodeImage(full)
	if err != nil {
		return session.ToolResult{}, err
	}
	left := pctArg(args, "left", 0)
	top := pctArg(args, "top", 0)
	right := pctArg(args, "right", 100)
	bottom := pctArg(args, "bottom", 100)
	if left > 0 || top > 0 || right < 100 || bottom < 100 {
		img = cropPercent(img, left, top, right, bottom)
	}
	zoom := intArg(args, "zoom", 0)
	if zoom <= 0 {
		zoom = 1280
	}
	img = scaleToWidth(img, zoom)
	b64, err := encodeJPEGBase64(img)
	if err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{
		Text:        fmt.Sprintf("Image %s (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx) attached.", pathArg, left, top, right, bottom, zoom),
		ImageBase64: b64,
		ImageMIME:   "image/jpeg",
	}, nil
}

func (t *ViewImageTool) resolve(rel string) (string, error) {
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("只允许相对路径")
	}
	full := filepath.Join(t.Root, clean)
	rootAbs, _ := filepath.Abs(t.Root)
	fullAbs, _ := filepath.Abs(full)
	if !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) && fullAbs != rootAbs {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	if !fileExists(full) {
		return "", fmt.Errorf("文件不存在: %s", rel)
	}
	return full, nil
}

// ReadMDTool lets the style analyst read the organized Markdown in
// bounded line windows.
type ReadMDTool struct {
	Path string
}

func (t *ReadMDTool) Name() string { return "read_md" }

func (t *ReadMDTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "read_md",
		"description": "Read a window of lines (1-based, inclusive) from the organized Markdown file. Max 400 lines per call.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start_line": map[string]any{"type": "integer", "description": "First line to read (1-based)."},
				"end_line":   map[string]any{"type": "integer", "description": "Last line to read (inclusive)."},
			},
			"required": []string{"start_line", "end_line"},
		},
	}}
}

func (t *ReadMDTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	start := intArg(args, "start_line", 1)
	end := intArg(args, "end_line", start)
	return readLinesFrom(t.Path, start, end, 400)
}

// SubmitStyleTool receives the structured style package: cls + usage
// manual + example. The captured payload is validated and persisted by
// the runner.
type SubmitStyleTool struct {
	Cls     string
	Manual  string
	Example string
	Set     bool
}

func (t *SubmitStyleTool) Name() string { return "submit_style" }

func (t *SubmitStyleTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit_style",
		"description": "Submit the complete style package: the .cls file content, the structured usage manual (Markdown), and a complete compilable example .tex.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cls":     map[string]any{"type": "string", "description": "Full content of the custom .cls file."},
				"manual":  map[string]any{"type": "string", "description": "Usage manual in Markdown with sections: '## Document class options', '## Commands', '## Environments', '## Examples'."},
				"example": map[string]any{"type": "string", "description": "Complete compilable example .tex using the class."},
			},
			"required": []string{"cls", "manual", "example"},
		},
	}}
}

func (t *SubmitStyleTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	cls, _ := args["cls"].(string)
	manual, _ := args["manual"].(string)
	example, _ := args["example"].(string)
	if strings.TrimSpace(cls) == "" || strings.TrimSpace(example) == "" {
		return session.ToolResult{Text: "REJECTED: cls and example are required."}, nil
	}
	if !strings.Contains(cls, "\\ProvidesClass") {
		return session.ToolResult{Text: "REJECTED: the cls must contain \\ProvidesClass{...}."}, nil
	}
	if !strings.Contains(example, "\\documentclass") {
		return session.ToolResult{Text: "REJECTED: the example must be a complete document with \\documentclass."}, nil
	}
	t.Cls, t.Manual, t.Example, t.Set = cls, manual, example, true
	return session.ToolResult{Text: "SUBMITTED. The example will now be test-compiled; you will be told if it fails."}, nil
}

// ------------------------------------------------------------------
// small image / text helpers shared by the tools
// ------------------------------------------------------------------

func imageSize(p string) (int, int) {
	f, err := os.Open(p)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func decodeImage(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码图片失败: %w", err)
	}
	return flattenToOpaque(img), nil
}

// flattenToOpaque composites an alpha-bearing image over white.
func flattenToOpaque(src image.Image) image.Image {
	b := src.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	canvas.Set(canvas.Bounds().Min.X, canvas.Bounds().Min.Y, color.RGBA{255, 255, 255, 255})
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			canvas.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	// Only opaque pixels need overriding; drawing the source over a
	// white canvas handles alpha correctly and cheaply.
	out := image.NewRGBA(canvas.Bounds())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	copyRGBA(out, canvas)
	return out
}

// copyRGBA is a minimal blit (avoids pulling draw into every caller).
func copyRGBA(dst, src *image.RGBA) {
	for y := 0; y < src.Bounds().Dy(); y++ {
		offD := dst.PixOffset(dst.Bounds().Min.X, dst.Bounds().Min.Y+y)
		offS := src.PixOffset(src.Bounds().Min.X, src.Bounds().Min.Y+y)
		copy(dst.Pix[offD:offD+src.Bounds().Dx()*4], src.Pix[offS:offS+src.Bounds().Dx()*4])
	}
}

// cropPercent crops the image by percentage edges (0-100).
func cropPercent(img image.Image, left, top, right, bottom float64) image.Image {
	b := img.Bounds()
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	x0 := b.Min.X + int(float64(b.Dx())*clamp(left)/100)
	x1 := b.Min.X + int(float64(b.Dx())*clamp(right)/100)
	y0 := b.Min.Y + int(float64(b.Dy())*clamp(top)/100)
	y1 := b.Min.Y + int(float64(b.Dy())*clamp(bottom)/100)
	if x1 <= x0 || y1 <= y0 {
		return img
	}
	return cropToRect(img, image.Rect(x0, y0, x1, y1))
}

func cropToRect(img image.Image, r image.Rectangle) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			out.Set(x, y, img.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return out
}

// scaleToWidth resizes (nearest-neighbour via per-pixel sampling) so
// the width equals target; aspect ratio is preserved.
func scaleToWidth(img image.Image, target int) image.Image {
	b := img.Bounds()
	if target <= 0 || b.Dx() <= target {
		return img
	}
	h := b.Dy() * target / b.Dx()
	if h < 1 {
		h = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, target, h))
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*b.Dy()/h
		for x := 0; x < target; x++ {
			sx := b.Min.X + x*b.Dx()/target
			out.Set(x, y, img.At(sx, sy))
		}
	}
	return out
}

func encodeJPEGBase64(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return "", err
	}
	return b64Encode(buf.Bytes()), nil
}

func pctArg(args map[string]interface{}, key string, def float64) float64 {
	if v, ok := args[key].(float64); ok {
		return v
	}
	return def
}

func intArg(args map[string]interface{}, key string, def int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

// readLinesFrom reads a 1-based inclusive line window with bounds and
// per-line truncation, shared by the md-reading tools.
func readLinesFrom(path string, start, end, maxLines int) (session.ToolResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	lines := strings.Split(string(data), "\n")
	total := len(lines)
	if start < 1 {
		start = 1
	}
	if end > total {
		end = total
	}
	if end < start {
		return session.ToolResult{Text: fmt.Sprintf("(empty range; file has %d lines)", total)}, nil
	}
	if end-start+1 > maxLines {
		end = start + maxLines - 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[lines %d-%d of %d]\n", start, end, total)
	for i := start; i <= end; i++ {
		l := lines[i-1]
		if len(l) > 500 {
			l = l[:500] + "..."
		}
		fmt.Fprintf(&b, "%d: %s\n", i, l)
	}
	return session.ToolResult{Text: b.String()}, nil
}
