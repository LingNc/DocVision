package latex

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/png"
	"mineru-tools/internal/session"
)

// ViewImageTool returns an image to the model, optionally cropping a
// percentage-defined sub-region and scaling it up (zoom inspection).
type ViewImageTool struct {
	// Root is the base directory; path arguments are resolved inside
	// it (absolute paths outside Root are rejected).
	Root string
	// Subject is the current document's image subfolder (e.g.
	// "测试-概率论"); it lets a bare file name resolve without a path.
	Subject string
	// SoftMax / WarnRatio implement the view budget for ONE image file:
	// from ceil(SoftMax*WarnRatio) looks at that file on, the result
	// carries a short reminder of how many views are left, and past
	// SoftMax it says the budget is spent. It NEVER blocks the call — the
	// model decides. Configured by tools.view.image_max / warn_ratio.
	// Counting per image (not per session) keeps a session that has to
	// inspect many figures from being starved by one stubborn bitmap.
	// SoftMax 0 = no budget.
	SoftMax   int
	WarnRatio float64
	views     map[string]int  // resolved image path -> look count
	warned    map[string]bool // already reminded about this image
	// Measure, when set, adds the ORIGINAL figure's printed size (mm, px,
	// effective dpi) to every result — the model otherwise has no idea how
	// big the figure is on the page and draws it page-sized.
	Measure func() string
}

// viewBudgetNote renders the budget reminder appended to a view result.
// The budget is soft: the call is never blocked, the model is just told how
// many views it has left. `label` identifies the counted subject (the tool
// name plus the image file / PDF page it applies to); softMax 0 means no
// budget.
func viewBudgetNote(label string, used, softMax int, ratio float64) string {
	if softMax <= 0 {
		return ""
	}
	warnFrom := int(math.Ceil(float64(softMax) * ratio))
	if warnFrom < 1 {
		warnFrom = 1
	}
	switch {
	case used > softMax:
		return fmt.Sprintf(" VIEW BUDGET SPENT (%s: %d/%d).", label, used, softMax)
	case used >= warnFrom:
		return fmt.Sprintf(" VIEW BUDGET: %s used %d/%d, %d left.", label, used, softMax, softMax-used)
	default:
		return ""
	}
}

// viewBudgetNoteOnce is viewBudgetNote limited to ONE reminder per counted
// object: repeating "only N left" on every single look was pure noise that
// pushed the model away from looking at all (the budget is soft anyway).
func viewBudgetNoteOnce(warned map[string]bool, key, label string, used, softMax int, ratio float64) string {
	if softMax <= 0 || used <= softMax {
		if warned[key] {
			return ""
		}
		if note := viewBudgetNote(label, used, softMax, ratio); note != "" {
			warned[key] = true
			return note
		}
		return ""
	}
	return viewBudgetNote(label, used, softMax, ratio)
}

// budgetOnce appends the (soft) view-budget reminder at most once per image.
func (t *ViewImageTool) budgetOnce(full string, used int) string {
	if t.warned == nil {
		t.warned = map[string]bool{}
	}
	return viewBudgetNoteOnce(t.warned, full, "view_image on "+filepath.Base(full), used, t.SoftMax, t.WarnRatio)
}

// measureNote reports the original figure's printed size. Callers with a
// pre-computed value supply Measure; otherwise it is measured from the
// resolved file (the MinerU parse of its part directory is looked up).
func (t *ViewImageTool) measureNote(full string) string {
	if t.Measure != nil {
		if m := t.Measure(); m != "" {
			return " " + m
		}
		return ""
	}
	if m := measureHint(full); m != "" {
		return " " + m
	}
	return ""
}

// cropNote reports the printed size of the CROPPED region (mm): the crop is
// taken in bitmap pixels, and the mm-per-pixel scale is known from the
// original measurement, so "how big is what I am looking at now" is a
// division — the model needs it to size a redrawn detail.
func (t *ViewImageTool) cropNote(full string, left, top, right, bottom float64, cropped bool) string {
	if !cropped {
		return ""
	}
	m := MeasureImage(full)
	if !m.Found {
		return ""
	}
	if h := m.Crop(left, top, right, bottom).CropHint(); h != "" {
		return " " + h + "."
	}
	return ""
}

func (t *ViewImageTool) Name() string { return "view_image" }

