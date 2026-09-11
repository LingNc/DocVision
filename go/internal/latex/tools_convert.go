package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// resolveInside maps rel under root, refusing escapes.
func resolveInside(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("路径越界（绝对路径）: %s", rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	rootAbs, _ := filepath.Abs(root)
	fullAbs, _ := filepath.Abs(filepath.Join(root, clean))
	if rootAbs != fullAbs && !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	return filepath.Join(root, clean), nil
}

// CompileChapterTool compiles a chapter .tex inside a scratch wrapper
// that uses the book class, returning the error log only.
//
// WrapperFile is the file actually compiled (`\documentclass{...}` +
// `\input{<chapter>.tex}`); SourcePath is the live chapter fragment in
// the session's virtual tree, copied into the scratch before each run.
// Compiling the FRAGMENT directly (what this tool used to do) can never
// work — a fragment has no \documentclass, so the class never loads and
// LaTeX reports "The font size command \normalsize is not defined" /
// "Undefined control sequence" at the first \section. In the 2026-09-11
// run that made 87 of 87 chapter compiles fail, which sent three
// conversion sessions into 133/80/98 rounds of blind compile-fix loops
// until the account ran out of credit.
type CompileChapterTool struct {
	Comp        *Compiler
	Scratch     string // per-chapter scratch dir (contains wrapper + cls copy)
	MainFile    string // the session's own chapter fragment, e.g. chapter_003.tex
	WrapperFile string // what gets compiled, e.g. chapter_003_wrapper.tex ("" = MainFile)
	SourcePath  string // live chapter .tex in the virtual work tree; copied into Scratch before compiling
	// WorkDir is the session's writable tree; a {path} argument naming
	// another .tex inside it is copied into the scratch and compiled in
	// the same wrapper (probe files, extra \input parts, …).
	WorkDir string
	Log     *logger.Logger
	Tid     int
}

func (t *CompileChapterTool) Name() string { return "compile" }

func (t *CompileChapterTool) Definition() map[string]any {
	desc := "Compile your chapter with the book class in a scratch wrapper (wrapper file + your .tex). " +
		"Returns COMPILE OK plus the artifact PDF name/pages, or the LaTeX error log. " +
		"With no argument it compiles your own main file (" + t.MainFile + "). " +
		"path names another .tex in your workspace (e.g. a probe or an \\input part) to compile alone for testing."
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "compile",
		"description": desc,
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "optional .tex path in your workspace; default = your main file"},
		}},
	}}
}

// mainFile returns the wrapper to compile and the fragment to refresh
// from the live tree ("" when nothing needs copying).
func (t *CompileChapterTool) mainFile() (compileFile, sourceFile, liveName string) {
	compileFile = t.WrapperFile
	if compileFile == "" {
		compileFile = t.MainFile
	}
	sourceFile = t.SourcePath
	liveName = t.MainFile
	return
}

func (t *CompileChapterTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, _ := parseJSONObject(argsJSON)
	rel, _ := args["path"].(string)
	rel = strings.TrimSpace(rel)
	compileFile, sourceFile, liveName := t.mainFile()
	if rel != "" && rel != t.MainFile {
		// 编译工作区里的另一个 .tex：先把它拷进 scratch（同一 wrapper 下
		// 编译），这样"探针文件/分片"能真正验证类命令是否存在。
		if t.WorkDir == "" {
			return session.ToolResult{Text: "COMPILE SKIPPED: this session can only compile " + t.MainFile + "."}, nil
		}
		clean := strings.TrimPrefix(filepath.ToSlash(rel), "./")
		clean = strings.TrimPrefix(clean, "work:")
		full, err := resolveInside(t.WorkDir, clean)
		if err != nil {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + err.Error()}, nil
		}
		if !fileExists(full) {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + clean + " does not exist in your workspace."}, nil
		}
		base := strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean))
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			return session.ToolResult{}, rerr
		}
		if err := os.WriteFile(filepath.Join(t.Scratch, base+".tex"), data, 0o644); err != nil {
			return session.ToolResult{}, err
		}
		// 用同一 wrapper 模板换成 input 这个文件。
		wrapper := "\\documentclass{" + t.clsName() + "}\n" +
			"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
			"\\graphicspath{{figures/}}\n" +
			"\\begin{document}\n\\input{" + base + ".tex}\n\\end{document}\n"
		compileFile = base + "_wrapper.tex"
		if werr := os.WriteFile(filepath.Join(t.Scratch, compileFile), []byte(wrapper), 0o644); werr != nil {
			return session.ToolResult{}, werr
		}
		sourceFile, liveName = "", base+".tex"
	} else if sourceFile != "" {
		data, err := os.ReadFile(sourceFile)
		if err != nil {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + t.MainFile + " has not been written yet (call write_file first)."}, nil
		}
		if err := os.WriteFile(filepath.Join(t.Scratch, t.MainFile), data, 0o644); err != nil {
			return session.ToolResult{}, err
		}
	}
	start := time.Now()
	res := t.Comp.Compile(t.Scratch, compileFile)
	LogCompileResult(t.Log, t.Tid, "chapter", res, time.Since(start))
	if res.OK {
		// 成功时提供有用信息：产物 PDF 名 + 页数，便于 view_pdf 检视。
		detail := ""
		pdf := filepath.Join(t.Scratch, strings.TrimSuffix(compileFile, ".tex")+".pdf")
		if n, err := pdfPageCount(pdf); err == nil {
			detail = fmt.Sprintf("\nOutput: %s.pdf (%d pages). Inspect with view_pdf {path, page}.", strings.TrimSuffix(compileFile, ".tex"), n)
		}
		if w := res.WarningSummary(); w != "" {
			return session.ToolResult{Text: "COMPILE OK.\n" + truncateStr(w, 1500) + detail}, nil
		}
		return session.ToolResult{Text: "COMPILE OK." + detail}, nil
	}
	text := "COMPILE FAILED (" + liveName + "):\n" + res.Err
	if w := res.WarningSummary(); w != "" {
		text += "\n" + truncateStr(w, 1500)
	}
	return session.ToolResult{Text: text}, nil
}

