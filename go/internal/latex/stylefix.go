package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

// ------------------------------------------------------------------
// T43：风格修复大循环
//
// 旧行为（多数派门）：超过半数章节汇报样式问题才打回原样式会话。
// 新行为：只要有任何章节汇报手册/样式问题就进入修复流程——
//   1. 整合会话（stylefix_integrate_r<N>）：类似章节划分会话，把各章
//      汇报整合成不重复的问题清单（submit_problems，固定格式）。
//   2. 风格修复会话（stylefix_style_r<N>）：复用原样式会话上下文（信息
//      不丢失；缓存失效只是成本问题，按用户口径"衡量"后选复用）修复
//      cls/手册，提交后编译 example 验证。
//   3. 影响评估会话（stylefix_assess_r<N>）：按问题划分章节块
//      （submit_blocks，每块带问题编号与章节范围）——样式问题常是全局
//      性的，不能只修汇报章。
//   4. 分块修复会话（stylefix_blk_<N>_<i>，并行）：每块一个会话，在临时
//      工作区里对块内章节做**定位修改**（绝不重做），逐章汇报是否仍有
//      问题。
//   5. 任一章仍有问题且未到 latex.style_fix.max_rounds → 下一轮
//      style-f<N+1>；到上限则告警保留现状（编译与终审仍是硬关卡）。
// ------------------------------------------------------------------

// StyleProblem 是整合会话交出的一个问题。
type StyleProblem struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Chapters []string `json:"chapters"`
	Checked  bool     `json:"checked,omitempty"`
}

// StyleBlock 是评估会话交出的一个修复块。
type StyleBlock struct {
	Title      string   `json:"title"`
	ProblemIDs []string `json:"problem_ids"`
	Chapters   []string `json:"chapters"`
}

// styleFixProblemStorePath 是共享问题清单的位置（整合会话写、下游会话读勾）。
func styleFixProblemStorePath(proj string) string {
	return filepath.Join(proj, "work", "style_fix", "problems.json")
}

// SubmitProblemsTool 收整合会话的提交。T64（重构）：清单不再内联传
// （问题多时一次输出不全，还曾出现重复提交），而是会话用 problems
// 工具逐条写入共享存储，submit_problems 无参收账——代码读存储校验。
type SubmitProblemsTool struct {
	Store *ProblemStore
	Goal  *GoalTool
	Set   bool
}

func (t *SubmitProblemsTool) Name() string { return "submit_problems" }

func (t *SubmitProblemsTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "submit_problems",
		"description": "Submit the consolidated problem list you built with the problems tool (no arguments — the list is read from the shared store). " +
			"All goal-plan items must be checked or deleted first.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"report": map[string]any{"type": "string", "description": "one-line summary of what you consolidated"},
		}},
	}}
}

func (t *SubmitProblemsTool) Execute(argsJSON string) (session.ToolResult, error) {
	if _, err := parseJSONObject(argsJSON); err != nil {
		return session.ToolResult{}, err
	}
	if msg := goalGate(t.Goal); msg != "" {
		return session.ToolResult{Text: msg}, nil
	}
	ids := t.Store.AllIDs()
	if len(ids) == 0 {
		return session.ToolResult{Text: "REJECTED: 清单为空——先用 problems add 逐条记录问题，再提交。"}, nil
	}
	// 每条都要有 title/detail（空字段说明没整理完）。
	t.Store.mu.Lock()
	var bad []string
	for _, p := range t.Store.Problems {
		if p.Title == "" || p.Detail == "" {
			bad = append(bad, p.ID)
		}
	}
	t.Store.mu.Unlock()
	if len(bad) > 0 {
		return session.ToolResult{Text: "REJECTED: 这些问题缺 title/detail: " + strings.Join(bad, ", ")}, nil
	}
	t.Set = true
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. %d problems recorded.", len(ids)), Terminal: true}, nil
}

// BlocksTool 维护评估会话的 blocks.json（split/problems/goal 同族）：
// 逐条或批量操作、每次操作即校验（标题/章节非空、章节跨块重复当场拒），
// 落盘 {"blocks":[...]}；edit_file 直改保留为逃生舱（每次操作前重读文件），
// submit_blocks 无参终审。
type BlocksTool struct {
	path string
	mu   sync.Mutex
	// Known 是"可以分块的章节名"（顶层章节）。细分父章节的正文在各层
	// `<章>/children/**` 里，但分块单位仍然是那个顶层章节——子节名混进来会让
	// 分块会话去找一份不存在的产物、悄悄跳过这一章（T281）。
	Known []string
}

func NewBlocksTool(path string, known ...string) *BlocksTool {
	return &BlocksTool{path: path, Known: known}
}

func (t *BlocksTool) Name() string { return "blocks" }

func (t *BlocksTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "blocks",
		"description": "Maintain the fix-block partition (persisted to blocks.json). " +
			"ops: list, add {title, problem_ids, chapters} or {blocks: [...]}, update {n, ...} or {blocks: [{n, ...}, ...]}, delete {n} or {ns: [...]}, clear — every op takes one or many at once. " +
			"A chapter may appear in only ONE block (duplicates are rejected on the spot). When done, call submit_blocks.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"op":          map[string]any{"type": "string", "enum": []string{"list", "add", "update", "delete", "clear"}},
			"n":           map[string]any{"type": "integer"},
			"ns":          map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"title":       map[string]any{"type": "string"},
			"problem_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"chapters":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"blocks":      map[string]any{"type": "array", "description": "batch form: add=[{title,problem_ids,chapters},...]; update=[{n,...},...]"},
		}, "required": []string{"op"}},
	}}
}

func (t *BlocksTool) loadLocked() []StyleBlock {
	var obj struct {
		Blocks []StyleBlock `json:"blocks"`
	}
	if data, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(data, &obj)
	}
	return obj.Blocks
}

func (t *BlocksTool) saveLocked(blocks []StyleBlock) {
	data, _ := json.MarshalIndent(map[string]any{"blocks": blocks}, "", "  ")
	_ = os.WriteFile(t.path, append(data, '\n'), 0o644)
}

// unknownChapters 返回 chapters 里不在 known 中的名字（known 为空不校验）。
func unknownChapters(chapters, known []string) []string {
	if len(known) == 0 {
		return nil
	}
	ok := make(map[string]bool, len(known))
	for _, k := range known {
		ok[k] = true
	}
	var bad []string
	for _, c := range chapters {
		if !ok[c] {
			bad = append(bad, c)
		}
	}
	return bad
}