func (t *ViewImageTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_image",
		"description": "LOOK at an image (pixels). path is the image as it appears in the markdown (images/<subject>/foo.jpg) or just its file name (foo.jpg); it is joined to this document's image folder — a wrong name is reported as an error. Compiled PDFs are NOT images: inspect them with view_pdf. Optionally crop a region by percentages (left/top/right/bottom, 0-100) and scale it up for detail inspection. For TEXT context around an image use image_context instead.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Image file name (foo.jpg) or the markdown ref (images/<subject>/foo.jpg)."},
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
	// Same ceiling as view_pdf: an unbounded zoom on a small bitmap only
	// produces a bigger blurry picture and burns tokens.
	if zoom > 6000 {
		zoom = 6000
	}
	if t.views == nil {
		t.views = map[string]int{}
	}
	t.views[full]++
	used := t.views[full]

	img, err := decodeImage(full)
	if err != nil {
		return session.ToolResult{}, err
	}
	if left > 0 || top > 0 || right < 100 || bottom < 100 {
		img = cropPercent(img, left, top, right, bottom)
	}
	img = scaleToWidth(img, zoom)
	b64, err := encodeJPEGBase64(img)
	if err != nil {
		return session.ToolResult{}, err
	}
	cropped := left > 0 || top > 0 || right < 100 || bottom < 100
	return session.ToolResult{
		Text: "Image " + fmt.Sprintf("%s (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx) attached.", pathArg, left, top, right, bottom, zoom) +
			t.measureNote(full) +
			t.cropNote(full, left, top, right, bottom, cropped) +
			t.budgetOnce(full, used),
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
	return "", fmt.Errorf("文件不存在: %s（%s）", rel, t.missingHint(rel))
}

// missingHint tells the model what the tool DID see around the missing
// path. A real style session asked for `images/测试-概率论/<sha>.jpg` and
// got only "文件不存在"，于是它以为"这个路径里没有图片"而放弃看图；实际上
// 是项目 source 树里根本没铺图片（见 linkProjectImages）。给出目录里真实
// 存在的名字，模型下一轮就能自己纠正，而不是反复猜。
func (t *ViewImageTool) missingHint(rel string) string {
	root := t.Root
	if t.Subject != "" {
		root = filepath.Join(t.Root, t.Subject)
	}
	if !pathExists(root) {
		return "图片根目录不存在: " + root
	}
	var dirs, files []string
	if ents, err := os.ReadDir(root); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			} else {
				files = append(files, e.Name())
			}
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	parts := []string{}
	if len(dirs) > 0 {
		parts = append(parts, "子目录 "+strings.Join(cap8(dirs), ", "))
	}
	if len(files) > 0 {
		parts = append(parts, "文件 "+strings.Join(cap8(files), ", "))
	}
	head := "目录 " + root + " 下没有这个文件"
	if len(parts) > 0 {
		head += "；实际有: " + strings.Join(parts, "；")
	}
	// 同名的文件在某个子目录里 → 直接把完整引用写出来。
	if base := filepath.Base(filepath.ToSlash(strings.TrimSpace(rel))); base != "" && base != "." {
		var hits []string
		for _, d := range dirs {
			if fileExists(filepath.Join(root, d, base)) {
				hits = append(hits, filepath.ToSlash(filepath.Join(t.Subject, d, base)))
			}
		}
		if len(hits) == 1 {
			return head + "；你要找的 " + base + " 在 " + hits[0] + "（直接把这个路径给我即可）"
		}
		if len(hits) > 1 {
			return head + "；同名文件有多个: " + strings.Join(hits, ", ") + "（请给完整路径）"
		}
	}
	return head
}

// cap8 keeps a hint short; the model only needs a couple of real names.
func cap8(in []string) []string {
	if len(in) <= 8 {
		return in
	}
	return append(in[:8], "…")
}

// SubmitStyleTool receives the style package: which FILES the session
// wants to submit, by workspace path, plus an optional report on
// missing fonts/limits. The captured payload is validated and persisted
// by the runner.
//
// Submitting by PATH (not by pasting contents) is deliberate: the three
// artifacts are already written files, so inline submission made the
// model re-read and re-emit ~47KB (cls 18KB + example 17KB + manual
// 11KB) that the process already had on disk — and it silently dropped
// everything else the session had produced.
type SubmitStyleTool struct {
	Cls     string
	Manual  string
	Example string
	// Extra holds additional submitted files: workspace path -> file
	// content (fonts tables, .sty helpers, TikZ style files, samples…).
	Extra map[string]string
	// Reported is the optional report text (missing fonts, known limits).
	Reported string
	Set      bool
	// Workspace lets the args reference files previously written via
	// write_file instead of full inline contents.
	Workspace string
}

