package latex

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/img2text"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// ImagesOptions drives the level-2 image pipeline.
type ImagesOptions struct {
	Inline   bool // 档位1：矢量图内嵌 tikz 代码块而非 figures/*.pdf 资源
	TestMode bool
	Number   int
	Seed     string
	// Step filters the phases: "" (all), "classify", "process".
	Step string
	// SourceDir overrides paths.output_dir (defaults to it).
	SourceDir string
	// OutDir overrides paths.latex.output_dir (used by the level-1
	// book pipeline to place processed source inside its project dir).
	// When empty the workspace is <paths.latex_output>/<项目名>/ (see
	// resolveProjectDir).
	OutDir string
	// Project overrides the project name (--project). Default: the
	// book's 主题名 (main markdown base name, same as images/<主题>/).
	Project string
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

// FigureEnv carries the document context a figure session needs for
// cross-page merge decisions (neighbour images + their text).
type FigureEnv struct {
	MDContent  string // full markdown of the current document
	CurrentImg string // image path (as in markdown) being drawn
	ImagesDir  string // absolute images root for view_image
	MaxUp      int    // image_context expansion caps (from options.max_window_*)
	MaxDown    int
	OutputLang string // {OUTPUT_LANG} placeholder value (options.output_language)
	// CurrentImgAbs is the absolute path of the source bitmap: the view
	// tool measures its printed size (mm/dpi) from the MinerU parse so the
	// model can keep the original scale.
	CurrentImgAbs string
	// ViewImageMax / ViewPDFMax / ViewWarnRatio: soft view budgets
	// (tools.view.*); 0 = no budget.
	ViewImageMax  int
	ViewPDFMax    int
	ViewWarnRatio float64
}

// renderPrompt fills the placeholders used by the built-in session
// prompts: {MAX_ROUNDS} (session tool budget) and {OUTPUT_LANG} (the
// configured output language). The prompts are plain strings, so an
// unsubstituted placeholder reaches the model verbatim — which is why the
// substitution itself lives in internal/prompts and is covered by guard
// tests there (every declared placeholder must be gone after rendering).
func renderPrompt(prompt string, tuning config.SessionTuning, lang string) string {
	if prompt == "" {
		return ""
	}
	if lang == "" {
		lang = "Chinese"
	}
	return prompts.Fill(prompt, map[string]string{
		"OUTPUT_LANG": lang,
		"MAX_ROUNDS":  strconv.Itoa(session.EffectiveToolRounds(tuning)),
	})
}

// outputLang is the configured {OUTPUT_LANG} value (default Chinese).
func (r *Runner) outputLang() string {
	if r.cfg == nil || r.cfg.Options.OutputLanguage == "" {
		return "Chinese"
	}
	return r.cfg.Options.OutputLanguage
}

// imageProgress is the per-image persisted state (断点续传).
type imageProgress struct {
	Key        string   `json:"key"`
	MDName     string   `json:"md"`
	ImgPath    string   `json:"img"`
	Class      string   `json:"class,omitempty"`
	Label      string   `json:"label,omitempty"`
	Reason     string   `json:"class_reason,omitempty"`
	Status     string   `json:"status"` // classified | done | error
	Content    string   `json:"content,omitempty"`
	TikzCode   string   `json:"tikz_code,omitempty"`
	FigPDF     string   `json:"figure_pdf,omitempty"`
	FigPNG     string   `json:"figure_png,omitempty"`
	FigSVG     string   `json:"figure_svg,omitempty"`
	Kept       bool     `json:"original_kept,omitempty"`
	SVGFail    bool     `json:"svg_failed,omitempty"`  // vector ok but dvisvgm failed (degraded to PNG/PDF link)
	Styled     bool     `json:"styled,omitempty"`      // styled-text image (needs book-level restyle)
	StyleNote  string   `json:"style_note,omitempty"`  // what the styling looks like
	Absorbed   bool     `json:"absorbed,omitempty"`    // cross-page continuation merged into an earlier figure
	MergedRefs []string `json:"merged_refs,omitempty"` // refs absorbed by THIS combined figure
	Error      string   `json:"error,omitempty"`
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

	wm       *WatermarkMemory // 水印工作记忆（流程开始时检测，贯穿所有会话）
	inline   bool             // 档位1 inline 模式：tikz 代码直接内嵌进 markdown
	docIndex *DocIndex        // 原始文档只读索引（doc_search/原书页面）
	docPages *pageIndex       // 与索引对齐的全局页表（水印采样/索引对齐）
	projDir  string           // 项目根（project 挂载点，只读）
	pdfView  *pdfView         // 原书 PDF 的最小视图（source 挂载点，只读）
	projView string           // 项目的最小只读视图（project 挂载点）

	// lastSplitError remembers the latest split validation failure so
	// the chapter session can be re-prompted with a concrete reason.
	lastSplitError string

	// python is the shared Python environment handed to session bash
	// (tools.python): created once per run, prepared on the host
	// (venv creation + configured packages) before the first session.
	python     *PythonEnv
	pythonOnce sync.Once

	// consoleVerbose mirrors the book-run --verbose flag: when false
	// (compact mode) the convert phase shows a single [convert k/N]
	// progress line instead of streaming every session event.
	consoleVerbose bool
}

// phaseNote returns a console printer for phase-level progress notes.
// In compact mode (book run without --verbose) it prints directly to
// stdout — the logger is quiet then, so this is the only way phase
// progress stays visible. In verbose mode it is a no-op (the logger
// already streams everything).
func (r *Runner) phaseNote() func(format string, a ...any) {
	if r.consoleVerbose {
		return func(string, ...any) {}
	}
	return func(format string, a ...any) {
		fmt.Fprintf(os.Stdout, format+"\n", a...)
	}
}

// NewRunner builds the shared runner (clients resolved per registry
// key with fallback to the top-level ai block).
func NewRunner(cfg *config.Config, log *logger.Logger) *Runner {
	// 原图尺寸测量要靠 MinerU 解析（content_list/layout）把位图换算成 mm；
	// 会话看的是拷进项目的位图，只有知道 mineru_output 在哪才找得到解析。
	SetImageParseRoot(cfg.Paths.MineruOutput)
	comp := NewCompiler(cfg.Latex.Compile)
	comp.fontsDir = cfg.Paths.Fonts // 项目字体目录对编译可见（TEXINPUTS/OSFONTDIR）
	return &Runner{
		cfg:     cfg,
		log:     log,
		comp:    comp,
		clients: map[string]*session.Client{},
		models:  map[string]config.ModelConfig{},
	}
}

// keepTemp reports whether temporary work directories survive the run:
// latex.keep_temp_dirs, or any debug logging (--debug / log_level: debug).
func (r *Runner) keepTemp() bool {
	return r.cfg.Latex.KeepTempDirsEnabled() || r.log.DebugEnabled()
}

// keepRecords reports whether session transcripts survive after a
// session succeeds: latex.keep_session_records, or any debug logging.
func (r *Runner) keepRecords() bool {
	return r.cfg.Latex.KeepSessionRecordsEnabled() || r.log.DebugEnabled()
}

// tempDir creates a temporary work directory INSIDE the project
// (<proj>/work/temp/<name>) instead of /tmp: it sits next to the rest of
// the run so it can be inspected, and whether it is deleted afterwards
// depends on keepTemp(). The returned cleanup function is safe to call
// more than once.
func (r *Runner) tempDir(proj, name string) (string, func(), error) {
	root := filepath.Join(proj, "work", "temp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", func() {}, err
	}
	dir := filepath.Join(root, name)
	if err := os.RemoveAll(dir); err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		if r.keepTemp() {
			r.log.Log(0, "[temp] 保留临时工作目录:", dir)
			return
		}
		_ = os.RemoveAll(dir)
	}
	return dir, cleanup, nil
}