// validateBlock 校验单条；excl 是 update 时自己的下标（跨块查重跳过自己）。
func validateBlock(list []StyleBlock, b StyleBlock, excl int, known []string) string {
	if strings.TrimSpace(b.Title) == "" || len(b.Chapters) == 0 {
		return fmt.Sprintf("块非法（title 非空、chapters 至少一章）: %+v", b)
	}
	if bad := unknownChapters(b.Chapters, known); len(bad) > 0 {
		return "这些不是可分的章节名: " + strings.Join(bad, ", ") + " — 只能写顶层章节（" +
			strings.Join(known, ", ") + "）；细分父章节里的子节不单独分块，按父章节名写。"
	}
	seen := map[string]bool{}
	for _, c := range b.Chapters {
		if seen[c] {
			return fmt.Sprintf("块内章节重复: %s", c)
		}
		seen[c] = true
	}
	for i, o := range list {
		if i == excl {
			continue
		}
		for _, c := range b.Chapters {
			for _, oc := range o.Chapters {
				if c == oc {
					return fmt.Sprintf("章节 %s 已在第 %d 块（%s）里——每章只能属于一块。", c, i+1, o.Title)
				}
			}
		}
	}
	return ""
}

func (t *BlocksTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	list := t.loadLocked()

	render := func() string {
		if len(list) == 0 {
			return "blocks.json 为空。"
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d 块:\n", len(list))
		for i, bl := range list {
			fmt.Fprintf(&b, "%d. %s · 问题 %s · 章节 %s\n", i+1, bl.Title, strings.Join(bl.ProblemIDs, ","), strings.Join(bl.Chapters, ","))
		}
		return b.String()
	}

	switch strArg(args, "op") {
	case "list":
		return session.ToolResult{Text: render()}, nil
	case "clear":
		t.saveLocked(nil)
		return session.ToolResult{Text: "OK. blocks.json 已清空。"}, nil
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
			return session.ToolResult{Text: "REJECTED: delete 需要 n（单条）或 ns 数组（多条）。"}, nil
		}
		for _, n := range ns {
			if n < 1 || n > len(list) {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: n=%d 超出范围（当前 %d 块）——整批未动。", n, len(list))}, nil
			}
		}
		drop := map[int]bool{}
		for _, n := range ns {
			drop[n-1] = true
		}
		var kept []StyleBlock
		for i, bl := range list {
			if !drop[i] {
				kept = append(kept, bl)
			}
		}
		list = kept
		t.saveLocked(list)
		return session.ToolResult{Text: "OK.\n" + render()}, nil
	case "add", "update":
		raw, _ := args["blocks"].([]interface{})
		if len(raw) == 0 {
			raw = []interface{}{args}
		}
		work := append([]StyleBlock(nil), list...)
		for i, it := range raw {
			m, ok := it.(map[string]interface{})
			if !ok {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素不是对象——整批未动。", i+1)}, nil
			}
			var b StyleBlock
			excl := -1
			if strArg(args, "op") == "add" {
				b = StyleBlock{Title: strArg(m, "title"), ProblemIDs: strSliceArg(m, "problem_ids"), Chapters: strSliceArg(m, "chapters")}
			} else {
				n := intArg(m, "n", 0)
				if n < 1 || n > len(work) {
					return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素 n=%d 超出范围（当前 %d 块）——整批未动。", i+1, n, len(work))}, nil
				}
				excl = n - 1
				b = work[excl]
				if v := strArg(m, "title"); v != "" {
					b.Title = v
				}
				if vs := strSliceArg(m, "problem_ids"); len(vs) > 0 {
					b.ProblemIDs = vs
				}
				if vs := strSliceArg(m, "chapters"); len(vs) > 0 {
					b.Chapters = vs
				}
			}
			if msg := validateBlock(work, b, excl, t.Known); msg != "" {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: 第 %d 个元素: %s——整批未动。", i+1, msg)}, nil
			}
			if excl >= 0 {
				work[excl] = b
			} else {
				work = append(work, b)
			}
		}
		t.saveLocked(work)
		list = work
		return session.ToolResult{Text: "OK.\n" + render()}, nil
	}
	return session.ToolResult{Text: "REJECTED: 未知 op（list/add/update/delete/clear）。"}, nil
}

// SubmitBlocksTool 收评估会话的修复块划分。
// 与整合会话同一套提交方式：会话在自己的虚拟工作区里 write/edit
// blocks.json（可多次编辑、随时查看），最后 submit_blocks 不带参数——
// 工具从工作区读 blocks.json（结构内联传参也兼容，二选一）。
type SubmitBlocksTool struct {
	Blocks    []StyleBlock
	Set       bool
	Workspace string    // 虚拟工作区根（blocks.json 所在目录）；空 = 必须内联传参
	Goal      *GoalTool // T64：提交前计划清单必须清空（勾完或删掉）
	Known     []string  // 合法章节名（顶层章节，T281）
}

func (t *SubmitBlocksTool) Name() string { return "submit_blocks" }

func (t *SubmitBlocksTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit_blocks",
		"description": "Submit the chapter fix blocks. Write/edit them in work:blocks.json first ({\"blocks\": [{title, problem_ids, chapters}...]}) — group chapters that share the same problems into ONE block; every affected chapter must appear in exactly one block), then call this with NO arguments to submit from the file. Passing the same structure inline also works.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blocks": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":       map[string]any{"type": "string"},
						"problem_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"chapters":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []string{"title", "problem_ids", "chapters"},
				}},
			},
		},
	}}
}

func (t *SubmitBlocksTool) Execute(argsJSON string) (session.ToolResult, error) {
	if msg := goalGate(t.Goal); msg != "" {
		return session.ToolResult{Text: msg}, nil
	}
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["blocks"].([]interface{})
	if (!ok || len(items) == 0) && t.Workspace != "" {
		// 无内联参数：从工作区 blocks.json 读（会话可反复编辑后再提交）。
		if data, rerr := os.ReadFile(filepath.Join(t.Workspace, "blocks.json")); rerr == nil {
			fileArgs, perr := parseJSONObject(string(data))
			if perr != nil {
				return session.ToolResult{Text: "REJECTED: work:blocks.json 不是合法 JSON 对象: " + perr.Error() + " — 修正后重新提交。"}, nil
			}
			items, ok = fileArgs["blocks"].([]interface{})
		} else {
			return session.ToolResult{Text: "REJECTED: 未传 blocks 且工作区没有 blocks.json — 先 write_file blocks.json（或内联传参）再提交。"}, nil
		}
	}
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: blocks must be a non-empty array."}, nil
	}
	var out []StyleBlock
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		b := StyleBlock{Title: strArg(m, "title")}
		for _, k := range []string{"problem_ids", "chapters"} {
			if vs, ok := m[k].([]interface{}); ok {
				for _, v := range vs {
					if s, ok := v.(string); ok {
						if k == "problem_ids" {
							b.ProblemIDs = append(b.ProblemIDs, s)
						} else {
							b.Chapters = append(b.Chapters, s)
						}
					}
				}
			}
		}
		if b.Title == "" || len(b.Chapters) == 0 {
			return session.ToolResult{Text: "REJECTED: each block needs a title and at least one chapter."}, nil
		}
		// T281：直接编辑 blocks.json 的会话也要过同一道门——子节名混进来会让
		// 那一章被悄悄跳过（分块会话找不到产物，而它仍以为自己交了）。
		if bad := unknownChapters(b.Chapters, t.Known); len(bad) > 0 {
			return session.ToolResult{Text: "REJECTED: 这些不是可分的章节名: " + strings.Join(bad, ", ") +
				" — 只能写顶层章节（" + strings.Join(t.Known, ", ") + "）；细分父章节里的子节不单独分块，按父章节名写。"}, nil
		}
		out = append(out, b)
	}
	t.Blocks, t.Set = out, true
	return session.ToolResult{Text: fmt.Sprintf("SUBMITTED. %d blocks recorded.", len(out)), Terminal: true}, nil
}

