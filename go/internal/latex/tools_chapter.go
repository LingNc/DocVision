package latex

import (
	"fmt"
	"strings"

	"mineru-tools/internal/session"
)

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
