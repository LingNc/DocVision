package latex

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/img2text"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// ImagesOptions drives the level-2 image pipeline.
type ImagesOptions struct {
	TestMode bool
	Number   int
	Seed     string
	// Step filters the phases: "" (all), "classify", "process".
	Step string
	// SourceDir overrides paths.output_dir (defaults to it).
	SourceDir string
	// OutDir overrides paths.latex.output_dir (used by the level-1
	// book pipeline to place processed source inside its project dir).
	OutDir string
	// Files selects specific markdown files (base names with or
	// without .md, or paths). Empty = every *.md in SourceDir (batch).
	Files []string
	// Verbose keeps per-image console output. Default (false) shows a
	// compact img2text-style progress line instead; details always go
	// to the log file.
	Verbose bool
}

// filterFiles keeps only the md files matching opts.Files (base-name
// match, with or without extension). Empty selection = no filtering.
func filterFiles(mdFiles []string, selected []string) []string {
	if len(selected) == 0 {
		return mdFiles
	}
	want := map[string]bool{}
	for _, f := range selected {
		f = filepath.Clean(f)
		want[f] = true
		want[strings.TrimSuffix(f, filepath.Ext(f))] = true
		want[filepath.Base(f)] = true
		want[strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))] = true
	}
	var out []string
	for _, f := range mdFiles {
		base := filepath.Base(f)
		if want[base] || want[strings.TrimSuffix(base, filepath.Ext(base))] || want[f] {
			out = append(out, f)
		}
	}
	return out
}

// imageProgress is the per-image persisted state (断点续传).
type imageProgress struct {
	Key      string `json:"key"`
	MDName   string `json:"md"`
	ImgPath  string `json:"img"`
	Class    string `json:"class,omitempty"`
	Label    string `json:"label,omitempty"`
	Reason   string `json:"class_reason,omitempty"`
	Status   string `json:"status"` // classified | done | error
	Content  string `json:"content,omitempty"`
	TikzCode string `json:"tikz_code,omitempty"`
	FigPDF   string `json:"figure_pdf,omitempty"`
	FigPNG   string `json:"figure_png,omitempty"`
	FigSVG   string `json:"figure_svg,omitempty"`
	Kept     bool   `json:"original_kept,omitempty"`
	Error    string `json:"error,omitempty"`
}

// mdFile caches one scanned markdown file.
type mdFile struct {
	name    string
	content string
	lines   []string
	starts  []int
}

// task is one unique image (with all its occurrences) in one md file.
type task struct {
	mdName  string
	imgPath string
	lineIdx int
	offsets [][2]int
}

func (t *task) key() string { return t.mdName + "::" + t.imgPath }

// Runner owns the shared dependencies of the latex pipelines.
type Runner struct {
	cfg     *config.Config
	log     *logger.Logger
	comp    *Compiler
	clients map[string]*session.Client
	models  map[string]config.ModelConfig

	// lastSplitError remembers the latest split validation failure so
	// the chapter session can be re-prompted with a concrete reason.
	lastSplitError string
}

// NewRunner builds the shared runner (clients resolved per registry
// key with fallback to the top-level ai block).
func NewRunner(cfg *config.Config, log *logger.Logger) *Runner {
	return &Runner{
		cfg:     cfg,
		log:     log,
		comp:    NewCompiler(cfg.Latex.Compile),
		clients: map[string]*session.Client{},
		models:  map[string]config.ModelConfig{},
	}
}

func (r *Runner) clientFor(name string) *session.Client {
	if c, ok := r.clients[name]; ok {
		return c
	}
	mc, _ := r.cfg.ResolveModel(name)
	c := session.NewClient(mc)
	c.SetLogger(r.log)
	r.clients[name] = c
	r.models[name] = mc
	return c
}