// pythonEnv returns the run's shared Python environment (nil when
// tools.python.enabled is false). Prepared once, on the host: the
// sandbox has no network, so nothing about Python can be fixed from
// inside a session.
func (r *Runner) pythonEnv(proj string) *PythonEnv {
	r.pythonOnce.Do(func() {
		report := ""
		if proj != "" {
			report = filepath.Join(proj, "work", "python")
		}
		r.python = NewPythonEnv(r.cfg.PythonConfig(), report, r.log)
		if r.python != nil {
			r.python.Prepare()
		}
	})
	return r.python
}

// sessionBashTemp creates the scratch directory a session's sandboxed
// bash sees as /tmp. It lives inside the project (<proj>/work/temp/
// bash_<name>) so it can be inspected next to the rest of the run, it
// PERSISTS across that session's bash calls (a per-call tmpfs silently
// deleted files between calls), and it is removed when the session ends
// unless latex.keep_temp_dirs / debug keeps it.
func (r *Runner) sessionBashTemp(proj, name string) (string, func()) {
	if proj == "" {
		return "", func() {}
	}
	dir, cleanup, err := r.tempDir(proj, name)
	if err != nil {
		r.log.LogWarning(0, "[bash] 会话临时目录创建失败，/tmp 退回会话内 tmpfs:", err)
		return "", func() {}
	}
	return dir, cleanup
}

