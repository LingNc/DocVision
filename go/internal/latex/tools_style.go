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
	"strings"
	"time"

	"io"
	"net/http"

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
}

func (t *ViewImageTool) Name() string { return "view_image" }

func (t *ViewImageTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_image",
		"description": "LOOK at an image (pixels). path may be a bare file name (e.g. foo.jpg) — it is resolved inside this document's image folder — or a path relative to the assets root (a leading images/ prefix is optional). Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection. For TEXT context around an image use image_context instead.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Image file name (e.g. foo.jpg) or path relative to the assets root, e.g. images/subject/foo.jpg."},
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
	clean := filepath.Clean(strings.TrimSpace(rel))
	if clean == "" || clean == "." {
		return "", fmt.Errorf("path 不能为空")
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("只允许相对路径: %s", rel)
	}
	// Accepted forms, in order:
	//   1. path as given (document root / assets root)
	//   2. without the markdown "images/" prefix (assets root = images/)
	//   3. <subject>/<path>  (bare name inside this document's folder)
	//   4. bare file name inside this document's folder
	//   5. bare file name at the assets root
	cands := []string{clean}
	if stripped := stripImagesPrefix(clean); stripped != clean {
		cands = append(cands, stripped)
	}
	base := filepath.Base(clean)
	if t.Subject != "" {
		cands = append(cands,
			filepath.Join(t.Subject, clean),
			filepath.Join(t.Subject, base),
		)
	}
	cands = append(cands, base)
	seen := map[string]bool{}
	rootAbs, err := filepath.Abs(t.Root)
	if err != nil {
		return "", err
	}
	var escapeErr error
	for _, cand := range cands {
		if seen[cand] {
			continue
		}
		seen[cand] = true
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
	// Last resort: a unique file with that name anywhere under the root
	// (bounded walk, ambiguity is reported instead of guessing).
	if matches := findByName(rootAbs, base, 2); len(matches) == 1 {
		return matches[0], nil
	} else if len(matches) > 1 {
		return "", fmt.Errorf("文件名 %s 在素材目录中有多个匹配，请带上子目录: %s", base, strings.Join(shortNames(rootAbs, matches), ", "))
	}
	if escapeErr != nil {
		return "", escapeErr
	}
	return "", fmt.Errorf("文件不存在: %s（可直接用文件名，程序会在本文档图片目录内查找；用 list_images 查看可用图片）", rel)
}

// findByName walks root (bounded by maxDepth) looking for files whose
// base name equals name.
func findByName(root, name string, maxDepth int) []string {
	var out []string
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if strings.Count(filepath.Clean(p), string(filepath.Separator))-rootDepth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(p) == name {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// shortNames renders candidate paths relative to root for messages.
func shortNames(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if rel, err := filepath.Rel(root, p); err == nil {
			out = append(out, rel)
			continue
		}
		out = append(out, p)
	}
	return out
}

// stripImagesPrefix removes a leading "images/" (the markdown-relative
// prefix) so a ref resolves against an images root as well.
func stripImagesPrefix(p string) string {
	slashed := filepath.ToSlash(p)
	if strings.HasPrefix(slashed, "images/") {
		return strings.TrimPrefix(slashed, "images/")
	}
	return p
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
}

func (t *WriteWorkFileTool) Name() string { return "write_file" }

func (t *WriteWorkFileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "write_file",
		"description": "Write a file into YOUR workspace (full content replaces the file). Allowed extensions: .cls .sty .tex .md. Keep drafts here so later edits are small diffs instead of full re-outputs.",
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
	if !workFileExtRe.MatchString(rel) {
		return session.ToolResult{Text: "REJECTED: only .cls .sty .tex .md files are allowed."}, nil
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
		"description": "List usable fonts: files in the project fonts directory (install_font can add more) and installed system fonts. Use this before referencing a font in the cls; if a needed font is missing, name the expected substitution in the manual.",
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
		b.WriteString("Project fonts directory is empty (install_font can add files).\n")
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

// InstallFontTool downloads a font file (ttf/otf) into the project
// fonts directory so the cls can reference it directly.
type InstallFontTool struct {
	FontsDir string
}

func (t *InstallFontTool) Name() string { return "install_font" }

func (t *InstallFontTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "install_font",
		"description": "Download one font file (.ttf/.otf) from a URL into the project fonts directory. Only use URLs you are confident provide the font legally (official releases, open-source fonts like SIL OFL families).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":  map[string]any{"type": "string", "description": "direct font file URL"},
				"name": map[string]any{"type": "string", "description": "file name to save as, e.g. SourceHanSerifSC-Regular.otf"},
			},
			"required": []string{"url", "name"},
		},
	}}
}

func (t *InstallFontTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	url, _ := args["url"].(string)
	name, _ := args["name"].(string)
	if strings.TrimSpace(url) == "" || strings.TrimSpace(name) == "" {
		return session.ToolResult{Text: "REJECTED: url and name are required."}, nil
	}
	if !workFileExtRe.MatchString(name) {
		return session.ToolResult{Text: "REJECTED: name must end with .ttf or .otf."}, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return session.ToolResult{Text: "REJECTED: url must be http(s)."}, nil
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return session.ToolResult{Text: "DOWNLOAD FAILED: " + err.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return session.ToolResult{Text: fmt.Sprintf("DOWNLOAD FAILED: HTTP %d", resp.StatusCode)}, nil
	}
	limited := io.LimitReader(resp.Body, 64<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return session.ToolResult{Text: "DOWNLOAD FAILED: " + err.Error()}, nil
	}
	if len(data) < 1000 {
		return session.ToolResult{Text: "REJECTED: file too small to be a font."}, nil
	}
	if err := os.MkdirAll(t.FontsDir, 0o755); err != nil {
		return session.ToolResult{}, err
	}
	dst := filepath.Join(t.FontsDir, filepath.Base(name))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{Text: "INSTALLED " + dst + fmt.Sprintf(" (%d bytes). Reference it in the cls with its file name.", len(data))}, nil
}
