package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"mineru-tools/internal/session"
)

// MotifGroupsTool is the editable, persisted staging area for a large AI
// grouping job. It mirrors split/goal semantics: every mutation is validated
// before the JSON file changes, so a long review can be resumed and corrected
// without asking the model to emit the whole result again.
type MotifGroupsTool struct {
	path  string
	count int
	mu    sync.Mutex
}

func NewMotifGroupsTool(path string, count int) *MotifGroupsTool {
	return &MotifGroupsTool{path: path, count: count}
}
func (t *MotifGroupsTool) Name() string { return "motif_groups" }
func (t *MotifGroupsTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "motif_groups",
		"description": "Maintain final structural image groups in groups.json. ops: list, add {group:[ids]} or {groups:[[ids],...]}, update {n,group}, delete {n} or {ns:[...]}, clear. IDs are 1-based candidate-pool indices; a member may occur in at most one group. Mutations are validated and persisted immediately. When finished, call submit with no groups argument.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":     map[string]any{"type": "string", "enum": []string{"list", "add", "update", "delete", "clear"}},
			"group":  map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}},
			"groups": map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}}},
			"n":      map[string]any{"type": "integer", "minimum": 1},
			"ns":     map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}},
		}, "required": []string{"op"}},
	}}
}

func (t *MotifGroupsTool) loadLocked() [][]int {
	var obj struct {
		Groups [][]int `json:"groups"`
	}
	data, err := os.ReadFile(t.path)
	if err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	return obj.Groups
}
func (t *MotifGroupsTool) saveLocked(groups [][]int) error {
	data, err := json.MarshalIndent(map[string]any{"groups": groups}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, append(data, '\n'), 0644)
}
func normalizeMotifGroup(raw []int, count int) ([]int, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("group 至少需要 2 个 ID")
	}
	seen := map[int]bool{}
	out := append([]int(nil), raw...)
	for _, id := range out {
		if id < 1 || id > count {
			return nil, fmt.Errorf("ID %d 超出 1..%d", id, count)
		}
		if seen[id] {
			return nil, fmt.Errorf("ID %d 在同组重复", id)
		}
		seen[id] = true
	}
	sort.Ints(out)
	return out, nil
}
func validateMotifGroups(groups [][]int, count int) error {
	used := map[int]int{}
	for i, g := range groups {
		ng, err := normalizeMotifGroup(g, count)
		if err != nil {
			return fmt.Errorf("第 %d 组：%w", i+1, err)
		}
		for _, id := range ng {
			if prev, ok := used[id]; ok {
				return fmt.Errorf("ID %d 同时出现在第 %d、%d 组", id, prev+1, i+1)
			}
			used[id] = i
		}
	}
	return nil
}
func renderMotifGroups(groups [][]int) string {
	if len(groups) == 0 {
		return "groups.json 为空，当前没有暂存分组。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d 个分组：\n", len(groups))
	for i, g := range groups {
		fmt.Fprintf(&b, "%d. %v\n", i+1, g)
	}
	return b.String()
}
func (t *MotifGroupsTool) Groups() ([][]int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	groups := t.loadLocked()
	if err := validateMotifGroups(groups, t.count); err != nil {
		return nil, err
	}
	return groups, nil
}

func (t *MotifGroupsTool) Execute(raw string) (session.ToolResult, error) {
	args, err := parseJSONObject(raw)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	groups := t.loadLocked()
	op := strArg(args, "op")
	render := func() string { return renderMotifGroups(groups) }
	switch op {
	case "list":
		return session.ToolResult{Text: render()}, nil
	case "clear":
		if err := t.saveLocked(nil); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "OK. groups.json 已清空。"}, nil
	case "delete":
		var ns []int
		if rawNS, ok := args["ns"].([]any); ok {
			for _, v := range rawNS {
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
		drop := map[int]bool{}
		for _, n := range ns {
			if n < 1 || n > len(groups) {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: n=%d 超出当前 %d 组，整批未动。", n, len(groups))}, nil
			}
			drop[n-1] = true
		}
		kept := make([][]int, 0, len(groups))
		for i, g := range groups {
			if !drop[i] {
				kept = append(kept, g)
			}
		}
		if err := t.saveLocked(kept); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "OK.\n" + renderMotifGroups(kept)}, nil
	case "add", "update":
		work := make([][]int, len(groups))
		for i, g := range groups {
			work[i] = append([]int(nil), g...)
		}
		var raws []any
		if op == "add" {
			if v, ok := args["groups"].([]any); ok && len(v) > 0 {
				raws = v
			} else if v, ok := args["group"].([]any); ok {
				raws = []any{v}
			} else {
				return session.ToolResult{Text: "REJECTED: add 需要 group 或 groups。"}, nil
			}
			for i, rawGroup := range raws {
				vals, ok := rawGroup.([]any)
				if !ok {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个 group 不是数组，整批未动。", i+1)}, nil
				}
				ids := make([]int, 0, len(vals))
				for _, v := range vals {
					f, ok := v.(float64)
					if !ok {
						return session.ToolResult{Text: "REJECTED: group ID 必须是整数，整批未动。"}, nil
					}
					ids = append(ids, int(f))
				}
				g, e := normalizeMotifGroup(ids, t.count)
				if e != nil {
					return session.ToolResult{Text: "REJECTED: " + e.Error() + "，整批未动。"}, nil
				}
				work = append(work, g)
			}
		} else {
			n := intArg(args, "n", 0)
			if n < 1 || n > len(work) {
				return session.ToolResult{Text: "REJECTED: update n 超出范围，整批未动。"}, nil
			}
			vals, ok := args["group"].([]any)
			if !ok {
				return session.ToolResult{Text: "REJECTED: update 需要 group。"}, nil
			}
			ids := make([]int, 0, len(vals))
			for _, v := range vals {
				f, ok := v.(float64)
				if !ok {
					return session.ToolResult{Text: "REJECTED: group ID 必须是整数，整批未动。"}, nil
				}
				ids = append(ids, int(f))
			}
			g, e := normalizeMotifGroup(ids, t.count)
			if e != nil {
				return session.ToolResult{Text: "REJECTED: " + e.Error() + "，整批未动。"}, nil
			}
			work[n-1] = g
		}
		if err := validateMotifGroups(work, t.count); err != nil {
			return session.ToolResult{Text: "REJECTED: " + err.Error() + "，整批未动。"}, nil
		}
		if err := t.saveLocked(work); err != nil {
			return session.ToolResult{}, err
		}
		groups = work
		return session.ToolResult{Text: "OK.\n" + render()}, nil
	}
	return session.ToolResult{Text: "REJECTED: 未知 op（list/add/update/delete/clear）。"}, nil
}
