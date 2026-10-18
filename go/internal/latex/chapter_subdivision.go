package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/session"
)

const maxChapterChildren = 16
const maxChapterSplitDepth = 3

// ChapterSubdivision is one ordered, 1-based inclusive range of the parent Markdown.
type ChapterSubdivision struct {
	Title     string `json:"title"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// RequestChapterSplitTool accepts a complete, contiguous partition and persists
// the exact source ranges for orchestration. It never writes child output itself.
type RequestChapterSplitTool struct {
	Root      string
	TotalLine int
	Depth     int
	Children  []ChapterSubdivision
	Requested bool
	Restored  bool
}

func (t *RequestChapterSplitTool) Name() string { return "request_chapter_split" }

func (t *RequestChapterSplitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "request_chapter_split",
		"description": fmt.Sprintf("Request semantic subdivision of this chapter into 2-%d ordered line ranges. Ranges must cover every Markdown line exactly once. Accepted only before recursion depth %d.", maxChapterChildren, maxChapterSplitDepth),
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"children": map[string]any{"type": "array", "minItems": 2, "maxItems": maxChapterChildren, "items": map[string]any{"type": "object", "properties": map[string]any{
				"title":      map[string]any{"type": "string"},
				"start_line": map[string]any{"type": "integer", "minimum": 1},
				"end_line":   map[string]any{"type": "integer", "minimum": 1},
			}, "required": []string{"title", "start_line", "end_line"}}},
		}, "required": []string{"children"}},
	}}
}

func (t *RequestChapterSplitTool) Restore() error {
	if t.Depth >= maxChapterSplitDepth {
		return nil
	}
	path := filepath.Join(t.Root, "subdivision.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var saved struct {
		Depth    int                  `json:"depth"`
		Children []ChapterSubdivision `json:"children"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("读取已保存的细分计划失败: %w", err)
	}
	if err := validateChapterSubdivisions(saved.Children, t.TotalLine); err != nil {
		return fmt.Errorf("已保存的细分计划无效: %w", err)
	}
	if saved.Depth != t.Depth+1 {
		return fmt.Errorf("已保存的细分计划深度不匹配: got %d, want %d", saved.Depth, t.Depth+1)
	}
	t.Children, t.Requested, t.Restored = saved.Children, true, true
	return nil
}

func (t *RequestChapterSplitTool) Execute(argsJSON string) (session.ToolResult, error) {
	if t.Depth >= maxChapterSplitDepth {
		return session.ToolResult{Text: "REJECTED: recursive subdivision depth limit reached; convert this chapter directly."}, nil
	}
	var req struct {
		Children []ChapterSubdivision `json:"children"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &req); err != nil {
		return session.ToolResult{}, err
	}
	if err := validateChapterSubdivisions(req.Children, t.TotalLine); err != nil {
		return session.ToolResult{Text: "REJECTED: " + err.Error()}, nil
	}
	if t.Root == "" {
		return session.ToolResult{}, fmt.Errorf("chapter subdivision root is empty")
	}
	if err := os.MkdirAll(t.Root, 0o755); err != nil {
		return session.ToolResult{}, err
	}
	data, err := json.MarshalIndent(struct {
		Depth    int                  `json:"depth"`
		Children []ChapterSubdivision `json:"children"`
	}{t.Depth + 1, req.Children}, "", "  ")
	if err != nil {
		return session.ToolResult{}, err
	}
	path := filepath.Join(t.Root, "subdivision.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	t.Children = req.Children
	t.Requested = true
	return session.ToolResult{Text: fmt.Sprintf("SUBDIVISION REQUESTED: %d child ranges persisted. The runner will convert and check each child before returning their artifacts for parent integration.", len(req.Children)), Terminal: true}, nil
}

func validateChapterSubdivisions(children []ChapterSubdivision, totalLines int) error {
	if totalLines < 2 {
		return fmt.Errorf("chapter has fewer than two lines")
	}
	if len(children) < 2 || len(children) > maxChapterChildren {
		return fmt.Errorf("children must contain 2-%d ranges", maxChapterChildren)
	}
	next := 1
	for i, child := range children {
		if strings.TrimSpace(child.Title) == "" {
			return fmt.Errorf("child %d has an empty title", i+1)
		}
		if child.StartLine != next || child.EndLine < child.StartLine || child.EndLine > totalLines {
			return fmt.Errorf("child %d must start at line %d and end within line %d", i+1, next, totalLines)
		}
		next = child.EndLine + 1
	}
	if next != totalLines+1 {
		return fmt.Errorf("ranges stop at line %d; expected full coverage through line %d", next-1, totalLines)
	}
	return nil
}

func chapterSubrange(lines []string, r ChapterSubdivision) string {
	return strings.Join(lines[r.StartLine-1:r.EndLine], "\n")
}
