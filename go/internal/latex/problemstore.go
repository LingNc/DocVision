package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mineru-tools/internal/session"
)

// ------------------------------------------------------------------
// T64（重构）：样式修复循环的「问题清单」不再靠 submit 内联大数组 +
// 塞进各会话提示词，而是一块共享存储 work/style_fix/problems.json +
// 一个 problems 工具——整合会话独占写权限（add/update/delete，无打勾），
// 下游会话（样式/评估/分块）只能查看与打勾；提交时由代码核对勾选状态。
// 问题再多也不怕模型一次输出不全——它逐条 add，随时 list/get 回看。
// ------------------------------------------------------------------

// ProblemStore 是 problems.json 的带锁句柄（分块会话并行打勾，必须互斥）。
type ProblemStore struct {
	mu       sync.Mutex
	path     string
	Problems []StyleProblem
}

func loadProblemStore(path string) *ProblemStore {
	st := &ProblemStore{path: path}
	if data, err := os.ReadFile(path); err == nil {
		var w struct {
			Problems []StyleProblem `json:"problems"`
		}
		if json.Unmarshal(data, &w) == nil {
			st.Problems = w.Problems
		}
	}
	return st
}

func (s *ProblemStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"problems": s.Problems}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *ProblemStore) findLocked(id string) int {
	for i := range s.Problems {
		if s.Problems[i].ID == id {
			return i
		}
	}
	return -1
}

// nextIDLocked 分配 P<N>——取现有最大编号 +1，删除中间项不留坑。
func (s *ProblemStore) nextIDLocked() string {
	max := 0
	for _, p := range s.Problems {
		if n, err := strconv.Atoi(strings.TrimPrefix(p.ID, "P")); err == nil && n > max {
			max = n
		}
	}
	return "P" + strconv.Itoa(max+1)
}

// Unchecked 返回未打勾的问题 id（提交门用）。
func (s *ProblemStore) Unchecked(ids []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, id := range ids {
		if i := s.findLocked(id); i < 0 || !s.Problems[i].Checked {
			out = append(out, id)
		}
	}
	return out
}

// AllIDs 返回当前全部问题 id。
func (s *ProblemStore) AllIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.Problems))
	for _, p := range s.Problems {
		out = append(out, p.ID)
	}
	return out
}

// ProblemsTool 是会话侧的问题清单工具。Write=true（整合会话）开放
// add/update/delete、关闭 check；Write=false（下游会话）开放 list/get/
// search/check、关闭写。
type ProblemsTool struct {
	Store *ProblemStore
	Write bool
}

func (t *ProblemsTool) Name() string { return "problems" }

func (t *ProblemsTool) Definition() map[string]any {
	desc := "View the style-problem list. ops: list (id/title/status/chapters, no detail), get {id} (full incl. detail), search {query}."
	if t.Write {
		desc = "Manage the style-problem list. ops: list, get {id}, search {query}, " +
			"add {title, detail, chapters[]} or {items: [{title, detail, ...}, ...]} (one or many at once; ids auto-assigned P1, P2, ...), " +
			"update {id, title?, detail?, chapters?}, delete {id}."
	} else {
		desc += " check {ids: [...]} / {id, checked: false} marks problems resolved/unresolved — tick each one off as you finish it."
	}
	props := map[string]any{
		"op":       map[string]any{"type": "string", "enum": []string{"list", "get", "search", "add", "update", "delete", "check"}},
		"id":       map[string]any{"type": "string"},
		"ids":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"query":    map[string]any{"type": "string"},
		"title":    map[string]any{"type": "string"},
		"detail":   map[string]any{"type": "string"},
		"items":    map[string]any{"type": "array", "description": "add 批量形: [{title, detail, chapters?}, ...]"},
		"chapters": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"checked":  map[string]any{"type": "boolean"},
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "problems",
		"description": desc,
		"parameters":  map[string]any{"type": "object", "properties": props, "required": []string{"op"}},
	}}
}

