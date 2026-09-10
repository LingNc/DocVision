// Package latex implements the LaTeX output feature: per-image
// classification and vectorisation (level 2) and full-book LaTeX
// conversion (level 1), both built on top of the session engine.
package latex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// Compiler wraps the local LaTeX toolchain (engine + pdftoppm). All
// runs happen in dedicated scratch directories so the workspace stays
// clean.
type Compiler struct {
	engine  string
	timeout time.Duration
	raster  string
	dpi     int
	// fontsDir 项目字体目录（可空）。非空时每次编译注入 TEXINPUTS /
	// OSFONTDIR，cls 里 \setmainfont{字体文件名} 可直接命中用户手动
	// 放入的字体文件。
	fontsDir string
}

// NewCompiler builds a Compiler from the latex compile config.
func NewCompiler(cfg config.LatexCompileConfig) *Compiler {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	engine := cfg.Engine
	if engine == "" {
		engine = "xelatex"
	}
	raster := cfg.RasterCommand
	if raster == "" {
		raster = "pdftoppm"
	}
	dpi := cfg.RasterDPI
	if dpi <= 0 {
		dpi = 110
	}
	return &Compiler{
		engine:  engine,
		timeout: timeout,
		raster:  raster,
		dpi:     dpi,
	}
}

// Available reports whether the configured engine and raster tool are
// installed. Used for auto-mode style degradation (mirroring the
// tools.mermaid.validation behaviour of img2text).
func (c *Compiler) Available() error {
	for _, bin := range []string{c.engine, c.raster} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("未找到 %s：请安装 TeX 发行版（含 %s）与 poppler-utils（%s）", bin, c.engine, c.raster)
		}
	}
	return nil
}

// CompileResult captures one compile attempt.
type CompileResult struct {
	OK  bool
	Log string // full log output (bounded)
	PDF string // absolute path when OK
	Err string // human-readable error summary when failed
	// Warnings are the deduplicated warning lines (Overfull/Underfull
	// boxes, LaTeX/Package/Class warnings), bounded; WarningCount is the
	// total number seen. The full log stays out of the AI context and
	// the debug log — only this summary travels.
	Warnings     []string
	WarningCount int
}

// errExtractors cut noisy LaTeX logs down to the interesting lines.
var errExtractors = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^! .*$`),
	regexp.MustCompile(`(?m)^l\.\d+.*$`),
}

// warnExtractors collect the warning lines worth reporting.
var warnExtractors = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^(?:LaTeX|Package|Class|Module)\b.*Warning:.*$`),
	regexp.MustCompile(`(?m)^(?:Overfull|Underfull) \\[hv]box.*$`),
}

// extractLatexWarnings deduplicates warning lines and bounds the list
// while keeping the true total count.
func extractLatexWarnings(logText string, limit int) ([]string, int) {
	if limit <= 0 {
		limit = 20
	}
	seen := map[string]bool{}
	var out []string
	total := 0
	for _, re := range warnExtractors {
		for _, m := range re.FindAllString(logText, -1) {
			line := strings.TrimSpace(m)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			total++
			if len(out) < limit {
				out = append(out, truncateStr(line, 300))
			}
		}
	}
	return out, total
}

// WarningSummary renders the bounded warning summary ("" when none).
func (r CompileResult) WarningSummary() string {
	if r.WarningCount == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "WARNINGS (%d", r.WarningCount)
	if r.WarningCount > len(r.Warnings) {
		fmt.Fprintf(&b, ", showing first %d", len(r.Warnings))
	}
	b.WriteString("):")
	for _, w := range r.Warnings {
		b.WriteString("\n  - " + w)
	}
	return b.String()
}

// CompileOptions tunes one build (multi-pass, bibliography, engine
// override, extra flags). The zero value is the historical single-pass
// behaviour.
type CompileOptions struct {
	// Engine overrides the configured engine: xelatex / pdflatex /
	// lualatex, or "latexmk" for a full multi-pass build that also
	// resolves the bibliography automatically.
	Engine string
	// Passes is the number of engine runs. 0 = auto (2 when the main
	// file needs a second pass for refs/toc/bibliography).
	Passes int
	// Bib runs "bibtex" or "biber" once, after the first pass.
	Bib string
	// ShellEscape adds -shell-escape (minted, externalised pgfplots).
	ShellEscape bool
	// ExtraArgs are appended to every engine invocation.
	ExtraArgs []string
	// Timeout overrides the configured per-run timeout.
	Timeout time.Duration
}

// Compile runs the engine once inside dir for mainFile (basename).
// The output log is returned bounded; on success the produced PDF path
// is resolved.
func (c *Compiler) Compile(dir, mainFile string) CompileResult {
	return c.CompileOpts(dir, mainFile, CompileOptions{})
}

