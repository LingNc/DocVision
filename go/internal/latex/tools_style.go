package latex

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
	"mineru-tools/internal/session"
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
	// Subject is the current document's image subfolder (e.g.
	// "测试-概率论"); it lets a bare file name resolve without a path.
	Subject string
	// Previews returns this session's compiled figure previews (oldest
	// first). When set, the virtual names "preview.png" (newest) and
	// "preview-<n>.png" resolve to them so a compile preview can be
	// inspected with crop/zoom like any image; zooming re-renders from
	// the stored PDF at higher resolution when possible.
	Previews func() []previewEntry
}

func (t *ViewImageTool) Name() string { return "view_image" }

func (t *ViewImageTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_image",
		"description": "LOOK at an image (pixels). path is the image as it appears in the markdown (images/<subject>/foo.jpg) or just its file name (foo.jpg); it is joined to this document's image folder — a wrong name is reported as an error. In a figure session the compiled previews are also addressable as \"preview.png\" (newest) and \"preview-<n>.png\". Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection. For TEXT context around an image use image_context instead.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Image file name (foo.jpg) or the markdown ref (images/<subject>/foo.jpg); in a figure session also preview.png / preview-<n>.png."},
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
	left := pctArg(args, "left", 0)
	top := pctArg(args, "top", 0)
	right := pctArg(args, "right", 100)
	bottom := pctArg(args, "bottom", 100)
	zoom := intArg(args, "zoom", 0)
	if zoom <= 0 {
		zoom = 1280
	}

	// Compile previews keep their PDF: render the crop from the PDF at a
	// higher resolution instead of upscaling the already-rasterised PNG,
	// so small labels stay readable.
	img, rendered := t.zoomFromPDF(pathArg, left, top, right, bottom, zoom)
	if !rendered {
		img, err = decodeImage(full)
		if err != nil {
			return session.ToolResult{}, err
		}
		if left > 0 || top > 0 || right < 100 || bottom < 100 {
			img = cropPercent(img, left, top, right, bottom)
		}
		img = scaleToWidth(img, zoom)
	}
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

// previewNameRe matches the versioned compile-preview names.
var previewNameRe = regexp.MustCompile(`^preview-(\d+)\.png$`)

// previewTarget resolves a virtual preview name against the session's
// preview list. handled=false means "not a preview name".
func (t *ViewImageTool) previewTarget(rel string) (entry previewEntry, handled bool, err error) {
	if t.Previews == nil {
		return previewEntry{}, false, nil
	}
	clean := filepath.Clean(strings.TrimSpace(rel))
	if filepath.Dir(clean) != "." { // previews are top-level names only
		return previewEntry{}, false, nil
	}
	name := clean
	if name != "preview.png" && !strings.HasPrefix(name, "preview-") {
		return previewEntry{}, false, nil
	}
	list := t.Previews()
	if len(list) == 0 {
		return previewEntry{}, true, fmt.Errorf("当前会话还没有编译预览图（请先调用 compile）")
	}
	if name == "preview.png" {
		return list[len(list)-1], true, nil
	}
	m := previewNameRe.FindStringSubmatch(name)
	if m == nil {
		return previewEntry{}, true, fmt.Errorf("未知的预览名 %s；可用: preview.png（最新）、preview-1.png … preview-%d.png", rel, len(list))
	}
	n, _ := strconv.Atoi(m[1])
	if n < 1 || n > len(list) {
		return previewEntry{}, true, fmt.Errorf("预览 %s 不存在（本次会话共 %d 个：preview-1.png … preview-%d.png）", rel, len(list), len(list))
	}
	return list[n-1], true, nil
}

// resolvePreview handles the virtual preview names (compile previews of
// the current figure session). handled=false means "not a preview name,
// use the normal image resolution".
func (t *ViewImageTool) resolvePreview(rel string) (path string, handled bool, err error) {
	entry, handled, err := t.previewTarget(rel)
	if !handled || err != nil {
		return "", handled, err
	}
	return entry.png, true, nil
}

