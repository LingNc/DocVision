package latex

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"mineru-tools/internal/session"
)

// GrepMDTool is a bounded regex search over the chapter-split source
// markdown; it returns line numbers so the model never needs to load
// the full document.
type GrepMDTool struct {
	Path string
}

func (t *GrepMDTool) Name() string { return "grep" }

func (t *GrepMDTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "grep",
		"description": "Regex search over the markdown. Returns up to 150 matches as 'LINENO: line' plus the total match count. Use it to map the heading structure (e.g. '^# ', '^## ') without reading the file.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":     map[string]any{"type": "string", "description": "Go regexp pattern."},
				"ignore_case": map[string]any{"type": "boolean", "description": "Case-insensitive matching. Default false."},
			},
			"required": []string{"pattern"},
		},
	}}
}

func (t *GrepMDTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return session.ToolResult{}, fmt.Errorf("pattern 为空")
	}
	expr := pattern
	if b, _ := args["ignore_case"].(bool); b {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return session.ToolResult{}, fmt.Errorf("正则无效: %v", err)
	}
	data, err := os.ReadFile(t.Path)
	if err != nil {
		return session.ToolResult{}, err
	}
	var b strings.Builder
	count := 0
	shown := 0
	for i, l := range strings.Split(string(data), "\n") {
		if re.MatchString(l) {
			count++
			if shown < 150 {
				shown++
				out := l
				if len(out) > 240 {
					out = out[:240] + "..."
				}
				fmt.Fprintf(&b, "%d: %s\n", i+1, out)
			}
		}
	}
	if count == 0 {
		return session.ToolResult{Text: "(no matches)"}, nil
	}
	if count > shown {
		fmt.Fprintf(&b, "... (%d more matches not shown; total %d)\n", count-shown, count)
	}
	return session.ToolResult{Text: b.String()}, nil
}

// ReadLinesTool reads a narrow line window of the split source.
type ReadLinesTool struct {
	Path string
}

func (t *ReadLinesTool) Name() string { return "read_lines" }

func (t *ReadLinesTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "read_lines",
		"description": "Read a window of lines (1-based, inclusive) from the markdown. Max 250 lines per call.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start_line": map[string]any{"type": "integer"},
				"end_line":   map[string]any{"type": "integer"},
			},
			"required": []string{"start_line", "end_line"},
		},
	}}
}

func (t *ReadLinesTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	start := intArg(args, "start_line", 1)
	end := intArg(args, "end_line", start)
	return readLinesFrom(t.Path, start, end, 250)
}

// SandboxBashTool is the minimal virtual shell for the splitting
// session: the sandbox directory is populated with ONLY the source
// markdown (as book.md). Commands run with a timeout; obviously
// dangerous commands are rejected. Nothing outside the sandbox is
// writable through normal usage, and the source of truth for the split
// is still the structured submit_split call.
type SandboxBashTool struct {
	Dir string
}

func (t *SandboxBashTool) Name() string { return "bash" }

func (t *SandboxBashTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "bash",
		"description": "Run a shell command inside the minimal sandbox. The virtual filesystem contains ONLY book.md (the chapter-split source). Useful for wc -l, sed -n ranges, grep -n, awk. Timeout 30s; output capped at 4000 chars.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	}}
}

// blockedPatterns rejects commands that try to escape the sandbox or
// damage the host. This is a tripwire, not a security boundary: the
// tool runs with the user's own privileges on their own machine.
var blockedPatterns = []string{
	"sudo", "rm -rf /", "mkfs", ":(){", "fork bomb", "dd if=",
	"curl", "wget", "/etc/", "/dev/sd", "chmod 777 /", "mv / ",
}

func (t *SandboxBashTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		return session.ToolResult{}, fmt.Errorf("command 为空")
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
		if len(text) > 4000 {
			text = text[:4000] + "..."
		}
		if err != nil {
			return session.ToolResult{Text: fmt.Sprintf("exit error: %v\n%s", err, text)}, nil
		}
		return session.ToolResult{Text: text}, nil
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return session.ToolResult{Text: "TIMEOUT after 30s."}, nil
	}
}

// SubmitSplitTool receives the structured chapter split. Validation of
// full coverage happens in the runner (it knows the total line count).
type SubmitSplitTool struct {
	Chapters []ChapterRange
	Set      bool
}

// ChapterRange is one chapter of the submitted split.
type ChapterRange struct {
	Title     string `json:"title"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func (t *SubmitSplitTool) Name() string { return "submit_split" }

func (t *SubmitSplitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit_split",
		"description": "Submit the final chapter split: an ordered list of {title, start_line, end_line} (1-based inclusive) covering the whole file without gaps or overlaps.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapters": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title":      map[string]any{"type": "string"},
							"start_line": map[string]any{"type": "integer"},
							"end_line":   map[string]any{"type": "integer"},
						},
						"required": []string{"title", "start_line", "end_line"},
					},
				},
			},
			"required": []string{"chapters"},
		},
	}}
}

func (t *SubmitSplitTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["chapters"].([]interface{})
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: chapters must be a non-empty array."}, nil
	}
	var out []ChapterRange
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		cr := ChapterRange{
			Title:     strings.TrimSpace(title),
			StartLine: intArg(m, "start_line", 0),
			EndLine:   intArg(m, "end_line", 0),
		}
		if cr.Title == "" || cr.StartLine < 1 || cr.EndLine < cr.StartLine {
			return session.ToolResult{Text: "REJECTED: invalid chapter entry " + fmt.Sprintf("%+v", cr)}, nil
		}
		out = append(out, cr)
	}
	// Basic ordering / overlap check; coverage vs EOF is checked later.
	for i := 1; i < len(out); i++ {
		if out[i].StartLine <= out[i-1].EndLine {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: chapter %d overlaps the previous one (starts at line %d before previous end %d).", i+1, out[i].StartLine, out[i-1].EndLine)}, nil
		}
	}
	t.Chapters, t.Set = out, true
	return session.ToolResult{Text: "SUBMITTED. The split will be validated for full coverage."}, nil
}
