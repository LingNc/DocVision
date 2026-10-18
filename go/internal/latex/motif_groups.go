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

// MotifGroup is one final structural group. Key is the representative the AI
// picked as the most characteristic member: the one the shared drawing is made
// from, and the one every other member is compared against. A group without a
// key is incomplete — reducing N drawings to one requires naming which one.
type MotifGroup struct {
	Key     int   `json:"key"`
	Members []int `json:"members"`
}

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
		"description": "Maintain final structural image groups in groups.json. ops: list, add {group:[ids],key} or {groups:[{members:[ids],key}]}, update {n,group,key}（只给 n+key 表示只换代表图，成员不动）, remove {n,ids:[...]}, append {n,ids:[...]}, delete {n} or {ns:[...]}, clear. IDs are 1-based pool indices; a member may occur in at most one group. Every group needs key = the single most representative member (the one that gets redrawn; others reuse it). Mutations are validated and persisted immediately. When finished, call submit with no groups argument.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":     map[string]any{"type": "string", "enum": []string{"list", "add", "update", "remove", "append", "delete", "clear"}},
			"group":  map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}},
			"groups": map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}}},
			"ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}, "description": "remove/append 要移出或加入第 n 组的成员 ID"},
			"key":    map[string]any{"type": "integer", "minimum": 1, "description": "代表图/关键成员 ID，必须是该组成员"},
			"n":      map[string]any{"type": "integer", "minimum": 1},
			"ns":     map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}},
		}, "required": []string{"op"}},
	}}
}