// CompileFull builds a multi-file document the way a human would:
// latexmk when it is installed (handles passes, bib/biber, glossaries,
// makeindex), otherwise two engine passes. Used for the assembled book.
func (c *Compiler) CompileFull(dir, mainFile string) CompileResult {
	if _, err := exec.LookPath("latexmk"); err == nil {
		return c.CompileOpts(dir, mainFile, CompileOptions{Engine: "latexmk"})
	}
	return c.CompileOpts(dir, mainFile, CompileOptions{Passes: 2})
}

// CompileOpts runs the LaTeX toolchain with explicit options. Errors
// and warnings are summarised; the full log stays bounded.
func (c *Compiler) CompileOpts(dir, mainFile string, o CompileOptions) CompileResult {
	engine := strings.TrimSpace(o.Engine)
	if engine == "" {
		engine = c.engine
	}
	timeout := c.timeout
	if o.Timeout > 0 {
		timeout = o.Timeout
	}
	if engine == "latexmk" {
		return c.runLatexmk(dir, mainFile, o, timeout)
	}
	passes := o.Passes
	if passes <= 0 {
		passes = 1
		if needsAuxPass(dir, mainFile) {
			passes = 2
		}
	}
	var res CompileResult
	for i := 0; i < passes; i++ {
		res = c.runEngine(dir, mainFile, engine, o, timeout)
		if !res.OK {
			return res
		}
		if i == 0 && o.Bib != "" {
			if msg := runBibTool(dir, mainFile, o.Bib, timeout); msg != "" {
				res.Warnings = append(res.Warnings, msg)
				res.WarningCount++
			}
		}
	}
	return res
}

// runEngine executes one engine pass.
func (c *Compiler) runEngine(dir, mainFile, engine string, o CompileOptions, timeout time.Duration) CompileResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"-interaction=nonstopmode", "-halt-on-error", "-file-line-error"}
	if o.ShellEscape {
		args = append(args, "-shell-escape")
	}
	args = append(args, o.ExtraArgs...)
	args = append(args, mainFile)
	cmd := exec.CommandContext(ctx, engine, args...)
	cmd.Dir = dir
	cmd.Env = c.buildEnv()
	out, err := cmd.CombinedOutput()
	logText := string(out)
	if len(logText) > 65536 {
		logText = logText[:32768] + "\n...[log truncated]...\n" + logText[len(logText)-32768:]
	}
	return c.result(dir, mainFile, logText, err, ctx.Err() == context.DeadlineExceeded, timeout)
}

// runLatexmk drives a full latexmk build (multi-pass + bibliography).
func (c *Compiler) runLatexmk(dir, mainFile string, o CompileOptions, timeout time.Duration) CompileResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout*3)
	defer cancel()
	mode := "-pdf"
	switch strings.ToLower(c.engine) {
	case "xelatex":
		mode = "-xelatex"
	case "lualatex":
		mode = "-lualatex"
	}
	args := []string{mode, "-interaction=nonstopmode", "-file-line-error", "-halt-on-error"}
	if o.ShellEscape {
		args = append(args, "-shell-escape")
	}
	args = append(args, o.ExtraArgs...)
	args = append(args, mainFile)
	cmd := exec.CommandContext(ctx, "latexmk", args...)
	cmd.Dir = dir
	cmd.Env = c.buildEnv()
	out, err := cmd.CombinedOutput()
	logText := string(out)
	if len(logText) > 65536 {
		logText = logText[:32768] + "\n...[log truncated]...\n" + logText[len(logText)-32768:]
	}
	return c.result(dir, mainFile, logText, err, ctx.Err() == context.DeadlineExceeded, timeout*3)
}

// result turns one finished process into a CompileResult.
func (c *Compiler) result(dir, mainFile, logText string, err error, timedOut bool, timeout time.Duration) CompileResult {
	res := CompileResult{Log: logText}
	res.Warnings, res.WarningCount = extractLatexWarnings(logText, 20)
	base := strings.TrimSuffix(mainFile, filepath.Ext(mainFile))
	pdf := filepath.Join(dir, base+".pdf")
	if err == nil {
		if _, statErr := os.Stat(pdf); statErr == nil {
			res.OK = true
			res.PDF = pdf
			return res
		}
		res.Err = "编译命令成功但未生成 PDF"
		return res
	}
	if timedOut {
		res.Err = fmt.Sprintf("编译超时（>%ds）", int(timeout.Seconds()))
		return res
	}
	res.Err = summarizeLatexError(logText, err)
	return res
}

// buildEnv is the process environment for every LaTeX run: permissive
// file IO plus the project font directory.
func (c *Compiler) buildEnv() []string {
	env := append(os.Environ(), "openout_any=a", "openin_any=a")
	if c.fontsDir != "" {
		if abs, err := filepath.Abs(c.fontsDir); err == nil {
			// TEXINPUTS 末尾双斜杠 = 递归检索；OSFONTDIR 让 fontspec
			// 按文件名找到项目字体。
			env = append(env, "TEXINPUTS="+abs+string(os.PathListSeparator), "OSFONTDIR="+abs)
		}
	}
	return env
}