func (t *ProblemsTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	op := strArg(args, "op")
	s := t.Store
	s.mu.Lock()
	defer s.mu.Unlock()

	summary := func(p StyleProblem) string {
		status := "[ ]"
		if p.Checked {
			status = "[x]"
		}
		return fmt.Sprintf("%s %s %s (chapters: %s)", status, p.ID, p.Title, strings.Join(p.Chapters, ", "))
	}

	switch op {
	case "list":
		if len(s.Problems) == 0 {
			return session.ToolResult{Text: "问题清单为空。"}, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "共 %d 个问题：\n", len(s.Problems))
		for _, p := range s.Problems {
			b.WriteString(summary(p) + "\n")
		}
		return session.ToolResult{Text: strings.TrimRight(b.String(), "\n")}, nil

	case "get":
		i := s.findLocked(strArg(args, "id"))
		if i < 0 {
			return session.ToolResult{Text: "REJECTED: 没有这个 id — 先 list 看现有问题。"}, nil
		}
		p := s.Problems[i]
		return session.ToolResult{Text: summary(p) + "\n\n" + p.Detail}, nil

	case "search":
		q := strings.ToLower(strArg(args, "query"))
		if q == "" {
			return session.ToolResult{Text: "REJECTED: search 需要 query。"}, nil
		}
		var hits []string
		for _, p := range s.Problems {
			if strings.Contains(strings.ToLower(p.Title), q) || strings.Contains(strings.ToLower(p.Detail), q) {
				hits = append(hits, summary(p))
			}
		}
		if len(hits) == 0 {
			return session.ToolResult{Text: "没有匹配的问题。"}, nil
		}
		return session.ToolResult{Text: strings.Join(hits, "\n")}, nil

	case "add":
		if !t.Write {
			return session.ToolResult{Text: "REJECTED: 本会话没有问题清单的写权限（只有整合会话能增删改；你只能查看和打勾）。"}, nil
		}
		// 单条 {title, detail, ...} 或批量 {items:[{...},...]}（任一非法整批不动）。
		raw, _ := args["items"].([]interface{})
		if len(raw) == 0 {
			raw = []interface{}{args}
		}
		type draft struct {
			id      string
			problem StyleProblem
		}
		var drafts []draft
		for i, it := range raw {
			m, ok := it.(map[string]interface{})
			if !ok {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: items 第 %d 个元素不是对象——整批未动。", i+1)}, nil
			}
			title, detail := strArg(m, "title"), strArg(m, "detail")
			if title == "" || detail == "" {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 条缺 title 或 detail——整批未动。", i+1)}, nil
			}
			id := strArg(m, "id")
			if id != "" && s.findLocked(id) >= 0 {
				return session.ToolResult{Text: "REJECTED: id " + id + " 已存在——用 update 修改，或不传 id 自动分配。整批未动。"}, nil
			}
			drafts = append(drafts, draft{id, StyleProblem{Title: title, Detail: detail, Chapters: strSliceArg(m, "chapters")}})
		}
		var ids []string
		for _, d := range drafts {
			id := d.id
			if id == "" {
				id = s.nextIDLocked()
			}
			d.problem.ID = id
			s.Problems = append(s.Problems, d.problem)
			ids = append(ids, id)
		}
		if err := s.saveLocked(); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "已记录 " + strings.Join(ids, ", ") + "（共 " + strconv.Itoa(len(s.Problems)) + " 个）。"}, nil

	case "update":
		if !t.Write {
			return session.ToolResult{Text: "REJECTED: 本会话没有问题清单的写权限。"}, nil
		}
		i := s.findLocked(strArg(args, "id"))
		if i < 0 {
			return session.ToolResult{Text: "REJECTED: 没有这个 id。"}, nil
		}
		if v := strArg(args, "title"); v != "" {
			s.Problems[i].Title = v
		}
		if v := strArg(args, "detail"); v != "" {
			s.Problems[i].Detail = v
		}
		if vs := strSliceArg(args, "chapters"); len(vs) > 0 {
			s.Problems[i].Chapters = vs
		}
		if err := s.saveLocked(); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "已更新 " + s.Problems[i].ID + "。"}, nil

	case "delete":
		if !t.Write {
			return session.ToolResult{Text: "REJECTED: 本会话没有问题清单的写权限。"}, nil
		}
		i := s.findLocked(strArg(args, "id"))
		if i < 0 {
			return session.ToolResult{Text: "REJECTED: 没有这个 id。"}, nil
		}
		id := s.Problems[i].ID
		s.Problems = append(s.Problems[:i], s.Problems[i+1:]...)
		if err := s.saveLocked(); err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{Text: "已删除 " + id + "。"}, nil

	case "check":
		if t.Write {
			return session.ToolResult{Text: "REJECTED: 整合会话只负责建清单，没有打勾操作（打勾是修复侧的事）。"}, nil
		}
		checked := true
		if v, ok := args["checked"].(bool); ok {
			checked = v
		}
		ids := strSliceArg(args, "ids")
		if id := strArg(args, "id"); id != "" {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return session.ToolResult{Text: "REJECTED: check 需要 id 或 ids。"}, nil
		}
		var bad []string
		for _, id := range ids {
			if i := s.findLocked(id); i >= 0 {
				s.Problems[i].Checked = checked
			} else {
				bad = append(bad, id)
			}
		}
		if err := s.saveLocked(); err != nil {
			return session.ToolResult{}, err
		}
		mark := "打勾"
		if !checked {
			mark = "取消打勾"
		}
		msg := "已" + mark + " " + strconv.Itoa(len(ids)-len(bad)) + " 个问题。"
		if len(bad) > 0 {
			msg += " 不存在的 id（已忽略）: " + strings.Join(bad, ", ")
		}
		return session.ToolResult{Text: msg}, nil
	}
	return session.ToolResult{Text: "REJECTED: 未知 op " + strconv.Quote(op) + "（list/get/search" + map[bool]string{true: "/add/update/delete", false: "/check"}[t.Write] + "）。"}, nil
}