func (t *MotifGroupsTool) loadLocked() []MotifGroup {
	var obj struct {
		Groups []MotifGroup `json:"groups"`
	}
	data, err := os.ReadFile(t.path)
	if err == nil && json.Unmarshal(data, &obj) == nil {
		return obj.Groups
	}
	// Tolerate the pre-key schema ([[ids],...]) so an interrupted older run can
	// resume: those groups simply have no key yet and cannot be submitted until
	// one is chosen.
	var legacy struct {
		Groups [][]int `json:"groups"`
	}
	if err == nil && json.Unmarshal(data, &legacy) == nil {
		out := make([]MotifGroup, 0, len(legacy.Groups))
		for _, g := range legacy.Groups {
			out = append(out, MotifGroup{Members: g})
		}
		return out
	}
	return nil
}
func (t *MotifGroupsTool) saveLocked(groups []MotifGroup) error {
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
func cloneMotifGroups(groups []MotifGroup) []MotifGroup {
	out := make([]MotifGroup, len(groups))
	for i, g := range groups {
		out[i] = MotifGroup{Key: g.Key, Members: append([]int(nil), g.Members...)}
	}
	return out
}
func validateMotifGroups(groups []MotifGroup, count int) error {
	used := map[int]int{}
	for i, g := range groups {
		ng, err := normalizeMotifGroup(g.Members, count)
		if err != nil {
			return fmt.Errorf("第 %d 组：%w", i+1, err)
		}
		if g.Key != 0 {
			found := false
			for _, id := range ng {
				if id == g.Key {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("第 %d 组：key %d 不在该组里（key 必须是组内最有代表性的一张）", i+1, g.Key)
			}
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

// validateMotifGroupShape enforces the one thing that makes a group useful:
// a chosen representative. Putting the whole pool in a single group is allowed
// (the user explicitly wants that possible) as long as one member is named as
// the key: process then draws only that key and every other member reuses it.
func validateMotifGroupShape(groups []MotifGroup, count int) error {
	for i, g := range groups {
		if g.Key == 0 {
			return fmt.Errorf("第 %d 组还没有 key：请挑出该组最有代表性的一张作为 key（其余成员之后直接复用它的图，只有 key 会被重画）", i+1)
		}
	}
	return nil
}

func renderMotifGroups(groups []MotifGroup) string {
	if len(groups) == 0 {
		return "groups.json 为空，当前没有暂存分组。"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d 个分组：\n", len(groups))
	for i, g := range groups {
		key := "key=? (必须指定)"
		if g.Key != 0 {
			key = fmt.Sprintf("key=%d", g.Key)
		}
		fmt.Fprintf(&b, "%d. %s %v\n", i+1, key, g.Members)
	}
	return b.String()
}
func (t *MotifGroupsTool) Groups() ([]MotifGroup, error) {
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
		// Report what was thrown away and name the surgical alternative: when
		// clear is the only undo the model knows, "the group is not perfect" turns
		// into "clear everything" (T126: one 90-member bucket in, nothing out).
		dropped, members := len(groups), 0
		for _, g := range groups {
			members += len(g.Members)
		}
		if err := t.saveLocked(nil); err != nil {
			return session.ToolResult{}, err
		}
		if dropped == 0 {
			return session.ToolResult{Text: "OK. groups.json 本来就是空的（0 组）。"}, nil
		}
		return session.ToolResult{Text: fmt.Sprintf(
			"OK. groups.json 已清空，删掉 %d 组 %d 张。只想去掉个别成员用 remove {n,ids:[...]}，只想换代表图用 update {n,key}；清空之后这些图全部按独立处理，除非你重新 add 回去。",
			dropped, members)}, nil
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
		kept := make([]MotifGroup, 0, len(groups))
		for i, g := range groups {
			if !drop[i] {
				kept = append(kept, g)
			}
		}
		if err := t.saveLocked(kept); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "OK.\n" + renderMotifGroups(kept)}, nil
	case "remove", "append":
		n := intArg(args, "n", 0)
		if n < 1 || n > len(groups) {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: n=%d 超出当前 %d 组，整批未动。", n, len(groups))}, nil
		}
		var ids []int
		if rawIDs, ok := args["ids"].([]any); ok {
			for _, v := range rawIDs {
				f, ok := v.(float64)
				if !ok {
					return session.ToolResult{Text: "REJECTED: ids 必须是整数，整批未动。"}, nil
				}
				ids = append(ids, int(f))
			}
		}
		if len(ids) == 0 {
			return session.ToolResult{Text: "REJECTED: " + op + " 需要 ids:[...]，整批未动。"}, nil
		}
		for _, id := range ids {
			if id < 1 || id > t.count {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: ID %d 超出 1..%d，整批未动。", id, t.count)}, nil
			}
		}
		work := cloneMotifGroups(groups)
		target := &work[n-1]
		if op == "remove" {
			mine := make(map[int]bool, len(target.Members))
			for _, id := range target.Members {
				mine[id] = true
			}
			drop := make(map[int]bool, len(ids))
			for _, id := range ids {
				if !mine[id] {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: ID %d 不在第 %d 组里，整批未动。", id, n)}, nil
				}
				drop[id] = true
			}
			kept := make([]int, 0, len(target.Members))
			for _, id := range target.Members {
				if !drop[id] {
					kept = append(kept, id)
				}
			}
			note := ""
			if target.Key != 0 && drop[target.Key] {
				note = fmt.Sprintf("\n第 %d 组的 key %d 已被移出，请用 update {n:%d,key:...} 重新指定代表图。", n, target.Key, n)
				target.Key = 0
			}
			if len(kept) < 2 {
				// 单张不需要分组：剩下的 ID 回到「独立」状态，该组解散。
				work = append(work[:n-1], work[n:]...)
				if err := t.saveLocked(work); err != nil {
					return session.ToolResult{}, err
				}
				return session.ToolResult{Text: fmt.Sprintf("OK. 第 %d 组移出 %d 个 ID 后只剩 %d 张，已解散该组（单张不必成组）。%s\n%s", n, len(drop), len(kept), note, renderMotifGroups(work))}, nil
			}
			target.Members = kept
			if err := validateMotifGroups(work, t.count); err != nil {
				return session.ToolResult{Text: "REJECTED: " + err.Error() + "，整批未动。"}, nil
			}
			if err := t.saveLocked(work); err != nil {
				return session.ToolResult{}, err
			}
			return session.ToolResult{Text: fmt.Sprintf("OK. 第 %d 组剩 %d 张。%s\n%s", n, len(kept), note, renderMotifGroups(work))}, nil
		}
		owner := make(map[int]int, t.count)
		for i := range work {
			for _, id := range work[i].Members {
				owner[id] = i
			}
		}
		var extra []int
		same := 0
		for _, id := range ids {
			if i, ok := owner[id]; ok {
				if i == n-1 {
					same++
					continue
				}
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: ID %d 已在第 %d 组，整批未动（一个 ID 只能属于一个组；要先移出那边再 append）。", id, i+1)}, nil
			}
			extra = append(extra, id)
		}
		if len(extra) == 0 {
			return session.ToolResult{Text: fmt.Sprintf("OK. 第 %d 组本来就有这些 ID（%d 个），没有变化。\n%s", n, same, renderMotifGroups(work))}, nil
		}
		target.Members = append(target.Members, extra...)
		sort.Ints(target.Members)
		if err := validateMotifGroups(work, t.count); err != nil {
			return session.ToolResult{Text: "REJECTED: " + err.Error() + "，整批未动。"}, nil
		}
		if err := t.saveLocked(work); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: fmt.Sprintf("OK. 第 %d 组加入 %d 个 ID，现 %d 张。\n%s", n, len(extra), len(target.Members), renderMotifGroups(work))}, nil
	case "add", "update":
		work := cloneMotifGroups(groups)
		parseGroup := func(raw any) (MotifGroup, string) {
			var ids []int
			key := 0
			switch v := raw.(type) {
			case []any: // legacy: bare id array, no key yet
				for _, item := range v {
					f, ok := item.(float64)
					if !ok {
						return MotifGroup{}, "group ID 必须是整数，整批未动。"
					}
					ids = append(ids, int(f))
				}
			case map[string]any:
				members, _ := v["members"].([]any)
				if members == nil {
					members, _ = v["group"].([]any)
				}
				for _, item := range members {
					f, ok := item.(float64)
					if !ok {
						return MotifGroup{}, "members ID 必须是整数，整批未动。"
					}
					ids = append(ids, int(f))
				}
				if f, ok := v["key"].(float64); ok {
					key = int(f)
				}
			default:
				return MotifGroup{}, "group 既不是 ID 数组也不是 {members,key} 对象，整批未动。"
			}
			members, e := normalizeMotifGroup(ids, t.count)
			if e != nil {
				return MotifGroup{}, e.Error() + "，整批未动。"
			}
			return MotifGroup{Key: key, Members: members}, ""
		}
		if op == "add" {
			var raws []any
			if v, ok := args["groups"].([]any); ok && len(v) > 0 {
				raws = v
			} else if v, ok := args["group"].([]any); ok {
				raws = []any{v}
			} else {
				return session.ToolResult{Text: "REJECTED: add 需要 group 或 groups。"}, nil
			}
			topKey := intArg(args, "key", 0)
			if topKey > 0 && len(raws) > 1 {
				return session.ToolResult{Text: "REJECTED: 顶层 key 只能配合单个 group；批量请让每个 group 自带 key，整批未动。"}, nil
			}
			for i, rawGroup := range raws {
				g, msg := parseGroup(rawGroup)
				if msg != "" {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 组：%s", i+1, msg)}, nil
				}
				if _, isMap := rawGroup.(map[string]any); !isMap && topKey > 0 {
					g.Key = topKey
				}
				work = append(work, g)
			}
		} else {
			n := intArg(args, "n", 0)
			if n < 1 || n > len(work) {
				return session.ToolResult{Text: "REJECTED: update n 超出范围，整批未动。"}, nil
			}
			raw, ok := args["group"]
			if !ok {
				raw = args["members"]
			}
			if raw == nil {
				// 只换代表图：成员原样保留。旧版本这里拒单，模型只好把整组 ID 重打
				// 一遍（336 个 ID 约 1.3k tokens，还可能抄漏成员）。
				k := intArg(args, "key", 0)
				if k == 0 {
					return session.ToolResult{Text: "REJECTED: update 需要 group 或 members；只想换代表图时只给 n 和 key。"}, nil
				}
				work[n-1].Key = k
			} else {
				g, msg := parseGroup(raw)
				if msg != "" {
					return session.ToolResult{Text: "REJECTED: " + msg}, nil
				}
				if k := intArg(args, "key", 0); k > 0 {
					g.Key = k
				} else if g.Key == 0 {
					// Keep the previously chosen representative across member edits,
					// but drop it when that member is no longer part of the group.
					prev := work[n-1].Key
					for _, id := range g.Members {
						if id == prev {
							g.Key = prev
							break
						}
					}
				}
				work[n-1] = g
			}
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