// needsAuxPass reports whether the document needs a second pass (TOC,
// cross-references, bibliography). It looks at the main file plus the
// files it \input/\include s, one level deep.
func needsAuxPass(dir, mainFile string) bool {
	markers := []string{"\\tableofcontents", "\\bibliography", "\\addbibresource",
		"\\printbibliography", "\\ref{", "\\pageref{", "\\cite{", "\\cite[", "\\label{"}
	seen := map[string]bool{}
	var check func(rel string, depth int) bool
	check = func(rel string, depth int) bool {
		if seen[rel] || depth > 2 {
			return false
		}
		seen[rel] = true
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return false
		}
		text := string(data)
		for _, m := range markers {
			if strings.Contains(text, m) {
				return true
			}
		}
		re := regexp.MustCompile(`\\(?:input|include)\{([^}]+)\}`)
		for _, mm := range re.FindAllStringSubmatch(text, -1) {
			inc := strings.TrimSpace(mm[1])
			if inc == "" {
				continue
			}
			if !strings.HasSuffix(inc, ".tex") {
				inc += ".tex"
			}
			if check(inc, depth+1) {
				return true
			}
		}
		return false
	}
	return check(mainFile, 0)
}

// runBibTool runs bibtex/biber once; it returns a warning line on
// failure (a missing .bib is common and non-fatal).
func runBibTool(dir, mainFile, bib string, timeout time.Duration) string {
	tool := strings.ToLower(strings.TrimSpace(bib))
	if tool != "bibtex" && tool != "biber" {
		return ""
	}
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Sprintf("%s 未安装（引用未解析）", tool)
	}
	base := strings.TrimSuffix(filepath.Base(mainFile), filepath.Ext(mainFile))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, base)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s 失败: %s", tool, truncateStr(lastNonEmptyString(string(out)), 200))
	}
	return ""
}

// summarizeLatexError extracts the meaningful error lines from a TeX
// log so the AI repair prompt stays compact.
func summarizeLatexError(logText string, execErr error) string {
	var lines []string
	for _, re := range errExtractors {
		for _, m := range re.FindAllString(logText, 8) {
			lines = append(lines, m)
		}
	}
	tail := lastNonEmptyLines(logText, 12)
	for _, t := range tail {
		if !containsLine(lines, t) {
			lines = append(lines, t)
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%v", execErr)
	}
	summary := strings.Join(lines, "\n")
	if len(summary) > 4096 {
		summary = summary[:4096] + "..."
	}
	return summary
}

func containsLine(lines []string, l string) bool {
	for _, x := range lines {
		if x == l {
			return true
		}
	}
	return false
}

// lastNonEmptyLines returns up to n trailing non-empty lines of a log.
func lastNonEmptyLines(s string, n int) []string {
	raw := strings.Split(strings.TrimSpace(s), "\n")
	var out []string
	for i := len(raw) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.TrimSpace(raw[i])
		if l == "" {
			continue
		}
		out = append([]string{l}, out...)
	}
	return out
}

// Rasterize renders the first page of pdfPath to outPNG (no extension;
// pdftoppm -singlefile appends nothing).
func (c *Compiler) Rasterize(pdfPath, outPNG string) error {
	if _, err := exec.LookPath(c.raster); err != nil {
		return fmt.Errorf("未找到 %s", c.raster)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.raster,
		"-png", "-r", strconv.Itoa(c.dpi), "-singlefile",
		pdfPath, outPNG,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("栅格化失败: %v: %s", err, truncateStr(string(out), 400))
	}
	return nil
}

// RasterizePages renders pages first..last (1-based) of pdfPath to
// outPNG (no extension; used for on-demand single-page rendering).
func (c *Compiler) RasterizePages(pdfPath string, first, last int, outPNG string) error {
	if _, err := exec.LookPath(c.raster); err != nil {
		return fmt.Errorf("未找到 %s", c.raster)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.raster,
		"-png", "-r", strconv.Itoa(c.dpi), "-singlefile",
		"-f", strconv.Itoa(first), "-l", strconv.Itoa(last),
		pdfPath, outPNG,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("栅格化失败: %v: %s", err, truncateStr(string(out), 400))
	}
	return nil
}

// RasterizeAll renders every page of pdfPath to outBase-1.png,
// outBase-2.png, ...
func (c *Compiler) RasterizeAll(pdfPath, outBase string) ([]string, error) {
	if _, err := exec.LookPath(c.raster); err != nil {
		return nil, fmt.Errorf("未找到 %s", c.raster)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.raster, "-png", "-r", strconv.Itoa(c.dpi), pdfPath, outBase)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	matches, _ := filepath.Glob(outBase + "-*.png")
	return matches, nil
}