// RunImages executes the level-2 pipeline: classify every image, then
// process per class (text extraction / TikZ vectorisation / keep
// raster), finally rebuild the markdown files under latex.output_dir.
func (r *Runner) RunImages(opts ImagesOptions) error {
	cfg := r.cfg
	srcDir := opts.SourceDir
	if srcDir == "" {
		srcDir = cfg.Paths.OutputDir
	}
	outDir := opts.OutDir
	if outDir == "" {
		outDir = cfg.Paths.LatexOutput
	}
	for _, d := range []string{outDir, filepath.Join(outDir, "figures"),
		filepath.Join(outDir, "images"), filepath.Join(outDir, "tikz"),
		filepath.Join(outDir, "progress_items")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}

	compErr := r.comp.Available()
	if compErr != nil {
		r.log.LogWarning(0, "LaTeX 工具链不可用，vector 图像将回退为保留原图:", compErr)
	}

	// Scan markdown files.
	mdFiles, err := filepath.Glob(filepath.Join(srcDir, "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(mdFiles)
	mdFiles = filterFiles(mdFiles, opts.Files)
	if len(mdFiles) == 0 {
		return fmt.Errorf("没有匹配的 markdown 文件（指定文件请用: docvision latex <文件名.md> ...）")
	}

	mdCache := map[string]*mdFile{}
	tasks := map[string]*task{}
	refCount := 0
	for _, mdf := range mdFiles {
		data, err := os.ReadFile(mdf)
		if err != nil {
			r.log.LogWarning(0, "读取失败", mdf, err)
			continue
		}
		content := string(data)
		lines := strings.Split(content, "\n")
		starts := make([]int, len(lines)+1)
		for i, l := range lines {
			starts[i+1] = starts[i] + len(l) + 1
		}
		name := filepath.Base(mdf)
		mdCache[name] = &mdFile{name: name, content: content, lines: lines, starts: starts}
		matches := imageRefRe.FindAllStringSubmatchIndex(content, -1)
		refCount += len(matches)
		r.log.Log(0, "  ", name+":", strconv.Itoa(len(matches)), "images")
		for _, m := range matches {
			imgPath := content[m[2]:m[3]]
			key := name + "::" + imgPath
			off := [2]int{m[0], m[1]}
			if t, ok := tasks[key]; ok {
				t.offsets = append(t.offsets, off)
				continue
			}
			tasks[key] = &task{
				mdName: name, imgPath: imgPath,
				lineIdx: findLineIdx(starts, m[0]),
				offsets: [][2]int{off},
			}
		}
	}
	all := make([]*task, 0, len(tasks))
	for _, t := range tasks {
		sort.Slice(t.offsets, func(i, j int) bool { return t.offsets[i][0] < t.offsets[j][0] })
		all = append(all, t)
	}
	r.log.Log(0, "Total image refs:", strconv.Itoa(refCount), "| unique tasks:", strconv.Itoa(len(all)))

	// Load progress.
	progDir := filepath.Join(outDir, "progress_items")
	prog := map[string]*imageProgress{}
	loadProgress(progDir, prog)

	pending := make([]*task, 0, len(all))
	for _, t := range all {
		if p, ok := prog[t.key()]; ok && p.Status == "done" {
			continue
		}
		pending = append(pending, t)
	}
	if opts.TestMode {
		pending = sampleTasks(pending, opts.Number, opts.Seed, r.log)
	}
	r.log.Log(0, "Already done:", strconv.Itoa(len(all)-len(pending)), "| to process:", strconv.Itoa(len(pending)))

	// Default console behaviour mirrors img2text: one compact progress
	// line per phase; every detail line goes to the log file only.
	verbose := opts.Verbose
	if !verbose {
		r.log.SetQuiet(true)
		defer func() { r.log.SetQuiet(false) }()
	}

	// Phase 1: classification.
	if opts.Step == "" || opts.Step == "classify" {
		r.classifyPhase(pending, mdCache, prog, progDir, verbose)
	}
	if opts.Step == "classify" {
		if !verbose {
			r.log.SetQuiet(false)
			fmt.Println()
		}
		r.log.Log(0, "classify step finished.")
		return nil
	}

	// Phase 2: per-class processing.
	r.processPhase(pending, mdCache, prog, progDir, outDir, compErr, verbose)

	if !verbose {
		r.log.SetQuiet(false)
		fmt.Println()
	}
	// Phase 3: rebuild markdown.
	return r.rebuildPhase(mdCache, prog, outDir)
}

func findLineIdx(starts []int, pos int) int {
	for i := 0; i < len(starts)-1; i++ {
		if starts[i] <= pos && pos < starts[i+1] {
			return i
		}
	}
	return len(starts) - 1
}

// ------------------------------------------------------------------
// phase 1: classification
// ------------------------------------------------------------------

func (r *Runner) classifyPhase(pending []*task, mdCache map[string]*mdFile,
	prog map[string]*imageProgress, progDir string, verbose bool) {

	conc := r.cfg.Latex.Concurrency
	if conc <= 0 {
		conc = 3
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	total, done, failed := len(pending), 0, 0
	progress := func() {
		if verbose || total == 0 {
			return
		}
		pct := float64(done) * 100.0 / float64(total)
		fmt.Fprintf(os.Stdout, "\r[classify %d/%d] %.2f%% (失败: %d)          ", done, total, pct, failed)
	}

	client := r.clientFor(r.cfg.Latex.ClassifierModel)
	modelCfg := r.models[r.cfg.Latex.ClassifierModel]

	for _, t := range pending {
		wg.Add(1)
		tid := <-tidPool
		go func(tt *task) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			defer func() {
				mu.Lock()
				done++
				mu.Unlock()
				progress()
			}()
			mu.Lock()
			_, seen := prog[tt.key()]
			mu.Unlock()
			if seen {
				return // already classified
			}
			imgFile, err := resolveImageFile(r.cfg.Paths.ImagesDir, tt.imgPath, subjectOf(tt.mdName))
			if err != nil {
				r.log.LogWarning(tid, "[classify] 图片缺失:", tt.imgPath)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			img64, err := img2text.ImageToBase64(imgFile, 1280)
			if err != nil {
				r.log.LogWarning(tid, "[classify] 读取图片失败:", tt.imgPath, err)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			class, err := ClassifyImage(client, modelCfg, img64)
			if err != nil {
				// One retry with a stricter instruction.
				class, err = ClassifyImageStrict(client, modelCfg, img64)
				if err != nil {
					r.log.LogWarning(tid, "[classify] 失败（按 raster 处理）:", tt.imgPath, err)
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}
			if err != nil {
				class = Classification{Kind: ClassRaster, Label: "unclassified"}
			}
			p := &imageProgress{
				Key: tt.key(), MDName: tt.mdName, ImgPath: tt.imgPath,
				Class: class.Kind, Label: class.Label, Reason: class.Reason,
				Status: "classified",
			}
			mu.Lock()
			prog[tt.key()] = p
			mu.Unlock()
			saveProgress(progDir, p)
			r.log.Log(tid, "[classify]", tt.imgPath, "->", class.Kind,
				"("+class.Label+")", class.Reason)
		}(t)
	}
	wg.Wait()
}

// ClassifyImageStrict is the second-chance call with an explicit
// "JSON only" reminder.
func ClassifyImageStrict(client *session.Client, modelCfg config.ModelConfig, imgBase64 string) (Classification, error) {
	c, err := ClassifyImage(client, modelCfg, imgBase64)
	if err == nil {
		return c, nil
	}
	// Fall back to a manual request path with a stronger format hint.
	req := &session.ChatRequest{
		Model: client.Model(),
		Messages: []session.ChatMessage{
			{Role: "system", Content: classifierSystemPrompt + "\n\nIMPORTANT: reply with the raw JSON object ONLY."},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": "Classify this document image. JSON only."},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imgBase64,
				}},
			}},
		},
		MaxTokens: 512, Temperature: 0.0,
	}
	resp, sentinel, status := client.CallWithRetry(req, 3, 20)
	if status != "" {
		return Classification{}, fmt.Errorf("分类请求失败: %s", sentinel)
	}
	if len(resp.Choices) == 0 {
		return Classification{}, fmt.Errorf("分类响应为空")
	}
	return parseClassification(session.ContentString(resp.Choices[0].Message))
}

// ------------------------------------------------------------------
// phase 2: per-class processing
// ------------------------------------------------------------------

func (r *Runner) processPhase(pending []*task, mdCache map[string]*mdFile,
	prog map[string]*imageProgress, progDir, outDir string, compErr error, verbose bool) {

	conc := r.cfg.Latex.Concurrency
	if conc <= 0 {
		conc = 3
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	total, done, failed, warned := 0, 0, 0, 0
	for _, t := range pending {
		mu.Lock()
		p, ok := prog[t.key()]
		mu.Unlock()
		if !ok || p.Class == "" {
			continue // not classified yet
		}
		total++
	}
	progress := func() {
		if verbose || total == 0 {
			return
		}
		pct := float64(done) * 100.0 / float64(total)
		fmt.Fprintf(os.Stdout, "\r[process %d/%d] %.2f%% (失败: %d, 回退: %d)          ", done, total, pct, failed, warned)
	}

	for _, t := range pending {
		mu.Lock()
		p, ok := prog[t.key()]
		mu.Unlock()
		if !ok || p.Class == "" {
			continue // not classified yet
		}
		wg.Add(1)
		tid := <-tidPool
		go func(tt *task, pp *imageProgress) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			defer func() {
				mu.Lock()
				done++
				mu.Unlock()
				progress()
			}()
			if rec := recover(); rec != nil {
				r.log.LogError(tid, "[process] panic:", rec)
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			mf := mdCache[tt.mdName]
			switch pp.Class {
			case ClassText:
				content, err := r.processTextImage(mf, tt, tid)
				if err != nil {
					r.log.LogError(tid, "[text] 失败:", tt.imgPath, err)
					mu.Lock()
					failed++
					mu.Unlock()
					return
				}
				pp.Content = content
				pp.Status = "done"
				r.log.Log(tid, "[text]", tt.imgPath, "done")

			case ClassVector:
				if compErr != nil {
					r.log.LogWarning(tid, "[vector] 工具链不可用，保留原图:", tt.imgPath)
					pp.Kept = true
					pp.Status = "done"
					break
				}
				res := r.processVectorImage(mf, tt, pp, outDir, tid)
				if res {
					pp.Status = "done"
				} else {
					// Fallback: keep the original (logged as warning).
					r.log.LogWarning(tid, "[vector] TikZ 未通过，保留原图（警告：矢量转换失败）:", tt.imgPath, pp.Error)
					mu.Lock()
					warned++
					mu.Unlock()
					pp.Kept = true
					pp.Status = "done"
				}

			case ClassRaster:
				if r.cfg.Latex.InsertImageDescription {
					content, err := r.processTextImage(mf, tt, tid)
					if err != nil {
						r.log.LogWarning(tid, "[raster] 解释生成失败，仅保留链接:", tt.imgPath, err)
					} else {
						pp.Content = content
					}
				}
				pp.Kept = true
				pp.Status = "done"
				r.log.Log(tid, "[raster]", tt.imgPath, "kept as original")
			}
			saveProgress(progDir, pp)
		}(t, p)
	}
	wg.Wait()
}

// processTextImage runs the existing img2text pipeline and strips the
// [IMG_TYPE: ...] header so only the pure content is embedded.
func (r *Runner) processTextImage(mf *mdFile, t *task, tid int) (string, error) {
	mc, _ := r.cfg.ResolveModel("") // top-level ai block
	aiCfg := config.AIConfig{
		BaseURL: mc.BaseURL, APIKey: mc.APIKey, Model: mc.Model, RequestBody: mc.RequestBody,
	}
	client := img2text.NewAIClient(aiCfg, r.cfg.Options)
	subject := subjectOf(t.mdName)
	result, status := img2text.ProcessOneImage(
		client, r.cfg.Paths.ImagesDir, t.imgPath, subject,
		mf.lines, t.lineIdx, r.log, tid, r.cfg.Options,
	)
	if status != img2text.StatusOK {
		return "", fmt.Errorf("img2text 状态 %s: %s", status, truncateStr(result, 200))
	}
	return stripImgTypeHeader(result), nil
}

// stripImgTypeHeader removes the leading [IMG_TYPE: xxx] line.
func stripImgTypeHeader(s string) string {
	idx := strings.Index(s, "[IMG_TYPE:")
	if idx < 0 {
		return strings.TrimSpace(s)
	}
	end := strings.Index(s[idx:], "]")
	if end < 0 {
		return strings.TrimSpace(s[idx:])
	}
	rest := s[idx+end+1:]
	return strings.TrimSpace(strings.TrimLeft(rest, " \t\n"))
}

// processVectorImage runs one TikZ session; returns true when the
// figure was confirmed and persisted.
func (r *Runner) processVectorImage(mf *mdFile, t *task, pp *imageProgress, outDir string, tid int) bool {
	imgFile, err := resolveImageFile(r.cfg.Paths.ImagesDir, t.imgPath, subjectOf(t.mdName))
	if err != nil {
		r.log.LogError(tid, "[vector] 图片缺失:", t.imgPath)
		return false
	}
	img64, err := img2text.ImageToBase64(imgFile, 1280)
	if err != nil {
		r.log.LogError(tid, "[vector] 读取图片失败:", t.imgPath, err)
		return false
	}
	contextText := strings.Join(img2text.GetContextLines(mf.lines, t.lineIdx,
		r.cfg.Options.MaxContextLinesUp, r.cfg.Options.MaxContextLinesDown), "\n")

	modelCfg := r.models[r.cfg.Latex.DrawingModel]
	client := r.clientFor(r.cfg.Latex.DrawingModel)
	tuning := r.cfg.LatexSession("drawing")

	base := sanitizeName(strings.TrimSuffix(filepath.Base(t.imgPath), filepath.Ext(t.imgPath))) +
		"__" + sanitizeName(pp.Label)
	if base == "__" || len(base) < 3 {
		base = sanitizeName(strings.TrimSuffix(filepath.Base(t.imgPath), filepath.Ext(t.imgPath)))
	}
	if base == "" {
		base = fmt.Sprintf("fig_%d", time.Now().UnixNano()%100000)
	}
	name := sanitizeName(strings.TrimSuffix(t.mdName, filepath.Ext(t.mdName))) + "__" + base
	dstTex := filepath.Join(outDir, "tikz", name+".tex")
	dstPDF := filepath.Join(outDir, "figures", name+".pdf")
	dstPNG := filepath.Join(outDir, "figures", name+".png")

	res, err := RunTikZSession(client, modelCfg, tuning, r.comp, img64, contextText,
		outDir, dstTex, dstPDF, dstPNG, r.log, tid)
	if err != nil {
		pp.Error = err.Error()
		return false
	}
	if !res.Submitted {
		pp.Error = res.Reason
		return false
	}
	pp.TikzCode = res.Code
	pp.FigPDF = "figures/" + filepath.Base(dstPDF)
	pp.FigPNG = "figures/" + filepath.Base(dstPNG)
	// Markdown 无法内嵌 PDF：用 dvisvgm 把矢量图编译为 SVG 供嵌入
	//（失败时回退 PNG/PDF 链接，仅记录警告）。
	if _, err := exec.LookPath("dvisvgm"); err == nil {
		dstSVG := filepath.Join(outDir, "figures", name+".svg")
		cmd := exec.Command("dvisvgm", "--pdf", "--exact", "--output",
			filepath.Base(dstSVG), filepath.Base(dstPDF))
		cmd.Dir = outDir
		if err := cmd.Run(); err != nil {
			r.log.LogWarning(tid, "[vector] SVG 转换失败（回退 PDF/PNG 链接）:", err)
		} else {
			pp.FigSVG = "figures/" + name + ".svg"
		}
	}
	return true
}

// ------------------------------------------------------------------
// phase 3: markdown rebuild
// ------------------------------------------------------------------

func (r *Runner) rebuildPhase(mdCache map[string]*mdFile, prog map[string]*imageProgress, outDir string) error {
	r.log.Log(0, "\nRebuilding markdown files...")
	for name, mf := range mdCache {
		type rep struct {
			off [2]int
			p   *imageProgress
		}
		var reps []rep
		for k, p := range prog {
			parts := strings.SplitN(k, "::", 2)
			if len(parts) != 2 || parts[0] != name || p.Status != "done" {
				continue
			}
			for _, off := range validOffsets(mf.content, p.ImgPath, p.Key) {
				reps = append(reps, rep{off: off, p: p})
			}
		}
		if len(reps) == 0 {
			continue
			// no replacements for this file
		}
		sort.Slice(reps, func(i, j int) bool { return reps[i].off[0] > reps[j].off[0] })

		nc := mf.content
		for _, rp := range reps {
			block := r.embedBlock(rp.p, name, outDir)
			if block == "" {
				continue
			}
			// Replacements are applied right-to-left, so earlier
			// (leftward) offsets are never invalidated.
			nc = nc[:rp.off[0]] + block + nc[rp.off[1]:]
		}
		outPath := filepath.Join(outDir, name)
		if err := os.WriteFile(outPath, []byte(nc), 0o644); err != nil {
			r.log.LogError(0, "  写入失败:", name, err)
			continue
		}
		r.log.Log(0, "  Saved:", name, "("+strconv.Itoa(len(reps))+" replacements)")
	}
	r.log.Log(0, "Level-2 output ready:", outDir)
	return nil
}

// embedBlock renders the replacement for one processed image per the
// level-2 embedding rules.
func (r *Runner) embedBlock(p *imageProgress, mdName, outDir string) string {
	switch p.Class {
	case ClassText:
		if p.Content == "" {
			return ""
		}
		return p.Content
	case ClassVector:
		if p.Kept || p.FigPDF == "" {
			return r.rasterBlock(p, mdName, outDir)
		}
		if r.cfg.Latex.InsertImageDescription && p.TikzCode != "" {
			return "```tikz\n" + p.TikzCode + "\n```"
		}
		label := p.Label
		if label == "" {
			label = "figure"
		}
		// Markdown 不支持内嵌 PDF：优先嵌入 dvisvgm 生成的 SVG。
		if p.FigSVG != "" {
			return "![" + label + "](" + p.FigSVG + ")"
		}
		if p.FigPNG != "" {
			return "![" + label + "](" + p.FigPNG + ")"
		}
		return "![" + label + "](" + p.FigPDF + ")"
	case ClassRaster:
		return r.rasterBlock(p, mdName, outDir)
	}
	return ""
}

// rasterBlock keeps the original image link (copying the file into the
// output tree) or embeds the readable explanation.
func (r *Runner) rasterBlock(p *imageProgress, mdName, outDir string) string {
	if r.cfg.Latex.InsertImageDescription && p.Content != "" {
		return "[Image]( " + p.Content + " )"
	}
	// Copy the original image and keep the link.
	rel := p.ImgPath
	if i := strings.Index(rel, "/"); i >= 0 {
		rel = rel[i+1:]
	}
	src, err := resolveImageFile(r.cfg.Paths.ImagesDir, p.ImgPath, subjectOf(mdName))
	if err != nil {
		r.log.LogWarning(0, "保留原图失败（文件缺失）:", p.ImgPath)
		return ""
	}
	dst := filepath.Join(outDir, "images", rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return ""
	}
	if !fileExists(dst) {
		if err := copyFile(src, dst); err != nil {
			r.log.LogWarning(0, "复制原图失败:", src, err)
			return ""
		}
	}
	return "![image](images/" + rel + ")"
}

// validOffsets re-checks that each recorded occurrence still points at
// the same image before replacing (same contract as img2text).
func validOffsets(content, imgPath, key string) [][2]int {
	var out [][2]int
	for _, m := range imageRefRe.FindAllStringSubmatchIndex(content, -1) {
		if content[m[2]:m[3]] == imgPath {
			out = append(out, [2]int{m[0], m[1]})
		}
	}
	return out
}

// ------------------------------------------------------------------
// progress persistence + helpers
// ------------------------------------------------------------------

func loadProgress(progDir string, prog map[string]*imageProgress) {
	entries, err := os.ReadDir(progDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(progDir, e.Name())
		files, _ := os.ReadDir(sub)
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(sub, f.Name()))
			if err != nil {
				continue
			}
			var p imageProgress
			if json.Unmarshal(data, &p) == nil && p.Key != "" {
				prog[p.Key] = &p
			}
		}
	}
}

