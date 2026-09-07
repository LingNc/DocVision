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
)

// Compiler wraps the local LaTeX toolchain (engine + pdftoppm). All
// runs happen in dedicated scratch directories so the workspace stays
// clean.
type Compiler struct {
	engine  string
	timeout time.Duration
	raster  string
	dpi     int
}

// NewCompiler builds a Compiler from the latex compile config.
func NewCompiler(cfg config.LatexCompileConfig) *Compiler {
	return &Compiler{
		engine:  cfg.Engine,
		timeout: time.Duration(cfg.Timeout) * time.Second,
		raster:  cfg.RasterCommand,
		dpi:     cfg.RasterDPI,
	}
}

// Available reports whether the configured engine and raster tool are
// installed. Used for auto-mode style degradation (mirroring the
// mermaid_validation behaviour of img2text).
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
}

// errExtractors cut noisy LaTeX logs down to the interesting lines.
var errExtractors = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^! .*$`),
	regexp.MustCompile(`(?m)^l\.\d+.*$`),
}

// Compile runs the engine once inside dir for mainFile (basename).
// The output log is returned bounded; on success the produced PDF path
// is resolved.
func (c *Compiler) Compile(dir, mainFile string) CompileResult {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.engine,
		"-interaction=nonstopmode",
		"-halt-on-error",
		"-file-line-error",
		mainFile,
	)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "openout_any=a", "openin_any=a")

	out, err := cmd.CombinedOutput()
	logText := string(out)
	if len(logText) > 65536 {
		logText = logText[:32768] + "\n...[log truncated]...\n" + logText[len(logText)-32768:]
	}

	res := CompileResult{Log: logText}
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
	if ctx.Err() == context.DeadlineExceeded {
		res.Err = fmt.Sprintf("编译超时（>%ds）", int(c.timeout.Seconds()))
		return res
	}
	res.Err = summarizeLatexError(logText, err)
	return res
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

// ReadImageFile loads a PNG/JPEG file as base64 (no data: prefix).
func ReadImageFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return b64Encode(data), nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