// keepSessionFile deletes a session transcript unless records are kept.
func (r *Runner) keepSessionFile(path string) {
	if r.keepRecords() {
		r.log.Log(0, "[session] 保留会话记录:", path)
		return
	}
	_ = os.Remove(path)
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
// raster), finally rebuild the markdown files under
// <paths.latex_output>/<项目名>/ (or opts.OutDir when the level-1 book
// pipeline nested the call, or the output root itself for legacy
// single-project layouts — see resolveProjectDir).
func (r *Runner) RunImages(opts ImagesOptions) error {
	cfg := r.cfg
	r.inline = opts.Inline
	srcDir := opts.SourceDir
	if srcDir == "" {
		srcDir = cfg.Paths.OutputDir
	}

	// Scan markdown files first: the git-style project name is derived
	// from them (主题名), and the workspace must be resolved before any
	// output directory is created.
	mdFiles, err := filepath.Glob(filepath.Join(srcDir, "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(mdFiles)
	mdFiles = filterFiles(mdFiles, opts.Files)
	if len(mdFiles) == 0 {
		return fmt.Errorf("没有匹配的 markdown 文件（指定文件请用: docvision latex <文件名.md> ...）")
	}

	outDir := opts.OutDir
	if outDir == "" {
		// 独立档位2：工作区是 <paths.latex_output>/<项目名>/。档位1 会把
		// OutDir 指到 <proj>/source，此时项目根已由 RunBook 决定。
		proj, err := r.useProjectDir(cfg.Paths.LatexOutput, opts.Project, projectSourceName(srcDir, mdFiles), 2)
		if err != nil {
			return err
		}
		outDir = proj
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
	r.migrateProgress(prog, progDir, outDir)

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
	done0 := len(all) - len(pending) // 断点续传：此前已完成数
	// Restore the PREVIOUS quiet state: RunBook already silenced the
	// console for its compact phase lines, and hard-coding SetQuiet(false)
	// here re-enabled info-level output for every later phase (style,
	// chapters, convert, assemble), which is what leaked session internals
	// into the terminal.
	prevQuiet := r.log.Quiet()
	if !verbose {
		r.log.SetQuiet(true)
		defer r.log.SetQuiet(prevQuiet)
	}

	// Phase 1: classification.
	if opts.Step == "" || opts.Step == "classify" {
		// 水印工作记忆：最先检测（全览页 + md 统计），结果贯穿全部会话。
		if r.cfg.Latex.RemoveWatermark {
			var samples []watermarkSample
			for name, mf := range mdCache {
				samples = append(samples, watermarkSample{name: name, content: mf.content})
			}
			sort.Slice(samples, func(i, j int) bool { return samples[i].name < samples[j].name })
			r.detectWatermarkPhase(samples)
		}
		r.classifyPhase(pending, mdCache, prog, progDir, verbose, done0)
	}
	if opts.Step == "classify" {
		if !verbose {
			r.log.SetQuiet(false)
		}
		r.log.Log(0, "classify step finished.")
		return nil
	}

	// Phase 2: per-class processing.
	r.processPhase(pending, mdCache, prog, progDir, outDir, compErr, verbose, done0)

	if !verbose {
		r.log.SetQuiet(prevQuiet)
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
// live console progress
// ------------------------------------------------------------------

// liveProgress renders ONE repainting console line. The text callback is
// called under the owner's own lock state (it must lock its counters
// itself), and a ticker re-renders every few seconds so a line never
// freezes at its 0/N start value while long sessions run — a frozen
// "running: 0" looked like a counter bug.
type liveProgress struct {
	text func() string
	stop chan struct{}
	once sync.Once
	// tty 决定重绘方式：终端里用 `\r` + 清行原地覆写（一行；会话实时行
	// 也会在它上面覆写），管道/重定向里按节拍整行输出（否则日志里全是
	// 裸 CR，且两次"整行 + 换行"会多出空行）。
	tty bool
	mu  sync.Mutex
	// last 是最近一次真正画出去的那一行，Close 用它重画最终状态。
	last string
}

func newLiveProgress(text func() string) *liveProgress {
	return newLiveProgressTTY(text, stdoutIsTerminal())
}

// newLiveProgressTTY 是 newLiveProgress 的可注入 tty 版本（测试用）。
func newLiveProgressTTY(text func() string, tty bool) *liveProgress {
	p := &liveProgress{text: text, stop: make(chan struct{}), tty: tty}
	p.render()
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-t.C:
				p.render()
			}
		}
	}()
	return p
}

func (p *liveProgress) render() {
	if p == nil || p.text == nil {
		return
	}
	line := p.text()
	if line == "" {
		return
	}
	p.mu.Lock()
	p.last = line
	p.mu.Unlock()
	if p.tty {
		// \x1b[K 先清掉本行残留（上一次更长的进度行、或会话实时行留下的
		// 尾巴），否则会看到两行文字叠在一起的残影。
		fmt.Fprintf(os.Stdout, "\r\x1b[K%s", line)
		return
	}
	fmt.Fprintln(os.Stdout, line)
}

// Close stops the ticker and terminates the line.
//
// 终端里**重画**最终一行再换行：期间会话实时行（book.go 的
// livePhaseLine）结束时会把这一行清掉（`\r\x1b[K`），此时只补一个 `\n`
// 就会在阶段之间留下一行空白——实测 `[classify 8/8] …` 与
// `[process 2/8] …` 之间就是这么来的。管道里最后一次 render 已经整行
// 换过行了，再补就是重复行，所以什么都不打。
func (p *liveProgress) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		close(p.stop)
		p.mu.Lock()
		line := p.last
		p.mu.Unlock()
		if line == "" {
			return
		}
		if p.tty {
			fmt.Fprintf(os.Stdout, "\r\x1b[K%s\n", line)
		}
	})
}

// classifyPhase starts here
// ------------------------------------------------------------------
// phase 1: classification
// ------------------------------------------------------------------

