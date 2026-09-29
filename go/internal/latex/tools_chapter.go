package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/session"
)

// SubmitSplitTool receives the structured chapter split. Validation of
// full coverage happens in the runner (it knows the total line count).
//
// T92：提交读独立文件 split.json（工作区内，模型用 write_file/edit_file
// 增量维护），不再内联整份 JSON——内联时同一份章节清单在转录/webui 里
// 出现两次（buffer 一遍、submit args 一遍），书大时还是一轮巨型参数。
// 旧转录续跑重放的是内联 args，内联解析保留作 replay 通道（不出现在
// 工具定义里，模型不知道）。
type SubmitSplitTool struct {
	Chapters []ChapterRange
	Set      bool
	// Root 是沙箱目录（split.json 所在）。
	Root string
	// Gate 是可选提交门（P27：goal 计划清空才放行）；返回打回文案即拒绝。
	Gate func() string
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
		"name": "submit_split",
		"description": "Submit the final chapter split. Write it to split.json first (write_file/edit_file): " +
			`a JSON object {"chapters": [{"title", "start_line", "end_line"}, ...]} — an ordered list of 1-based inclusive line ranges covering the whole file without gaps or overlaps. ` +
			`Then call submit_split (no arguments needed; "path" overrides the default split.json).`,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "split file, default split.json"},
			},
		},
	}}
}

func (t *SubmitSplitTool) Execute(argsJSON string) (session.ToolResult, error) {
	if t.Gate != nil {
		if msg := t.Gate(); msg != "" {
			return session.ToolResult{Text: msg}, nil
		}
	}
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	var items []interface{}
	if inline, ok := args["chapters"].([]interface{}); ok && len(inline) > 0 {
		items = inline // 旧转录 replay 通道（工具定义不再暴露）
	} else {
		path := strArg(args, "path")
		if path == "" {
			path = "split.json"
		}
		data, err := os.ReadFile(filepath.Join(t.Root, filepath.Clean(path)))
		if err != nil {
			return session.ToolResult{Text: "REJECTED: " + path + " 不存在或不可读——先用 write_file 把划分写进去再提交。"}, nil
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return session.ToolResult{Text: "REJECTED: " + path + " 不是合法 JSON: " + err.Error()}, nil
		}
		items, _ = obj["chapters"].([]interface{})
	}
	if len(items) == 0 {
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
	return session.ToolResult{Text: "SUBMITTED. The split will be validated for full coverage.", Terminal: true}, nil
}