// SubmitBlockFixTool 收分块修复会话的逐章结论。提交后代码把工作区里
// 块内章节的 .tex 与 parts 目录拷回 work/chapters/。
// T64（重构，原 T66）：勾选核对从「参数报 checked_problem_ids」改为
// 「共享存储里本块问题必须全部 Checked」——清单状态由代码核对存储，
// 不靠模型自觉；计划清单（goal）也必须清空。
type SubmitBlockFixTool struct {
	// ChapterResults: chapter base → 是否已解决（false=仍有问题）。
	ChapterResults map[string]bool
	Notes          map[string]string
	Submitted      bool
	// Assigned 是本块要修的问题 id（代码在会话启动前注入）。
	Assigned []string
	Store    *ProblemStore
	Goal     *GoalTool
}

func (t *SubmitBlockFixTool) Name() string { return "submit" }

func (t *SubmitBlockFixTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit",
		"description": "Submit the fixed chapters of this block. chapters: [{chapter (base name, e.g. chapter_003), resolved (true/false), note}] — one entry per chapter in your block. Every assigned problem must be ticked off with the problems tool first, and your goal plan must be complete — otherwise the submit is REJECTED.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chapters": map[string]any{"type": "array", "items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"chapter":  map[string]any{"type": "string"},
						"resolved": map[string]any{"type": "boolean"},
						"note":     map[string]any{"type": "string"},
					},
					"required": []string{"chapter", "resolved"},
				}},
			},
			"required": []string{"chapters"},
		},
	}}
}

func (t *SubmitBlockFixTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	items, ok := args["chapters"].([]interface{})
	if !ok || len(items) == 0 {
		return session.ToolResult{Text: "REJECTED: chapters must be a non-empty array."}, nil
	}
	// T64（重构，原 T66）：勾选核对——共享存储里本块的每个问题都必须 Checked。
	if msg := goalGate(t.Goal); msg != "" {
		return session.ToolResult{Text: msg}, nil
	}
	if t.Store != nil {
		if missing := t.Store.Unchecked(t.Assigned); len(missing) > 0 {
			return session.ToolResult{Text: "REJECTED: 本块这些问题未勾选: " + strings.Join(missing, ", ") +
				" — 修完后用 problems check 打勾，再重新提交。"}, nil
		}
	}
	t.ChapterResults, t.Notes = map[string]bool{}, map[string]string{}
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		base := strArg(m, "chapter")
		resolved, _ := m["resolved"].(bool)
		t.ChapterResults[base] = resolved
		t.Notes[base] = strArg(m, "note")
	}
	if len(t.ChapterResults) == 0 {
		return session.ToolResult{Text: "REJECTED: no valid chapter entries."}, nil
	}
	t.Submitted = true
	return session.ToolResult{Text: "SUBMITTED.", Terminal: true}, nil
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// styleFixReportsSHA 给这一轮的"有问题"工作汇报做内容指纹（T279）。
//
// 用它判断"要不要再跑一次整合会话"：`--step convert` 这类重入（章节全跳过、
// 只补风格修复）以前每次都会把整合会话从第 1 轮重新烧一遍——报告一个字没变，
// 清单也一样，用户看到的只是"这个会话怎么又从头开始了、工具调用归零"。
func styleFixReportsSHA(issues map[string]string) string {
	keys := make([]string, 0, len(issues))
	for k := range issues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%s\x00", k, issues[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// reuseStyleProblems 判断能不能沿用上一轮整合出来的清单（T279）：只有指纹
// 对得上、清单还在才沿用，并且**只清打勾状态**。返回的切片是副本，调用方怎么
// 用都不影响存储。
func reuseStyleProblems(store *ProblemStore, sha string) ([]StyleProblem, bool) {
	if store == nil || sha == "" || store.ReportsSHA != sha || len(store.Problems) == 0 {
		return nil, false
	}
	for i := range store.Problems {
		store.Problems[i].Checked = false
	}
	return append([]StyleProblem(nil), store.Problems...), true
}

// collectIssueReports 汇总工作汇报，返回 (报告总数, 有问题的报告 base→内容)。
func collectIssueReports(proj string) (int, map[string]string) {
	reportsDir := filepath.Join(proj, "work", "reports")
	files, _ := filepath.Glob(filepath.Join(reportsDir, "*.md"))
	issues := map[string]string{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "- 结论: 存在问题") {
			issues[strings.TrimSuffix(filepath.Base(f), ".md")] = string(data)
		}
	}
	return len(files), issues
}

