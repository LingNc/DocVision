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

// CompileChapterTool compiles a .tex of the session's OWN workspace
// inside a wrapper that loads the book class, returning the error log
// only.
//
// Dir is the session's whole writable tree: it holds the class, the
// manual, example.tex, the chapter fragment and the wrapper, so a
// compile is just "run the wrapper here" — nothing is copied in from
// anywhere else. WrapperFile is what gets compiled by default
// (`\documentclass{...}` + `\input{<chapter>.tex}`); a {path} argument
// compiles any other .tex of the same tree under the same wrapper
// (probes, extra \input parts).
//
// Compiling the FRAGMENT directly (what this tool used to do) can never
// work — a fragment has no \documentclass, so the class never loads and
// LaTeX reports "The font size command \normalsize is not defined" /
// "Undefined control sequence" at the first \section. In the 2026-09-11
// run that made 87 of 87 chapter compiles fail, which sent three
// conversion sessions into 133/80/98 rounds of blind compile-fix loops
// until the account ran out of credit.
type CompileChapterTool struct {
	Comp        *Compiler
	Dir         string // the session's writable tree (class + manual + chapter + wrapper)
	MainFile    string // the session's own chapter fragment, e.g. chapter_003.tex
	WrapperFile string // what gets compiled, e.g. chapter_003_wrapper.tex ("" = MainFile)
	Log         *logger.Logger
	Tid         int
}

func (t *CompileChapterTool) Name() string { return "compile" }

func (t *CompileChapterTool) Definition() map[string]any {
	// 说明只讲"怎么用"（用户：compile 就是编译，不要附带一堆信息和额外要求）。
	desc := "Compile a .tex file of your workspace (default " + t.MainFile + ") inside a wrapper that loads the book class, " +
		"and get the result: the LaTeX error to fix, or OK plus the output PDF name and page count. The file must already exist (write_file/edit_file first)."
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "compile",
		"description": desc,
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "workspace-relative .tex file (default " + t.MainFile + ")"},
			"engine": map[string]any{"type": "string", "description": "optional: latexmk | xelatex | pdflatex | lualatex (default: configured engine)"},
			"passes": map[string]any{"type": "integer", "description": "optional number of engine passes (default: 2)"},
			"args":   map[string]any{"type": "string", "description": "optional extra engine flags, space separated"},
		}},
	}}
}

// mainFile returns the file compiled by default.
func (t *CompileChapterTool) mainFile() (compileFile, liveName string) {
	compileFile = t.WrapperFile
	if compileFile == "" {
		compileFile = t.MainFile
	}
	liveName = t.MainFile
	return
}

