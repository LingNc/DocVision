package latex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// AltRoot is an extra read-only root tried after the primary workspace
// root (label is shown to the model so it knows where the file came
// from, e.g. "project").
type AltRoot struct {
	Label string
	Dir   string
}

// ReadFileTool is the ONE file-reading tool for every session type. It
// replaces the old read_file / read_md / read_lines variants: a path
// (workspace-relative, optionally under a read-only extra root) plus an
// optional 1-based inclusive line window. Whole-file reads are capped;
// line windows return numbered lines.
type ReadFileTool struct {
	Root string
	// AltRoots are additional READ-ONLY roots tried when the path does
	// not resolve under Root (e.g. the project tree for a session whose
	// workspace is a subdirectory).
	AltRoots []AltRoot
	// Mounts overrides Root/AltRoots with an explicit virtual workspace
	// mount table (name + dir + writability).
	Mounts []Mount
	// MaxBytes caps a whole-file read (default 64KB).
	MaxBytes int
	// MaxLines caps one line-window read (default 400).
	MaxLines int
}

func (t *ReadFileTool) Name() string { return "read_file" }

// vfs returns the session's virtual workspace (Mounts wins over
// Root/AltRoots when both are set).
func (t *ReadFileTool) vfs() *VFS {
	if len(t.Mounts) > 0 {
		return &VFS{Mounts: t.Mounts}
	}
	return vfsFrom(t.Root, t.AltRoots)
}

func (t *ReadFileTool) Definition() map[string]any {
	desc := "Read a text file (read-only): your workspace files AND, where allowed, the project files (source markdown, class manual, other chapters). " +
		"Without start_line/end_line the whole file is returned (truncated when large); with start_line/end_line a numbered window of at most 400 lines is returned. " +
		"Paths are workspace-relative."
	if d := t.vfs().Describe(); d != "" {
		desc += " " + d
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "read_file",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "file path relative to the workspace root (or name:path for another mount)"},
				"start_line": map[string]any{"type": "integer", "description": "optional first line (1-based) of a window"},
				"end_line":   map[string]any{"type": "integer", "description": "optional last line (inclusive) of a window"},
			},
			"required": []string{"path"},
		},
	}}
}

// resolve finds the file through the virtual workspace mounts. Plain
// paths try the workspace first, then the read-only roots (historic
// behaviour); "name:path" / "/name/path" address one mount directly.
func (t *ReadFileTool) resolve(rel string) (full, label string, err error) {
	if strings.TrimSpace(rel) == "" {
		return "", "", fmt.Errorf("path 为空")
	}
	v := t.vfs()
	// 显式挂载点（name:path / /name/path）直接定位，不做跨挂载点搜索。
	if explicitMountOf(rel) != "" {
		full, label, rerr := v.Resolve(rel, false)
		if rerr != nil {
			return "", "", rerr
		}
		if !fileExists(full) {
			if st, se := os.Stat(full); se == nil && st.IsDir() {
				return "", "", fmt.Errorf("%s 是目录（用 grep 或 bash ls 查看目录内容）", rel)
			}
			return "", "", fmt.Errorf("文件不存在: %s%s（可用挂载点: %s）", rel, suggestNear(full, 12), t.vfs().Names())
		}
		return full, label, nil
	}
	if len(t.Mounts) > 0 {
		// 有挂载表：先默认挂载点（可写的工作区），再依次查其它挂载点
		// —— 与 bash 的路径空间一致，"自己的文件优先、项目文件兜底"。
		if def, derr := v.DefaultMount(false); derr == nil {
			if p, e := resolveInside(def.Dir, rel); e == nil && fileExists(p) {
				return p, def.Name, nil
			}
		}
		for _, m := range t.Mounts {
			if m.Dir == "" {
				continue
			}
			if p, e := resolveInside(m.Dir, rel); e == nil && fileExists(p) {
				return p, m.Name, nil
			}
		}
		if def, derr := v.DefaultMount(false); derr == nil {
			if p, e := resolveInside(def.Dir, rel); e == nil {
				if st, se := os.Stat(p); se == nil && st.IsDir() {
					return "", "", fmt.Errorf("%s 是目录（用 grep 或 bash ls 查看目录内容）", rel)
				}
				return "", "", fmt.Errorf("文件不存在: %s%s（可用挂载点: %s）", rel, suggestNear(p, 12), v.Names())
			}
		}
		return "", "", fmt.Errorf("文件不存在: %s（可用挂载点: %s）", rel, v.Names())
	}
	if p, e := resolveInside(t.Root, rel); e == nil && fileExists(p) {
		return p, "work", nil
	}
	for _, ar := range t.AltRoots {
		if ar.Dir == "" {
			continue
		}
		if p, e := resolveInside(ar.Dir, rel); e == nil && fileExists(p) {
			return p, ar.Label, nil
		}
	}
	if p, e := resolveInside(t.Root, rel); e == nil {
		if st, se := os.Stat(p); se == nil && st.IsDir() {
			return "", "", fmt.Errorf("%s 是目录（用 grep 或 bash ls 查看目录内容）", rel)
		}
		return "", "", fmt.Errorf("文件不存在: %s%s（可用挂载点: %s）", rel, suggestNear(p, 12), t.vfs().Names())
	}
	return "", "", fmt.Errorf("文件不存在: %s（可用挂载点: %s）", rel, t.vfs().Names())
}

