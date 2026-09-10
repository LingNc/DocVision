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
			return "", "", fmt.Errorf("文件不存在: %s", rel)
		}
		return full, label, nil
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
	}
	return "", "", fmt.Errorf("文件不存在: %s", rel)
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
			detail = fmt.Sprintf(" Output: %s (%d pages). Inspect with view_pdf {path: %q, page: 1}.", outName, n, outName)
		} else {
			detail = " Output: " + outName + "."
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
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Definition() map[string]any {
	desc := "Search files for a pattern (Go regexp). path: a file, or a directory (recursive, text files only). " +
		"Returns matched lines with 1-based line numbers; use read_file with those lines for context."
	if v := vfsFrom(t.Root, t.AltRoots); len(v.Mounts) > 1 {
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
	if rel == "" {
		rel = "."
	}
	maxMatches := intArg(args, "max_matches", 50)
	if maxMatches > 200 {
		maxMatches = 200
	}
	// 主工作区 + 只读附加根（后者命中加 label/ 前缀）。
	mounts := []struct {
		dir, label string
	}{{t.Root, ""}}
	for _, ar := range t.AltRoots {
		if ar.Dir != "" {
			mounts = append(mounts, struct{ dir, label string }{ar.Dir, ar.Label})
		}
	}
	var out strings.Builder
	total := 0
	for _, m := range mounts {
		full, rerr := resolveInside(m.dir, rel)
		if rerr != nil {
			if m.label == "" {
				return session.ToolResult{}, rerr
			}
			continue
		}
		if !fileExists(full) && m.label != "" {
			continue
		}
		text, herr := runGrep(pattern, full, maxMatches-total)
		if herr != nil {
			if m.label == "" {
				return session.ToolResult{Text: "grep error: " + herr.Error()}, nil
			}
			continue
		}
		if text == "" {
			continue
		}
		// 绝对路径改回工作区相对路径，省 token
		text = strings.ReplaceAll(text, full+"/", "")
		text = strings.ReplaceAll(text, full, ".")
		if m.label != "" {
			lines := strings.Split(text, "\n")
			for i, ln := range lines {
				lines[i] = m.label + "/" + ln
			}
			text = strings.Join(lines, "\n")
		}
		out.WriteString(text)
		out.WriteString("\n")
		total += strings.Count(text, "\n")
		if total >= maxMatches {
			break
		}
	}
	res := strings.TrimRight(out.String(), "\n")
	if res == "" {
		return session.ToolResult{Text: "NO MATCHES: " + pattern}, nil
	}
	return session.ToolResult{Text: res}, nil
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
		"Only /tmp is a scratch area. Useful for wc/sed/awk/ls/diff. timeout: seconds (default 30, max 300). Output is capped."
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
	args := []string{"--die-with-parent", "--unshare-net", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp"}
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
		entries, err := os.ReadDir(m.Dir)
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
				src := filepath.Join(m.Dir, e.Name())
				if target, lerr := filepath.EvalSymlinks(src); lerr == nil {
					src = target
				}
				if _, serr := os.Stat(src); serr != nil {
					continue
				}
				args = append(args, "--ro-bind", src, filepath.Join(dst, e.Name()))
			}
		} else if m.Writable {
			args = append(args, "--dir", dst, "--bind", m.Dir, dst)
		} else {
			args = append(args, "--dir", dst, "--ro-bind", m.Dir, dst)
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
		"--setenv", "PATH", "/usr/local/texlive/2026/bin/x86_64-linux:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/texlive/bin")
	if lang := os.Getenv("LANG"); lang != "" {
		args = append(args, "--setenv", "LANG", lang)
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
		cmd.Env = append(os.Environ(), "HOME="+dir)
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
		if len(text) > cap {
			text = text[:cap] + fmt.Sprintf("...(%d bytes more)", len(out)-cap)
		}
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
	// MaxViews (0 = unlimited) caps how often one session may re-render
	// the same page; figure sessions use it to break the "look again,
	// tweak, look again" loop.
	MaxViews int
	views    map[string]int
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
		return session.ToolResult{}, fmt.Errorf("文件不存在: %s", rel)
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
	// 会话级看图预算（按 文件+页 计数）：真正渲染前拦截，既省 token
	// 也打断"看一眼→改一点→再看一眼"的空转。
	if t.MaxViews > 0 {
		if t.views == nil {
			t.views = map[string]int{}
		}
		k := fmt.Sprintf("%s#%d", rel, page)
		t.views[k]++
		if t.views[k] > t.MaxViews {
			return session.ToolResult{Text: viewBudgetMessage}, nil
		}
	}
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
	return session.ToolResult{
		Text:        fmt.Sprintf("PDF page %s p%d (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx)%s attached. If this rendering matches the original, stop viewing and call submit.", rel, page, left, top, right, bottom, zoom, note),
		ImageBase64: b64,
		ImageMIME:   "image/jpeg",
	}, nil
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
	for _, ln := range strings.Split(string(out), "\n") {
		v, ok := strings.CutPrefix(ln, "Page size:")
		if !ok {
			continue
		}
		var a, b float64
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f x %f", &a, &b); err == nil {
			return a, b, nil
		}
	}
	return 0, 0, fmt.Errorf("pdfinfo: 未找到 Page size 行")
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
