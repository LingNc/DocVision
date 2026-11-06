package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// includeRef 抓取 \input{…} / \include{…} / \includegraphics[opts]{…} / \subfile{…}
// 的目标路径。只用于"提交产物是否自包含"这一项检查，不解析 TeX。
var includeRef = regexp.MustCompile(`\\(input|include|includegraphics|subfile|InputIfFileExists)\s*(?:\[[^\]]*\])?\s*\{([^}]*)\}`)

// unportableIncludes 找出提交产物里指向**临时树**的包含路径。chapter 提交只把
// `<base>.tex` 与 `<base>/` 拷进书里，任何 `_subdivision/…`、`node/result/…`
// 之类的引用在晋级后就断了；汇编/编译要等很久才炸，而模型往往就在这时候
// 放弃编译直接提交。这里在 submit 当场挡回去。
func unportableIncludes(workRoot, base string) []string {
	files := []string{filepath.Join(workRoot, base+".tex")}
	_ = filepath.Walk(filepath.Join(workRoot, base), func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	var bad []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(workRoot, f)
		for _, m := range includeRef.FindAllStringSubmatch(string(data), -1) {
			target := strings.TrimSpace(m[2])
			if target == "" || strings.ContainsAny(target, "\\#") || strings.Contains(target, "://") {
				continue
			}
			norm := filepath.ToSlash(target)
			if strings.Contains(norm, "_subdivision/") || strings.HasSuffix(norm, "_subdivision") ||
				strings.Contains(norm, "node/result/") || strings.HasPrefix(norm, "/temp/") {
				bad = append(bad, filepath.ToSlash(rel)+": \\"+m[1]+"{"+target+"}")
			}
		}
	}
	sort.Strings(bad)
	return bad
}

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
	// Gate 记录「编译成功」这件事，供 submit 的门禁核对（T165）。
	Gate *chapterGate
	Log  *logger.Logger
	Tid  int
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
	if res.OK && t.Gate != nil {
		pdfName := strings.TrimSuffix(compileFile, ".tex") + ".pdf"
		if err := t.Gate.markCompiled(filepath.Join(t.Dir, pdfName)); err != nil {
			t.Log.LogWarning(t.Tid, "[gate] 记录编译印记失败:", err)
		}
	}
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
	if h := missingFileHints(res.Err, t.Dir); h != "" {
		text += "\n" + h
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
	// ReportPrompt overrides the report's description in the tool schema:
	// the checker's wording is about cls/manual conformance, while the
	// figure check asks for a visual comparison verdict.
	ReportPrompt string
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
		desc := t.ReportPrompt
		if desc == "" {
			desc = "REQUIRED work report (工作汇报) on style/manual conformance. status=pass when your conversion used the manual/cls correctly with no style problems; status=issues when the cls/manual could not express what the book really does (missing environment, wrong heading style, unusable table style...). Describe each problem concretely and give suggestions for the style package."
		}
		props["report"] = map[string]any{
			"type":        "object",
			"description": desc,
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
	return session.ToolResult{Text: "SUBMITTED.", Terminal: true}, nil
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
	// Gate 是可选提交门（P27：goal 计划清空才放行）；返回打回文案即拒绝。
	Gate func() string
	// GateRef 是同一个门禁的引用（T168）：「覆盖率不达标但第二次放行」的缺口
	// 要写进这一章的报告，得从门禁的印记里读。
	GateRef *chapterGate
	// Depth 是这一章在递归树里的深度（顶层 0），写进 issue 条目。
	Depth int
	// IssuesPath 是本节点的 issue store（<work>/.issues/issues.json）：继承来的
	// 子章节问题 + 自己新报的，都落在这里，父节点直接读文件继承。
	IssuesPath string
	// InheritedPath 是父节点写下的"子章节未解决问题"（
	// <work>/_subdivision/issues-inherited.json）；不存在就是没有。
	InheritedPath string
	// OpenIssues / NewIssues 是提交后算出的结果，供调用方记录日志。
	OpenIssues []ChapterIssue
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
					"status": map[string]any{"type": "string", "enum": []string{"pass", "issues"}},
					// issues 两种写法都收：字符串（旧）或字符串数组（推荐，一条
					// 一个条目，便于跨层合并）；子章节的问题会由机制继承进来。
					"issues": map[string]any{
						"description": "Concrete cls/manual/format problems you found — one problem per item (a string array is preferred; a plain string still works).",
						"anyOf": []any{
							map[string]any{"type": "string"},
							map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
					"suggestions": map[string]any{"type": "string", "description": "Concrete suggestions for the cls/manual (empty for pass)."},
					// resolved：父章节整合时修掉了哪几条**继承来的**问题，用它们的
					// ID；机制会据此把条目划掉，没写的原样向上。
					// coverage_note：内容覆盖率不达标又被放行时的一句解释，
					// 会连同覆盖率数字一起进报告的未解问题、向上合并。
					"coverage_note": map[string]any{
						"type":        "string",
						"description": "If submit warned that too little of the chapter markdown shows up in your PDF (OCR/duplicate headers can make that unavoidable), explain here why the missing text is expected. It is carried upward with the coverage number.",
					},
					"resolved": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "IDs of inherited child issues you actually FIXED while integrating (from children-summary.md). Leave a problem out if you did not fix it — it is carried upward automatically.",
					},
				},
				"required": []string{"status"},
			},
			"notes": map[string]any{"type": "string", "description": "Optional short handover notes."},
		}, "required": required},
	}}
}