// explicitMountOf returns the mount name when the path uses the
// "name:path" or "/name/path" form.
func explicitMountOf(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "/") {
		rest := strings.TrimPrefix(p, "/")
		if i := strings.Index(rest, "/"); i > 0 {
			return rest[:i]
		}
		return rest
	}
	if i := strings.Index(p, ":"); i > 0 {
		return p[:i]
	}
	return ""
}

func (t *ReadFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	full, label, err := t.resolve(rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	shown := rel
	if m := explicitMountOf(rel); m != "" {
		shown = label + ":" + stripMount(rel)
	} else if label != "" {
		shown = label + "/" + rel
	}
	_, hasStart := args["start_line"]
	_, hasEnd := args["end_line"]
	if hasStart || hasEnd {
		start := intArg(args, "start_line", 1)
		end := intArg(args, "end_line", start)
		maxLines := t.MaxLines
		if maxLines <= 0 {
			maxLines = 400
		}
		return readLinesFrom(full, start, end, maxLines)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return session.ToolResult{}, err
	}
	max := t.MaxBytes
	if max <= 0 {
		max = 65536
	}
	text := string(data)
	note := ""
	if len(text) > max {
		text = text[:max]
		note = "\n...[truncated; use start_line/end_line for later parts]..."
	}
	return session.ToolResult{Text: fmt.Sprintf("[%s, %d bytes]\n%s%s", shown, len(data), text, note)}, nil
}

// CompileTexTool is the general LaTeX build tool for a workspace: it
// compiles ONE .tex file that already exists in the workspace (never
// inline code) and reports the result. Multi-file projects work because
// the main file may \input / \include other workspace files; passes /
// bibliography / engine / extra flags can be chosen by the model, and
// engine "latexmk" runs a complete multi-pass build.
type CompileTexTool struct {
	Comp     *Compiler
	Root     string // workspace root = build directory
	MainFile string // default main file, workspace-relative
	Tag      string // log tag ("chapter", "book", "style", ...)
	Log      *logger.Logger
	Tid      int
	LastOK   bool
	LastPDF  string
	Pages    int
}

func (t *CompileTexTool) Name() string { return "compile" }

func (t *CompileTexTool) Definition() map[string]any {
	def := t.MainFile
	if def == "" {
		def = "main.tex"
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "compile",
		"description": "Compile a .tex file of your workspace (default " + def + ") and get the result: errors to fix, or OK plus the output PDF name and page count. " +
			"The file must already exist (write_file/edit_file first) - never paste code here. Multi-file projects work: the main file may \\input other workspace files. " +
			"Use engine 'latexmk' for a full multi-pass build (bibliography, toc, refs); passes/bib/args cover the rest. Inspect the result with view_pdf.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":         map[string]any{"type": "string", "description": "workspace-relative main .tex file (default " + def + ")"},
				"engine":       map[string]any{"type": "string", "description": "optional: latexmk | xelatex | pdflatex | lualatex"},
				"passes":       map[string]any{"type": "integer", "description": "number of engine passes (default auto: 2 when toc/refs/bibliography are present)"},
				"bib":          map[string]any{"type": "string", "description": "optional: run bibtex or biber after the first pass"},
				"shell_escape": map[string]any{"type": "boolean", "description": "add -shell-escape (minted / externalised pgfplots)"},
				"args":         map[string]any{"type": "string", "description": "extra engine flags, space separated"},
				"timeout":      map[string]any{"type": "integer", "description": "seconds per engine pass (default: configured latex.compile.timeout)"},
			},
		},
	}}
}

func (t *CompileTexTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	if strings.TrimSpace(rel) == "" {
		rel = t.MainFile
	}
	if strings.TrimSpace(rel) == "" {
		return session.ToolResult{Text: "REJECTED: path 为空（未配置默认主文件）"}, nil
	}
	full, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	if !strings.HasSuffix(strings.ToLower(full), ".tex") {
		return session.ToolResult{Text: "REJECTED: compile 只接受 .tex 主文件（先 write_file 写好）"}, nil
	}
	if !fileExists(full) {
		return session.ToolResult{Text: "NOT FOUND: " + rel + "（先 write_file 写入内容，compile 只接收路径）"}, nil
	}
	opts := CompileOptions{
		Engine:      strings.TrimSpace(strArg(args, "engine")),
		Passes:      intArg(args, "passes", 0),
		Bib:         strings.TrimSpace(strArg(args, "bib")),
		ShellEscape: boolArg(args, "shell_escape"),
	}
	if extra := strings.TrimSpace(strArg(args, "args")); extra != "" {
		opts.ExtraArgs = strings.Fields(extra)
	}
	if secs := intArg(args, "timeout", 0); secs > 0 {
		opts.Timeout = time.Duration(secs) * time.Second
	}
	start := time.Now()
	res := t.Comp.CompileOpts(t.Root, rel, opts)
	tag := t.Tag
	if tag == "" {
		tag = "work"
	}
	LogCompileResult(t.Log, t.Tid, tag, res, time.Since(start))
	t.LastOK = res.OK
	t.LastPDF = res.PDF
	if res.OK {
		outName := strings.TrimSuffix(filepath.Base(rel), ".tex") + ".pdf"
		detail := ""
		if n, perr := pdfPageCount(res.PDF); perr == nil {
			t.Pages = n
			detail = fmt.Sprintf(" Output: %s (%d pages). See it with view_pdf {path: %q, page: 1}.", outName, n, outName)
		} else {
			detail = fmt.Sprintf(" Output: %s. See it with view_pdf {path: %q, page: 1}.", outName, outName)
		}
		text := "COMPILE OK." + detail
		if w := res.WarningSummary(); w != "" {
			text += "\n" + truncateStr(w, 1500)
		}
		return session.ToolResult{Text: text}, nil
	}
	text := "COMPILE FAILED:\n" + res.Err
	if w := res.WarningSummary(); w != "" {
		text += "\n" + truncateStr(w, 1500)
	}
	return session.ToolResult{Text: text}, nil
}