// LogCompileResult writes one compact [compile] line to the debug log:
// OK / compile error, duration, warning count, the bounded warning list
// and — on failure — exactly the error text handed to the AI. The full
// LaTeX log is never dumped (too long to be useful). Interactive compile
// failures (figure/chapter iteration) are marked as "returned to the AI
// for repair" so they are not mistaken for a failed session.
func LogCompileResult(log *logger.Logger, tid int, tag string, res CompileResult, elapsed time.Duration) {
	if log == nil || !log.DebugEnabled() {
		return
	}
	status := "OK"
	if !res.OK {
		status = "compile error"
	}
	line := fmt.Sprintf("[compile:%s] %s (%.1fs)", tag, status, elapsed.Seconds())
	if res.WarningCount > 0 {
		line += fmt.Sprintf(" warnings=%d", res.WarningCount)
	}
	if !res.OK && isInteractiveCompile(tag) {
		line += " → 已返回 AI 修复"
	}
	log.Debug(tid, line)
	if w := res.WarningSummary(); w != "" {
		log.Debug(tid, "[compile:"+tag+"] "+w)
	}
	if !res.OK {
		log.Debug(tid, "[compile:"+tag+"] error returned to AI:\n"+truncateStr(res.Err, 2000))
	}
}

// isInteractiveCompile reports whether a failed compile is just feedback
// for the model (figure/chapter iteration) rather than a hard failure.
func isInteractiveCompile(tag string) bool {
	switch tag {
	case "preview", "chapter", "convert-check":
		return true
	}
	return false
}

// ReadImageFile loads a PNG/JPEG file as base64 (no data: prefix).
func ReadImageFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return b64Encode(data), nil
}

// svgBackend is one PDF→SVG converter. run is invoked with the absolute
// input/output paths and must produce outSVG.
type svgBackend struct {
	name string
	run  func(pdf, svg string) error
}

// svgBackends lists the supported PDF→SVG converters in preference
// order. dvisvgm is fastest but needs Ghostscript < 10.01 or mutool for
// PDF input (newer Ghostscript is rejected: "Ghostscript version 10.05.1
// is not supported"); pdftocairo comes with poppler-utils, which the
// pipeline already requires for rasterisation; mutool/inkscape are
// optional extras.
func svgBackends() []svgBackend {
	return []svgBackend{
		{name: "dvisvgm", run: func(pdf, svg string) error {
			return runSVGTool(120*time.Second, svg, "dvisvgm", "--pdf", "--exact", "--output="+svg, pdf)
		}},
		{name: "pdftocairo", run: func(pdf, svg string) error {
			return runSVGTool(120*time.Second, svg, "pdftocairo", "-svg", pdf, svg)
		}},
		{name: "mutool", run: func(pdf, svg string) error {
			return runSVGTool(120*time.Second, svg, "mutool", "draw", "-F", "svg", "-o", svg, pdf)
		}},
		{name: "inkscape", run: func(pdf, svg string) error {
			return runSVGTool(180*time.Second, svg, "inkscape", "--pdf-poppler",
				"--export-type=svg", "--export-filename="+svg, pdf)
		}},
	}
}

// runSVGTool runs one converter, returning a compact error. out is the
// file the backend is expected to create.
func runSVGTool(timeout time.Duration, out, bin string, args ...string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("未安装 %s", bin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", err, truncateStr(lastNonEmptyString(msg), 300))
	}
	if _, statErr := os.Stat(out); statErr != nil {
		return fmt.Errorf("%s 未生成 SVG", bin)
	}
	return nil
}

// lastNonEmptyString keeps the most informative tail of a tool message.
func lastNonEmptyString(s string) string {
	lines := lastNonEmptyLines(s, 3)
	if len(lines) == 0 {
		return s
	}
	return strings.Join(lines, " | ")
}

// ConvertPDFToSVG converts pdfPath to svgPath, trying every available
// backend until one succeeds. It returns the backend name that produced
// the file, or an error listing what each backend reported (so the log
// shows WHY the conversion failed instead of a bare exit status).
func ConvertPDFToSVG(pdfPath, svgPath string) (string, error) {
	var failures []string
	for _, b := range svgBackends() {
		os.Remove(svgPath) // never accept a stale file from a previous attempt
		if err := b.run(pdfPath, svgPath); err != nil {
			failures = append(failures, b.name+"("+err.Error()+")")
			continue
		}
		if info, err := os.Stat(svgPath); err == nil && info.Size() > 0 {
			return b.name, nil
		}
		failures = append(failures, b.name+"(输出为空)")
	}
	return "", fmt.Errorf("所有 PDF→SVG 后端均失败: %s", strings.Join(failures, "; "))
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
