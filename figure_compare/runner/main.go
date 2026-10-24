package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

type harness struct {
	root, original, script, python string
	contextText                    string
	compileOK                      bool
	lastPDF                        string
	hasComparison                  bool
	lastScore                      float64
	compareRound                   int
}

type tool struct {
	h    *harness
	name string
}

func schema(name, description string, props map[string]any, required []string) map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": name, "description": description,
		"parameters": map[string]any{"type": "object", "properties": props, "required": required},
	}}
}
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func (t *tool) Name() string { return t.name }
func (t *tool) Definition() map[string]any {
	switch t.name {
	case "write_file":
		return schema(t.name, "Write text to a workspace file.", map[string]any{"path": strProp("Workspace-relative path"), "content": strProp("Full file contents")}, []string{"path", "content"})
	case "edit_file":
		return schema(t.name, "Replace exact text in a workspace file.", map[string]any{"path": strProp("Workspace-relative path"), "old": strProp("Exact text to replace"), "new": strProp("Replacement text")}, []string{"path", "old", "new"})
	case "read_file":
		return schema(t.name, "Read a workspace text file.", map[string]any{"path": strProp("Workspace-relative path")}, []string{"path"})
	case "grep":
		return schema(t.name, "Search workspace text files for a literal phrase.", map[string]any{"query": strProp("Literal search text")}, []string{"query"})
	case "compile":
		return schema(t.name, "Compile figure.tex with XeLaTeX. On success the PDF is standalone.pdf; inspect that PDF with view_pdf, then compare with compare_figure.", map[string]any{"path": strProp("Workspace-relative TikZ body file; defaults to figure.tex")}, nil)
	case "view_pdf":
		return schema(t.name, "Render one PDF page as an attached image before comparison. Supports crop percentages and target pixel width.", map[string]any{"path": strProp("Workspace-relative PDF path, usually standalone.pdf"), "page": map[string]any{"type": "integer", "description": "1-based page number"}, "left": map[string]any{"type": "number", "description": "crop left percent"}, "top": map[string]any{"type": "number", "description": "crop top percent"}, "right": map[string]any{"type": "number", "description": "crop right percent"}, "bottom": map[string]any{"type": "number", "description": "crop bottom percent"}, "zoom": map[string]any{"type": "integer", "description": "target output width in pixels"}}, []string{"path"})
	case "compare_figure":
		return schema(t.name, "Compare reference and compiled figure; reports weakest region and exact crop percentages for targeted re-check.", map[string]any{"pdf_path": strProp("Workspace-relative compiled PDF path (default standalone.pdf)"), "detail": map[string]any{"type": "string", "enum": []string{"all", "gray", "r", "g", "b", "rg", "rb", "gb"}, "description": "2x2 board or a full-resolution channel combination"}, "left": map[string]any{"type": "number", "description": "crop left percent 0-100"}, "top": map[string]any{"type": "number", "description": "crop top percent 0-100"}, "right": map[string]any{"type": "number", "description": "crop right percent 0-100"}, "bottom": map[string]any{"type": "number", "description": "crop bottom percent 0-100"}, "zoom": map[string]any{"type": "integer", "description": "target pixel width for region; 0 keeps native size"}}, nil)
	case "view_image":
		return schema(t.name, "View an original or workspace image at full resolution.", map[string]any{"path": strProp("original.jpg or workspace-relative image path")}, []string{"path"})
	case "image_context":
		return schema(t.name, "Return adjacent image references and surrounding source text when a source Markdown file is supplied.", map[string]any{}, nil)
	case "submit":
		return schema(t.name, "Submit the compiled figure after visual review. Requires a successful compile and comparison score >= 50%.", map[string]any{"path": strProp("Workspace-relative TikZ body file; defaults to figure.tex")}, nil)
	}
	return schema(t.name, "", map[string]any{}, nil)
}