// strArg / boolArg are small typed accessors for tool arguments.
func strArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

func boolArg(args map[string]interface{}, key string) bool {
	v, _ := args[key].(bool)
	return v
}

// EditWorkFileTool applies ONE literal find/replace to any text file in
// the session workspace (增量编辑，不必整文件重写). append:true 直接把
// replace 追加到文件末尾（等价于"追加"操作，无需 find）。
type EditWorkFileTool struct {
	Root string
	// Prefixes, when non-empty, restricts edits to these workspace paths
	// (a path is allowed when it equals a prefix or lies under it): the
	// convert / style-fix sessions may only touch their OWN chapter file
	// and asset folder, never a sibling chapter's.
	Prefixes []string
}

// pathAllowed reports whether rel is one of the allowed prefixes or a
// file under them.
func (t *EditWorkFileTool) pathAllowed(rel string) bool {
	if len(t.Prefixes) == 0 {
		return true
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	for _, p := range t.Prefixes {
		p = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(p)), "/")
		if clean == p || strings.HasPrefix(clean, p+"/") {
			return true
		}
	}
	return false
}

func (t *EditWorkFileTool) Name() string { return "edit_file" }

func (t *EditWorkFileTool) Definition() map[string]any {
	desc := "Edit a file in your workspace by exact find/replace (or append:true). "
	if len(t.Prefixes) > 0 {
		desc += "Editable paths: " + strings.Join(t.Prefixes, ", ") + " (and files under them)."
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "edit_file",
		"description": desc + "Either (a) literal find/replace: give find+replace (find must occur exactly once unless replace_all=true), or " +
			"(b) append: set append=true and give replace (the text is appended to the end of the file; find ignored). " +
			"Never rewrite a whole file when a small edit suffices.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "workspace-relative file path"},
				"find":        map[string]any{"type": "string", "description": "literal text to find (exact, once)"},
				"replace":     map[string]any{"type": "string", "description": "replacement text / appended text"},
				"append":      map[string]any{"type": "boolean", "description": "true = append replace to end of file"},
				"replace_all": map[string]any{"type": "boolean", "description": "replace every occurrence of find"},
			},
			"required": []string{"path", "replace"},
		},
	}}
}

func (t *EditWorkFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	find, _ := args["find"].(string)
	repl, _ := args["replace"].(string)
	appendMode, _ := args["append"].(bool)
	replaceAll, _ := args["replace_all"].(bool)
	if strings.TrimSpace(rel) == "" {
		return session.ToolResult{}, fmt.Errorf("path 为空")
	}
	if m := explicitMountOf(rel); m != "" {
		if m != "work" {
			return session.ToolResult{Text: "REJECTED: mount \"" + m + "\" is read-only here; edit files under your own workspace (work:...)."}, nil
		}
		rel = stripMount(rel)
	}
	if !t.pathAllowed(rel) {
		return session.ToolResult{Text: "REJECTED: " + rel + " is outside your own files (" +
			strings.Join(t.Prefixes, ", ") + "). Other chapters are READ-ONLY reference material — never edit them."}, nil
	}
	full, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	var data []byte
	if st, serr := os.Stat(full); serr == nil {
		if st.IsDir() {
			return session.ToolResult{}, fmt.Errorf("%s 是目录", rel)
		}
		data, err = os.ReadFile(full)
		if err != nil {
			return session.ToolResult{}, err
		}
	} else if appendMode {
		// append 到不存在的文件 = 新建
	} else {
		return session.ToolResult{Text: "NOT FOUND: " + rel + "（新建文件请用 write_file）"}, nil
	}
	if appendMode {
		if len(data) > 0 && data[len(data)-1] != '\n' {
			data = append(data, '\n')
		}
		data = append(data, []byte(repl)...)
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: fmt.Sprintf("APPENDED %d bytes to %s.", len(repl), rel)}, nil
	}
	if find == "" {
		return session.ToolResult{Text: "REJECTED: find 为空（追加请设 append=true）"}, nil
	}
	count := strings.Count(string(data), find)
	if count == 0 {
		return session.ToolResult{Text: "NOT FOUND: 给出的 find 文本在 " + rel + " 中没有出现（注意精确匹配，包括缩进）"}, nil
	}
	if count > 1 && !replaceAll {
		return session.ToolResult{Text: fmt.Sprintf("AMBIGUOUS: find 出现 %d 次；补充上下文让匹配唯一，或设 replace_all=true", count)}, nil
	}
	var newData string
	if replaceAll {
		newData = strings.ReplaceAll(string(data), find, repl)
	} else {
		newData = strings.Replace(string(data), find, repl, 1)
	}
	if err := os.WriteFile(full, []byte(newData), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{Text: fmt.Sprintf("REPLACED %d occurrence(s) in %s (%d -> %d bytes).", count, rel, len(data), len(newData))}, nil
}