// styleFeedbackLoop 是 T43 的入口：任何章节汇报问题即进入风格修复大循环。
func (r *Runner) styleFeedbackLoop(proj string, round int) error {
	total, issues := collectIssueReports(proj)
	if len(issues) == 0 {
		return nil
	}
	r.log.Log(0, "[style-fix] 工作汇报:", strconv.Itoa(total), "份，其中",
		strconv.Itoa(len(issues)), "份报告 cls/手册问题")
	r.phaseNote()("[style-fix] 工作汇报 %d 份，其中 %d 份报告 cls/手册问题", total, len(issues))

	maxRounds := r.cfg.Latex.StyleFix.MaxRounds
	if maxRounds < 0 {
		r.log.LogWarning(0, "[style-fix] style_fix.max_rounds<0，跳过风格修复（问题留给 checker/终审）")
		return nil
	}

	// 问题清单（整合会话跑一轮，跨轮复用——修复轮之间问题集不变，变的是修复结果）。
	// T64（重构）：清单存 work/style_fix/problems.json，problems 工具读写——
	// 进循环前重置（旧一轮的勾选状态不带入新清单）。
	store := loadProblemStore(styleFixProblemStorePath(proj))
	sha := styleFixReportsSHA(issues)
	var problems []StyleProblem
	if reused, ok := reuseStyleProblems(store, sha); ok {
		problems = reused
		// T279：工作汇报一个字没变 → 沿用上一次的清单，只把打勾状态清掉
		// （问题定义照旧，别让上一轮的勾选把这一轮的活提前判成"已解决"）。
		if err := store.saveLocked(); err != nil {
			r.log.LogWarning(1, "[style-fix] 问题清单打勾状态重置失败:", err)
		}
		r.log.Log(0, "[style-fix] 工作汇报与上次一致（"+sha[:8]+"）——沿用上次整合的清单，不重跑整合会话")
	} else {
		if store.ReportsSHA != "" && len(store.Problems) > 0 {
			r.log.Log(0, "[style-fix] 工作汇报有变化（旧 "+store.ReportsSHA[:8]+" → 新 "+sha[:8]+"）——重新整合问题清单")
		}
		store.Problems = nil
		store.ReportsSHA = ""
		if err := store.saveLocked(); err != nil {
			r.log.LogWarning(1, "[style-fix] 问题清单重置失败:", err)
		}
		var err error
		problems, err = r.styleFixIntegrate(proj, issues, store)
		if err != nil {
			return fmt.Errorf("问题整合会话失败: %w", err)
		}
		// 只有整合真的成功才把指纹记上：失败时留着旧指纹会让下一轮误判"清单可用"。
		store.ReportsSHA = sha
		if err := store.saveLocked(); err != nil {
			r.log.LogWarning(1, "[style-fix] 问题清单指纹落盘失败:", err)
		}
	}
	r.log.Log(0, "[style-fix] 问题清单:", strconv.Itoa(len(problems)), "项")

	for round = 1; round <= maxRounds; round++ {
		r.phaseNote()("[style-fix] 第 %d/%d 轮风格修复", round, maxRounds)

		// 1. 风格修复会话：按问题清单修 cls/手册。
		if err := r.styleFixStyleSession(proj, round, problems, store); err != nil {
			return fmt.Errorf("风格修复会话（第 %d 轮）失败: %w", round, err)
		}

		// 2. 影响评估：按问题划分章节块。
		blocks, err := r.styleFixAssess(proj, round, problems, issues, store)
		if err != nil {
			return fmt.Errorf("影响评估会话（第 %d 轮）失败: %w", round, err)
		}
		r.log.Log(0, "[style-fix] 修复块:", strconv.Itoa(len(blocks)), "块")

		// 3. 分块并行修复（定位修改，绝不重做）。
		remaining := r.styleFixBlocks(proj, round, blocks, store)

		// 4. 收尾判定。
		if len(remaining) == 0 {
			r.log.Log(0, "[style-fix] 第", strconv.Itoa(round), "轮后所有问题已解决")
			r.phaseNote()("[style-fix] 第 %d 轮后所有问题已解决", round)
			return nil
		}
		r.log.LogWarning(0, "[style-fix] 第", strconv.Itoa(round), "轮后仍有",
			strconv.Itoa(len(remaining)), "章报告问题:", strings.Join(remaining, ", "))
	}
	r.log.LogWarning(0, "[style-fix] 已达最大轮数("+strconv.Itoa(maxRounds)+")，停止风格修复循环（遗留问题交给编译/终审）")
	r.phaseNote()("[style-fix] 已达最大轮数(%d)，遗留问题交给编译/终审", maxRounds)
	return nil
}

// styleFixIntegrate 跑问题整合会话。
// T64：会话有一个虚拟工作区 work/stylefix_integrate/——各章工作汇报按
// reports/<章>.md 落盘，配 read/grep/write/edit 文件工具，模型自己翻、
// 自己整理，不再靠一条巨型 user 消息灌全文。
// T64（重构）：清单用 problems 工具逐条写入共享存储（整合会话独占
// 写权限），submit_problems 无参收账；goal 工具给模型自己的计划清单
// （plan.md），提交前必须全部打勾。T67 的 list.md 随之退役。
func (r *Runner) styleFixIntegrate(proj string, issues map[string]string, store *ProblemStore) ([]StyleProblem, error) {
	// 虚拟工作区：汇报原文落盘（持久目录，跨轮可查）。
	wsDir := filepath.Join(proj, "work", "stylefix_integrate")
	reportsDir := filepath.Join(wsDir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(issues))
	for k := range issues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = os.WriteFile(filepath.Join(reportsDir, k+".md"), []byte(issues[k]), 0o644)
	}

	goal := NewGoalTool(filepath.Join(wsDir, "plan.md"))
	submit := &SubmitProblemsTool{Store: store, Goal: goal}
	mounts := []Mount{{Name: "work", Dir: wsDir}}
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&GrepTool{Mounts: mounts},
		&WriteWorkFileTool{Root: wsDir, AnyExt: true},
		&EditWorkFileTool{Root: wsDir},
		&ProblemsTool{Store: store, Write: true},
		goal,
		submit,
	}
	// T65：integrate_model 单独配模型；空则沿用 style 模型。
	modelKey := r.cfg.Latex.StyleFix.IntegrateModel
	if modelKey == "" {
		modelKey = r.cfg.Latex.StyleModel
	}
	sess := session.NewSession(r.clientFor(modelKey), r.modelOf(modelKey),
		r.cfg.LatexSession("style"), renderPrompt(prompts.Must(prompts.StyleFixIntegrateSystem), r.cfg.LatexSession("style"), r.outputLang()),
		tools, r.log, 1, "stylefix-integrate")
	// T283：整合会话也一样能续——被打断就接着那条转录把清单写完；要重来由用户
	// 在外面删掉转录与工作区文件，代码不替人删。
	msgs, trPath, resume := sessionAttemptPath(filepath.Join(proj, "work", "sessions"),
		"stylefix_integrate", "submit_problems")
	if resume {
		sess.SetMessages(msgs)
		if _, st, serr := session.LoadTranscriptStats(trPath); serr == nil {
			sess.SeedCounters(st.Rounds, st.Tools)
		}
		r.log.Log(0, "[style-fix] 整合会话接着上次没交清单的转录继续"+resumeNote(len(msgs)))
	}
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow("stylefix-integrate", "stylefix-integrate")
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := fmt.Sprintf("The work reports from the chapter conversion sessions that flagged style/manual problems are in your workspace at work:reports/<chapter>.md (%d files). "+
		"Read/grep them, then build the consolidated, de-duplicated problem list with the problems tool (one add call can take several items), and call submit_problems when the list is complete.", len(keys))
	if resume {
		userText = "Continue the consolidation from where it stopped (the previous attempt ended without submit_problems; the reports and whatever you already added are still in the workspace). Finish the list and call submit_problems."
	}
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, err
	}
	if !r.remindSubmit(sess, 1, "[style-fix]", "integrate", "submit_problems", func() bool { return submit.Set }) {
		return nil, fmt.Errorf("整合会话未提交 submit_problems（已提醒两次）")
	}
	r.keepSessionFile(trPath)
	store.mu.Lock()
	problems := append([]StyleProblem(nil), store.Problems...)
	store.mu.Unlock()
	return problems, nil
}

