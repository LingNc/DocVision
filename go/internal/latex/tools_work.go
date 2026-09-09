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

	"mineru-tools/internal/session"
)

// EditWorkFileTool applies ONE literal find/replace to any text file in
// the session workspace (增量编辑，不必整文件重写). append:true 直接把
// replace 追加到文件末尾（等价于"追加"操作，无需 find）。
type EditWorkFileTool struct {
	Root string
}

func (t *EditWorkFileTool) Name() string { return "edit_file" }

func (t *EditWorkFileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "edit_file",
		"description": "Edit a workspace text file incrementally. Either " +
			"(a) literal find/replace: give find+replace (find must occur exactly once unless replace_all=true), or " +
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
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "grep",
		"description": "Search files for a pattern (Go regexp). path: a file, or a directory (recursive, text files only). " +
			"Returns matched lines with 1-based line numbers; use read_file with those lines for context.",
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
	full, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	maxMatches := intArg(args, "max_matches", 50)
	if maxMatches > 200 {
		maxMatches = 200
	}
	cmd := exec.Command("grep", "-rIn", "-n", "-e", pattern, "--", full)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return session.ToolResult{Text: "NO MATCHES: " + pattern}, nil
		}
		// grep 传目录时会递归；其他错误（坏正则等）原样返回
		return session.ToolResult{Text: "grep error: " + text}, nil
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > maxMatches {
		text = strings.Join(lines[:maxMatches], "\n") + fmt.Sprintf("\n...(%d more, 收窄 pattern)", len(lines)-maxMatches)
	}
	// 绝对路径改回工作区相对路径，省 token
	text = strings.ReplaceAll(text, full+"/", "")
	text = strings.ReplaceAll(text, full, ".")
	return session.ToolResult{Text: text}, nil
}

// WorkBashTool runs a shell command with cwd = the session workspace
// root. timeout (seconds, default 30, max 300) is chosen by the model;
// output is capped at maxOutput chars (default 5000).
type WorkBashTool struct {
	Dir       string
	MaxOutput int    // 字符上限；<=0 取默认 5000
	RootNote  string // 提示词中说明工作区内容的备注
}

func (t *WorkBashTool) Name() string { return "bash" }

func (t *WorkBashTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "bash",
		"description": "Run a shell command inside the session workspace (cwd = workspace root; it is NOT a security sandbox - do not write outside). " +
			"Useful for wc/sed/awk/ls to inspect files. timeout: seconds (default 30, max 300). Output capped at 5000 chars.",
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
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = t.Dir
	cmd.Env = append(os.Environ(), "HOME="+t.Dir)
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
	// Comp provides the pdftoppm rasterizer settings (dpi); nil uses a
	// plain pdftoppm call.
	Comp *Compiler
}

func (t *ViewPDFTool) Name() string { return "view_pdf" }

func (t *ViewPDFTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "view_pdf",
		"description": "Render ONE page of a workspace PDF and attach it as an image. " +
			"Works for compiled chapter/figure/preview PDFs. Crop and zoom behave like view_image; " +
			"the page is re-rendered from the PDF at the requested width (sharp at any zoom).",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "workspace-relative PDF path"},
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
	full, err := resolveInside(t.Root, rel)
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
	left := pctArg(args, "left", 0)
	top := pctArg(args, "top", 0)
	right := pctArg(args, "right", 100)
	bottom := pctArg(args, "bottom", 100)
	zoom := intArg(args, "zoom", 0)
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
		Text:        fmt.Sprintf("PDF page %s p%d (crop %.0f%%,%.0f%%-%.0f%%,%.0f%%, width %dpx)%s attached.", rel, page, left, top, right, bottom, zoom, note),
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