// GrepTool searches a workspace file (or the whole workspace tree) with
// a literal substring or Go regexp, returning matched lines with 1-based
// line numbers.
type GrepTool struct {
	Root string
	// AltRoots are additional read-only roots searched after Root; hits
	// are prefixed with the mount label so the model knows where the
	// file lives.
	AltRoots []AltRoot
	// Mounts overrides Root/AltRoots with the session's real mount table
	// (the same one its sandboxed bash sees). Without this, grep and
	// read_file searched ONE tree while write_file wrote another and the
	// sandbox showed a third — the convert sessions burned their first
	// turns guessing which of "work", "project", "style" meant what.
	Mounts []Mount
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Definition() map[string]any {
	desc := "Search files for a pattern (Go regexp). path: a file, or a directory (recursive, text files only). " +
		"Returns matched lines with 1-based line numbers; use read_file with those lines for context."
	if v := t.vfs(); len(v.Mounts) > 1 {
		desc += " " + v.Describe()
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "grep",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":     map[string]any{"type": "string", "description": "Go regular expression"},
				"path":        map[string]any{"type": "string", "description": "file or directory, workspace-relative; default = workspace root"},
				"max_matches": map[string]any{"type": "integer", "description": "cap on returned lines (default 50)"},
			},
			"required": []string{"pattern"},
		},
	}}
}

// vfs returns the tool's namespace (Mounts wins over Root/AltRoots).
func (t *GrepTool) vfs() *VFS {
	if len(t.Mounts) > 0 {
		return &VFS{Mounts: t.Mounts}
	}
	return vfsFrom(t.Root, t.AltRoots)
}

func (t *GrepTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return session.ToolResult{}, fmt.Errorf("pattern 为空")
	}
	rel, _ := args["path"].(string)
	rel = strings.TrimSpace(rel)
	bare := rel == "" || rel == "." || rel == "./"
	if bare {
		rel = "."
	}
	maxMatches := intArg(args, "max_matches", 50)
	if maxMatches > 200 {
		maxMatches = 200
	}
	// 有挂载表时按 VFS 解析（`project:style`、`/work/x.tex` 都认），
	// 无前缀的相对路径 = 遍历全部挂载点（提示词承诺 grep 搜整个项目）。
	type target struct {
		dir, label string
		explicit   bool
	}
	var targets []target
	if len(t.Mounts) > 0 {
		v := &VFS{Mounts: t.Mounts}
		if bare {
			for _, m := range t.Mounts {
				if m.Dir != "" {
					targets = append(targets, target{dir: m.Dir, label: m.Name, explicit: true})
				}
			}
		} else {
			full, label, rerr := v.Resolve(rel, false)
			if rerr != nil {
				return session.ToolResult{Text: "grep error: " + rerr.Error()}, nil
			}
			targets = append(targets, target{dir: full, label: label, explicit: true})
		}
	} else {
		targets = append(targets, target{dir: filepath.Join(t.Root, rel)})
		for _, ar := range t.AltRoots {
			if ar.Dir != "" {
				full, rerr := resolveInside(ar.Dir, rel)
				if rerr != nil {
					continue
				}
				targets = append(targets, target{dir: full, label: ar.Label, explicit: true})
			}
		}
	}
	var out strings.Builder
	total := 0
	multi := len(targets) > 1
	for _, tg := range targets {
		full := tg.dir
		if !fileExists(full) {
			continue
		}
		text, herr := runGrep(pattern, full, maxMatches-total)
		if herr != nil {
			if len(targets) == 1 {
				return session.ToolResult{Text: "grep error: " + herr.Error()}, nil
			}
			continue
		}
		if text == "" {
			continue
		}
		// 绝对路径改回挂载点相对路径，省 token
		for _, m := range t.Mounts {
			if m.Dir == "" {
				continue
			}
			if abs, aerr := filepath.Abs(m.Dir); aerr == nil {
				text = strings.ReplaceAll(text, abs+"/", m.Name+"/")
			}
		}
		text = strings.ReplaceAll(text, full+"/", "")
		text = strings.ReplaceAll(text, full, ".")
		if multi && tg.label != "" && !strings.HasPrefix(text, tg.label+"/") {
			lines := strings.Split(text, "\n")
			for i, ln := range lines {
				if ln != "" {
					lines[i] = tg.label + "/" + ln
				}
			}
			text = strings.Join(lines, "\n")
		}
		out.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			out.WriteString("\n")
		}
		total += strings.Count(text, "\n")
		if total >= maxMatches {
			break
		}
	}
	if out.Len() == 0 {
		return session.ToolResult{Text: "NO MATCHES: " + pattern}, nil
	}
	return session.ToolResult{Text: out.String()}, nil
}

// runGrep runs grep -rIn on a file or directory and returns the raw
// output, or an error for real failures (bad regexp, ...). "No match"
// is not an error.
func runGrep(pattern, full string, limit int) (string, error) {
	if limit < 1 {
		limit = 1
	}
	cmd := exec.Command("grep", "-rIn", "-n", "-e", pattern, "--", full)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("%s", text)
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > limit {
		text = strings.Join(lines[:limit], "\n") + fmt.Sprintf("\n...(%d more, 收窄 pattern)", len(lines)-limit)
	}
	return text, nil
}

// blockedPatterns rejects commands that try to escape the workspace or
// damage the host. This is a tripwire, not a security boundary: the
// tool runs with the user's own privileges on their own machine.
var blockedPatterns = []string{
	"sudo", "rm -rf /", "mkfs", ":(){", "fork bomb", "dd if=",
	"curl", "wget", "/etc/", "/dev/sd", "chmod 777 /", "mv / ",
}

