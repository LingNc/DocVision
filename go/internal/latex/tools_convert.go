package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/session"
)

// ReadFileTool gives a conversion session read-only access to the
// project files (manual, class, own + other chapters, original md).
// Paths are resolved under a whitelist root and results are bounded.
type ReadFileTool struct {
	Root string
	// MaxBytes caps one read (default 64KB).
	MaxBytes int
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "read_file",
		"description": "Read a project file (path relative to the project root): the class manual (/style/manual.md), your chapter markdown, other chapters (read-only, for cross-references), the original markdown. Read-only.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
	}}
}

func (t *ReadFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	full, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{}, err
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
		note = "\n...[truncated; use offset reads of the source markdown for big files]..."
	}
	return session.ToolResult{Text: fmt.Sprintf("[%s, %d bytes]\n%s%s", rel, len(data), text, note)}, nil
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

// WriteFileTool writes EXACTLY one assigned file inside the virtual
// project tree (the chapter's own .tex). Content replaces the file.
type WriteFileTool struct {
	Root       string
	AllowedRel string // the only writable path, relative to Root
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "write_file",
		"description": "Write YOUR chapter .tex file with the FULL new content (it replaces the file). This is the only file you can write.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string"},
			},
			"required": []string{"content"},
		},
	}}
}

func (t *WriteFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return session.ToolResult{Text: "REJECTED: empty content."}, nil
	}
	if strings.Contains(content, "\\documentclass") {
		return session.ToolResult{Text: "REJECTED: the chapter must be an \\input fragment — no \\documentclass / preamble."}, nil
	}
	full, err := resolveInside(t.Root, t.AllowedRel)
	if err != nil {
		return session.ToolResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return session.ToolResult{}, err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{Text: fmt.Sprintf("WROTE %s (%d bytes). Call compile to check, then submit when clean.", t.AllowedRel, len(content))}, nil
}

// CompileChapterTool compiles the chapter .tex inside a scratch
// wrapper that uses the book class, returning the error log only.
type CompileChapterTool struct {
	Comp       *Compiler
	Scratch    string // per-chapter scratch dir (contains wrapper + cls copy)
	MainFile   string
	SourcePath string // live chapter .tex in the virtual work tree; copied into Scratch before compiling
}

func (t *CompileChapterTool) Name() string { return "compile" }

func (t *CompileChapterTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "compile",
		"description": "Compile your current .tex with the book class in a scratch wrapper. Returns OK or the error log.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *CompileChapterTool) Execute(_ string) (session.ToolResult, error) {
	if t.SourcePath != "" {
		data, err := os.ReadFile(t.SourcePath)
		if err != nil {
			return session.ToolResult{Text: "COMPILE SKIPPED: " + t.MainFile + " has not been written yet (call write_file first)."}, nil
		}
		if err := os.WriteFile(filepath.Join(t.Scratch, t.MainFile), data, 0o644); err != nil {
			return session.ToolResult{}, err
		}
	}
	res := t.Comp.Compile(t.Scratch, t.MainFile)
	if res.OK {
		return session.ToolResult{Text: "COMPILE OK."}, nil
	}
	return session.ToolResult{Text: "COMPILE FAILED:\n" + res.Err}, nil
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

// EditFileTool applies a literal search/replace batch to one project
// file (fix session). Read/repair only; no structural rewrites.
type EditFileTool struct {
	Root string
}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "edit_file",
		"description": "Apply ONE literal find/replace to a project .tex file. Minimal changes only.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"find":    map[string]any{"type": "string"},
				"replace": map[string]any{"type": "string"},
			},
			"required": []string{"path", "find", "replace"},
		},
	}}
}

func (t *EditFileTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	rel, _ := args["path"].(string)
	find, _ := args["find"].(string)
	repl, _ := args["replace"].(string)
	if find == "" {
		return session.ToolResult{}, fmt.Errorf("find 为空")
	}
	full, err := resolveInside(t.Root, rel)
	if err != nil {
		return session.ToolResult{}, err
	}
	if !strings.HasSuffix(full, ".tex") && !strings.HasSuffix(full, ".cls") {
		return session.ToolResult{Text: "REJECTED: only .tex/.cls files may be edited."}, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return session.ToolResult{}, err
	}
	count := strings.Count(string(data), find)
	if count == 0 {
		return session.ToolResult{Text: "NOT FOUND: " + rel}, nil
	}
	newData := strings.ReplaceAll(string(data), find, repl)
	if err := os.WriteFile(full, []byte(newData), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{Text: fmt.Sprintf("REPLACED %d occurrence(s) in %s.", count, rel)}, nil
}

// RecompileTool reruns the full-book compile for the fix session.
type RecompileTool struct {
	Comp     *Compiler
	Dir      string
	MainFile string
	LastOK   bool
}

func (t *RecompileTool) Name() string { return "recompile" }

func (t *RecompileTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "recompile",
		"description": "Recompile the full book. Returns OK when the PDF builds.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *RecompileTool) Execute(_ string) (session.ToolResult, error) {
	res := t.Comp.Compile(t.Dir, t.MainFile)
	t.LastOK = res.OK
	if res.OK {
		return session.ToolResult{Text: "COMPILE OK. The book PDF built successfully."}, nil
	}
	return session.ToolResult{Text: "COMPILE FAILED:\n" + res.Err}, nil
}
