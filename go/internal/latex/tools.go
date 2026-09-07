package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mineru-tools/internal/session"
)

// ------------------------------------------------------------------
// TikZ figure session tools (level 2 vector path)
// ------------------------------------------------------------------

// tikzState carries the live state of one figure-drawing session.
type tikzState struct {
	workDir    string // scratch dir for the standalone wrapper
	lastCode   string
	lastPDF    string // absolute path of the last successful compile
	compileOK  bool
	submitted  bool
	finalCode  string
	compileErr string   // last error, reported by the runner on give-up
	merges     []string // image paths absorbed into this figure (cross-page merge)
}

// CompilePreviewTool compiles model-supplied TikZ body code inside a
// standalone wrapper, rasterises the PDF and feeds the preview PNG
// back to the model.
type CompilePreviewTool struct {
	Comp       *Compiler
	State      *tikzState
	EngineIsXe bool
}

func (t *CompilePreviewTool) Name() string { return "compile_preview" }

func (t *CompilePreviewTool) Definition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "compile_preview",
			"description": "Compile TikZ body code and receive the compile log plus a rasterised PNG preview of the figure for visual comparison with the original image.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{
						"type":        "string",
						"description": "The TikZ code that goes between \\begin{document} and \\end{document}. Include \\usetikzlibrary / \\usepgfplotslibrary lines at the top; they are hoisted into the preamble.",
					},
				},
				"required": []string{"code"},
			},
		},
	}
}

func (t *CompilePreviewTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	code, _ := args["code"].(string)
	if arr, ok := args["merges"].([]interface{}); ok {
		for _, v := range arr {
			if p, ok := v.(string); ok && strings.TrimSpace(p) != "" {
				t.State.merges = append(t.State.merges, strings.TrimSpace(p))
			}
		}
	}
	if strings.TrimSpace(code) == "" {
		return session.ToolResult{}, fmt.Errorf("code 为空")
	}
	t.State.lastCode = code
	t.State.compileOK = false
	t.State.lastPDF = ""

	texFile := filepath.Join(t.State.workDir, "figure.tex")
	if err := os.WriteFile(texFile, []byte(buildStandalone(code, t.EngineIsXe)), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	// Clear previous outputs so a stale PDF can never pass as fresh.
	os.Remove(filepath.Join(t.State.workDir, "figure.pdf"))
	os.Remove(filepath.Join(t.State.workDir, "figure.png"))

	res := t.Comp.Compile(t.State.workDir, "figure.tex")
	if !res.OK {
		t.State.compileErr = res.Err
		return session.ToolResult{
			Text: "COMPILE FAILED. Fix the code and call compile_preview again.\nError:\n" + res.Err,
		}, nil
	}
	png := filepath.Join(t.State.workDir, "figure") // pdftoppm appends .png
	if err := t.Comp.Rasterize(res.PDF, png); err != nil {
		t.State.compileErr = err.Error()
		return session.ToolResult{Text: "Compiled OK but rasterisation failed: " + err.Error()}, nil
	}
	t.State.compileOK = true
	t.State.lastPDF = res.PDF
	t.State.compileErr = ""
	img64, err := ReadImageFile(png + ".png")
	if err != nil {
		return session.ToolResult{Text: "Compiled OK but preview could not be loaded: " + err.Error()}, nil
	}
	return session.ToolResult{
		Text:        "COMPILE OK. The preview PNG is attached. Compare it with the original image; if it faithfully matches, call submit; otherwise fix the differences and compile again.",
		ImageBase64: img64,
		ImageMIME:   "image/png",
	}, nil
}

// SubmitFigureTool records the model's confirmed final TikZ code.
type SubmitFigureTool struct {
	State *tikzState
}

func (t *SubmitFigureTool) Name() string { return "submit" }

func (t *SubmitFigureTool) Definition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "submit",
			"description": "Submit the final TikZ code. Allowed only after a successful compile_preview of EXACTLY this code.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code":   map[string]any{"type": "string", "description": "The final TikZ body code (identical to the last successful compile_preview)."},
					"merges": map[string]any{"type": "string", "description": "Image paths (as in the markdown) of page-boundary continuations absorbed into this combined figure."},
				},
				"required": []string{"code"},
			},
		},
	}
}

func (t *SubmitFigureTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	code, _ := args["code"].(string)
	if strings.TrimSpace(code) == "" {
		return session.ToolResult{}, fmt.Errorf("code 为空")
	}
	if !t.State.compileOK || strings.TrimSpace(code) != strings.TrimSpace(t.State.lastCode) {
		return session.ToolResult{Text: "REJECTED: the submitted code differs from the last successful compile. Call compile_preview with this exact code first."}, nil
	}
	t.State.submitted = true
	t.State.finalCode = code
	return session.ToolResult{Text: "SUBMITTED. The figure is accepted. Reply with a one-line confirmation and nothing else."}, nil
}

// buildStandalone wraps TikZ body code in a compilable standalone
// document. \usetikzlibrary / \usepgfplotslibrary / \usepackage /
// \tikzset / \pgfplotsset lines found in the body are hoisted into the
// preamble. ctex is loaded when the engine supports it so Chinese
// labels compile.
func buildStandalone(body string, engineIsXe bool) string {
	var hoisted []string
	preambleRe := regexp.MustCompile(`(?m)^\\(usetikzlibrary|usepgfplotslibrary|usetikzlibrary{arrows}|usepackage|tikzset|pgfplotsset)\b[^\n]*`)
	hoisted = preambleRe.FindAllString(body, -1)
	rest := strings.TrimSpace(preambleRe.ReplaceAllString(body, ""))

	pre := []string{}
	if engineIsXe {
		pre = append(pre, "\\usepackage{ctex}")
	}
	pre = append(pre, hoisted...)

	var b strings.Builder
	b.WriteString("\\documentclass[border=6pt]{standalone}\n")
	b.WriteString("\\usepackage{tikz}\n")
	b.WriteString("\\usepackage{pgfplots}\n")
	b.WriteString("\\pgfplotsset{compat=1.18}\n")
	b.WriteString("\\usepackage{amsmath,amssymb}\n")
	b.WriteString(strings.Join(pre, "\n"))
	b.WriteString("\n\\begin{document}\n")
	b.WriteString(rest)
	b.WriteString("\n\\end{document}\n")
	return b.String()
}