// styleFixStyleSession 跑一轮风格修复会话。
//
// 上下文：这一次尝试**没交过 submit_style** → 接着那条转录继续（T284）；否则是
// 新的一次尝试 → 从样式阶段的 `work/style_session.jsonl` 起（它记着 cls 为什么
// 这么设计），两者都没有就用空上下文，不再因为"上一轮跑完了"而把旧结论续上来。
func (r *Runner) styleFixStyleSession(proj string, round int, problems []StyleProblem, store *ProblemStore) error {
	ctxPath := filepath.Join(proj, "work", "style_session.jsonl")
	msgs, trPath, resumed := sessionAttemptPath(filepath.Join(proj, "work", "sessions"),
		fmt.Sprintf("stylefix_style_r%d", round), "submit_style")
	seedPath := trPath
	if !resumed {
		msgs = nil
		seedPath = ""
		if ctxMsgs, cerr := loadSessionContext(ctxPath); cerr == nil && len(ctxMsgs) > 0 {
			msgs, seedPath = ctxMsgs, ctxPath
		}
	}
	styleDir := filepath.Join(proj, "style")
	workDir := filepath.Join(proj, "work", "style")

	// T70：stylefix_style_model 独立配模型；空 = 沿用 style 模型。
	styleKey := r.cfg.Latex.StyleFix.StyleFixStyleModel
	if styleKey == "" {
		styleKey = r.cfg.Latex.StyleModel
	}
	client := r.clientFor(styleKey)
	modelCfg := r.modelOf(styleKey)
	tuning := r.cfg.LatexSession("style")

	goal := NewGoalTool(filepath.Join(workDir, "plan.md"))
	submit := &SubmitStyleTool{Workspace: workDir, Gate: func() string {
		// T64：问题清单全打勾 + 计划全完成才放行。
		if un := store.Unchecked(store.AllIDs()); len(un) > 0 {
			return "REJECTED: 问题清单还有 " + strconv.Itoa(len(un)) + " 个未打勾: " + strings.Join(un, ", ") +
				" —— 逐个修到不会再发生后用 problems check 打勾（认为不属于样式包责任的用 detail 说明后同样打勾并在提交报告里注明），再重新提交。"
		}
		return goalGate(goal)
	}}
	tag := fmt.Sprintf("stylefix-style-r%d", round)
	bashTmp, cleanBash := r.sessionBashTemp(proj, "bash_stylefix")
	defer cleanBash()
	tools := r.styleSessionTools(proj, workDir, filepath.Join(proj, "source"), tag, bashTmp, submit)
	tools = append(tools, &ProblemsTool{Store: store}, goal)

	sess := session.NewSession(client, modelCfg, tuning, r.styleSystemPrompt(), tools, r.log, 1, tag)
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()
	sess.SetMessages(msgs)
	if resumed {
		// 接着上一次被打断的尝试跑：轮次/工具数也续上。
		if _, st, serr := session.LoadTranscriptStats(seedPath); serr == nil {
			sess.SeedCounters(st.Rounds, st.Tools)
		}
		r.log.Log(0, "[style-fix] 接着上一次没交 submit_style 的尝试继续（"+filepath.Base(seedPath)+"，"+
			resumeNote(len(msgs))+"）")
	} else if len(msgs) > 0 {
		r.log.Log(0, "[style-fix] 新一轮风格修复：上下文来自样式阶段（"+filepath.Base(seedPath)+"，"+
			strconv.Itoa(len(msgs))+" 条历史）")
	} else {
		r.log.Log(0, "[style-fix] 新一轮风格修复：没有可用的样式上下文，从空会话开始")
	}
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}

	feedback := fmt.Sprintf("The chapter conversion phase finished and %d style/manual problems were consolidated. "+
		"They are NOT pasted here — use the problems tool to see them. "+
		"Fix the cls/manual/example so these problems cannot recur, tick each one off with problems check as you resolve it, then submit_style.", len(problems))
	if resumed {
		feedback = "Continue this round from where it stopped (the previous attempt ended without submit_style). " + feedback
	}
	if _, err := sess.Run(session.RunOptions{UserText: feedback}); err != nil {
		return fmt.Errorf("风格修复会话失败: %w", err)
	}
	if err := saveSessionContext(sess, ctxPath); err != nil {
		r.log.LogWarning(1, "[style-fix] 会话上下文回写失败:", err)
	}
	if !r.remindSubmit(sess, 1, "[style-fix]", "style", "submit_style", func() bool { return submit.Set }) {
		return fmt.Errorf("风格修复会话未提交 submit_style（已提醒两次）")
	}
	clsName := classNameOf(submit.Cls)
	if clsName == "" {
		return fmt.Errorf("风格修复提交的 cls 缺少 \\ProvidesClass{...}")
	}
	if err := os.WriteFile(filepath.Join(styleDir, clsName+".cls"), []byte(submit.Cls), 0o644); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(styleDir, "manual.md"), []byte(submit.Manual), 0o644)
	_ = os.WriteFile(filepath.Join(styleDir, "example.tex"), []byte(submit.Example), 0o644)
	r.writeStyleExtras(styleDir, submit)

	// example 编译验证（与样式阶段同一门槛）。
	scratch, cleanScratch, err := r.tempDir(proj, "stylefix-check")
	if err != nil {
		return err
	}
	defer cleanScratch()
	if err := copyFile(filepath.Join(styleDir, clsName+".cls"), filepath.Join(scratch, clsName+".cls")); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(styleDir, "example.tex"), filepath.Join(scratch, "example.tex")); err != nil {
		return err
	}
	copyStyleExtras(styleDir, scratch, submit, clsName)
	start := time.Now()
	res := r.comp.Compile(scratch, "example.tex")
	LogCompileResult(r.log, 1, tag, res, time.Since(start))
	if !res.OK {
		return fmt.Errorf("修复后的 example 编译失败: %s", res.Err)
	}
	r.keepSessionFile(trPath)
	r.log.Log(1, "[style-fix] 样式包已更新:", clsName+".cls")
	return nil
}

