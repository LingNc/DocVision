package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mineru-tools/internal/logger"
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
	uncertain  bool     // submitted code contains % [?] uncertainty marks
}

// CompileFigureTool compiles the TikZ body code held in a WORKSPACE
// FILE (default figure.tex) inside a standalone wrapper. It is a pure
// COMPILE tool: it returns the log, the output PDF name and the page
// count — the figure is INSPECTED with view_pdf {path:"standalone.pdf"}
// (crop/zoom re-render from the PDF), so no preview image is produced.
// The code is never passed inline: write_file / edit_file first.
type CompileFigureTool struct {
	Comp       *Compiler
	State      *tikzState
	EngineIsXe bool
	Log        *logger.Logger
	Tid        int
}

func (t *CompileFigureTool) Name() string { return "compile" }

func (t *CompileFigureTool) Definition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "compile",
			"description": "Compile your figure code from a workspace file (default figure.tex) and get the compile log plus the output PDF name. Inspect the result with view_pdf {path: standalone.pdf, page: 1}. " +
				"Write the code with write_file/edit_file first - never paste code into this tool. The file holds the TikZ body (between \\begin{document} and \\end{document}); \\usetikzlibrary / \\usepgfplotslibrary / \\usepackage lines at the top are hoisted into the preamble. " +
				"There is no preview image: inspect the compiled PDF with view_pdf (crop/zoom re-render it at high resolution), and compare it with the original image via view_image.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "workspace-relative file holding the TikZ body (default figure.tex)",
					},
					"merges": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "optional: image paths (as in the markdown) of page-boundary continuations absorbed into this figure",
					},
				},
			},
		},
	}
}

func (t *CompileFigureTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	if arr, ok := args["merges"].([]interface{}); ok {
		for _, v := range arr {
			if p, ok := v.(string); ok && strings.TrimSpace(p) != "" {
				t.State.merges = append(t.State.merges, strings.TrimSpace(p))
			}
		}
	}
	rel := strings.TrimSpace(strArg(args, "path"))
	if rel == "" {
		rel = "figure.tex"
	}
	src, err := resolveInside(t.State.workDir, rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return session.ToolResult{Text: "NOT FOUND: " + rel + "（先用 write_file 把图形代码写入该文件，compile 只接收路径）"}, nil
	}
	code := string(data)
	if strings.TrimSpace(code) == "" {
		return session.ToolResult{Text: "REJECTED: " + rel + " 为空。"}, nil
	}
	t.State.lastCode = code
	t.State.compileOK = false
	t.State.lastPDF = ""

	// 包装成 standalone 文档后编译（figure.tex 保持为模型的原样代码）。
	texFile := filepath.Join(t.State.workDir, "standalone.tex")
	if err := os.WriteFile(texFile, []byte(buildStandalone(code, t.EngineIsXe)), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	// Clear previous outputs so a stale PDF can never pass as fresh.
	os.Remove(filepath.Join(t.State.workDir, "standalone.pdf"))
	os.Remove(filepath.Join(t.State.workDir, "standalone.png"))

	start := time.Now()
	res := t.Comp.Compile(t.State.workDir, "standalone.tex")
	LogCompileResult(t.Log, t.Tid, "preview", res, time.Since(start))
	if !res.OK {
		t.State.compileErr = res.Err
		text := "COMPILE FAILED. Fix " + rel + " and call compile again.\nError:\n" + res.Err
		if w := res.WarningSummary(); w != "" {
			text += "\n" + truncateStr(w, 1500)
		}
		return session.ToolResult{Text: text}, nil
	}
	t.State.compileOK = true
	t.State.lastPDF = res.PDF
	t.State.compileErr = ""
	pdfDetail := ""
	if n, err := pdfPageCount(res.PDF); err == nil {
		pdfDetail = fmt.Sprintf(" (%d page)", n)
	}
	text := "COMPILE OK. Output: standalone.pdf" + pdfDetail +
		". Look at it with view_pdf {path: \"standalone.pdf\", page: 1} and compare it with the ORIGINAL image (structure, labels, overlaps/crowding; crop with left/top/right/bottom and zoom to a pixel width to inspect details). " +
		"If it faithfully matches, call submit; otherwise fix " + rel + " and compile again."
	if w := res.WarningSummary(); w != "" {
		text += "\n" + truncateStr(w, 1500)
	}
	return session.ToolResult{Text: text}, nil
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
			"description": "Submit the final TikZ figure. Write the final code to a workspace file (write_file figure.tex) and submit {path: 'figure.tex'} — no need to repeat the code. Allowed only after a successful compile of EXACTLY this code.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "workspace-relative file holding the final TikZ code (must equal the last successful compile)."},
					"code":   map[string]any{"type": "string", "description": "The final TikZ body code inline (only when not using path)."},
					"merges": map[string]any{"type": "string", "description": "Image paths (as in the markdown) of page-boundary continuations absorbed into this combined figure."},
				},
			},
		},
	}
}

func (t *SubmitFigureTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	// code 支持按工作区文件路径引用（避免整段重输出）：path 优先。
	code, _ := args["code"].(string)
	if p, _ := args["path"].(string); strings.TrimSpace(p) != "" {
		full, err := resolveInside(t.State.workDir, p)
		if err != nil {
			return session.ToolResult{}, err
		}
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			return session.ToolResult{Text: "NOT FOUND: " + p + "（先 write_file 写入最终代码）"}, nil
		}
		code = string(data)
	}
	if strings.TrimSpace(code) == "" {
		return session.ToolResult{}, fmt.Errorf("code 为空（或 path 文件不存在）")
	}
	if !t.State.compileOK || strings.TrimSpace(code) != strings.TrimSpace(t.State.lastCode) {
		return session.ToolResult{Text: "REJECTED: the submitted code differs from the last successful compile. Call compile {path} with this exact file first."}, nil
	}
	t.State.submitted = true
	t.State.finalCode = code
	t.State.uncertain = strings.Contains(code, "[?]")
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
