package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
		"description": "Submit the final chapter split, read from split.json. Maintain that file with the split tool " +
			"(structured add/update/delete, validated on every op) — editing split.json directly with edit_file also works. " +
			`Format: {"chapters": [{"title", "start_line", "end_line"}, ...]}, ordered 1-based inclusive ranges covering the whole file without gaps or overlaps. ` +
			`Call submit_split with no arguments when done ("path" overrides the default split.json).`,
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

// ------------------------------------------------------------------
// split 工具（problems/goal 同族，用户拍板）：章节划分不进 write_file
// 裸写 JSON——逐条增量操作（list/add/update/delete/clear），每次操作
// 即校验字段与重叠，列表始终按 start_line 排序落盘 split.json。
// submit_split 仍读同一份文件做全覆盖终审。每次 Execute 先重读文件：
// 模型若用 bash 绕过工具改文件也能被看见。
// ------------------------------------------------------------------

type SplitTool struct {
	path string
	mu   sync.Mutex
}

func NewSplitTool(path string) *SplitTool { return &SplitTool{path: path} }

func (t *SplitTool) Name() string { return "split" }

func (t *SplitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "split",
		"description": "Maintain the chapter split (persisted to split.json). " +
			"ops: list, add {title, start_line, end_line} or {chapters: [...]} (one or many at once, batch is all-or-nothing), " +
			"update {n, ...} or {chapters: [{n, ...}, ...]}, delete {n} or {ns: [...]}, clear. " +
			"Entries are 1-based inclusive line ranges, kept sorted by start_line; overlapping a neighbour is rejected on the spot. " +
			"When the list covers the whole file with no gaps, call submit_split.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":         map[string]any{"type": "string", "enum": []string{"list", "add", "update", "delete", "clear"}},
			"chapters":   map[string]any{"type": "array", "description": "batch form: add=[{title,start_line,end_line},...]; update=[{n,...},...]"},
			"n":          map[string]any{"type": "integer"},
			"ns":         map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"title":      map[string]any{"type": "string"},
			"start_line": map[string]any{"type": "integer"},
			"end_line":   map[string]any{"type": "integer"},
		}, "required": []string{"op"}},
	}}
}

func (t *SplitTool) loadLocked() []ChapterRange {
	var obj struct {
		Chapters []ChapterRange `json:"chapters"`
	}
	if data, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	return obj.Chapters
}

func (t *SplitTool) saveLocked(chs []ChapterRange) {
	data, _ := json.MarshalIndent(map[string]any{"chapters": chs}, "", "  ")
	_ = os.WriteFile(t.path, append(data, '\n'), 0o644)
}

// validateEntry 校验单条；overlap 时对邻居逐条查（excl 是 update 时自己的下标）。
func validateEntry(list []ChapterRange, c ChapterRange, excl int) string {
	if strings.TrimSpace(c.Title) == "" || c.StartLine < 1 || c.EndLine < c.StartLine {
		return fmt.Sprintf("条目非法（title 非空、1<=start_line<=end_line）: %+v", c)
	}
	for i, o := range list {
		if i == excl {
			continue
		}
		if c.StartLine <= o.EndLine && o.StartLine <= c.EndLine {
			return fmt.Sprintf("与第 %d 条重叠（%d-%d 与 %d-%d）——先调整邻居再来。", i+1, c.StartLine, c.EndLine, o.StartLine, o.EndLine)
		}
	}
	return ""
}

func sortSplit(chs []ChapterRange) {
	sort.Slice(chs, func(i, j int) bool { return chs[i].StartLine < chs[j].StartLine })
}

func (t *SplitTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	list := t.loadLocked()

	render := func() string {
		if len(list) == 0 {
			return "split.json 为空。"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d 章:\n", len(list))
		for i, c := range list {
			fmt.Fprintf(&b, "%d. [%d-%d] %s\n", i+1, c.StartLine, c.EndLine, c.Title)
		}
		return b.String()
	}

	switch strArg(args, "op") {
	case "list":
		return session.ToolResult{Text: render()}, nil
	case "clear":
		t.saveLocked(nil)
		return session.ToolResult{Text: "OK. split.json 已清空。"}, nil
	case "delete":
		// 单条 {n} 或批量 {ns:[...]}（任一越界整批不动）。
		var ns []int
		if raw, ok := args["ns"].([]interface{}); ok {
			for _, v := range raw {
				if f, ok := v.(float64); ok {
					ns = append(ns, int(f))
				}
			}
		} else if n := intArg(args, "n", 0); n > 0 {
			ns = []int{n}
		}
		if len(ns) == 0 {
			return session.ToolResult{Text: "REJECTED: delete 需要 n（单条）或 ns 数组（多条）。"}, nil
		}
		for _, n := range ns {
			if n < 1 || n > len(list) {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: n=%d 超出范围（当前 %d 条）——整批未动。", n, len(list))}, nil
			}
		}
		drop := map[int]bool{}
		for _, n := range ns {
			drop[n-1] = true
		}
		var kept []ChapterRange
		for i, c := range list {
			if !drop[i] {
				kept = append(kept, c)
			}
		}
		list = kept
		t.saveLocked(list)
		return session.ToolResult{Text: "OK.\n" + render()}, nil
	case "add", "update":
		// 单条直接给字段；批量给 chapters 数组（全量预检、任一非法整批不动）。
		raw, _ := args["chapters"].([]interface{})
		if len(raw) == 0 {
			raw = []interface{}{args} // 单条：参数本体即条目
		}
		work := append([]ChapterRange(nil), list...)
		touched := map[int]bool{}
		for i, it := range raw {
			m, ok := it.(map[string]interface{})
			if !ok {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素不是对象。", i+1)}, nil
			}
			var c ChapterRange
			excl := -1
			if strArg(args, "op") == "add" {
				c = ChapterRange{Title: strArg(m, "title"), StartLine: intArg(m, "start_line", 0), EndLine: intArg(m, "end_line", 0)}
			} else {
				n := intArg(m, "n", 0)
				if n < 1 || n > len(work) {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素 n=%d 超出范围（当前 %d 条）——整批未动。", i+1, n, len(work))}, nil
				}
				if touched[n] {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 条被 update 了两次——整批未动。", n)}, nil
				}
				touched[n] = true
				excl = n - 1
				c = work[excl]
				if v := strArg(m, "title"); v != "" {
					c.Title = v
				}
				if v := intArg(m, "start_line", 0); v > 0 {
					c.StartLine = v
				}
				if v := intArg(m, "end_line", 0); v > 0 {
					c.EndLine = v
				}
			}
			if msg := validateEntry(work, c, excl); msg != "" {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素: %s——整批未动。", i+1, msg)}, nil
			}
			if excl >= 0 {
				work[excl] = c
			} else {
				work = append(work, c)
				sortSplit(work) // 让后续元素的重叠检查看到已插入的新条目
			}
		}
		sortSplit(work)
		t.saveLocked(work)
		list = work
		return session.ToolResult{Text: "OK.\n" + render()}, nil
	}
	return session.ToolResult{Text: "REJECTED: 未知 op（list/add/update/delete/clear）。"}, nil
}