func (t *CompileChapterTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, _ := parseJSONObject(argsJSON)
	rel, _ := args["path"].(string)
	rel = strings.TrimSpace(rel)
	compileFile, liveName := t.mainFile()
	if rel != "" && rel != t.MainFile {
		// 编译工作区里的另一个 .tex：同一 wrapper 下编译它（探针文件／
		// \input 分片都靠这个真正验证类命令是否存在）。
		clean := strings.TrimPrefix(filepath.ToSlash(rel), "./")
		clean = strings.TrimPrefix(clean, "work:")
		if _, err := resolveInside(t.Dir, clean); err != nil {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + err.Error()}, nil
		}
		if !fileExists(filepath.Join(t.Dir, clean)) {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + clean + " does not exist in your workspace."}, nil
		}
		// 用同一 wrapper 模板换成 \input 这个文件（文件本来就在同一棵树
		// 里，子目录里的探针/分片用相对路径照样能找到）。
		inputRel := strings.TrimSuffix(clean, filepath.Ext(clean))
		wrapper := "\\documentclass{" + t.clsName() + "}\n" +
			"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
			"\\graphicspath{{figures/}}\n" +
			"\\begin{document}\n\\input{" + inputRel + ".tex}\n\\end{document}\n"
		compileFile = strings.NewReplacer("/", "_", "\\", "_").Replace(inputRel) + "_wrapper.tex"
		if werr := os.WriteFile(filepath.Join(t.Dir, compileFile), []byte(wrapper), 0o644); werr != nil {
			return session.ToolResult{}, werr
		}
		liveName = clean
	} else if !fileExists(filepath.Join(t.Dir, compileFile)) {
		return session.ToolResult{Text: "COMPILE SKIPPED: " + t.MainFile + " has not been written yet (call write_file first)."}, nil
	}
	// 引擎/额外参数可以现场指定（与通用 compile 工具同一套参数）：有时候
	// 需要 latexmk 多遍，或临时加一个 flag 试出来。
	opts := CompileOptions{Engine: strings.TrimSpace(strArg(args, "engine")), Passes: intArg(args, "passes", 0)}
	if extra := strings.TrimSpace(strArg(args, "args")); extra != "" {
		opts.ExtraArgs = strings.Fields(extra)
	}
	start := time.Now()
	res := t.Comp.CompileOpts(t.Dir, compileFile, opts)
	LogCompileResult(t.Log, t.Tid, "chapter", res, time.Since(start))
	if res.OK {
		// 成功只报事实：产物名 + 页数 + 一句"用 view_pdf 看它"（怎么细看是模型自己的事）。
		detail := ""
		outName := strings.TrimSuffix(compileFile, ".tex") + ".pdf"
		pdf := filepath.Join(t.Dir, outName)
		if n, err := pdfPageCount(pdf); err == nil {
			detail = fmt.Sprintf("\nOutput: %s (%d pages). See it with view_pdf {path: %q, page: 1}.", outName, n, outName)
		} else {
			detail = fmt.Sprintf("\nOutput: %s. See it with view_pdf {path: %q, page: 1}.", outName, outName)
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

// clsName recovers the book class from the wrapper of the session's
// workspace (the workspace holds exactly one *_wrapper.tex per chapter).
func (t *CompileChapterTool) clsName() string {
	if data, err := os.ReadFile(filepath.Join(t.Dir, t.WrapperFile)); err == nil {
		line := string(data)
		if i := strings.Index(line, "\\documentclass{"); i >= 0 {
			rest := line[i+len("\\documentclass{"):]
			if j := strings.Index(rest, "}"); j > 0 {
				return rest[:j]
			}
		}
	}
	if entries, err := os.ReadDir(t.Dir); err == nil {
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
	// RequireReport makes the report mandatory even when nothing is
	// persisted (the checker session reports its verdict that way).
	RequireReport bool
	// Raw report fields, kept for callers that need the structured
	// verdict instead of the rendered text.
	Status      string // "pass" | "issues"
	Issues      string
	Suggestions string
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
	if t.ReportPath != "" || t.RequireReport {
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
	if (t.ReportPath != "" || t.RequireReport) && report == nil {
		return session.ToolResult{Text: "REJECTED: submit requires a 'report' object (work report / 工作汇报). Call submit again with report.status = pass or issues."}, nil
	}
	t.Submitted = true
	t.Notes = notes
	if report != nil {
		t.Status, _ = report["status"].(string)
		t.Issues, _ = report["issues"].(string)
		t.Suggestions, _ = report["suggestions"].(string)
	}
	if report != nil && t.ReportPath != "" {
		t.Report = renderWorkReport(t.Label, notes, report)
		if err := os.MkdirAll(filepath.Dir(t.ReportPath), 0o755); err == nil {
			if err := os.WriteFile(t.ReportPath, []byte(t.Report), 0o644); err != nil {
				return session.ToolResult{}, fmt.Errorf("write work report: %w", err)
			}
		}
	}
	return session.ToolResult{Text: "SUBMITTED. Reply with a one-line confirmation and nothing else."}, nil
}

// SubmitChapterTool is the convert session's submit: the chapter is
// handed in as TWO explicit paths — the main .tex file and the folder
// holding its \input parts — and the runner copies exactly those two
// into the real chapter tree. Nothing else of the session's workspace
// (probe files, scratch PDFs, leftovers) is part of the submission, so
// experiments can never leak into the book.
type SubmitChapterTool struct {
	WorkRoot    string // the session's writable tree (temp workspace)
	SubmitRoot  string // <proj>/work/chapters — where the book reads chapters from
	Base        string // canonical chapter name, e.g. chapter_003
	ReportPath  string // 工作汇报 destination (work/reports/<base>.md)
	Submitted   bool
	Notes       string
	Status      string
	Issues      string
	Suggestions string
	Report      string
	Placed      int // files copied into the submission tree
	// ReportOptional drops the mandatory 工作汇报 (the style-fix session
	// only has to hand in an adapted chapter).
	ReportOptional bool
}

func (t *SubmitChapterTool) Name() string { return "submit" }

func (t *SubmitChapterTool) Definition() map[string]any {
	texName := t.Base + ".tex"
	dirName := t.Base + "/"
	reportRule := "A `report` (工作汇报) on style/manual conformance is required."
	required := []string{"path", "dir", "report"}
	if t.ReportOptional {
		reportRule = "A `report` is optional here."
		required = []string{"path", "dir"}
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit",
		"description": "Hand in this chapter — the ONLY thing that counts as the submission. Give the two paths: " +
			"`path` = the chapter's main .tex (must be " + texName + ") and `dir` = the folder with that chapter's \\input parts " +
			"(must be " + dirName + "; create it even when empty). The runner copies exactly those two into the book; everything else in your " +
			"workspace (probe files, experiments, PDFs) stays behind. " + reportRule + " " +
			"Only call submit after a clean compile.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "the chapter's main .tex file (" + texName + ")"},
			"dir":  map[string]any{"type": "string", "description": "the folder holding that chapter's \\input parts (" + dirName + ")"},
			"report": map[string]any{
				"type":        "object",
				"description": "REQUIRED work report (工作汇报) on style/manual conformance. status=pass when your conversion used the manual/cls correctly with no style problems; status=issues when the cls/manual could not express what the book really does.",
				"properties": map[string]any{
					"status":      map[string]any{"type": "string", "enum": []string{"pass", "issues"}},
					"issues":      map[string]any{"type": "string", "description": "Concrete cls/manual/format problems found (empty for pass)."},
					"suggestions": map[string]any{"type": "string", "description": "Concrete suggestions for the cls/manual (empty for pass)."},
				},
				"required": []string{"status"},
			},
			"notes": map[string]any{"type": "string", "description": "Optional short handover notes."},
		}, "required": required},
	}}
}

func (t *SubmitChapterTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	texRel := strings.TrimSpace(strArg(args, "path"))
	dirRel := strings.TrimSpace(strArg(args, "dir"))
	notes := strArg(args, "notes")
	report, _ := args["report"].(map[string]any)
	wantTex, wantDir := t.Base+".tex", t.Base
	switch {
	case texRel == "":
		return session.ToolResult{Text: "REJECTED: submit needs `path` = your main .tex (" + wantTex + ")."}, nil
	case dirRel == "":
		return session.ToolResult{Text: "REJECTED: submit needs `dir` = the folder with this chapter's \\input parts (" + wantDir + "/). Create it (even empty) and submit again."}, nil
	case report == nil && !t.ReportOptional:
		return session.ToolResult{Text: "REJECTED: submit needs the `report` object (工作汇报). Submit again with report.status = pass or issues."}, nil
	}
	trim := func(p string) string {
		p = strings.TrimPrefix(filepath.ToSlash(p), "./")
		p = strings.TrimPrefix(p, "work:")
		return strings.TrimSuffix(p, "/")
	}
	texRel, dirRel = trim(texRel), trim(dirRel)
	if filepath.Base(texRel) != wantTex {
		return session.ToolResult{Text: "REJECTED: the main file must be named " + wantTex + " (got " + filepath.Base(texRel) + ") — the book reads chapters/" + wantTex + "."}, nil
	}
	if filepath.Base(dirRel) != wantDir {
		return session.ToolResult{Text: "REJECTED: the parts folder must be named " + wantDir + "/ (got " + filepath.Base(dirRel) + "/)."}, nil
	}
	texPath, err := resolveInside(t.WorkRoot, texRel)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	dirPath, err := resolveInside(t.WorkRoot, dirRel)
	if err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	if !fileExists(texPath) {
		return session.ToolResult{Text: "NOT FOUND: " + texRel + " (write it with write_file first)."}, nil
	}
	if st, serr := os.Stat(dirPath); serr != nil || !st.IsDir() {
		return session.ToolResult{Text: "NOT FOUND: " + dirRel + "/ is not a folder yet. Create it (bash mkdir, or write a file inside it) and submit again."}, nil
	}
	data, rerr := os.ReadFile(texPath)
	if rerr != nil {
		return session.ToolResult{}, rerr
	}
	// 分片不能自带前导：wrapper 已经加载了类。
	if strings.Contains(string(data), `\documentclass`) {
		return session.ToolResult{Text: "REJECTED: " + wantTex + " must be an \\input fragment (no \\documentclass) — the book's wrapper provides the preamble."}, nil
	}
	// 只把这两个路径拷进书里；文件夹整棵替换，旧内容与历史残留不会留下。
	placed, perr := placeChapterFiles(t.WorkRoot, t.SubmitRoot, t.Base)
	if perr != nil {
		return session.ToolResult{}, perr
	}
	t.Placed = placed
	t.Submitted = true
	t.Notes = notes
	t.Status, _ = report["status"].(string)
	t.Issues, _ = report["issues"].(string)
	t.Suggestions, _ = report["suggestions"].(string)
	t.Report = renderWorkReport("chapter "+t.Base, notes, report)
	if t.ReportPath != "" {
		if err := os.MkdirAll(filepath.Dir(t.ReportPath), 0o755); err == nil {
			if werr := os.WriteFile(t.ReportPath, []byte(t.Report), 0o644); werr != nil {
				return session.ToolResult{}, fmt.Errorf("write work report: %w", werr)
			}
		}
	}
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. chapters/%s and chapters/%s/ (%d files) are in the book. Reply with a one-line confirmation and nothing else.",
		wantTex, wantDir, t.Placed)}, nil
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

// countTreeFiles counts the files under dir (used for the submit receipt:
// "N files are in the book").
func countTreeFiles(dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}