func parseArgs(raw string) (map[string]any, error) {
	var m map[string]any
	err := json.Unmarshal([]byte(raw), &m)
	return m, err
}
func arg(m map[string]any, key, def string) string {
	if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return def
}
func (h *harness) inside(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", errors.New("absolute paths are not allowed")
	}
	full := filepath.Join(h.root, filepath.Clean(path))
	rel, err := filepath.Rel(h.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("path escapes workspace")
	}
	return full, nil
}
func (t *tool) Execute(raw string) (session.ToolResult, error) {
	args, err := parseArgs(raw)
	if err != nil {
		return session.ToolResult{}, err
	}
	switch t.name {
	case "write_file":
		p, err := t.h.inside(arg(args, "path", ""))
		if err != nil {
			return session.ToolResult{}, err
		}
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return session.ToolResult{}, err
		}
		body, _ := args["content"].(string)
		if err = os.WriteFile(p, []byte(body), 0644); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "Wrote " + filepath.ToSlash(filepath.Clean(arg(args, "path", "")))}, nil
	case "edit_file":
		p, err := t.h.inside(arg(args, "path", ""))
		if err != nil {
			return session.ToolResult{}, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return session.ToolResult{}, err
		}
		old, _ := args["old"].(string)
		next, _ := args["new"].(string)
		if old == "" || !bytes.Contains(b, []byte(old)) {
			return session.ToolResult{Text: "NOT FOUND: old text"}, nil
		}
		b = bytes.Replace(b, []byte(old), []byte(next), 1)
		if err = os.WriteFile(p, b, 0644); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "Updated " + filepath.ToSlash(arg(args, "path", ""))}, nil
	case "read_file":
		p, err := t.h.inside(arg(args, "path", ""))
		if err != nil {
			return session.ToolResult{}, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return session.ToolResult{}, err
		}
		if len(b) > 30000 {
			b = b[:30000]
		}
		return session.ToolResult{Text: string(b)}, nil
	case "grep":
		q := arg(args, "query", "")
		var out []string
		_ = filepath.WalkDir(t.h.root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			if d.IsDir() {
				if p != t.h.root && (d.Name() == "media" || d.Name() == "compare") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() {
				b, er := os.ReadFile(p)
				if er == nil && bytes.Contains(b, []byte(q)) {
					out = append(out, filepath.Base(p)+": "+q)
					if len(out) >= 30 {
						return errors.New("limit")
					}
				}
			}
			return nil
		})
		return session.ToolResult{Text: strings.Join(out, "\n")}, nil
	case "compile":
		return t.compile(arg(args, "path", "figure.tex"))
	case "view_pdf":
		return t.viewPDF(args)
	case "compare_figure":
		return t.compare(args)
	case "view_image":
		name := arg(args, "path", "")
		var p string
		if name == "original" || name == "original.jpg" {
			p = t.h.original
		} else {
			p, err = t.h.inside(name)
			if err != nil {
				return session.ToolResult{}, err
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return session.ToolResult{}, err
		}
		mime := "image/png"
		if strings.HasSuffix(strings.ToLower(p), ".jpg") || strings.HasSuffix(strings.ToLower(p), ".jpeg") {
			mime = "image/jpeg"
		}
		return session.ToolResult{Text: "Image attached: " + name, ImageBase64: base64.StdEncoding.EncodeToString(b), ImageMIME: mime}, nil
	case "image_context":
		if t.h.contextText != "" {
			return session.ToolResult{Text: t.h.contextText}, nil
		}
		return session.ToolResult{Text: "No neighboring image references; this is a standalone figure."}, nil
	case "submit":
		if !t.h.compileOK {
			return session.ToolResult{Text: "REJECTED: compile the latest figure.tex successfully first."}, nil
		}
		if !t.h.hasComparison {
			return session.ToolResult{Text: "REJECTED: run compare_figure after the latest compile."}, nil
		}
		if t.h.lastScore < 50 {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: similarity %.2f%% is below 50%%; revise the figure and compare again.", t.h.lastScore)}, nil
		}
		p, err := t.h.inside(arg(args, "path", "figure.tex"))
		if err != nil {
			return session.ToolResult{}, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return session.ToolResult{}, err
		}
		if err = os.WriteFile(filepath.Join(t.h.root, "submitted.tex"), b, 0644); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. Similarity %.2f%% >= 50%%. See submitted.tex and compare/.", t.h.lastScore), Terminal: true}, nil
	}
	return session.ToolResult{}, fmt.Errorf("unknown tool %q", t.name)
}