// zoomFromPDF re-renders the requested crop straight from the preview's
// PDF so zooming adds real resolution instead of upscaling pixels.
// ok=false means "not a preview / no PDF / rendering failed" — the
// caller then falls back to cropping the stored PNG.
func (t *ViewImageTool) zoomFromPDF(rel string, left, top, right, bottom float64, zoom int) (image.Image, bool) {
	entry, handled, err := t.previewTarget(rel)
	if !handled || err != nil || entry.pdf == "" {
		return nil, false
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, false
	}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	frac := (clamp(right) - clamp(left)) / 100
	if frac < 0.02 {
		frac = 1 // degenerate crop: render the whole page
	}
	pageW := int(float64(zoom)/frac) + 8
	if pageW < 400 {
		pageW = 400
	}
	if pageW > 6000 {
		pageW = 6000
	}
	dir, err := os.MkdirTemp("", "dsv-zoom-")
	if err != nil {
		return nil, false
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "page")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftoppm", "-png", "-singlefile",
		"-scale-to-x", strconv.Itoa(pageW), "-scale-to-y", "-1", entry.pdf, out)
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	img, err := decodeImage(out + ".png")
	if err != nil {
		return nil, false
	}
	if left > 0 || top > 0 || right < 100 || bottom < 100 {
		img = cropPercent(img, left, top, right, bottom)
	}
	return scaleToWidth(img, zoom), true
}

// resolve maps an image argument to a file. Resolution is a plain join
// against the document's image folder — there is NO searching: the model
// may pass the markdown ref (images/<subject>/foo.jpg) or just foo.jpg,
// and both land on <Root>/<Subject>/foo.jpg. A missing file is an error
// (wrong name), not something to hunt for.
func (t *ViewImageTool) resolve(rel string) (string, error) {
	if full, handled, err := t.resolvePreview(rel); handled {
		return full, err
	}
	clean := filepath.Clean(strings.TrimSpace(rel))
	if clean == "" || clean == "." {
		return "", fmt.Errorf("path 不能为空")
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("只允许相对路径: %s", rel)
	}
	// Drop the markdown "images/" prefix; what remains is relative to
	// the images root (either already subject-prefixed or a bare name).
	sub := strings.TrimPrefix(filepath.ToSlash(clean), "./")
	sub = strings.TrimPrefix(sub, "images/")

	var cands []string
	if t.Subject != "" {
		if strings.HasPrefix(filepath.ToSlash(sub), filepath.ToSlash(t.Subject)+"/") {
			// Already carries the document folder (full markdown ref).
			cands = append(cands, sub)
		} else {
			// Bare file name (or a path inside the document folder).
			cands = append(cands, filepath.Join(t.Subject, sub))
		}
	} else {
		// No document folder: images live at the assets root.
		cands = append(cands, sub)
	}
	// Document-root relative (style analyst passes paths under images/).
	cands = append(cands, clean)

	rootAbs, err := filepath.Abs(t.Root)
	if err != nil {
		return "", err
	}
	var escapeErr error
	for _, cand := range cands {
		full := filepath.Join(t.Root, cand)
		fullAbs, err := filepath.Abs(full)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) && fullAbs != rootAbs {
			if escapeErr == nil {
				escapeErr = fmt.Errorf("路径越界: %s", rel)
			}
			continue
		}
		if fileExists(full) {
			return full, nil
		}
	}
	if escapeErr != nil {
		return "", escapeErr
	}
	return "", fmt.Errorf("文件不存在: %s（请用图片文件名或 markdown 中的引用路径）", rel)
}

// SubmitStyleTool receives the structured style package: cls + usage
// manual + example. The captured payload is validated and persisted by
// the runner.
type SubmitStyleTool struct {
	Cls     string
	Manual  string
	Example string
	Set     bool
	// Workspace (optional) lets the args reference files previously
	// written via write_file instead of full inline contents.
	Workspace string
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
	read := func(v string) string {
		if strings.TrimSpace(v) == "" || strings.Contains(v, "\n") {
			return v
		}
		if path, err := resolveInside(t.Workspace, v); err == nil {
			if data, err := os.ReadFile(path); err == nil {
				return string(data)
			}
		}
		return v
	}
	cls = read(cls)
	manual = read(manual)
	example = read(example)
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

// WriteWorkFileTool gives an analyst a persistent VIRTUAL WORKSPACE (a
// real directory under the project): it can draft the cls, manual and
// examples incrementally without re-emitting full contents every turn.
type WriteWorkFileTool struct {
	Root string // real workspace directory
	// AnyExt allows any text extension (build-fix workspace: .bib, .bst,
	// .cfg, ...); default restricts to .cls .sty .tex .md.
	AnyExt bool
	// Prefixes, when non-empty, restricts writes to these workspace
	// paths: a path is allowed when it equals a prefix or lies under it
	// (e.g. the convert session's own chapter file + its asset folder).
	Prefixes []string
	// RejectDocumentclass refuses content containing \documentclass
	// (chapter files are \input fragments, not standalone documents).
	RejectDocumentclass bool
	// Hint is appended to the tool description (workspace layout).
	Hint string
}

func (t *WriteWorkFileTool) Name() string { return "write_file" }

func (t *WriteWorkFileTool) Definition() map[string]any {
	allowed := "Allowed extensions: .cls .sty .tex .md."
	if t.AnyExt {
		allowed = "Any text file name is allowed (.tex .cls .sty .md .bib .bst .cfg ...)."
	}
	if len(t.Prefixes) > 0 {
		allowed += " Writable paths: " + strings.Join(t.Prefixes, ", ") + " (and files under them)."
	}
	desc := "Write a file into YOUR workspace (full content replaces the file). " + allowed +
		" Keep drafts here so later edits are small diffs instead of full re-outputs."
	if t.Hint != "" {
		desc += " " + t.Hint
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "write_file",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "relative path inside your workspace, e.g. class.cls, manual.md, examples/ch1.tex"},
				"content": map[string]any{"type": "string", "description": "full file content"},
			},
			"required": []string{"path", "content"},
		},
	}}
}