// WorkBashTool runs a shell command with cwd = the session workspace
// root. timeout (seconds, default 30, max 300) is chosen by the model;
// output is capped at maxOutput chars (default 5000).
type WorkBashTool struct {
	// Root is the session workspace (also the cwd); kept for the
	// non-sandboxed fallback when Mounts is empty.
	Root string
	// TmpDir, when set, is bound at /tmp inside the sandbox and persists
	// across bash calls of the SAME session (it is removed when the
	// session ends, unless keep_temp_dirs/debug keeps it). Without it the
	// sandbox gets a fresh tmpfs per call, so a file written by one bash
	// call is gone in the next — the 2026-09-11 run lost /tmp/p10-10.pgm
	// and a probe t5.tex this way and the session spent ~10 rounds plus
	// two FileNotFoundError tracebacks rediscovering it.
	TmpDir string
	// Python, when set, is the Python environment this session's bash
	// sees: the interpreter (system/venv/conda) is bound read-only into
	// the sandbox, PATH points at it, and a missing module is installed
	// on the HOST (the sandbox has --unshare-net) with the model told to
	// retry the same command.
	Python *PythonEnv
	// Mounts is the session's mount table. Inside the sandbox every
	// mount appears as a top-level directory of the same name, so the
	// path after the prefix is IDENTICAL in the structured tools and in
	// bash: work:chapters/a.tex <-> /work/chapters/a.tex.
	Mounts []Mount
	// Sandbox wraps the command in bubblewrap (kernel-enforced binds:
	// writable mounts are writable, read-only mounts are read-only,
	// everything else does not exist). Falls back to a plain shell with
	// a warning when bubblewrap is unavailable.
	Sandbox   bool
	MaxOutput int    // 字符上限；<=0 取默认 5000
	RootNote  string // 提示词中说明工作区内容的备注
	Log       *logger.Logger
	Tid       int
}

// Dir is the workspace (cwd) of the tool.
func (t *WorkBashTool) Dir() string {
	for _, m := range t.Mounts {
		if m.Name == "work" && m.Dir != "" {
			return m.Dir
		}
	}
	return t.Root
}

func (t *WorkBashTool) Name() string { return "bash" }

// defaultSandboxPath is the PATH inside the sandbox (TeX Live first:
// the sessions compile with xelatex).
const defaultSandboxPath = "/usr/local/texlive/2026/bin/x86_64-linux:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/texlive/bin"

// PathWith prepends this environment's bin to a PATH value.
func (p *PythonEnv) PathWith(path string) string {
	if p == nil {
		return path
	}
	if bin := p.BinDir(); bin != "" {
		return bin + ":" + path
	}
	return path
}

func (t *WorkBashTool) Definition() map[string]any {
	desc := "Run a shell command. cwd = your workspace. "
	if t.Sandbox {
		desc += "The command runs inside a kernel sandbox where only these trees exist (everything else, including the real host paths, is invisible; writes outside the writable one fail): "
	} else {
		desc += "WARNING: no sandbox is active, stay inside your workspace. Available trees: "
	}
	for _, m := range t.Mounts {
		if m.Dir == "" {
			continue
		}
		kind := "read-only"
		if m.Writable {
			kind = "writable"
		}
		desc += fmt.Sprintf("/%s (%s), ", m.Name, kind)
	}
	desc += "and these are exactly the trees of the file tools — the suffix after the prefix is the same: work:chapters/a.tex = /work/chapters/a.tex, project:source/book.md = /project/source/book.md, source:book_part1.pdf = /source/book_part1.pdf. " +
		"Only /tmp is a scratch area (it persists across your bash calls). Useful for wc/sed/awk/ls/diff. timeout: seconds (default 30, max 300). Output is capped."
	if t.Python != nil {
		if d := t.Python.Describe(); d != "" {
			desc += " " + d
		}
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "bash",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"timeout": map[string]any{"type": "integer", "description": "seconds (default 30, max 300)"},
			},
			"required": []string{"command"},
		},
	}}
}

// pathExists reports whether a directory OR file exists (util.FileExists
// intentionally excludes directories, which is wrong for mount sources).
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// logf writes a warning through the session logger (best effort).
func (t *WorkBashTool) logf(format string, a ...any) {
	if t.Log != nil {
		t.Log.LogWarning(t.Tid, fmt.Sprintf(format, a...))
	}
}

