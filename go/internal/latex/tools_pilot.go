package latex

// tools_pilot.go — T108：选材会话的 spec 不进 write_file 裸写 JSON——
// 与 split/problems/goal 同族的结构化工具：逐条增量操作（操作即校验，
// 批量全量预检整批原子），落盘 work/style/pilot/spec.json 供代码组装
// sample.md。每次 Execute 先重读文件：bash 绕过工具改文件也能被看见。

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

// specItem 是 spec.json 里的两种条目：md 行段 或 原书页集合。
type specItem struct {
	Kind  string `json:"kind"` // "md" | "pdf"
	File  string `json:"file"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
	Pages []int  `json:"pages,omitempty"`
}

type specFile struct {
	Items []specItem `json:"items"`
}

// PilotSpecTool 维护试点选材清单。
type PilotSpecTool struct {
	path      string // spec.json
	sourceDir string // <proj>/source，校验 file 存在
	mu        sync.Mutex
}

func NewPilotSpecTool(path, sourceDir string) *PilotSpecTool {
	return &PilotSpecTool{path: path, sourceDir: sourceDir}
}

func (t *PilotSpecTool) Name() string { return "pilot_spec" }

func (t *PilotSpecTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "pilot_spec",
		"description": "Maintain the pilot sample selection (persisted to spec.json). " +
			"ops: list, add {kind, file, start, end} / {kind, file, pages} or {items: [...]} (one or many at once, batch is all-or-nothing), " +
			"update {n, ...} or {items: [{n, ...}, ...]}, delete {n} or {ns: [...]}, clear. " +
			"kind \"md\" = a markdown line window (start/end are 1-based inclusive lines of file); kind \"pdf\" = original-book pages (pages is a list of page numbers of file). " +
			"When the selection covers every distinctive kind of content this book has, call submit.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":    map[string]any{"type": "string", "enum": []string{"list", "add", "update", "delete", "clear"}},
			"items": map[string]any{"type": "array", "description": "batch form: add=[{kind,file,start,end}|{kind,file,pages},...]; update=[{n,...},...]"},
			"n":     map[string]any{"type": "integer"},
			"ns":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"kind":  map[string]any{"type": "string", "enum": []string{"md", "pdf"}},
			"file":  map[string]any{"type": "string"},
			"start": map[string]any{"type": "integer"},
			"end":   map[string]any{"type": "integer"},
			"pages": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		}, "required": []string{"op"}},
	}}
}

func (t *PilotSpecTool) loadLocked() []specItem {
	var obj specFile
	if data, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	return obj.Items
}

func (t *PilotSpecTool) saveLocked(items []specItem) {
	data, _ := json.MarshalIndent(specFile{Items: items}, "", "  ")
	_ = os.WriteFile(t.path, append(data, '\n'), 0o644)
}

func renderSpec(items []specItem) string {
	if len(items) == 0 {
		return "spec.json 为空。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d 条选材:\n", len(items))
	for i, it := range items {
		if it.Kind == "md" {
			fmt.Fprintf(&b, "%d. [md] %s 第 %d-%d 行\n", i+1, it.File, it.Start, it.End)
		} else {
			fmt.Fprintf(&b, "%d. [pdf] %s 页 %v\n", i+1, it.File, it.Pages)
		}
	}
	return b.String()
}

// validateSpecItem 校验单条（md 文件必须在 source 下存在且行段合法）。
func (t *PilotSpecTool) validateSpecItem(it specItem) string {
	switch it.Kind {
	case "md":
		if strings.TrimSpace(it.File) == "" || it.Start < 1 || it.End < it.Start {
			return fmt.Sprintf("md 条目非法（file 非空、1<=start<=end）: %+v", it)
		}
		if !fileExists(filepath.Join(t.sourceDir, filepath.Base(it.File))) {
			return fmt.Sprintf("md 文件不在 source/ 下: %s", it.File)
		}
	case "pdf":
		if strings.TrimSpace(it.File) == "" || len(it.Pages) == 0 {
			return fmt.Sprintf("pdf 条目非法（file 非空、pages 非空）: %+v", it)
		}
		for _, p := range it.Pages {
			if p < 1 {
				return fmt.Sprintf("页码必须 >=1: %+v", it)
			}
		}
		sort.Ints(it.Pages)
	default:
		return fmt.Sprintf("kind 必须是 md 或 pdf: %+v", it)
	}
	return ""
}

// specItemFromArgs 从单条/批量子对象解出条目。
func specItemFromArgs(m map[string]any) specItem {
	it := specItem{
		Kind:  strArg(m, "kind"),
		File:  strArg(m, "file"),
		Start: intArg(m, "start", 0),
		End:   intArg(m, "end", 0),
	}
	if raw, ok := m["pages"].([]interface{}); ok {
		for _, v := range raw {
			if f, ok := v.(float64); ok {
				it.Pages = append(it.Pages, int(f))
			}
		}
	}
	return it
}

func (t *PilotSpecTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	items := t.loadLocked()

	switch strArg(args, "op") {
	case "list":
		return session.ToolResult{Text: renderSpec(items)}, nil
	case "clear":
		t.saveLocked(nil)
		return session.ToolResult{Text: "OK. spec.json 已清空。"}, nil
	case "delete":
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
			return session.ToolResult{Text: "REJECTED: delete 需要 n 或 ns。"}, nil
		}
		for _, n := range ns {
			if n < 1 || n > len(items) {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 下标 %d 越界（共 %d 条），整批不动。", n, len(items))}, nil
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ns)))
		for _, n := range ns {
			items = append(items[:n-1], items[n:]...)
		}
		t.saveLocked(items)
		return session.ToolResult{Text: "OK. " + renderSpec(items)}, nil
	case "add", "update":
		isUpdate := strArg(args, "op") == "update"
		var batch []map[string]any
		if raw, ok := args["items"].([]interface{}); ok {
			for _, v := range raw {
				if m, ok := v.(map[string]any); ok {
					batch = append(batch, m)
				}
			}
		} else {
			batch = []map[string]any{args}
		}
		// 全量预检：任一非法整批不动。
		type pending struct {
			n  int // update 的下标（add 时 0）
			it specItem
		}
		var pend []pending
		for _, m := range batch {
			it := specItemFromArgs(m)
			if msg := t.validateSpecItem(it); msg != "" {
				return session.ToolResult{Text: "REJECTED: " + msg}, nil
			}
			n := 0
			if isUpdate {
				n = intArg(m, "n", 0)
				if n < 1 || n > len(items) {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: update 下标 %d 越界（共 %d 条），整批不动。", n, len(items))}, nil
				}
			}
			pend = append(pend, pending{n, it})
		}
		for _, p := range pend {
			if p.n > 0 {
				items[p.n-1] = p.it
			} else {
				items = append(items, p.it)
			}
		}
		t.saveLocked(items)
		return session.ToolResult{Text: "OK. " + renderSpec(items)}, nil
	}
	return session.ToolResult{Text: "REJECTED: unknown op（list/add/update/delete/clear）。"}, nil
}

// specFromTool 把工具维护的 spec.json（items 形态）转成 pilotSpec
// （md_ranges/pdf_pages 两数组），供组装 sample.md。
func specFromTool(items []specItem) *pilotSpec {
	var spec pilotSpec
	for _, it := range items {
		if it.Kind == "md" {
			spec.MDRanges = append(spec.MDRanges, struct {
				File  string `json:"file"`
				Start int    `json:"start"`
				End   int    `json:"end"`
			}{it.File, it.Start, it.End})
		} else {
			spec.PDFPages = append(spec.PDFPages, struct {
				File  string `json:"file"`
				Pages []int  `json:"pages"`
			}{it.File, it.Pages})
		}
	}
	return &spec
}