var workFileExtRe = regexp.MustCompile(`(?i)\.(cls|sty|tex|md)$`)

// pathAllowed reports whether rel is one of the allowed prefixes or a
// file under them.
func (t *WriteWorkFileTool) pathAllowed(rel string) bool {
	if len(t.Prefixes) == 0 {
		return true
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	for _, p := range t.Prefixes {
		p = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(p)), "/")
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}

func (t *WriteWorkFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if strings.TrimSpace(rel) == "" {
		return session.ToolResult{Text: "REJECTED: path is required."}, nil
	}
	if !t.AnyExt && !workFileExtRe.MatchString(rel) {
		return session.ToolResult{Text: "REJECTED: only .cls .sty .tex .md files are allowed."}, nil
	}
	if m := explicitMountOf(rel); m != "" {
		if m != "work" {
			return session.ToolResult{Text: "REJECTED: mount \"" + m + "\" is read-only here; write under your own workspace (work:...)."}, nil
		}
		rel = stripMount(rel)
	}
	if !t.pathAllowed(rel) {
		return session.ToolResult{Text: "REJECTED: " + rel + " is outside your writable paths (" + strings.Join(t.Prefixes, ", ") + ")."}, nil
	}
	if t.RejectDocumentclass && strings.Contains(content, "\\documentclass") {
		return session.ToolResult{Text: "REJECTED: the chapter must be an \\input fragment — no \\documentclass / preamble."}, nil
	}
	path, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return session.ToolResult{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{Text: "WROTE " + rel + fmt.Sprintf(" (%d bytes)", len(content))}, nil
}

// ListFontsTool reports the fonts available to the LaTeX build: the
// project fonts directory (paths.fonts, AI-managed) plus system fonts
// (via fc-list when present).
type ListFontsTool struct {
	FontsDir string
}

func (t *ListFontsTool) Name() string { return "list_fonts" }

func (t *ListFontsTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "list_fonts",
		"description": "List usable fonts: files in the project fonts directory and installed system fonts. Use this before referencing a font in the cls; if a needed font is missing, name the expected substitution in the manual and report the missing font in the submit_style report (user downloads it manually into the fonts/ directory).",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *ListFontsTool) Execute(_ string) (session.ToolResult, error) {
	var b strings.Builder
	entries, _ := os.ReadDir(t.FontsDir)
	if len(entries) > 0 {
		b.WriteString("Project fonts directory (use with \\setCJKmainfont Path=... etc.):\n")
		for _, e := range entries {
			if !e.IsDir() {
				b.WriteString("  " + e.Name() + "\n")
			}
		}
	} else {
		b.WriteString("Project fonts directory is empty (the user can place font files there manually).\n")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cmd := exec.CommandContext(ctx, "fc-list", ":", "family", "file"); cmd.Run() == nil {
		out, _ := cmd.Output()
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 60 {
			lines = append(lines[:60], fmt.Sprintf("... (%d total)", len(lines)))
		}
		b.WriteString("\nSystem fonts (fc-list):\n")
		for _, l := range lines {
			b.WriteString("  " + l + "\n")
		}
	}
	return session.ToolResult{Text: b.String()}, nil
}