// clsName recovers the book class from the scratch copy of the wrapper
// (the scratch always holds exactly one *_wrapper.tex).
func (t *CompileChapterTool) clsName() string {
	if data, err := os.ReadFile(filepath.Join(t.Scratch, t.WrapperFile)); err == nil {
		line := string(data)
		if i := strings.Index(line, "\\documentclass{"); i >= 0 {
			rest := line[i+len("\\documentclass{"):]
			if j := strings.Index(rest, "}"); j > 0 {
				return rest[:j]
			}
		}
	}
	if entries, err := os.ReadDir(t.Scratch); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".cls") {
				return strings.TrimSuffix(e.Name(), ".cls")
			}
		}
	}
	return "book"
}

// SubmitDoneTool is the generic final confirmation for convert / fix
// sessions. When ReportPath is set the submit REQUIRES a report object
// (工作汇报) which is rendered in a fixed format and persisted to disk
// in real time — the style feedback loop later aggregates these files.
type SubmitDoneTool struct {
	Submitted  bool
	Notes      string
	Label      string
	ReportPath string // when set, persist the 工作汇报 there
	Report     string // rendered report content ("" when no report)
}

func (t *SubmitDoneTool) Name() string { return "submit" }

func (t *SubmitDoneTool) Definition() map[string]any {
	label := t.Label
	if label == "" {
		label = "the work"
	}
	props := map[string]any{
		"notes": map[string]any{"type": "string", "description": "Optional short handover notes."},
	}
	if t.ReportPath != "" {
		props["report"] = map[string]any{
			"type":        "object",
			"description": "REQUIRED work report (工作汇报) on style/manual conformance. status=pass when your conversion used the manual/cls correctly with no style problems; status=issues when the cls/manual could not express what the book really does (missing environment, wrong heading style, unusable table style...). Describe each problem concretely and give suggestions for the style package.",
			"properties": map[string]any{
				"status":      map[string]any{"type": "string", "enum": []string{"pass", "issues"}},
				"issues":      map[string]any{"type": "string", "description": "Concrete cls/manual/format problems found (empty for pass)."},
				"suggestions": map[string]any{"type": "string", "description": "Concrete suggestions for the cls/manual (empty for pass)."},
			},
			"required": []string{"status"},
		}
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit",
		"description": "Declare " + label + " final. Only call after a clean compile.",
		"parameters":  map[string]any{"type": "object", "properties": props},
	}}
}

func (t *SubmitDoneTool) Execute(argsJSON string) (session.ToolResult, error) {
	notes := ""
	var report map[string]any
	if args, err := parseJSONObject(argsJSON); err == nil {
		notes, _ = args["notes"].(string)
		if m, ok := args["report"].(map[string]any); ok {
			report = m
		}
	}
	if t.ReportPath != "" && report == nil {
		return session.ToolResult{Text: "REJECTED: submit requires a 'report' object (work report / 工作汇报). Call submit again with report.status = pass or issues."}, nil
	}
	t.Submitted = true
	t.Notes = notes
	if report != nil {
		t.Report = renderWorkReport(t.Label, notes, report)
		if err := os.MkdirAll(filepath.Dir(t.ReportPath), 0o755); err == nil {
			if err := os.WriteFile(t.ReportPath, []byte(t.Report), 0o644); err != nil {
				return session.ToolResult{}, fmt.Errorf("write work report: %w", err)
			}
		}
	}
	return session.ToolResult{Text: "SUBMITTED. Reply with a one-line confirmation and nothing else."}, nil
}

// renderWorkReport renders the fixed-format 工作汇报 file. The "结论:"
// line is the stable marker the style feedback loop parses.
func renderWorkReport(label, notes string, report map[string]any) string {
	status, _ := report["status"].(string)
	if status != "pass" && status != "issues" {
		status = "issues"
	}
	issues, _ := report["issues"].(string)
	suggestions, _ := report["suggestions"].(string)
	trim := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return "（无）"
		}
		return s
	}
	conclusion := "通过"
	if status == "issues" {
		conclusion = "存在问题"
	}
	var b strings.Builder
	b.WriteString("# 工作汇报\n")
	if label != "" {
		b.WriteString("\n- 对象: " + label + "\n")
	}
	b.WriteString("- 时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	b.WriteString("- 结论: " + conclusion + "\n")
	b.WriteString("\n## 问题\n\n" + trim(issues) + "\n")
	b.WriteString("\n## 对样式包的建议\n\n" + trim(suggestions) + "\n")
	if n := strings.TrimSpace(notes); n != "" {
		b.WriteString("\n## 备注\n\n" + n + "\n")
	}
	return b.String()
}