func (t *SubmitChapterTool) Execute(argsJSON string) (session.ToolResult, error) {
	if t.Gate != nil {
		if msg := t.Gate(); msg != "" {
			return session.ToolResult{Text: msg}, nil
		}
	}
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
	// 提交产物必须**自包含**：`\\input`/`\\includegraphics` 只能指向本目录，
	// 不能指向 `_subdivision/` 这类临时树。实测代价：一个嵌套父章节把
	// `\\input{_subdivision/child_01/node/result/...}` 交进了书里，路径只在它自己的
	// 临时工作区存在，晋级/汇编后全是断链——上一层的模型只好去 `find` 临时
	// 目录、手工补路径，最后连编译都放弃就提交了（这正是"父节点不编译不看"
	// 的一条真实来源）。在这里挡回去，让它把内容整合进 <base>/。
	if bad := unportableIncludes(t.WorkRoot, t.Base); len(bad) > 0 {
		shown := bad
		if len(shown) > 6 {
			shown = shown[:6]
		}
		return session.ToolResult{Text: "REJECTED: " + wantTex + " 或 " + wantDir + "/ 里有 " +
			strconv.Itoa(len(bad)) + " 处包含指向临时目录的路径（提交后必然断链）：\n  " +
			strings.Join(shown, "\n  ") +
			"\n只能引用本目录里的文件（\\input{" + wantDir + "/...} 或 figures/images 里的资源）。" +
			"把这些内容整合进 " + wantDir + "/ 后再提交。"}, nil
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
	ownIssues := reportTextList(report["issues"])
	// 覆盖率不达标但已放行（第二次）的缺口：不是"通过就没事"，而是变成一条
	// 未解问题进报告——父章节与顶层报告都能看到这一章实际缺了多少。
	if t.GateRef != nil {
		if gap, ok := t.GateRef.coverageGap(); ok {
			note, _ := report["coverage_note"].(string)
			ownIssues = append(ownIssues, coverageGapIssue(gap.Coverage, gap.Missing, note))
		}
	}
	t.Issues = strings.Join(ownIssues, "\n")
	t.Suggestions, _ = report["suggestions"].(string)
	// 问题合并（T166）：继承子章节未解决的条目 + 自己新报的，再去掉明确修掉的。
	// 这一节由代码写进报告，父会话不必"记得"把子章节的问题抄一遍。
	if t.IssuesPath != "" {
		merged := mergeChapterIssues(t.Base, t.Depth, ownIssues,
			readIssueStore(t.InheritedPath), reportTextList(report["resolved"]))
		if err := writeIssueStore(t.IssuesPath, merged); err != nil {
			return session.ToolResult{}, fmt.Errorf("write chapter issues: %w", err)
		}
		t.OpenIssues = openIssues(merged)
		if extra := renderIssuesSection(t.OpenIssues); extra != "" {
			t.Report = renderWorkReport("chapter "+t.Base, notes, report) + extra
		}
	}
	if t.Report == "" {
		t.Report = renderWorkReport("chapter "+t.Base, notes, report)
	}
	if t.ReportPath != "" {
		if err := os.MkdirAll(filepath.Dir(t.ReportPath), 0o755); err == nil {
			if werr := os.WriteFile(t.ReportPath, []byte(t.Report), 0o644); werr != nil {
				return session.ToolResult{}, fmt.Errorf("write work report: %w", werr)
			}
		}
	}
	extra := ""
	if t.IssuesPath != "" {
		extra = fmt.Sprintf(" Open issues carried upward: %d (fix some? put their IDs in report.resolved and submit again).", len(t.OpenIssues))
	}
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. chapters/%s and chapters/%s/ (%d files) are in the book.%s",
		wantTex, wantDir, t.Placed, extra), Terminal: true}, nil
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