// bwrapAvailable reports whether bubblewrap can be used on this host.
func bwrapAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// sandboxArgs builds the bubblewrap argv for one command: system trees
// are bound read-only, each mount of the session is bound at /<name>
// (writable mounts with --bind, the rest with --ro-bind), the network is
// unshared and /tmp is a private tmpfs. Symlinked entries (e.g. the
// pdfview of the original PDFs) are bound per file.
func (t *WorkBashTool) sandboxArgs(command string, fast bool) []string {
	tmp := "/tmp"
	if t.TmpDir != "" {
		if abs, err := filepath.Abs(t.TmpDir); err == nil {
			if err := os.MkdirAll(abs, 0o755); err == nil {
				tmp = abs
			}
		}
	}
	args := []string{"--die-with-parent", "--unshare-net", "--dev", "/dev", "--proc", "/proc"}
	if tmp == "/tmp" {
		args = append(args, "--tmpfs", "/tmp")
	} else {
		// 会话级持久 /tmp：同一个会话的多次 bash 调用看到同一批临时文件。
		args = append(args, "--bind", tmp, "/tmp")
	}
	for _, p := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if pathExists(p) {
			args = append(args, "--ro-bind", p, p)
		}
	}
	work := ""
	for _, m := range t.Mounts {
		if m.Dir == "" {
			continue
		}
		dst := "/" + m.Name
		// bwrap resolves its source paths itself (not through our process
		// CWD), and the default config is RELATIVE ("./latex_project",
		// "./mineru_output"). Handing those to bwrap made every sandboxed
		// bash fail with "bwrap: Can't find source path …" — the session
		// then lost ls/grep entirely and started guessing file names.
		mountDir, aerr := filepath.Abs(m.Dir)
		if aerr != nil {
			continue
		}
		entries, err := os.ReadDir(mountDir)
		if err != nil {
			continue
		}
		linked := false
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				linked = true
				break
			}
		}
		if linked {
			// A VIEW directory (symlinks to files and/or directories:
			// the original-PDF view, the project view): bind every target
			// individually, never following links outside the mounts.
			args = append(args, "--dir", dst)
			for _, e := range entries {
				src := filepath.Join(mountDir, e.Name())
				if target, lerr := filepath.EvalSymlinks(src); lerr == nil {
					src = target
				}
				if _, serr := os.Stat(src); serr != nil {
					continue
				}
				args = append(args, "--ro-bind", src, filepath.Join(dst, e.Name()))
			}
		} else if m.Writable {
			args = append(args, "--dir", dst, "--bind", mountDir, dst)
		} else {
			args = append(args, "--dir", dst, "--ro-bind", mountDir, dst)
		}
		if m.Name == "work" {
			work = dst
		}
	}
	if work == "" {
		work = "/work"
	}
	// PATH/LANG are set explicitly: the sandbox must not depend on the
	// inherited environment (and 'bash' is exec'd by absolute path).
	args = append(args, "--chdir", work, "--setenv", "HOME", work, "--setenv", "PWD", work,
		"--setenv", "TMPDIR", "/tmp", "--setenv", "TMP", "/tmp", "--setenv", "TEMP", "/tmp",
		"--setenv", "PATH", defaultSandboxPath)
	if lang := os.Getenv("LANG"); lang != "" {
		args = append(args, "--setenv", "LANG", lang)
	}
	if t.Python != nil {
		// 环境只读可见。venv/conda 前缀按**原路径**绑定：它们不可搬迁
		// （bin/ 下的脚本硬编码创建时的路径）。
		args = append(args, t.Python.BindArgs()...)
		for _, kv := range t.Python.EnvVars(defaultSandboxPath) {
			args = append(args, "--setenv", kv[0], kv[1])
		}
	}
	if fast {
		args = append(args, "--")
	}
	sh := "bash"
	if p, err := exec.LookPath("bash"); err == nil {
		sh = p // absolute: never depend on PATH lookup order
	}
	args = append(args, sh, "-c", command)
	return args
}

func (t *WorkBashTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		return session.ToolResult{}, fmt.Errorf("command 为空")
	}
	timeout := intArg(args, "timeout", 30)
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}
	cap := t.MaxOutput
	if cap <= 0 {
		cap = 5000
	}
	lower := strings.ToLower(command)
	for _, p := range blockedPatterns {
		if strings.Contains(lower, p) {
			return session.ToolResult{Text: "BLOCKED: command refused by the sandbox policy."}, nil
		}
	}
	dir := t.Dir()
	var cmd *exec.Cmd
	if t.Sandbox && bwrapAvailable() {
		cmd = exec.Command("bwrap", t.sandboxArgs(command, false)...)
		cmd.Dir = dir
		cmd.Env = os.Environ()
	} else {
		if t.Sandbox {
			t.logf("bash 沙箱不可用（bwrap 未安装），本会话回退为普通 shell")
		}
		cmd = exec.Command("bash", "-c", command)
		cmd.Dir = dir
		env := append(os.Environ(), "HOME="+dir)
		if t.Python != nil {
			env = append(env, "PATH="+t.Python.PathWith(os.Getenv("PATH")))
		}
		cmd.Env = env
	}
	done := make(chan error, 1)
	var out []byte
	go func() {
		o, err := cmd.CombinedOutput()
		out = o
		done <- err
	}()
	select {
	case err := <-done:
		text := string(out)
		// 缺包是"环境问题"而不是"模型写错了"：沙箱断网装不了，由宿主侧装好
		// 并让模型重试同一条命令（2026-09-11 的样式会话为 PIL 手写过一个
		// PGM 解析器，转换会话写了 /tmp/p10-10.pgm 也读不回来）。
		note := ""
		if t.Python != nil {
			note = t.Python.NoteFor(text)
		}
		if len(text) > cap {
			text = text[:cap] + fmt.Sprintf("...(%d bytes more)", len(out)-cap)
		}
		text += note
		if err != nil {
			return session.ToolResult{Text: fmt.Sprintf("exit error: %v\n%s", err, text)}, nil
		}
		return session.ToolResult{Text: text}, nil
	case <-time.After(time.Duration(timeout) * time.Second):
		_ = cmd.Process.Kill()
		return session.ToolResult{Text: fmt.Sprintf("TIMEOUT after %ds（可设更大的 timeout 参数，上限 300）", timeout)}, nil
	}
}