// strSliceArg 提取字符串数组参数。
func strSliceArg(m map[string]any, key string) []string {
	var out []string
	if vs, ok := m[key].([]interface{}); ok {
		for _, v := range vs {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// ------------------------------------------------------------------
// T64（重构）：goal 工具——会话自己的计划清单，落盘 plan.md（工作区里
// 可读可核对），有序（从上到下 = 添加顺序），支持增删改勾。提交时
// 代码核对：还有未完成的计划项就打回（模型可勾掉或删掉再提交）。
// 压缩之后模型随时能 list 找回"我接下来要干嘛"。
// ------------------------------------------------------------------

type GoalItem struct {
	Text    string `json:"text"`
	Checked bool   `json:"checked,omitempty"`
}

type GoalTool struct {
	path  string
	mu    sync.Mutex
	Items []GoalItem
}

func NewGoalTool(planPath string) *GoalTool {
	t := &GoalTool{path: planPath}
	if data, err := os.ReadFile(planPath); err == nil {
		// plan.md 是 Markdown 清单：- [ ]/- [x] 行
		for _, ln := range strings.Split(string(data), "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "- [ ] ") {
				t.Items = append(t.Items, GoalItem{Text: strings.TrimPrefix(ln, "- [ ] ")})
			} else if strings.HasPrefix(ln, "- [x] ") {
				t.Items = append(t.Items, GoalItem{Text: strings.TrimPrefix(ln, "- [x] "), Checked: true})
			}
		}
	}
	return t
}

func (t *GoalTool) Name() string { return "goal" }

func (t *GoalTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "goal",
		"description": "Your own task plan (persisted to work:plan.md, survives context compaction). " +
			"ops: list, add {text} or {items: [...]} (one or many at once), update {n, text}, delete {n}, check {n} / {n, checked: false}. " +
			"Tasks are numbered 1..N top to bottom. All tasks must be checked or deleted before you submit.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":      map[string]any{"type": "string", "enum": []string{"list", "add", "update", "delete", "check"}},
			"n":       map[string]any{"type": "integer"},
			"text":    map[string]any{"type": "string"},
			"items":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"checked": map[string]any{"type": "boolean"},
		}, "required": []string{"op"}},
	}}
}