func (t *tool) viewPDF(args map[string]any) (session.ToolResult, error) {
	path := arg(args, "path", "standalone.pdf")
	pdf, err := t.h.inside(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	if !strings.HasSuffix(strings.ToLower(pdf), ".pdf") {
		return session.ToolResult{Text: "REJECTED: view_pdf only accepts .pdf"}, nil
	}
	page, zoom := 1, 1280
	if v, ok := args["page"].(float64); ok && int(v) > 0 {
		page = int(v)
	}
	if v, ok := args["zoom"].(float64); ok && int(v) > 0 {
		zoom = int(v)
	}
	if zoom > 6000 {
		zoom = 6000
	}
	left, top, right, bottom := numberArg(args, "left", 0), numberArg(args, "top", 0), numberArg(args, "right", 100), numberArg(args, "bottom", 100)
	td, err := os.MkdirTemp("", "figure-view-")
	if err != nil {
		return session.ToolResult{}, err
	}
	defer os.RemoveAll(td)
	prefix := filepath.Join(td, "page")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftocairo", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", "-png", "-scale-to-x", strconv.Itoa(zoom), "-scale-to-y", "-1", pdf, prefix)
	if out, e := cmd.CombinedOutput(); e != nil {
		return session.ToolResult{Text: "VIEW PDF ERROR: " + tail(string(out), 2000)}, nil
	}
	f, e := os.Open(prefix + ".png")
	if e != nil {
		return session.ToolResult{}, e
	}
	img, e := png.Decode(f)
	f.Close()
	if e != nil {
		return session.ToolResult{}, e
	}
	img = cropImage(img, left, top, right, bottom)
	var buf bytes.Buffer
	if e = png.Encode(&buf, img); e != nil {
		return session.ToolResult{}, e
	}
	return session.ToolResult{Text: fmt.Sprintf("PDF page %s p%d (crop %.0f%%,%.0f%%-%.0f%%, width %dpx) attached.", path, page, left, top, right, img.Bounds().Dx()), ImageBase64: base64.StdEncoding.EncodeToString(buf.Bytes()), ImageMIME: "image/png"}, nil
}