func saveProgress(progDir string, p *imageProgress) {
	dir := filepath.Join(progDir, sanitizeName(p.MDName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := sanitizeName(p.ImgPath) + ".json"
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

func sampleTasks(pending []*task, n int, seedStr string, log *logger.Logger) []*task {
	if n <= 0 {
		n = 10
	}
	seed := int64(42)
	if seedStr == "random" {
		seed = time.Now().UnixNano()
	} else if v, err := strconv.ParseInt(seedStr, 10, 64); err == nil && seedStr != "" {
		seed = v
	}
	rng := rand.New(rand.NewSource(seed))
	log.Log(0, "Test seed:", strconv.FormatInt(seed, 10))
	if n > len(pending) {
		n = len(pending)
	}
	idx := rng.Perm(len(pending))[:n]
	out := make([]*task, 0, n)
	for _, i := range idx {
		out = append(out, pending[i])
	}
	return out
}

func subjectOf(mdName string) string {
	return strings.TrimSuffix(mdName, filepath.Ext(mdName))
}

// resolveImageFile mirrors img2text's resolution semantics.
func resolveImageFile(imagesDir, imgPath, subject string) (string, error) {
	rel := imgPath
	if i := strings.Index(imgPath, "/"); i >= 0 {
		rel = imgPath[i+1:]
	}
	if full := filepath.Join(imagesDir, rel); fileExists(full) {
		return full, nil
	}
	if subject != "" {
		if alt := filepath.Join(imagesDir, subject, filepath.Base(imgPath)); fileExists(alt) {
			return alt, nil
		}
	}
	return "", os.ErrNotExist
}