// ViewPDFTool lets the model LOOK at one page of a workspace PDF: the
// page is (re)rendered from the PDF itself at the requested width, so
// crop + zoom stay sharp at any magnification. Always re-renders, never
// scales existing pixels.
type ViewPDFTool struct {
	Root string
	// Mounts, when set, makes the tool resolve paths through the
	// session's virtual workspace (so the read-only "source" mount with
	// the ORIGINAL book PDFs is viewed with the same tool).
	Mounts []Mount
	// Comp provides the pdftoppm rasterizer settings (dpi); nil uses a
	// plain pdftoppm call.
	Comp *Compiler
	// SoftMax / WarnRatio implement the (soft) view budget for view_pdf,
	// counted per (file, page) — same semantics as ViewImageTool;
	// configured by tools.view.pdf_max / tools.view.warn_ratio.
	// Counting per page means a long book is not punished by a session
	// total: each page gets its own allowance.
	// SoftMax 0 = no budget.
	SoftMax   int
	WarnRatio float64
	views     map[string]int  // "file#page" -> render count
	warned    map[string]bool // already reminded about this page
}

// vfs returns the tool's namespace (Mounts wins over Root).
func (t *ViewPDFTool) vfs() *VFS {
	if len(t.Mounts) > 0 {
		return &VFS{Mounts: t.Mounts}
	}
	return vfsFrom(t.Root, nil)
}

func (t *ViewPDFTool) Name() string { return "view_pdf" }

func (t *ViewPDFTool) Definition() map[string]any {
	desc := "Render ONE page of a PDF and attach it as an image: PDFs you compiled (chapter/figure/book) AND, " +
		"where mounted, the ORIGINAL book PDFs. Crop (percent) and zoom (target pixel width) work the same in both cases; " +
		"the page is re-rendered from the PDF at the requested width, so zoom stays sharp."
	if d := t.vfs().Describe(); d != "" {
		desc += " " + d
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_pdf",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "PDF path (workspace-relative, or name:path for another mount)"},
				"page":   map[string]any{"type": "integer", "description": "1-based page number (default 1)"},
				"left":   map[string]any{"type": "number", "description": "crop left percent 0-100"},
				"top":    map[string]any{"type": "number", "description": "crop top percent 0-100"},
				"right":  map[string]any{"type": "number", "description": "crop right percent 0-100 (default 100)"},
				"bottom": map[string]any{"type": "number", "description": "crop bottom percent 0-100 (default 100)"},
				"zoom":   map[string]any{"type": "integer", "description": "target width in px (default 1280)"},
			},
			"required": []string{"path"},
		},
	}}
}

func (t *ViewPDFTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	if strings.TrimSpace(rel) == "" {
		return session.ToolResult{}, fmt.Errorf("path 为空")
	}
	full, _, err := t.vfs().Resolve(rel, false)
	if err != nil {
		return session.ToolResult{}, err
	}
	if !strings.HasSuffix(strings.ToLower(full), ".pdf") {
		return session.ToolResult{Text: "REJECTED: view_pdf 只接受 .pdf（图片请用 view_image）"}, nil
	}
	if _, err := os.Stat(full); err != nil {
		return session.ToolResult{}, fmt.Errorf("文件不存在: %s%s（可用挂载点: %s）", rel, suggestNear(full, 12), t.vfs().Names())
	}
	page := intArg(args, "page", 1)
	if page < 1 {
		page = 1
	}
	total := 0
	if n, err := pdfPageCount(full); err == nil {
		total = n
		if page > n {
			return session.ToolResult{Text: fmt.Sprintf("OUT OF RANGE: %s 共 %d 页，请求第 %d 页", rel, n, page)}, nil
		}
	}
	// 看图预算（软）：按 文件+页 计数，只提醒不拦截。
	if t.views == nil {
		t.views = map[string]int{}
	}
	key := fmt.Sprintf("%s#%d", rel, page)
	t.views[key]++
	used := t.views[key]
	left := pctArg(args, "left", 0)
	top := pctArg(args, "top", 0)
	right := pctArg(args, "right", 100)
	bottom := pctArg(args, "bottom", 100)
	zoom := intArg(args, "zoom", 0)
	if zoom <= 0 {
		zoom = intArg(args, "zoom_width", 0) // alias kept from the old source-page viewer
	}
	if zoom <= 0 {
		zoom = 1280
	}
	if zoom > 6000 {
		zoom = 6000
	}

	// 从 PDF 按目标宽度重渲染（裁剪按比例放大整页宽度），始终清晰。
	frac := (clampPct(right) - clampPct(left)) / 100
	if frac < 0.02 {
		frac = 1
	}
	pageW := int(float64(zoom)/frac) + 8
	if pageW < 400 {
		pageW = 400
	}
	if pageW > 6000 {
		pageW = 6000
	}
	dir, err := os.MkdirTemp("", "dsv-viewpdf-")
	if err != nil {
		return session.ToolResult{}, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "page")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftoppm", "-png", "-singlefile",
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page),
		"-scale-to-x", strconv.Itoa(pageW), "-scale-to-y", "-1",
		full, out)
	if o, err := cmd.CombinedOutput(); err != nil {
		return session.ToolResult{}, fmt.Errorf("PDF 渲染失败: %v: %s", err, truncateStr(string(o), 300))
	}
	img, err := decodeImage(out + ".png")
	if err != nil {
		return session.ToolResult{}, err
	}
	if left > 0 || top > 0 || right < 100 || bottom < 100 {
		img = cropPercent(img, left, top, right, bottom)
	}
	img = scaleToWidth(img, zoom)
	b64, err := encodeJPEGBase64(img)
	if err != nil {
		return session.ToolResult{}, err
	}
	note := ""
	if total > 0 {
		note = fmt.Sprintf(" (page %d/%d)", page, total)
	}
	// 物理尺寸：页面本身 + 当前裁剪区域（看原书页时就是真实书本尺寸，
	// 看自己编的 PDF 时就是成品的实际尺寸——两者同单位才好对照）。
	sizeNote := ""
	if wMM, hMM, ok := pdfPageSizeMM(full, page); ok {
		sizeNote = fmt.Sprintf(", page %.1fmm x %.1fmm", wMM, hMM)
		if left > 0 || top > 0 || right < 100 || bottom < 100 {
			fw := (clampPct(right) - clampPct(left)) / 100
			fh := (clampPct(bottom) - clampPct(top)) / 100
			if fw > 0 && fh > 0 {
				sizeNote += fmt.Sprintf(", this crop %.1fmm x %.1fmm", wMM*fw, hMM*fh)
			}
		}
	}
	return session.ToolResult{
		Text: fmt.Sprintf("PDF page %s p%d (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx%s)%s attached.", rel, page, left, top, right, bottom, zoom, sizeNote, note) +
			t.budgetOnce(key, rel, page, used),
		ImageBase64: b64,
		ImageMIME:   "image/jpeg",
	}, nil
}

