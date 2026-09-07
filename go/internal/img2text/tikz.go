package img2text

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var tikzBlockRe = regexp.MustCompile("(?is)```[ \t]*(?:tikz|latex)[ \t]*\\r?\\n(.*?)```") // latex 为主，tikz 为旧格式兼容

// ExtractTikZBlocks returns the contents of fenced TikZ code blocks.
func ExtractTikZBlocks(text string) []string {
	matches := tikzBlockRe.FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, m := range matches {
		blocks = append(blocks, strings.TrimSpace(m[1]))
	}
	return blocks
}

// hasMermaidBlock reports whether the text contains a fenced mermaid
// block (tikz check is skipped when the response is mermaid-based).
func hasMermaidBlock(text string) bool { return len(ExtractMermaidBlocks(text)) > 0 }

// resolveTikzEngine picks the LaTeX engine for the compile check:
// prefer the configured engine, fall back to any installed one.
func resolveTikzEngine(preferred string) string {
	candidates := []string{preferred, "xelatex", "pdflatex", "lualatex"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

// ValidateTikZ compiles the TikZ blocks in text (standalone wrapper)
// with the LaTeX toolchain. Responses without tikz blocks are valid by
// definition, mirroring ValidateMermaid's contract. A missing engine is
// reported as unavailable so the caller's auto/strict policy applies.
func ValidateTikZ(ctx context.Context, text, engine string, timeout time.Duration) MermaidValidationResult {
	blocks := ExtractTikZBlocks(text)
	if len(blocks) == 0 {
		return MermaidValidationResult{Valid: true}
	}
	engine = resolveTikzEngine(engine)
	if engine == "" {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  false,
			Error:      "LaTeX engine (xelatex/pdflatex/lualatex) not found for TikZ validation",
		}
	}
	for i, block := range blocks {
		dir, err := os.MkdirTemp("", "dsv-tikz-")
		if err != nil {
			return MermaidValidationResult{HasMermaid: true, Available: true, Error: err.Error()}
		}
		defer os.RemoveAll(dir)
		tex := filepath.Join(dir, "check.tex")
		content := buildTikzStandalone(engine, block)
		if err := os.WriteFile(tex, []byte(content), 0o644); err != nil {
			return MermaidValidationResult{HasMermaid: true, Available: true, Error: err.Error()}
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(cctx, engine, "-interaction=nonstopmode", "-halt-on-error", "check.tex")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			return MermaidValidationResult{
				HasMermaid: true,
				Available:  true,
				Valid:      false,
				Error:      fmt.Sprintf("tikz block %d failed: %v: %s", i+1, err, truncateOut(string(out), 400)),
			}
		}
	}
	return MermaidValidationResult{HasMermaid: true, Available: true, Valid: true}
}

// buildTikzStandalone wraps a TikZ body into a compilable standalone
// document. Chinese-capable engines get ctex; common TikZ libraries are
// preloaded so model output does not need usetikzlibrary hoisting.
func buildTikzStandalone(engine, body string) string {
	var b strings.Builder
	b.WriteString("\\documentclass[tikz,border=6pt]{standalone}\n")
	if engine == "xelatex" || engine == "lualatex" {
		b.WriteString("\\usepackage{ctex}\n")
	}
	b.WriteString("\\usetikzlibrary{arrows.meta,positioning,shapes,calc,patterns}\n")
	b.WriteString("\\usepackage{amsmath,amssymb}\n")
	b.WriteString("\\begin{document}\n")
	b.WriteString(body)
	b.WriteString("\n\\end{document}\n")
	return b.String()
}

// buildTikzRepairMessage is the repair-prompt builder for failed TikZ
// compiles, mirroring the Mermaid repair contract.
func buildTikzRepairMessage(currentResult, validationError string) string {
	return "Your previous response contained a TikZ block that FAILED to compile." +
		"\n\nLaTeX error:\n" + validationError +
		"\n\nPreserve the [IMG_TYPE:] prefix and ALL non-TikZ content, and return the SAME response with the TikZ block corrected." +
		"\nKeep the ```tikz fence. Use only standard TikZ/pgfplots; if Chinese text appears the engine already loads ctex." +
		"\nDo NOT write any explanation - output the corrected full response."
}

func truncateOut(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