func numberArg(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}
func cropImage(src image.Image, left, top, right, bottom float64) image.Image {
	b := src.Bounds()
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}
	x0 := b.Min.X + int(clamp(left)*float64(b.Dx())/100)
	y0 := b.Min.Y + int(clamp(top)*float64(b.Dy())/100)
	x1 := b.Min.X + int(clamp(right)*float64(b.Dx())/100)
	y1 := b.Min.Y + int(clamp(bottom)*float64(b.Dy())/100)
	if x1 <= x0 || y1 <= y0 {
		return src
	}
	if sub, ok := src.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(image.Rect(x0, y0, x1, y1))
	}
	return src
}
func (t *tool) compile(path string) (session.ToolResult, error) {
	p, err := t.h.inside(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	body, err := os.ReadFile(p)
	if err != nil {
		return session.ToolResult{Text: "NOT FOUND: " + path}, nil
	}
	re := regexp.MustCompile(`(?m)^\\(usetikzlibrary|usepgfplotslibrary|usepackage|tikzset|pgfplotsset)\b[^\n]*`)
	hoisted := re.FindAll(body, -1)
	rest := strings.TrimSpace(string(re.ReplaceAll(body, nil)))
	var src strings.Builder
	src.WriteString("\\documentclass[border=0pt]{standalone}\n\\usepackage{tikz}\n\\usepackage{pgfplots}\n\\pgfplotsset{compat=1.18}\n\\usepackage{amsmath,amssymb}\n")
	for _, line := range hoisted {
		src.Write(line)
		src.WriteByte('\n')
	}
	src.WriteString("\\begin{document}\n")
	src.WriteString(rest)
	src.WriteString("\n\\end{document}\n")
	tex := filepath.Join(t.h.root, "standalone.tex")
	if err = os.WriteFile(tex, []byte(src.String()), 0644); err != nil {
		return session.ToolResult{}, err
	}
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", "-halt-on-error", "-file-line-error", "standalone.tex")
	cmd.Dir = t.h.root
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.h.compileOK = false
		return session.ToolResult{Text: "COMPILE ERROR\n" + tail(string(out), 5000)}, nil
	}
	pdf := filepath.Join(t.h.root, "standalone.pdf")
	if _, err = os.Stat(pdf); err != nil {
		return session.ToolResult{Text: "COMPILE ERROR: PDF not produced\n" + tail(string(out), 3000)}, nil
	}
	t.h.compileOK = true
	t.h.lastPDF = pdf
	t.h.hasComparison = false
	info, _ := exec.Command("pdfinfo", pdf).Output()
	return session.ToolResult{Text: "COMPILE OK: standalone.pdf\n" + strings.TrimSpace(string(info)) + "\nReview the candidate with view_pdf {path:\"standalone.pdf\", page:1}, then compare with compare_figure."}, nil
}
func (t *tool) compare(args map[string]any) (session.ToolResult, error) {
	if !t.h.compileOK {
		return session.ToolResult{Text: "Compile successfully before comparing."}, nil
	}
	path := arg(args, "pdf_path", "standalone.pdf")
	detail := arg(args, "detail", "all")
	pdf, err := t.h.inside(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.h.compareRound++
	outDir := filepath.Join(t.h.root, "compare", fmt.Sprintf("round_%02d", t.h.compareRound))
	if err = os.MkdirAll(outDir, 0755); err != nil {
		return session.ToolResult{}, err
	}
	cmd := exec.Command(t.h.python, t.h.script, "--original", t.h.original, "--pdf", pdf, "--out", outDir, "--detail", detail, "--left", fmt.Sprint(numberArg(args, "left", 0)), "--top", fmt.Sprint(numberArg(args, "top", 0)), "--right", fmt.Sprint(numberArg(args, "right", 100)), "--bottom", fmt.Sprint(numberArg(args, "bottom", 100)), "--width", fmt.Sprint(int(numberArg(args, "zoom", 0))))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return session.ToolResult{Text: "COMPARE ERROR\n" + tail(string(out), 3000)}, nil
	}
	var result struct {
		Metrics struct {
			Similarity float64            `json:"similarity_percent"`
			Pass       bool               `json:"pass"`
			DX         map[string]float64 `json:"translation_percent"`
			Pixels     map[string]int     `json:"translation_px"`
			Worst      map[string]any     `json:"worst_region"`
		} `json:"metrics"`
		Selected string `json:"selected_image"`
	}
	if err = json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		return session.ToolResult{Text: "Could not parse comparison result: " + tail(string(out), 1000)}, nil
	}
	t.h.lastScore = result.Metrics.Similarity
	t.h.hasComparison = true
	img, err := os.ReadFile(result.Selected)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := filepath.Rel(t.h.root, result.Selected)
	text := fmt.Sprintf("Similarity: %.2f%% (foreground-stroke IoU; pass threshold 50%%: %v). Best center translation: dx=%d px (%.3f%% width), dy=%d px (%.3f%% height). Search levels: ±10%% → ±5%% → ±2%%. Worst region: left=%v%%, top=%v%%, right=%v%%, bottom=%v%%, share=%v%%. Re-run compare_figure with these percentages and zoom 2-8 to inspect the weakest region. Image: %s. Full-resolution panels: compare/round_%02d/gray.png, r.png, g.png, b.png.", result.Metrics.Similarity, result.Metrics.Pass, result.Metrics.Pixels["x"], result.Metrics.DX["x"], result.Metrics.Pixels["y"], result.Metrics.DX["y"], result.Metrics.Worst["left"], result.Metrics.Worst["top"], result.Metrics.Worst["right"], result.Metrics.Worst["bottom"], result.Metrics.Worst["share_percent"], filepath.ToSlash(rel), t.h.compareRound)
	return session.ToolResult{Text: text, ImageBase64: base64.StdEncoding.EncodeToString(img), ImageMIME: "image/png"}, nil
}
func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func main() {
	configFile := flag.String("config", "config.yaml", "path to the project YAML configuration")
	original := flag.String("original", "", "path to the reference image (required)")
	out := flag.String("out", "work/figure-compare-run", "run output directory")
	transcriptPath := flag.String("transcript", "", "JSONL transcript path; defaults to <out>/session.jsonl")
	modelName := flag.String("model", "", "model alias or model id; defaults to latex.drawing_model")
	contextText := flag.String("context", "", "surrounding document context for this figure session")
	compareScript := flag.String("compare-script", "algorithms/compare.py", "path to compare.py")
	python := flag.String("python", "python3", "Python executable used by compare.py")
	flag.Parse()
	if strings.TrimSpace(*original) == "" {
		flag.Usage()
		fatal(errors.New("-original is required"))
	}
	root, err := filepath.Abs(*out)
	fatal(err)
	err = os.MkdirAll(root, 0755)
	fatal(err)
	cfg, err := config.LoadConfig(*configFile)
	fatal(err)
	resolvedModel := *modelName
	if resolvedModel == "" {
		resolvedModel = cfg.Latex.DrawingModel
	}
	model, ok := cfg.ResolveModel(resolvedModel)
	if !ok {
		fatal(fmt.Errorf("drawing model %q is not configured", resolvedModel))
	}
	log, err := logger.NewLogger(filepath.Join(root, "session.log"), filepath.Join(root, "session.err.log"), 2)
	fatal(err)
	defer log.Close()
	log.SetQuiet(true)
	if *transcriptPath == "" {
		*transcriptPath = filepath.Join(root, "session.jsonl")
	}
	h := &harness{root: root, original: *original, script: *compareScript, python: *python, contextText: *contextText}
	names := []string{"write_file", "edit_file", "read_file", "grep", "compile", "view_pdf", "compare_figure", "view_image", "image_context", "submit"}
	tools := make([]session.Tool, 0, len(names))
	for _, n := range names {
		tools = append(tools, &tool{h: h, name: n})
	}
	system := prompts.Render(prompts.FigureSystem, map[string]string{"MAX_ROUNDS": "24", "OUTPUT_LANG": "English"})
	system = strings.ReplaceAll(system, "add left/top/right/bottom (percent) and zoom (target pixel width) to re-render a region straight from the PDF at high resolution, so small labels, arrows and overlaps are legible.", "The tool rasterizes to the exact original pixel canvas, searches three centered translation levels (10%, 5%, 2%), and returns a 2x2 grayscale/R/G/B overlay with a foreground-stroke IoU similarity score. Use detail=gray/r/g/b to inspect one panel at full resolution.")
	system = strings.ReplaceAll(system, "Compare what you see in view_pdf with the ORIGINAL image (view_image). Fix structure, geometry, label positions and proportions; compile again.", "Inspect the actual candidate PDF with view_pdf, then inspect compare_figure overlays and the ORIGINAL image (view_image). Fix structure, geometry, label positions and proportions; compile, view_pdf and compare again. The experimental acceptance gate is similarity >= 50%; submit is rejected below that score.")
	system = strings.ReplaceAll(system, "and view_pdf report the actual numbers in mm, the same unit as the original's measured size", "and compare_figure reports raster dimensions")
	system += "\n\nFor this isolated experiment, use compare_figure after every successful compile. Its similarity is foreground-stroke intersection-over-union after translation search; do not treat it as semantic correctness. Pass only at >=50%, and inspect each channel before submit."
	ctx := *contextText
	if strings.TrimSpace(ctx) == "" {
		ctx = "Standalone reference figure. Preserve the visible structure, geometry, labels, colors, and aspect ratio of the supplied reference image."
	}
	user := prompts.Render(prompts.FigureUser, map[string]string{"CONTEXT": ctx, "ORIGINAL_SIZE": "Match the supplied reference image aspect ratio and do not enlarge it to page size.", "VIEW_BUDGET": "View budget: 24 looks at the original image; use compare_figure for registered channel comparison."})
	transcript, err := session.NewTranscript(*transcriptPath)
	fatal(err)
	defer transcript.Close()
	sess := session.NewSession(session.NewClient(model), model, cfg.LatexSession("drawing"), system, tools, log, 1, "figure-compare-prototype")
	sess.SetTranscript(transcript)
	img, err := os.ReadFile(*original)
	fatal(err)
	_, err = sess.Run(session.RunOptions{UserText: user, Images: []string{base64.StdEncoding.EncodeToString(img)}})
	fatal(err)
	if !h.hasComparison {
		fmt.Fprintln(os.Stderr, "session ended without a compare_figure result")
	}
	fmt.Printf("transcript=%s\nmodel=%s\ncomparison_score=%.2f%%\nsubmitted=%v\n", *transcriptPath, model.Model, h.lastScore, func() bool { _, e := os.Stat(filepath.Join(root, "submitted.tex")); return e == nil }())
}
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