// transcriptSubmitted 报告一条转录里是否调过本会话的终态工具（T284）。
// 交过 = 那一次尝试已经完成（无论之后进程有没有被中断）。
func transcriptSubmitted(path, terminalTool string) bool {
	data, err := os.ReadFile(path)
	if err != nil || terminalTool == "" {
		return false
	}
	type record struct {
		T         string `json:"t"`
		ToolCalls []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var rec record
		if json.Unmarshal([]byte(ln), &rec) != nil || rec.T != "msg" {
			continue
		}
		for _, tc := range rec.ToolCalls {
			if tc.Function.Name == terminalTool {
				return true
			}
		}
	}
	return false
}

// sessionAttemptPath 给一个会话挑"这一次尝试"的转录（T284）。
//
// 规则与转换/核对一致：`<base>.jsonl` 存在且**还没交过终态工具** → 那一次被打断
// 了，接着它跑；已经交过 → 那一次已经跑完，换下一条 `<base>.2.jsonl`、
// `<base>.3.jsonl`……（旧记录原样留着，不覆盖也不删）。要真正从头再来，由用户
// 在外面删掉这些文件，代码不替人删。
func sessionAttemptPath(dir, base, terminalTool string) ([]session.ChatMessage, string, bool) {
	for k := 1; k < 1000; k++ {
		name := base + ".jsonl"
		if k > 1 {
			name = fmt.Sprintf("%s.%d.jsonl", base, k)
		}
		p := filepath.Join(dir, name)
		if !fileExists(p) {
			return nil, p, false
		}
		msgs, lerr := loadSessionContext(p)
		if lerr != nil || len(msgs) == 0 {
			return nil, p, false
		}
		if transcriptSubmitted(p, terminalTool) {
			continue
		}
		return msgs, p, true
	}
	return nil, filepath.Join(dir, base+".jsonl"), false
}

// resumeNote 是"接着上一次没交结论的尝试跑"的日志后缀（T283/T284）。
func resumeNote(n int) string {
	if n == 0 {
		return ""
	}
	return "（" + strconv.Itoa(n) + " 条历史）"
}

// subdividedChapters 挑出这些章节里的"细分父章节"：它们的顶层 .tex 只是
// 一串 `\input`，正文在 `<章>/children/**`（还可能再嵌一层子目录）。
// 分块单位仍是那个顶层章节，但评估/修复都必须知道该去哪个目录里找正文。
func subdividedChapters(proj string, names []string) []string {
	chapWork := filepath.Join(proj, "work", "chapters")
	var out []string
	for _, b := range names {
		if st, err := os.Stat(filepath.Join(chapWork, b, "children")); err == nil && st.IsDir() {
			out = append(out, b)
		}
	}
	return out
}

// styleFixAssess 跑影响评估会话：按问题划分章节块。
func (r *Runner) styleFixAssess(proj string, round int, problems []StyleProblem, issues map[string]string, store *ProblemStore) ([]StyleBlock, error) {
	chapWork := filepath.Join(proj, "work", "chapters")
	texs, _ := filepath.Glob(filepath.Join(chapWork, "*.tex"))
	var chapterNames []string
	for _, t := range texs {
		chapterNames = append(chapterNames, strings.TrimSuffix(filepath.Base(t), ".tex"))
	}
	sort.Strings(chapterNames)

	var issueList []string
	for k := range issues {
		issueList = append(issueList, k)
	}
	sort.Strings(issueList)

	// T64 同款：评估/分组会话有自己的虚拟工作区——blocks.json 由会话
	// write/edit（可多次修改、随时查看），submit_blocks 从文件读。
	wsDir := filepath.Join(proj, "work", "stylefix_assess")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		return nil, err
	}
	// T282：这个会话跟其余会话一样"能续就续"——磁盘上有转录就接着它跑，
	// plan.md/blocks.json 也原样留着（那就是上一轮的进度）。要真正从头开始，
	// 由用户在外面删掉转录（和 work/stylefix_assess/ 下的文件）；代码不替人删。
	msgs, trPath, resume := sessionAttemptPath(filepath.Join(proj, "work", "sessions"),
		fmt.Sprintf("stylefix_assess_r%d", round), "submit_blocks")
	if resume {
		r.log.Log(0, "[style-fix] 评估会话接着上次没交结论的转录继续"+resumeNote(len(msgs)))
	}
	goal := NewGoalTool(filepath.Join(wsDir, "plan.md"))
	blocks := NewBlocksTool(filepath.Join(wsDir, "blocks.json"), chapterNames...)
	submit := &SubmitBlocksTool{Workspace: wsDir, Goal: goal, Known: chapterNames}
	mounts := []Mount{{Name: "project", Dir: proj}, {Name: "work", Dir: wsDir}}
	// T282：评估会话以前只有 read_file/grep，没有 ls——想看"这一章有哪些子节
	// 文件"只能靠 grep 试出来（真实运行里为此烧掉十几轮、三十多分钟）。给它
	// bash：ls/find/wc/grep -rn 都能用，路径和 file 工具同一套前缀。
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, fmt.Sprintf("bash_assess_r%d", round))
	defer cleanBashTmp()
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&GrepTool{Mounts: mounts},
		&WriteWorkFileTool{Root: wsDir, AnyExt: true},
		&EditWorkFileTool{Root: wsDir},
		&WorkBashTool{Root: wsDir, Mounts: mounts, TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(),
			Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: 1},
		&ProblemsTool{Store: store},
		goal,
		blocks,
		submit,
	}
	tag := fmt.Sprintf("stylefix-assess-r%d", round)
	// T70：stylefix_assess_model 独立配模型；空 = 沿用 style 模型。
	assessKey := r.cfg.Latex.StyleFix.StyleFixAssessModel
	if assessKey == "" {
		assessKey = r.cfg.Latex.StyleModel
	}
	sess := session.NewSession(r.clientFor(assessKey), r.modelOf(assessKey),
		r.cfg.LatexSession("style"), renderPrompt(prompts.Must(prompts.StyleFixAssessSystem), r.cfg.LatexSession("style"), r.outputLang()),
		tools, r.log, 1, tag)
	// 上一轮没跑完就接着它（同一条转录往后追加、计数也续上）；跑完了就另开
	// 一条，页面不会把两次评估显示成一条会话。
	if resume {
		sess.SetMessages(msgs)
		if _, st, serr := session.LoadTranscriptStats(trPath); serr == nil {
			sess.SeedCounters(st.Rounds, st.Tools)
		}
	}
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := fmt.Sprintf("The consolidated style-problem list (%d problems) is available through the problems tool (list / get / search) — it is NOT pasted here.", len(problems)) +
		"\nChapters that explicitly reported problems: " + strings.Join(issueList, ", ") +
		"\nAll converted chapters: " + strings.Join(chapterNames, ", ") +
		"\nTheir .tex files are readable at project:work/chapters/<name>.tex (a chapter's parts live under project:work/chapters/<name>/). Style problems are often GLOBAL — assess which chapters each problem actually affects (grep the .tex files for the pattern when in doubt), then partition the affected chapters into fix blocks with the blocks tool (or edit work:blocks.json directly — both land in the same file), then call submit_blocks with NO arguments to submit from the file."
	// T281：细分父章节的顶层 .tex 只是 `\input` 列表，照着它 grep 什么都找不到——
	// 必须点名正文目录，否则评估会话会把最厚的几章判成"没受影响"。
	if sub := subdividedChapters(proj, chapterNames); len(sub) > 0 {
		userText += "\nSUBDIVIDED chapters — their top .tex is only `\\input` lines, the body is in work/chapters/<name>/children/** (grep THAT folder before deciding a chapter is unaffected): " + strings.Join(sub, ", ")
	}
	if resume {
		// 接着上次跑：长指令在历史里，这里只说接着干什么。
		userText = "Continue this assessment from where it stopped (the previous attempt ended without submit_blocks). Finish the partition and call submit_blocks."
	}
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, err
	}
	if !r.remindSubmit(sess, 1, "[style-fix]", "blocks", "submit_blocks", func() bool { return submit.Set }) {
		return nil, fmt.Errorf("评估会话未提交 submit_blocks（已提醒两次）")
	}
	r.keepSessionFile(trPath)
	// blocks.json 最终版回存工作区（会话编辑的最终态，跨轮可查）。
	if data, err := json.Marshal(map[string]any{"blocks": submit.Blocks}); err == nil {
		_ = os.WriteFile(filepath.Join(wsDir, "blocks.json"), data, 0o644)
	}
	return submit.Blocks, nil
}