// budgetOnce appends the (soft) view-budget reminder at most once per page.
func (t *ViewPDFTool) budgetOnce(key, rel string, page, used int) string {
	if t.warned == nil {
		t.warned = map[string]bool{}
	}
	return viewBudgetNoteOnce(t.warned, key, fmt.Sprintf("view_pdf on %s p%d", filepath.Base(rel), page), used, t.SoftMax, t.WarnRatio)
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// pdfPageCount returns the page count of a PDF via pdfinfo.
// pdfPageSize returns the first page's size in points (pdfinfo reports
// "Page size: 307.56 x 231.02 pts"). Used to tell the drawing session how
// large its picture actually came out, so it can match the original's
// physical size and aspect ratio.
func pdfPageSize(pdf string) (w, h float64, err error) {
	out, err := exec.Command("pdfinfo", pdf).CombinedOutput()
	if err != nil {
		return 0, 0, err
	}
	first, haveFirst, perPage := parsePDFInfoSizes(string(out))
	if haveFirst {
		return first[0], first[1], nil
	}
	if sz, ok := perPage[1]; ok {
		return sz[0], sz[1], nil
	}
	return 0, 0, fmt.Errorf("pdfinfo: 未找到 Page size 行")
}

// parsePDFInfoSizes extracts page sizes (points) from pdfinfo output.
// poppler prints the FIRST page as "Page size:  W x H pts" and, with
// -f/-l, each requested page as "Page    2 size:  W x H pts" — the number
// is padded with spaces, so the line is tokenized instead of prefix-matched
// (a prefix match silently misses every per-page size).
func parsePDFInfoSizes(out string) (first [2]float64, haveFirst bool, perPage map[int][2]float64) {
	perPage = map[int][2]float64{}
	for _, ln := range strings.Split(out, "\n") {
		f := strings.Fields(ln)
		if len(f) < 5 || f[0] != "Page" {
			continue
		}
		var num [2]float64
		var pageNo int
		var ok bool
		if f[1] == "size:" && len(f) >= 5 { // "Page size: W x H pts"
			ok = sscanf2(f[2], f[4], &num)
		} else if strings.HasSuffix(f[1], ":") || len(f) >= 7 { // 数字列
			ok = false
		}
		if len(f) >= 7 && f[2] == "size:" { // "Page 2 size: W x H pts"
			if n := atoiSafe(f[1]); n > 0 {
				pageNo = n
				ok = sscanf2(f[3], f[5], &num)
			}
		}
		if !ok || num[0] <= 0 || num[1] <= 0 {
			continue
		}
		if pageNo > 0 {
			perPage[pageNo] = num
		} else if !haveFirst {
			first, haveFirst = num, true
		}
	}
	return first, haveFirst, perPage
}

// sscanf2 parses "283.46" and "425.2" into a size pair.
func sscanf2(a, b string, out *[2]float64) bool {
	var x, y float64
	if _, err := fmt.Sscanf(a, "%f", &x); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(b, "%f", &y); err != nil {
		return false
	}
	out[0], out[1] = x, y
	return true
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func ptToMM(v float64) float64 { return v * 25.4 / 72 }

// pdfPageSizeMM returns ONE page's real size in millimetres. mm is the unit
// the original-figure measurement uses, so page size, crop size and drawn
// size can be compared directly — the model needs that to judge whether what
// it produces matches the real book.
func pdfPageSizeMM(pdf string, page int) (wMM, hMM float64, ok bool) {
	args := []string{}
	if page > 0 {
		args = append(args, "-f", strconv.Itoa(page), "-l", strconv.Itoa(page))
	}
	args = append(args, pdf)
	out, err := exec.Command("pdfinfo", args...).CombinedOutput()
	if err != nil {
		return 0, 0, false
	}
	first, haveFirst, perPage := parsePDFInfoSizes(string(out))
	if page > 0 {
		if sz, found := perPage[page]; found {
			return ptToMM(sz[0]), ptToMM(sz[1]), true
		}
	}
	if haveFirst {
		return ptToMM(first[0]), ptToMM(first[1]), true
	}
	for _, sz := range perPage {
		return ptToMM(sz[0]), ptToMM(sz[1]), true
	}
	return 0, 0, false
}

func pdfPageCount(pdf string) (int, error) {
	out, err := exec.Command("pdfinfo", pdf).CombinedOutput()
	if err != nil {
		return 0, err
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if n, ok := strings.CutPrefix(ln, "Pages:"); ok {
			var v int
			if _, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &v); err == nil {
				return v, nil
			}
		}
	}
	return 0, fmt.Errorf("pdfinfo: 未找到 Pages 行")
}