func (r *Runner) classifyPhase(pending []*task, mdCache map[string]*mdFile,
	prog map[string]*imageProgress, progDir string, verbose bool, done0 int) {

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
	// done0：断点续传时此前已完成的部分——进度直接从它起跳，与
	// "Already done" 行呼应，不再单列 skip。
	total, done, failed, running := len(pending)+done0, done0, 0, 0
	var progLive *liveProgress
	progress := func() {
		if progLive != nil {
			progLive.render()
		}
	}
	progLive = newLiveProgress(func() string {
		if verbose || total == 0 {
			return ""
		}
		mu.Lock()
		d, f, rn := done, failed, running
		mu.Unlock()
		pct := float64(d) * 100.0 / float64(total)
		return fmt.Sprintf("[classify %d/%d] %.2f%% (failed: %d, running: %d)", d, total, pct, f, rn)
	})
	defer progLive.Close()
	if total > 0 {
		r.log.Log(0, "[classify] 待分类", strconv.Itoa(len(pending)), "张，并发", strconv.Itoa(conc), "（latex.concurrency）")
	}

	// 水印图片引用：直接预标记 absorbed（重建时删除引用），
	// 不消耗任何 AI 会话，后续也不会重复处理。
	if r.wm != nil && len(r.wm.ImageRefs) > 0 {
		wmSet := map[string]bool{}
		for _, ref := range r.wm.ImageRefs {
			wmSet[strings.TrimSpace(ref)] = true
		}
		var kept []*task
		for _, t := range pending {
			if wmSet[t.imgPath] {
				pp := prog[t.key()]
				if pp != nil && pp.Status != "done" {
					pp.Status = "done"
					pp.Absorbed = true
					pp.Error = ""
					saveProgress(progDir, pp)
					r.log.Log(0, "[watermark] 跳过水印图片:", t.imgPath)
				}
				continue
			}
			kept = append(kept, t)
		}
		pending = kept
		total = len(pending)
	}

	client := r.clientFor(r.cfg.Latex.ClassifierModel)
	modelCfg := r.models[r.cfg.Latex.ClassifierModel]
	classifyExtra := r.wm.Block()

	// 只对尚未分类的任务调用分类会话（fallback 重试等已带 Class
	// 的条目直接进 process，不再重复分类，进度计数也不虚报）。
	toClassify := make([]*task, 0, len(pending))
	for _, t := range pending {
		if _, ok := prog[t.key()]; !ok {
			toClassify = append(toClassify, t)
		}
	}
	total = len(toClassify)
	if total > 0 {
		// 分类会话也耗时：先打出 0/N 起始行，第一张完成前控制台不空白。
		progress()
	}

	for _, t := range toClassify {
		wg.Add(1)
		tid := <-tidPool
		mu.Lock()
		running++
		mu.Unlock()
		go func(tt *task) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			defer func() {
				mu.Lock()
				done++
				running--
				mu.Unlock()
				progress()
			}()
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
			class, err := ClassifyImage(client, modelCfg, img64, classifyExtra)
			if err != nil {
				// One retry with a stricter instruction.
				class, err = ClassifyImageStrict(client, modelCfg, img64, classifyExtra)
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
				Styled: class.Styled, StyleNote: class.StyleNote,
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
	// 定格 classify 进度行并换行：下一阶段的 [process …] 行从新行开始，
	// 不会把 classify 的最终状态覆盖掉。
	if !verbose && total > 0 {
		progress()
		fmt.Fprintln(os.Stdout)
	}
}

// ClassifyImageStrict is the second-chance call with an explicit
// "JSON only" reminder.
func ClassifyImageStrict(client *session.Client, modelCfg config.ModelConfig, imgBase64, systemExtra string) (Classification, error) {
	c, err := ClassifyImage(client, modelCfg, imgBase64, systemExtra)
	if err == nil {
		return c, nil
	}
	// Fall back to a manual request path with a stronger format hint.
	req := &session.ChatRequest{
		Model: client.Model(),
		Messages: []session.ChatMessage{
			{Role: "system", Content: prompts.Must(prompts.ClassifierSystem) + "\n\nIMPORTANT: reply with the raw JSON object ONLY."},
			{Role: "user", Content: []map[string]interface{}{
				{"type": "text", "text": "Classify this document image. JSON only."},
				{"type": "image_url", "image_url": map[string]string{
					"url": "data:image/jpeg;base64," + imgBase64,
				}},
			}},
		},
		MaxTokens: 512, Temperature: 0.0,
		ResponseFormat: map[string]any{"type": "json_object"},
	}
	resp, sentinel, status := client.CallWithRetry(req)
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
	prog map[string]*imageProgress, progDir, outDir string, compErr error, verbose bool, done0 int) {

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
	// done0：断点续传时此前已完成的部分——进度直接从它起跳。
	total, done, failed, warned, running := done0, done0, 0, 0, 0
	// raster 计数：保留原图的图（档位1 每张都生成解释文本），单独显示
	// 让档位1 的控制台能看出"矢量 vs 原图"的比例。
	raster := 0
	for _, t := range pending {
		mu.Lock()
		p, ok := prog[t.key()]
		mu.Unlock()
		if !ok || p.Class == "" {
			continue // not classified yet
		}
		total++
		if p.Class == ClassRaster && p.Status == "done" {
			raster++ // 上次已完成的 raster（断点续传）
		}
	}
	var progLive *liveProgress
	progress := func() {
		if progLive != nil {
			progLive.render()
		}
	}
	progLive = newLiveProgress(func() string {
		if verbose || total == 0 {
			return ""
		}
		mu.Lock()
		d, f, w, rn, rs := done, failed, warned, running, raster
		mu.Unlock()
		pct := float64(d) * 100.0 / float64(total)
		ok := d - f - w
		if ok < 0 {
			ok = 0
		}
		return fmt.Sprintf("[process %d/%d] %.2f%% (done: %d, errors: %d, fallback: %d, raster: %d, running: %d)",
			d, total, pct, ok, f, w, rs, rn)
	})
	defer progLive.Close()
	if total > 0 {
		r.log.Log(0, "[process] 待处理", strconv.Itoa(len(pending)), "张，并发", strconv.Itoa(conc), "（latex.concurrency）")
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
		mu.Lock()
		running++
		mu.Unlock()
		go func(tt *task, pp *imageProgress) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			defer func() {
				mu.Lock()
				done++
				running--
				mu.Unlock()
				progress()
			}()
			mf := mdCache[tt.mdName]
			if pp.Absorbed {
				// 已被前面的合并图吸收：不再单独处理。
				return
			}
			// 会话标记（▶ START / ✓ DONE / ✗ FAILED）：与 img2text 日志
			// 同格式，日志分析器因此也能解析 latex 日志（含耗时/类型）。
			st := time.Now()
			r.log.Log(tid, "▶ START", tt.key())
			sessOK, sessType, sessMsg := false, "", ""
			// recover 必须在 defer 内调用才生效（原实现为普通语句，
			// panic 会直接击穿整个进程）。
			defer func() {
				if rec := recover(); rec != nil {
					sessMsg = fmt.Sprintf("panic: %v", rec)
					r.log.LogError(tid, "[process] panic:", rec)
					mu.Lock()
					failed++
					mu.Unlock()
				}
				el := strconv.FormatFloat(time.Since(st).Seconds(), 'f', 2, 64)
				if sessOK {
					r.log.Log(tid, "✓", "["+el+"s]", "DONE", "[IMG_TYPE: "+sessType+"]")
				} else {
					r.log.LogError(tid, "✗", "["+el+"s]", "FAILED", sessMsg)
				}
			}()
			switch pp.Class {
			case ClassText:
				content, err := r.processTextImage(mf, tt, tid)
				if err != nil {
					r.log.LogError(tid, "[text] 失败:", tt.imgPath, err)
					sessMsg = err.Error()
					mu.Lock()
					failed++
					mu.Unlock()
					return
				}
				if strings.TrimSpace(content) == "" {
					// 没提取到文本：保留原图（不丢内容），不算失败。
					pp.Kept = true
					pp.Status = "done"
					sessOK, sessType = true, "text-empty-keep"
					r.log.LogWarning(tid, "[text] 未提取到文本，保留原图:", tt.imgPath)
				} else {
					pp.Content = content
					pp.Status = "done"
					sessOK, sessType = true, "text"
					r.log.Log(tid, "[text]", tt.imgPath, "done")
				}

			case ClassVector:
				if compErr != nil {
					r.log.LogWarning(tid, "[vector] 工具链不可用，保留原图（下次运行重试）:", tt.imgPath)
					pp.Kept = true
					pp.Status = "fallback"
					sessType = "vector-fallback"
					sessMsg = "toolchain unavailable"
					break
				}
				res := r.processVectorImage(mf, tt, pp, outDir, tid, prog, progDir, &mu)
				if res && pp.SVGFail {
					// TikZ 成功但 dvisvgm 失败：以 PNG/PDF 链接嵌入，
					// 已按 ERROR 记录并在 markdown 标注；tikz 产物保留。
					pp.Status = "done"
					sessType = "vector-svg-fallback"
					sessMsg = pp.Error
					break
				}
				if res {
					pp.Status = "done"
					sessOK, sessType = true, "vector"
				} else {
					// Fallback: keep the original. Logged as ERROR and
					// annotated in the markdown; retried next run.
					r.log.LogError(tid, "[vector] TikZ 未通过，保留原图（下次运行自动重试）:", tt.imgPath, pp.Error)
					mu.Lock()
					warned++
					mu.Unlock()
					pp.Kept = true
					pp.Status = "fallback"
					sessType = "vector-fallback"
					sessMsg = pp.Error
				}

			case ClassRaster:
				// 档位1：raster 图总是生成解释文本（复用 img2text 的
				// 文本提取，一次视觉调用），进 DESCRIBE 块供转换会话参考；
				// 档位2 由 insert_image_description 开关控制。
				if r.inline || r.cfg.Latex.InsertImageDescription {
					content, err := r.processTextImage(mf, tt, tid)
					if err != nil {
						r.log.LogWarning(tid, "[raster] 解释生成失败，仅保留链接:", tt.imgPath, err)
					} else {
						pp.Content = content
					}
				}
				pp.Kept = true
				pp.Status = "done"
				sessOK, sessType = true, "raster"
				mu.Lock()
				raster++
				mu.Unlock()
				r.log.Log(tid, "[raster]", tt.imgPath, "kept as original",
					"described="+strconv.FormatBool(strings.TrimSpace(pp.Content) != ""))
			}
			saveProgress(progDir, pp)
		}(t, p)
	}
	wg.Wait()
	// 定格 process 进度行并换行：后续的 rebuild 输出从新行开始，
	// 最终状态保留在控制台上。
	if !verbose && total > 0 {
		progress()
		fmt.Fprintln(os.Stdout)
	}
}

// processTextImage extracts the VISIBLE TEXT of a text-class image (the
// latex pipeline replaces such images with their text). It never returns
// an image description — that would leak AI commentary into the
// markdown. An empty result means "no readable text": the caller keeps
// the original image.
func (r *Runner) processTextImage(mf *mdFile, t *task, tid int) (string, error) {
	mc, _ := r.cfg.ResolveModel("") // top-level ai block (+ options.* defaults)
	client := img2text.NewAIClient(mc)
	client.SetLogger(r.log)
	subject := subjectOf(t.mdName)
	imgFile, err := resolveImageFile(r.cfg.Paths.ImagesDir, t.imgPath, subject)
	if err != nil {
		return "", fmt.Errorf("图片缺失: %w", err)
	}
	img64, err := img2text.ImageToBase64(imgFile, 1280)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}
	contextText := strings.Join(img2text.GetContextLines(mf.lines, t.lineIdx,
		r.cfg.Options.MaxContextLinesUp, r.cfg.Options.MaxContextLinesDown), "\n")
	textOpts := r.cfg.Options
	if wb := r.wm.Block(); wb != "" {
		textOpts.ExtraInstruction = wb
	}
	text, status := img2text.ExtractTextOnly(client, img64, contextText, textOpts, r.log, tid)
	if status != img2text.StatusOK {
		return "", fmt.Errorf("文本提取状态 %s: %s", status, truncateStr(text, 200))
	}
	text = stripImgTypeHeader(text)
	if strings.TrimSpace(text) == img2text.NoTextMarker {
		return "", nil
	}
	return text, nil
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
func (r *Runner) processVectorImage(mf *mdFile, t *task, pp *imageProgress, outDir string, tid int, prog map[string]*imageProgress, progDir string, mu *sync.Mutex) bool {
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
	contextText := ""
	if wb := r.wm.Block(); wb != "" {
		contextText = wb + "\n\n---\n\n"
	}
	contextText += strings.Join(img2text.GetContextLines(mf.lines, t.lineIdx,
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
		outDir, dstTex, dstPDF, dstPNG,
		FigureEnv{MDContent: mf.content, CurrentImg: t.imgPath, ImagesDir: r.cfg.Paths.ImagesDir, MaxUp: r.cfg.Options.MaxWindowUp, MaxDown: r.cfg.Options.MaxWindowDown, OutputLang: r.outputLang(),
			CurrentImgAbs: imgFile, ViewImageMax: r.cfg.ViewImageMax(), ViewPDFMax: r.cfg.ViewPDFMax(), ViewWarnRatio: r.cfg.ViewWarnRatio()},
		r.log, tid, r.keepTemp(), r.keepRecords())
	if err != nil {
		pp.Error = err.Error()
		return false
	}
	if !res.Submitted {
		pp.Error = res.Reason
		return false
	}
	pp.TikzCode = res.Code
	if res.Uncertain {
		// 模型标注了不确定内容（% [?] 注释）：记警告便于人工复核。
		r.log.LogWarning(tid, "[tikz] 图形含不确定标注 [?]（建议人工复核）:", t.imgPath)
	}
	pp.FigPDF = "figures/" + filepath.Base(dstPDF)
	pp.FigPNG = "figures/" + filepath.Base(dstPNG)
	// Cross-page merge: mark every absorbed continuation image as
	// done+absorbed so it is never processed separately and its ref
	// is removed from the rebuilt markdown.
	if len(res.Merges) > 0 {
		pp.MergedRefs = append(pp.MergedRefs, res.Merges...)
		saveProgress(progDir, pp)
	}
	for _, imgPath := range res.Merges {
		key := t.mdName + "::" + imgPath
		mu.Lock()
		ap, ok := prog[key]
		if ok {
			ap.Status = "done"
			ap.Absorbed = true
			ap.Error = ""
			saveProgress(progDir, ap)
		}
		mu.Unlock()
		if !ok {
			r.log.LogWarning(tid, "[vector] merge 目标不在任务表中:", imgPath)
		} else {
			r.log.Log(tid, "[vector] 已合并跨页续片:", imgPath)
		}
	}
	// Markdown 无法内嵌 PDF：用 PDF→SVG 后端转换后嵌入（dvisvgm →
	// pdftocairo → mutool → inkscape 依次尝试；失败时回退 PNG/PDF 链接，
	// 仅记录警告）。
	if !r.inline { // 档位1 内嵌 tikz 代码，无需 SVG
		dstSVG := filepath.Join(outDir, "figures", name+".svg")
		backend, err := ConvertPDFToSVG(dstPDF, dstSVG)
		if err != nil {
			// 矢量产物已就绪但 SVG 化失败：降级为 PNG/PDF 链接。
			// 按 ERROR 记录并标记（markdown 重建时就地标注，便于查找）。
			pp.SVGFail = true
			pp.Error = "svg 转换失败: " + err.Error()
			r.log.LogError(tid, "[vector] SVG 转换失败（降级 PNG/PDF 链接，已在 markdown 标注）:", err)
		} else {
			pp.FigSVG = "figures/" + name + ".svg"
			if backend != "dvisvgm" {
				r.log.Debug(tid, "[vector] SVG 由", backend, "生成:", pp.FigSVG)
			}
		}
	}
	return true
}

// ------------------------------------------------------------------
// phase 3: markdown rebuild
// ------------------------------------------------------------------

func (r *Runner) rebuildPhase(mdCache map[string]*mdFile, prog map[string]*imageProgress, outDir string) error {
	// Cross-page merges win over any per-fragment state: refs absorbed
	// by a combined figure are always deleted from the markdown.
	absorbed := map[string]bool{}
	for _, p := range prog {
		if p.Status == "done" {
			for _, ref := range p.MergedRefs {
				absorbed[ref] = true
			}
		}
	}
	r.log.Log(0, "\nRebuilding markdown files...")
	for name, mf := range mdCache {
		type rep struct {
			off [2]int
			p   *imageProgress
		}
		var reps []rep
		for k, p := range prog {
			parts := strings.SplitN(k, "::", 2)
			if len(parts) != 2 || parts[0] != name ||
				(p.Status != "done" && p.Status != "fallback") {
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
			if rp.p.Absorbed || absorbed[rp.p.ImgPath] {
				// 已并入前面的合并图：直接删除该引用
				nc = nc[:rp.off[0]] + nc[rp.off[1]:]
				continue
			}
			if rp.p.Status == "fallback" {
				// 矢量转换失败：保留原图引用；下次运行会自动重试该图
				// （状态记在 progress.json）。
				if r.inline {
					// 档位1：就地标注便于全局搜索定位（注释不进 .tex）。
					note := "<!-- DOCVISION-ERROR: 矢量图转换失败，已保留原图（下次运行自动重试）"
					if rp.p.Error != "" {
						note += " | " + mdCommentSafe(rp.p.Error)
					}
					note += " -->"
					nc = nc[:rp.off[0]] + nc[rp.off[0]:rp.off[1]] + " " + note + nc[rp.off[1]:]
				}
				// 档位2：产物必须是干净 markdown——不加任何注释，
				// 失败信息只进日志与 progress.json。
				r.log.LogWarning(0, "[vector] 转换失败，保留原图（下次自动重试）:", rp.p.ImgPath)
				continue
			}
			block := r.embedBlock(rp.p, name, outDir)
			if block == "" {
				continue
			}
			if rp.p.SVGFail && r.inline {
				block += " <!-- DOCVISION-ERROR: SVG 转换失败，已降级为位图/PDF 链接 -->"
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
			// No readable text extracted (or extraction degraded): keep
			// the original image instead of dropping content.
			return r.rasterBlock(p, mdName, outDir)
		}
		if p.Styled && r.inline {
			// 档位1：样式化文本图。注释首行即闭合（`-->` 在行尾），
			// CONTENT/LINK 在注释外随行列出；LINK 用 [styled-text](路径)
			// 链接形式。转换会话按手册重排或 includegraphics；整块
			// 注释+字段不进入 .tex。档位2 不做样式保留。
			noteText := p.StyleNote
			if strings.TrimSpace(noteText) == "" {
				noteText = "样式未提供，见原图"
			}
			var b strings.Builder
			b.WriteString("<!-- DOCVISION-STYLED-TEXT: " + mdCommentSafe(noteText) + " -->\n")
			b.WriteString("CONTENT: " + mdCommentBody(p.Content) + "\n")
			if rel, ok := r.copyOriginalImage(p, mdName, outDir); ok {
				b.WriteString("LINK: [styled-text](" + rel + ")\n")
			}
			return strings.TrimRight(b.String(), "\n")
		}
		// 档位2（以及档位1 的非样式化文本图）：直接把提取到的原文
		// 嵌入正文，不保留样式、不加任何标记。
		return p.Content
	case ClassVector:
		if r.inline {
			// 档位1：直接内嵌 LaTeX 代码，转换 AI 原样粘贴进 .tex，
			// 不产生也不引用 figures/*.pdf 资源。代码块前保留一条
			// 机器注释指向原图，下游转换会话可据此找到原始图像/页面。
			if p.TikzCode != "" {
				if rel, ok := r.copyOriginalImage(p, mdName, outDir); ok {
					// 注释首行闭合；LINK 在注释外、latex 围栏上方，
					// 用 [vector](路径) 链接形式指向原图。
					label := p.Label
					if label == "" {
						label = "vector figure"
					}
					head := "<!-- DOCVISION-VECTOR: " + mdCommentSafe(label) + " -->\n" +
						"LINK: [vector](" + rel + ")"
					return head + "\n```latex\n" + p.TikzCode + "\n```"
				}
				return "```latex\n" + p.TikzCode + "\n```"
			}
			if p.Content != "" {
				return p.Content
			}
			return ""
		}
		if p.Kept || p.FigPDF == "" {
			return r.rasterBlock(p, mdName, outDir)
		}
		label := p.Label
		if label == "" {
			label = "figure"
		}
		// 档位2 只当普通 markdown 看：矢量图一律嵌入 SVG（Markdown
		// 不支持内嵌 PDF），SVG 不可用时退回 PNG / PDF 链接；
		// 绝不嵌入 latex 代码块、绝不加任何注释。
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
// output tree) or embeds the readable explanation. 档位1 有描述时用
// 注释携带解释（首行闭合），原图本身仍是图片形式紧随其后——图就是
// 正文，不能变成链接；档位2 不受影响。
func (r *Runner) rasterBlock(p *imageProgress, mdName, outDir string) string {
	// 档位2：insert_image_description 开关控制——开启且已有解释时嵌入
	// "[Image]( content )"（v1.4 规格），否则纯原图引用。
	if !r.inline && r.cfg.Latex.InsertImageDescription && p.Content != "" {
		return "[Image]( " + p.Content + " )"
	}
	rel, ok := r.copyOriginalImage(p, mdName, outDir)
	if !ok {
		return ""
	}
	// 档位1：process 阶段已为该图生成解释文本（复用 img2text 提取，
	// 不依赖 insert_image_description——那是档位2 的开关）。有就进
	// DESCRIBE 块；没有（无文本/提取失败）就只给 LINK。注释块整体不
	// 进入 .tex；LINK 指向的原图由转换会话 includegraphics。
	if r.inline {
		if p.Content != "" {
			label := p.Label
			if label == "" {
				label = "image"
			}
			return "<!-- DOCVISION-IMAGE: " + mdCommentSafe(label) + " -->\n" +
				"DESCRIBE: " + mdCommentBody(p.Content) + "\n" +
				"LINK: [image](" + rel + ")"
		}
		return "LINK: [image](" + rel + ")"
	}
	// 档位2 无解释：直接保留原图引用。
	return "![image](" + rel + ")"
}

// copyOriginalImage copies the source image into outDir/images and
// returns the markdown-relative link path ("" on failure).
func (r *Runner) copyOriginalImage(p *imageProgress, mdName, outDir string) (string, bool) {
	src, err := resolveImageFile(r.cfg.Paths.ImagesDir, p.ImgPath, subjectOf(mdName))
	if err != nil {
		r.log.LogWarning(0, "保留原图失败（文件缺失）:", p.ImgPath)
		return "", false
	}
	rel := p.ImgPath
	if i := strings.Index(rel, "/"); i >= 0 {
		rel = rel[i+1:]
	}
	dst := filepath.Join(outDir, "images", rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", false
	}
	if !fileExists(dst) {
		if err := copyFile(src, dst); err != nil {
			r.log.LogWarning(0, "复制原图失败:", src, err)
			return "", false
		}
	}
	return "images/" + rel, true
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

// mdCommentSafe strips characters that could break an HTML comment
// (newlines and the "--" sequence) from error text embedded in markdown.
func mdCommentSafe(s string) string {
	s = strings.ReplaceAll(s, "--", "—")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// mdCommentBody keeps the line structure of an HTML-comment payload
// (CONTENT: may be a multi-line list) but neutralises sequences that
// would terminate the comment.
func mdCommentBody(s string) string {
	s = strings.ReplaceAll(s, "--", "—")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

// migrateProgress upgrades legacy progress entries written by older
// versions:
//   - vector items recorded as done+kept (回退即完成) become "fallback"
//     so the next run retries the vector conversion;
//   - items with a failed SVG conversion get a free dvisvgm retry when
//     the figure PDF still exists (no AI session needed).
func (r *Runner) migrateProgress(prog map[string]*imageProgress, progDir, outDir string) {
	for _, p := range prog {
		if p.Class == ClassVector && p.Status == "done" && p.Kept && !p.Absorbed {
			p.Status = "fallback"
			saveProgress(progDir, p)
			r.log.Log(0, "[migrate] 旧版回退条目改为可重试:", p.ImgPath)
			continue
		}
		if p.SVGFail && p.Status == "done" && p.FigPDF != "" && p.FigSVG == "" {
			if r.retrySVG(p, outDir) {
				saveProgress(progDir, p)
			}
		}
	}
}

// retrySVG re-runs the PDF→SVG conversion for a previously failed
// entry (no AI session needed).
func (r *Runner) retrySVG(p *imageProgress, outDir string) bool {
	svgRel := strings.TrimSuffix(p.FigPDF, filepath.Ext(p.FigPDF)) + ".svg"
	backend, err := ConvertPDFToSVG(filepath.Join(outDir, p.FigPDF), filepath.Join(outDir, svgRel))
	if err != nil {
		r.log.Debug(0, "[migrate] SVG 补跑仍失败:", p.ImgPath, err)
		return false
	}
	p.FigSVG = svgRel
	p.SVGFail = false
	p.Error = ""
	r.log.Log(0, "[migrate] SVG 转换补跑成功:", p.ImgPath, "("+backend+")")
	return true
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