// subdividedBlockNote 给分块会话补一句"正文在这些子目录里"（T281）。
// 块里没有细分章节时返回空串，提示词保持原样。
func subdividedBlockNote(proj string, chapters []string) string {
	sub := subdividedChapters(proj, chapters)
	if len(sub) == 0 {
		return ""
	}
	var parts []string
	for _, b := range sub {
		parts = append(parts, b+" → chapters/"+b+"/children/")
	}
	return "\nSUBDIVIDED chapters in this block — the top .tex is only `\\input` lines, the body you must grep and edit is under " +
		strings.Join(parts, ", ")
}

// styleFixBlocks 并行跑分块修复会话，返回仍有问题的章节列表。
// 块会话失败的章节退回单章 fixChapterStyle，再失败删产物重转换。
func (r *Runner) styleFixBlocks(proj string, round int, blocks []StyleBlock, store *ProblemStore) []string {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	chapWork := filepath.Join(proj, "work", "chapters")
	reportsDir := filepath.Join(proj, "work", "reports")

	conc := r.cfg.Latex.Concurrency
	if conc <= 0 {
		conc = 3
	}
	tidPool := make(chan int, conc)
	for i := 1; i <= conc; i++ {
		tidPool <- i
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var remaining []string // 仍有问题
	var fallback []string  // 块会话失败 → 退回单章修复

	for bi, block := range blocks {
		wg.Add(1)
		tid := <-tidPool
		go func(bi int, block StyleBlock, tid int) {
			defer wg.Done()
			defer func() { tidPool <- tid }()
			results, notes, err := r.styleFixBlockSession(proj, round, bi+1, clsName, block, store, tid)
			if err != nil {
				r.log.LogWarning(tid, "[style-fix] 块", strconv.Itoa(bi+1), "会话失败，退回单章修复:", err)
				mu.Lock()
				fallback = append(fallback, block.Chapters...)
				mu.Unlock()
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, base := range block.Chapters {
				resolved, ok := results[base]
				note := notes[base]
				reportPath := filepath.Join(reportsDir, base+".md")
				if !ok {
					// 会话没汇报这一章：保守视为仍有问题。
					resolved = false
					note = "块修复会话未汇报本章"
				}
				conclusion := "无问题"
				if !resolved {
					conclusion = "存在问题"
					remaining = append(remaining, base)
				}
				report := "- 结论: " + conclusion + "\n\n样式修复（第 " + strconv.Itoa(round) + " 轮，块 " + strconv.Itoa(bi+1) + "）: " + note + "\n"
				_ = os.MkdirAll(reportsDir, 0o755)
				_ = os.WriteFile(reportPath, []byte(report), 0o644)
			}
		}(bi, block, tid)
	}
	wg.Wait()

	// 块会话失败的章节退回单章增量修复（复用既有 fixChapterStyle）。
	for _, base := range fallback {
		texPath := filepath.Join(chapWork, base+".tex")
		if !fileExists(texPath) {
			continue
		}
		issues := ""
		if data, err := os.ReadFile(filepath.Join(reportsDir, base+".md")); err == nil {
			issues = string(data)
		}
		if err := r.fixChapterStyle(proj, clsName, filepath.Join(proj, "style", "manual.md"),
			filepath.Join(proj, "chapters", base+".md"), filepath.Join(proj, "work"), base, issues, 1); err != nil {
			r.log.LogWarning(1, "[style-fix]", base, "单章修复也失败，删除产物待重转换:", err)
			_ = os.RemoveAll(texPath)
			_ = os.RemoveAll(filepath.Join(chapWork, base))
			r.keepSessionFile(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"))
			mu.Lock()
			remaining = append(remaining, base)
			mu.Unlock()
		}
	}
	sort.Strings(remaining)
	return remaining
}

// styleFixBlockSession 跑一个分块修复会话：块内章节的 .tex/parts 拷进临时
// 工作区，会话在里面做定位修改，提交后拷回 work/chapters/。
func (r *Runner) styleFixBlockSession(proj string, round, blockIdx int, clsName string, block StyleBlock, store *ProblemStore, tid int) (map[string]bool, map[string]string, error) {
	chapWork := filepath.Join(proj, "work", "chapters")
	tag := fmt.Sprintf("stylefix-blk-r%d-%d", round, blockIdx)

	// T283：块会话也能续——上一轮被打断时工作区（含已改过的章节副本）与转录都
	// 还在，直接接着用；要重来由用户在外面删 work/temp/<tag>/ 与转录。
	msgs, trPath, resume := sessionAttemptPath(filepath.Join(proj, "work", "sessions"), tag, "submit")
	// 临时工作区：样式包 + 块内章节（.tex + parts 目录）+ 图片软链。
	// 续跑时复用上一次被打断的工作区（里面有已改过的章节副本）；上一次已经交过
	// 就是新的一次尝试，重新铺一份。
	mkTemp := r.tempDir
	if resume {
		mkTemp = r.tempDirKeep
	}
	work, cleanWork, err := mkTemp(proj, tag)
	if err != nil {
		return nil, nil, err
	}
	defer cleanWork()
	copyFile(filepath.Join(proj, "style", clsName+".cls"), filepath.Join(work, clsName+".cls"))
	copyFile(filepath.Join(proj, "style", "manual.md"), filepath.Join(work, "manual.md"))
	copyFile(filepath.Join(proj, "style", "example.tex"), filepath.Join(work, "example.tex"))
	for _, asset := range []string{"images", "figures"} {
		src := filepath.Join(proj, "source", asset)
		if !fileExists(src) || pathExists(filepath.Join(work, asset)) {
			continue
		}
		if err := linkAbs(src, filepath.Join(work, asset)); err != nil {
			_ = copyDir(src, filepath.Join(work, asset))
		}
	}
	chapSub := filepath.Join(work, "chapters")
	_ = os.MkdirAll(chapSub, 0o755)
	// 续跑时工作区里已经有章节副本：不再整体重铺（那会把上一轮改过的地方覆盖回
	// 原始产物），只补缺失的。样式包以磁盘上的最新版为准（可能刚被上一轮改过）。
	for _, base := range block.Chapters {
		if resume && fileExists(filepath.Join(chapSub, base+".tex")) {
			continue
		}
		tex := filepath.Join(chapWork, base+".tex")
		if !fileExists(tex) {
			continue
		}
		copyFile(tex, filepath.Join(chapSub, base+".tex"))
		if parts := filepath.Join(chapWork, base); fileExists(parts) {
			_ = copyDir(parts, filepath.Join(chapSub, base))
		}
		// 每章一个编译 wrapper（工作区里可直接 compile 验证）。
		wrapper := "\\documentclass{" + clsName + "}\n" +
			"\\usepackage{graphicx,amsmath,amssymb,longtable,booktabs}\n" +
			"\\graphicspath{{figures/}}\n" +
			"\\begin{document}\n\\input{chapters/" + base + ".tex}\n\\end{document}\n"
		_ = os.WriteFile(filepath.Join(work, base+"_wrapper.tex"), []byte(wrapper), 0o644)
	}

	mounts := []Mount{{Name: "work", Dir: work}}
	// T282：块内章节的正文常在 chapters/<章>/children/**，只能靠 ls/find 才看得
	// 清楚（read_file/grep 都得先知道文件名）。
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, tag+"-bash")
	defer cleanBashTmp()
	goal := NewGoalTool(filepath.Join(work, "plan.md"))
	// T64（重构）：清单勾选改走共享存储的 problems 工具——提交时代码
	// 直接核对存储里本块问题的勾选状态（T66 的 list.md/checked_problem_ids
	// 退役，工作区里不再有清单文件）。
	submit := &SubmitBlockFixTool{Assigned: block.ProblemIDs, Store: store, Goal: goal}
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&EditWorkFileTool{Root: work},
		&WriteWorkFileTool{Root: work, AnyExt: true},
		&GrepTool{Mounts: mounts},
		&WorkBashTool{Root: work, Mounts: mounts, TmpDir: bashTmp, MaxOutput: r.cfg.BashMaxOutput(),
			Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: tid},
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		// T68：分块修复会话也要能看编译出的 PDF（真实运行中曾想看不能看）。
		&ViewPDFTool{Mounts: mounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&CompileTexTool{Comp: r.comp, Root: work, Log: r.log, Tid: tid, Tag: tag},
		&ProblemsTool{Store: store},
		goal,
		submit,
	}
	// T70：stylefix_block_model 独立配模型；空 = 沿用 convert 模型。
	blockKey := r.cfg.Latex.StyleFix.StyleFixBlockModel
	if blockKey == "" {
		blockKey = r.cfg.Latex.ConvertModel
	}
	sess := session.NewSession(r.clientFor(blockKey), r.modelOf(blockKey),
		r.cfg.LatexSession("convert"), renderPrompt(prompts.Must(prompts.StyleFixBlockSystem), r.cfg.LatexSession("convert"), r.outputLang()),
		tools, r.log, tid, tag)
	if resume {
		sess.SetMessages(msgs)
		if _, st, serr := session.LoadTranscriptStats(trPath); serr == nil {
			sess.SeedCounters(st.Rounds, st.Tools)
		}
		r.log.Log(0, "[style-fix] 块修复会话接着上次没交结论的转录继续"+resumeNote(len(msgs)))
	}
	if tr, terr := session.NewTranscript(trPath); terr == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	liveHook, liveClose := r.livePhaseRow(tag, tag)
	sess.SetProgressHook(liveHook)
	defer liveClose()

	userText := "Style problems assigned to this block: " + strings.Join(block.ProblemIDs, ", ") +
		" — use the problems tool to read each one's full detail; they are NOT pasted here." +
		"\nYour chapters (in work:chapters/): " + strings.Join(block.Chapters, ", ") +
		subdividedBlockNote(proj, block.Chapters) +
		"\nThe class and manual in your workspace are the REVISED versions. Make minimal, targeted edits — never re-convert. " +
		"Compile each chapter's wrapper (<base>_wrapper.tex) to verify (view_pdf the result if in doubt), " +
		"tick every assigned problem off with problems check as you resolve it, then submit with one entry per chapter."
	if resume {
		userText = "Continue this block fix from where it stopped (the previous attempt ended without submit; your edited chapters are still in the workspace and the assigned problems are unchanged). " + userText
	}
	if _, err := sess.Run(session.RunOptions{UserText: userText}); err != nil {
		return nil, nil, err
	}
	if !submit.Submitted {
		return nil, nil, fmt.Errorf("块修复会话未提交")
	}

	// 拷回块内章节的产物。
	for base := range submit.ChapterResults {
		src := filepath.Join(chapSub, base+".tex")
		if fileExists(src) {
			copyFile(src, filepath.Join(chapWork, base+".tex"))
		}
		parts := filepath.Join(chapSub, base)
		if fileExists(parts) {
			_ = copyDir(parts, filepath.Join(chapWork, base))
		}
	}
	r.keepSessionFile(trPath)
	return submit.ChapterResults, submit.Notes, nil
}