func (t *GoalTool) saveLocked() {
	var b strings.Builder
	b.WriteString("# 任务计划\n\n")
	for _, it := range t.Items {
		mark := " "
		if it.Checked {
			mark = "x"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, it.Text)
	}
	_ = os.WriteFile(t.path, []byte(b.String()), 0o644)
}

// Pending 返回未完成项的「n. text」（提交门用）。
func (t *GoalTool) Pending() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for i, it := range t.Items {
		if !it.Checked {
			out = append(out, strconv.Itoa(i+1)+". "+it.Text)
		}
	}
	return out
}

func (t *GoalTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	idx := func() (int, bool) {
		f, ok := args["n"].(float64)
		n := int(f)
		if !ok || n < 1 || n > len(t.Items) {
			return 0, false
		}
		return n - 1, true
	}

	switch strArg(args, "op") {
	case "list":
		if len(t.Items) == 0 {
			return session.ToolResult{Text: "计划为空。"}, nil
		}
		var b strings.Builder
		for i, it := range t.Items {
			mark := "[ ]"
			if it.Checked {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "%d. %s %s\n", i+1, mark, it.Text)
		}
		return session.ToolResult{Text: strings.TrimRight(b.String(), "\n")}, nil
	case "add":
		// 单条 {text} 或多条 {items:[...]} 一次调用加完。
		texts := strSliceArg(args, "items")
		if v := strArg(args, "text"); v != "" {
			texts = append(texts, v)
		}
		if len(texts) == 0 {
			return session.ToolResult{Text: "REJECTED: add 需要 text（单条）或 items 数组（多条）。"}, nil
		}
		for _, txt := range texts {
			t.Items = append(t.Items, GoalItem{Text: txt})
		}
		t.saveLocked()
		return session.ToolResult{Text: fmt.Sprintf("已添加 %d 项（共 %d 项）。", len(texts), len(t.Items))}, nil
	case "update":
		i, ok := idx()
		if !ok {
			return session.ToolResult{Text: "REJECTED: n 超出范围。"}, nil
		}
		if text := strArg(args, "text"); text != "" {
			t.Items[i].Text = text
		}
		t.saveLocked()
		return session.ToolResult{Text: "已更新第 " + strconv.Itoa(i+1) + " 项。"}, nil
	case "delete":
		i, ok := idx()
		if !ok {
			return session.ToolResult{Text: "REJECTED: n 超出范围。"}, nil
		}
		t.Items = append(t.Items[:i], t.Items[i+1:]...)
		t.saveLocked()
		return session.ToolResult{Text: "已删除第 " + strconv.Itoa(i+1) + " 项。"}, nil
	case "check":
		i, ok := idx()
		if !ok {
			return session.ToolResult{Text: "REJECTED: n 超出范围。"}, nil
		}
		checked := true
		if v, isB := args["checked"].(bool); isB {
			checked = v
		}
		t.Items[i].Checked = checked
		t.saveLocked()
		return session.ToolResult{Text: "已更新第 " + strconv.Itoa(i+1) + " 项状态。"}, nil
	}
	return session.ToolResult{Text: "REJECTED: 未知 op。"}, nil
}

// goalGate 生成提交门：还有未完成计划项时返回打回文案，否则空串。
func goalGate(g *GoalTool) string {
	if g == nil {
		return ""
	}
	if pend := g.Pending(); len(pend) > 0 {
		return "REJECTED: 计划清单还有 " + strconv.Itoa(len(pend)) + " 项未完成：\n" + strings.Join(pend, "\n") +
			"\n——逐项完成后用 goal check 打勾（不需要的项用 goal delete 删掉），再重新提交。"
	}
	return ""
}

// sortedIDs 排序去重（日志用）。
func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}