func (t *SubmitStyleTool) Name() string { return "submit_style" }

func (t *SubmitStyleTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit_style",
		"description": "Submit the style package by FILE PATH (not by pasting contents): cls, manual, example, " +
			"plus any extra files your class needs. The files are read from your workspace as they are on disk.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cls":     map[string]any{"type": "string", "description": "Workspace path of the class file, e.g. \"mybook.cls\"."},
				"manual":  map[string]any{"type": "string", "description": "Workspace path of the usage manual (Markdown, with '## Document class options', '## Commands', '## Environments', '## Examples', '## Vector figure style')."},
				"example": map[string]any{"type": "string", "description": "Workspace path of a complete compilable example .tex."},
				"extra": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional extra workspace paths to ship with the class (helper .sty, TikZ style files, fonts tables, sample pages). Their relative paths are preserved.",
				},
				"report": map[string]any{"type": "string", "description": "Optional report: missing fonts/characters, things the class cannot express, notes for the conversion sessions."},
			},
			"required": []string{"cls", "manual", "example"},
		},
	}}
}

// readSubmitted resolves one argument: ALWAYS a workspace path.
//
// There is no inline-content fallback any more (用户: "很多工具就别兼容了，
// 比如那个内联兼容给谁兼容看的？ai 又不知道以前工具啥样"). It also removed
// a real failure mode: the old heuristic had to guess "path or content",
// and a one-line inline value like "## Vector figure style" or a short cls
// begun with "%" was ambiguous — a wrong guess either wrote a filename as
// the class file or rejected a legitimate submission.
func (t *SubmitStyleTool) readSubmitted(v string) (string, string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", "", nil
	}
	clean := strings.TrimPrefix(filepath.ToSlash(v), "./")
	clean = strings.TrimPrefix(clean, "work:")
	if strings.Contains(clean, "\n") {
		return "", "", fmt.Errorf("submit_style 只收工作区路径，不收文件内容：先用 write_file 写好，再提交路径（例如 {\"cls\": \"gailvbook.cls\"}）")
	}
	path, err := resolveInside(t.Workspace, clean)
	if err != nil {
		return "", "", err
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		return "", "", fmt.Errorf("找不到提交的文件 %q（用 write_file 先写好，再按工作区相对路径提交；注意路径相对工作区）", v)
	}
	return string(data), clean, nil
}

func (t *SubmitStyleTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rawCls, _ := args["cls"].(string)
	rawManual, _ := args["manual"].(string)
	rawExample, _ := args["example"].(string)
	cls, clsPath, err := t.readSubmitted(rawCls)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	manual, _, err := t.readSubmitted(rawManual)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	example, _, err := t.readSubmitted(rawExample)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	if strings.TrimSpace(cls) == "" || strings.TrimSpace(example) == "" {
		return session.ToolResult{Text: "REJECTED: cls and example are required (submit their workspace paths)."}, nil
	}
	if !strings.Contains(cls, "\\ProvidesClass") {
		return session.ToolResult{Text: "REJECTED: the cls must contain \\ProvidesClass{...}."}, nil
	}
	if !strings.Contains(example, "\\documentclass") {
		return session.ToolResult{Text: "REJECTED: the example must be a complete document with \\documentclass."}, nil
	}
	// 额外文件：按工作区相对路径原样带走（保留层级）。
	extra := map[string]string{}
	if list, ok := args["extra"].([]any); ok {
		for _, item := range list {
			name, _ := item.(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			clean := strings.TrimPrefix(filepath.ToSlash(name), "./")
			clean = strings.TrimPrefix(clean, "work:")
			full, rerr := resolveInside(t.Workspace, clean)
			if rerr != nil {
				return session.ToolResult{Text: "REJECTED: extra file " + clean + ": " + rerr.Error()}, nil
			}
			data, rerr := os.ReadFile(full)
			if rerr != nil {
				return session.ToolResult{Text: "REJECTED: extra file " + clean + " does not exist in your workspace."}, nil
			}
			if clean == clsPath {
				continue
			}
			extra[clean] = string(data)
		}
	}
	report, _ := args["report"].(string)
	t.Cls, t.Manual, t.Example, t.Extra, t.Reported, t.Set = cls, manual, example, extra, strings.TrimSpace(report), true
	msg := "SUBMITTED. The example will now be test-compiled; failures come back to this session."
	if len(extra) > 0 {
		msg += fmt.Sprintf(" %d extra file(s) shipped with the class.", len(extra))
	}
	return session.ToolResult{Text: msg}, nil
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
