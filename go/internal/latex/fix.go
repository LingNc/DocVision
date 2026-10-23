package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

/*
 * P60：子章节「修复会话」。
 *
 * 背景（用户口径）：父章节重试代价太大。子会话都已跑过之后，应当能**按修复
 * 任务重新下派**到已经划好的章节与工作区上，用新会话继续改，父会话只做整合；
 * 父章节反复失败到与转换重试同一上限时才整章重开。
 *
 * 形态：
 *
 *   - 契约：父会话在整理阶段用 `dispatch_fix` 工具把「哪个子章节、按哪条规则
 *     改」写成 `work/fix/<根章>/plan.json`；工具**只记录计划**、不同步执行。
 *   - 调度（第二步）：`convertChapterNode` 在子章节转换+核对完、父会话整合之前，
 *     给它一次「看子章节结果、把还不对的还回去」的对话；它写下计划后，runner
 *     按计划逐个子章节跑 `runChildFix`（各自的工作区、各自的轮数、各自的
 *     核对），然后把产物刷新回父章节的 children/，再做今天的整合。
 *   - 预算：`latex.fix.max_rounds`（默认 2）；用完就退回老行为（父会话重试）。
 *   - 总开关：`latex.fix.enabled`，默认关闭——关着的时候既不注册工具、也不
 *     追加提示词、也不多跑任何一条会话，跑法与引入 P60 之前逐字节相同。
 */

const (
	// fixPlanVersion 是计划文件的 schema 版本。读到别的版本直接报错——宁可
	// 让调度停下，也不要按自己的猜测去执行一份不认识的计划。
	fixPlanVersion = 1
	// fixIssueIDMax 只做长度上限，避免一条 issue 把回执/日志刷爆。
	fixIssueIDMax = 120
)

// fixChildPattern 是一个合法子章节目录名：`child_01`、`child_16`。
// 名字不是自由文本——它是细分树里真实存在的目录名（book.go 用
// fmt.Sprintf("child_%02d", i+1) 分配）。
var fixChildPattern = regexp.MustCompile(`^child_[0-9]{2}$`)

// fixPlanIssue 是计划里的一条修复任务（父会话下发的最小单位）。
//
// 四个字段都必填：`where` 与 `what` 让修复会话知道改哪儿、改什么，`rule` 是
// 它必须满足的验收条件（核对按同一套规则判），`id` 是父会话追踪这条任务用
// 的稳定标识（可以是 issue 的 T/P 号，也可以是一个 slug）。
type fixPlanIssue struct {
	ID    string `json:"id"`
	What  string `json:"what"`
	Rule  string `json:"rule"`
	Where string `json:"where"`
}

// fixPlanChild 是一个子章节名下的全部修复任务。
type fixPlanChild struct {
	Child  string         `json:"child"`
	Issues []fixPlanIssue `json:"issues"`
}

// fixPlan 是 `work/fix/<根章>/plan.json` 的内容（schema v1）。
//
// 文件按子章节分组而不是按 issue 平铺：一个子章节一条修复会话，分组就是
// 调度单位；`root` 是下派它的根章节（同一次运行的父会话），`created` 是
// RFC3339 时间戳，便于事后对上一批文件改的是什么。
type fixPlan struct {
	Version  int            `json:"version"`
	Created  string         `json:"created"`
	Root     string         `json:"root"`
	Children []fixPlanChild `json:"children"`
}

// fixPlanDir 是一个根章节的计划目录：<proj>/work/fix/<根章>/。
func fixPlanDir(proj, rootChart string) string {
	return filepath.Join(proj, "work", "fix", rootChart)
}

// fixPlanPath 是计划文件的落点：<proj>/work/fix/<根章>/plan.json。
func fixPlanPath(proj, rootChart string) string {
	return filepath.Join(fixPlanDir(proj, rootChart), "plan.json")
}

// validateFixRoot 拒绝不能当目录名的根章节名（计划目录就是按它建的）。
func validateFixRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("根章节名为空")
	}
	if strings.ContainsAny(root, `/\`) || root == "." || root == ".." {
		return fmt.Errorf("根章节名不合法: %q", root)
	}
	return nil
}

// validateFixChild 校验一个子章节名：必须是 `child_NN` 的形状（细分树里的
// 真实目录名），不接受自由文本或路径。
func validateFixChild(child string) error {
	child = strings.TrimSpace(child)
	if child == "" {
		return fmt.Errorf("子章节名为空")
	}
	if !fixChildPattern.MatchString(child) {
		return fmt.Errorf("子章节名不合法: %q（应为 child_01 … child_NN，必须是已存在的细分目录）", child)
	}
	return nil
}

// normalizeFixPlan 补齐派生字段并做与写盘相关的校验（版本、根章、子章节名、
// 字段非空）。写与读走同一套校验：手改过的计划文件也要过。
func normalizeFixPlan(proj, root string, plan fixPlan) (fixPlan, error) {
	if err := validateFixRoot(root); err != nil {
		return fixPlan{}, err
	}
	if plan.Version == 0 {
		plan.Version = fixPlanVersion
	}
	if plan.Version != fixPlanVersion {
		return fixPlan{}, fmt.Errorf("修复计划的版本 %d 不受支持（当前 %d）", plan.Version, fixPlanVersion)
	}
	if plan.Root == "" {
		plan.Root = root
	}
	if plan.Root != root {
		return fixPlan{}, fmt.Errorf("修复计划的 root=%q 与目标根章节 %q 不一致", plan.Root, root)
	}
	if plan.Created == "" {
		plan.Created = time.Now().Format(time.RFC3339)
	}
	if len(plan.Children) == 0 {
		return fixPlan{}, fmt.Errorf("修复计划没有任何子章节")
	}
	seenChild := make(map[string]bool, len(plan.Children))
	seenIssue := make(map[string]string, 8)
	for i := range plan.Children {
		child := strings.TrimSpace(plan.Children[i].Child)
		if err := validateFixChild(child); err != nil {
			return fixPlan{}, err
		}
		if seenChild[child] {
			return fixPlan{}, fmt.Errorf("修复计划里 %s 出现了两次（每个子章节只能有一条）", child)
		}
		seenChild[child] = true
		plan.Children[i].Child = child
		if len(plan.Children[i].Issues) == 0 {
			return fixPlan{}, fmt.Errorf("修复计划里 %s 没有任何 issue", child)
		}
		for j := range plan.Children[i].Issues {
			it := &plan.Children[i].Issues[j]
			it.ID = strings.TrimSpace(it.ID)
			it.What = strings.TrimSpace(it.What)
			it.Rule = strings.TrimSpace(it.Rule)
			it.Where = strings.TrimSpace(it.Where)
			switch {
			case it.ID == "":
				return fixPlan{}, fmt.Errorf("修复计划里 %s 的第 %d 条 issue 没有 id", child, j+1)
			case len([]rune(it.ID)) > fixIssueIDMax:
				return fixPlan{}, fmt.Errorf("修复计划里 %s 的 issue id 太长（%d 字符）", child, len([]rune(it.ID)))
			case it.What == "":
				return fixPlan{}, fmt.Errorf("修复计划里 issue %s 没有 what", it.ID)
			case it.Where == "":
				return fixPlan{}, fmt.Errorf("修复计划里 issue %s 没有 where", it.ID)
			case it.Rule == "":
				return fixPlan{}, fmt.Errorf("修复计划里 issue %s 没有 rule", it.ID)
			}
			// id 是父会话回执与日志里点名的那一个，重复会让"这条改没改"
			// 无法回答。
			if prev, dup := seenIssue[it.ID]; dup {
				return fixPlan{}, fmt.Errorf("issue id %q 重复（已出现在 %s）", it.ID, prev)
			}
			seenIssue[it.ID] = child
		}
	}
	return plan, nil
}

// WriteFixPlan 原子写出一份修复计划并返回计划文件路径。
//
// 原子性照抄项目里其它落盘（同目录临时文件 + rename）：父会话可能一边写一边
// 有别的读端（预览页/下一次运行），半个 JSON 比没有文件更糟。
func WriteFixPlan(proj, root string, plan fixPlan) (string, error) {
	plan, err := normalizeFixPlan(proj, root, plan)
	if err != nil {
		return "", err
	}
	dir := fixPlanDir(proj, root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建修复计划目录失败: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	path := fixPlanPath(proj, root)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("写入修复计划失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("落位修复计划失败: %w", err)
	}
	return path, nil
}

// ReadFixPlan 读回一份修复计划（版本与字段校验与写同一套）。
func ReadFixPlan(proj, root string) (fixPlan, error) {
	data, err := os.ReadFile(fixPlanPath(proj, root))
	if err != nil {
		return fixPlan{}, fmt.Errorf("读取修复计划失败: %w", err)
	}
	var plan fixPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return fixPlan{}, fmt.Errorf("解析修复计划失败: %w", err)
	}
	return normalizeFixPlan(proj, root, plan)
}

// DispatchFixTool 是父（章节转换）会话能看到的唯一入口：`dispatch_fix`。
//
// 它**只记录计划**——把「哪个子章节、按哪条规则改」写成
// `work/fix/<根章>/plan.json` 并回执计划路径与将要下派的子章节数，绝不在这里
// 起会话（父会话还在跑，同步执行会把它的上下文与并发槽一起占住）。
type DispatchFixTool struct {
	// Proj 是项目根（计划落在 <proj>/work/fix/<Root>/）。
	Proj string
	// Root 是这次下派的根章节（父会话自己的章节名，如 chapter_014）。
	Root string
	// Work 是父会话自己的工作区；它的 `_subdivision/` 是子章节名的唯一真源。
	// 为空时按 <proj>/work/temp/conv_<Root>* 去找（见 findSubdivisionDir）。
	Work string

	// plannedPath 是本次会话写下计划后得到的路径（空 = 还没下派）。调度只认
	// 它：磁盘上可能还躺着上一次运行留下的 plan.json，那不是"这次刚下派的"，
	// 不能拿来执行（同一份计划被重复执行会把已经修过的子章节再修一遍）。
	plannedPath string
}

func (t *DispatchFixTool) Name() string { return "dispatch_fix" }

// PlannedPath 返回本次会话刚写下的计划文件路径（空 = 没有下派任何东西）。
func (t *DispatchFixTool) PlannedPath() string { return t.plannedPath }

func (t *DispatchFixTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "dispatch_fix",
		"description": "Record a fix plan for the child chapters you already subdivided (NOT a submission, and nothing runs here). " +
			"Give each problem an id, the child that owns it (child_01, child_02, … as listed in children-manifest.md), " +
			"where it is, what is wrong and the rule that must be satisfied. The runner schedules one fix session per child that has issues; " +
			"the receipt tells you the plan file and how many children will be scheduled. Keep converting/integrating yourself — this only hands work down.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"issues": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":    map[string]any{"type": "string", "description": "stable id or slug of this problem (used to report back what got fixed)"},
					"child": map[string]any{"type": "string", "description": "child chapter directory, e.g. child_02"},
					"where": map[string]any{"type": "string", "description": "file/line and, when known, the original page"},
					"what":  map[string]any{"type": "string", "description": "what is wrong"},
					"rule":  map[string]any{"type": "string", "description": "the requirement the fix must satisfy (this is what the checker judges)"},
				},
				"required": []string{"id", "child", "where", "what", "rule"},
			}},
			"note": map[string]any{"type": "string", "description": "optional note to the child sessions (shared context, e.g. which rules must hold)"},
		}, "required": []string{"issues"}},
	}}
}

// dispatchFixArgs 是 dispatch_fix 的入参。
type dispatchFixArgs struct {
	Issues []struct {
		ID    string `json:"id"`
		Child string `json:"child"`
		Where string `json:"where"`
		What  string `json:"what"`
		Rule  string `json:"rule"`
	} `json:"issues"`
	Note string `json:"note"`
}

// Execute 校验参数、按子章节分组、写计划并回执。校验失败**不写任何文件**：
// 一份半对的计划会让下派凭空少掉几条任务。
func (t *DispatchFixTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args dispatchFixArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return session.ToolResult{}, fmt.Errorf("dispatch_fix 参数解析失败: %w", err)
	}
	if err := validateFixRoot(t.Root); err != nil {
		return session.ToolResult{}, err
	}
	if t.Proj == "" {
		return session.ToolResult{}, fmt.Errorf("dispatch_fix 缺少项目目录")
	}
	if len(args.Issues) == 0 {
		return session.ToolResult{Text: "REJECTED: issues is empty — dispatch_fix needs at least one problem to hand down."}, nil
	}
	subdiv := findSubdivisionDir(t.Proj, t.Root, t.Work)
	if subdiv == "" {
		return session.ToolResult{Text: "REJECTED: this chapter has not been subdivided yet (no _subdivision/ with child_NN directories) — convert and check the children first, or convert the parent directly."}, nil
	}

	var order []string
	byChild := map[string][]fixPlanIssue{}
	seenID := map[string]string{}
	for i, in := range args.Issues {
		child := strings.TrimSpace(in.Child)
		if err := validateFixChild(child); err != nil {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: issue %d: %s", i+1, err.Error())}, nil
		}
		info, err := os.Stat(filepath.Join(subdiv, child))
		if err != nil || !info.IsDir() {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: issue %d: %s does not exist under _subdivision/ — child names come from children-manifest.md", i+1, child)}, nil
		}
		for _, f := range []struct{ name, val string }{
			{"id", in.ID}, {"where", in.Where}, {"what", in.What}, {"rule", in.Rule},
		} {
			if strings.TrimSpace(f.val) == "" {
				return session.ToolResult{Text: fmt.Sprintf("REJECTED: issue %d: %s is empty (id, where, what and rule are all required)", i+1, f.name)}, nil
			}
		}
		// id 是回执与报告里点名的那一个，重复会让"这条改没改"无法回答——
		// 当场退回让模型改，而不是写一份自己都自相矛盾的计划。
		if prev, dup := seenID[strings.TrimSpace(in.ID)]; dup {
			return session.ToolResult{Text: fmt.Sprintf("REJECTED: issue %d: id %q is already used by %s — every issue needs its own id", i+1, strings.TrimSpace(in.ID), prev)}, nil
		}
		seenID[strings.TrimSpace(in.ID)] = child
		if _, seen := byChild[child]; !seen {
			order = append(order, child)
		}
		byChild[child] = append(byChild[child], fixPlanIssue{
			ID: in.ID, What: in.What, Rule: in.Rule, Where: in.Where,
		})
	}
	sort.Strings(order)

	plan := fixPlan{Version: fixPlanVersion, Root: t.Root}
	for _, child := range order {
		plan.Children = append(plan.Children, fixPlanChild{Child: child, Issues: byChild[child]})
	}
	path, err := WriteFixPlan(t.Proj, t.Root, plan)
	if err != nil {
		return session.ToolResult{}, err
	}
	t.plannedPath = path

	parts := make([]string, 0, len(plan.Children))
	total := 0
	for _, c := range plan.Children {
		parts = append(parts, fmt.Sprintf("%s (%d)", c.Child, len(c.Issues)))
		total += len(c.Issues)
	}
	receipt := fmt.Sprintf("FIX PLAN RECORDED: %s — %d issue(s) for %d child chapter(s): %s.\n"+
		"Nothing has run yet: the runner schedules one fix session per child (its own workspace, its own rounds, its own checker), then you only integrate.",
		path, total, len(plan.Children), strings.Join(parts, ", "))
	if note := strings.TrimSpace(args.Note); note != "" {
		receipt += "\nNote carried to the child sessions: " + note
	}
	return session.ToolResult{Text: receipt}, nil
}

// findSubdivisionDir 定位父会话的 `_subdivision` 目录：优先用调用方给的工作区
// （运行期恒可用），否则在 <proj>/work/temp/conv_<根章>* 下找最近改动的那一棵
// （与 retry.go 同一套落点口径：细分树的根永远是某个转换会话的 work/_subdivision）。
func findSubdivisionDir(proj, root, work string) string {
	if work != "" {
		dir := filepath.Join(work, "_subdivision")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if proj == "" || root == "" {
		return ""
	}
	candidates, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+root+"*", "work", "_subdivision"))
	best, bestMod := "", time.Time{}
	for _, dir := range candidates {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = dir, info.ModTime()
		}
	}
	return best
}

// fixTarget 是一条修复会话在磁盘上的落点：它全都在父章节工作区的细分树里。
type fixTarget struct {
	Child    string // child_02
	Base     string // <父章>_child_02（细分树的章节 ID）
	NodeRoot string // <parentWork>/_subdivision/child_02/node
	InputMD  string // <parentWork>/_subdivision/child_02/input/chapter.md
	Depth    int    // 递归深度（子章节恒 >= 1）
}

// fixRoundsUsed 数这个子章节已经用掉几轮修复：转录里每条 `fix_target` 锚点
// 就是一轮起过的修复会话。读不到文件（第一轮）算 0。
func fixRoundsUsed(transcriptPath string) int {
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), `"kind":"fix_target"`)
}

/*
 * 第四步：整章重开的预算收口（P60）。
 *
 * 老行为：整合完的父章节被核对打回，就归档这一次尝试、换全新会话把整章（连同
 * 它下面所有子章节）从头再跑一遍。这在一个子章节已经把 `latex.fix.max_rounds`
 * 轮修复都用光之后是纯浪费——修复都到上限了，再整章重来一遍还是同样的子章节
 * 内容、同样的预算，只是把一轮很贵的对话重放一次。
 *
 * 所以：修复打开、且某个子章节的修复转录里已经攒满轮数时，父章节不再整章重开，
 * 保留这一版整合产物继续往上走（未解决的问题本来就走 issues 库与报告向上传递，
 * 不靠重开一次来"记住"），只记一行警告。开关关着、或没有任何子章节进过修复
 * 路径时返回空，行为与引入本步之前一字不差。
 *
 * 返回用光子章节的目录名（`child_02`，空 = 没有用光）与它用掉的轮数。
 */
func (r *Runner) fixBudgetExhausted(work, parentBase string, children []ChapterSubdivision) (string, int) {
	if !r.cfg.Latex.FixEnabled() || len(children) == 0 {
		return "", 0
	}
	budget := r.cfg.Latex.FixMaxRounds()
	for i := range children {
		id := fmt.Sprintf("child_%02d", i+1)
		tr := filepath.Join(work, "_subdivision", id, "node", "sessions", "fix_"+parentBase+"_"+id+".jsonl")
		if used := fixRoundsUsed(tr); used >= budget {
			return id, used
		}
	}
	return "", 0
}

// fixSessionModel 是修复会话用的模型条目：`latex.fix.model` 为空时继承
// convert（用户口径：默认不额外配模型）。
func (r *Runner) fixSessionModel() string {
	if r.cfg.Latex.Fix.Model != "" {
		return r.cfg.Latex.Fix.Model
	}
	return r.cfg.Latex.ConvertModel
}

// runChildFix 跑一个子章节的修复会话。
//
// 形态（P60 第一步已定，第二步接进调度）：
//   - 工作区：子章节**自己**那棵树（`<node>/work`，转换时建好、递归节点收尾
//     不删），不重建、不重新划分；
//   - 工具集：转换会话减去 `request_chapter_split`（已经划好的不再划），加上
//     `submit`；
//   - 转录：`<node>/sessions/fix_<子章>.jsonl`，每轮写一条
//     `t=meta kind=fix_target {"convert":"convert_<子章>[.tryN]","issue_ids":[...]}`；
//   - 提交后：跑该子章节**原本的核对会话**（只读视图与锚点都不变，P59）。
//
// 预算用完、工作区不在、未启用都直接报错——调用方（父章节）按老行为重试。
func (r *Runner) runChildFix(proj, clsName string, t fixTarget, issues []fixPlanIssue, tid int) error {
	if !r.cfg.Latex.FixEnabled() {
		return fmt.Errorf("修复会话未启用（latex.fix.enabled=false）")
	}
	base := strings.TrimSpace(t.Base)
	if base == "" {
		return fmt.Errorf("修复会话缺少子章节名")
	}
	if t.Depth < 1 {
		return fmt.Errorf("修复会话只作用于递归子章节（%s 的深度为 %d）", base, t.Depth)
	}
	if len(issues) == 0 {
		return fmt.Errorf("修复会话没有任务（%s）", base)
	}
	if t.NodeRoot == "" {
		return fmt.Errorf("修复会话缺少子章节工作目录（%s）", base)
	}
	work := filepath.Join(t.NodeRoot, "work")
	if !fileExists(work) {
		return fmt.Errorf("子章节工作区不存在（%s）：修复会话只复用已划好的工作区，不重建", work)
	}
	chapPath := t.InputMD
	if chapPath == "" || !fileExists(chapPath) {
		return fmt.Errorf("子章节 Markdown 不存在（%s）", chapPath)
	}
	budget := r.cfg.Latex.FixMaxRounds()
	trPath := filepath.Join(t.NodeRoot, "sessions", "fix_"+base+".jsonl")
	if used := fixRoundsUsed(trPath); used >= budget {
		return fmt.Errorf("%s 的修复轮数已用完（%d/%d）：退回父章节重试", base, used, budget)
	}

	submitRoot := filepath.Join(t.NodeRoot, "result")
	reportPath := filepath.Join(t.NodeRoot, "reports", "conversion.md")
	texPath := filepath.Join(submitRoot, base+".tex")
	partsPath := filepath.Join(submitRoot, base)
	// 锚点：这份产物是哪一次转换交出来的。修复会话与它之后的核对都写同一个
	// 锚点，预览页据此把它们挂到同一次转换下面（P59）。
	convertAttempt := lastAttemptNumber(filepath.Join(t.NodeRoot, "sessions", "convert_"+base+".jsonl"))

	goal := NewGoalTool(filepath.Join(work, "plan.md"))
	gate := r.submitGate(work, base, chapPath)
	submit := &SubmitChapterTool{
		GateRef:    gate,
		WorkRoot:   work,
		SubmitRoot: submitRoot,
		Base:       base,
		ReportPath: reportPath,
		Depth:      t.Depth,
		IssuesPath: filepath.Join(work, ".issues", "issues.json"),
		Gate: func() string {
			if msg := goalGate(goal); msg != "" {
				return msg
			}
			// 章节规模那一条对修复会话**不适用**：这一段本来就是父章节按
			// 语义拆下来的，而且修复会话没有 request_chapter_split（不能、
			// 也不该再拆一次）。传 requested=true 让这条门禁放行。
			if msg := chapterSizeReject(0, r.cfg.Latex.MaxChapterLines, t.Depth, true); msg != "" {
				return msg
			}
			if gate == nil {
				return ""
			}
			return gate.check()
		},
	}
	mounts := r.sessionMounts(kindConvert, work)
	if inputDir := filepath.Dir(chapPath); inputDir != "" {
		mounts = append(mounts, Mount{Name: "input", Dir: inputDir})
	}
	bashTmp, cleanBashTmp := r.sessionBashTemp(proj, "bash_fix_"+base)
	defer cleanBashTmp()
	tools := []session.Tool{
		&ReadFileTool{Mounts: mounts},
		&WriteWorkFileTool{Root: work, AnyExt: true, Hint: "Your chapter is " + base + ".tex plus the folder " + base +
			"/ for its \\input parts. Fix only what the issues name; everything else is already accepted."},
		&EditWorkFileTool{Root: work},
		&GrepTool{Mounts: mounts},
		&ViewImageTool{Root: filepath.Join(proj, "source"), Subject: "images", BareSearch: true, SoftMax: r.cfg.ViewImageMax(), WarnRatio: r.cfg.ViewWarnRatio()},
		&ViewPDFTool{Mounts: mounts, Comp: r.comp, SoftMax: r.cfg.ViewPDFMax(), WarnRatio: r.cfg.ViewWarnRatio(), Gate: gate},
		&CompileChapterTool{Comp: r.comp, Dir: work, MainFile: base + ".tex", WrapperFile: base + "_wrapper.tex", Gate: gate, Log: r.log, Tid: tid},
		&WorkBashTool{Mounts: mounts, Root: work, TmpDir: bashTmp,
			MaxOutput: r.cfg.BashMaxOutput(), Sandbox: r.cfg.BashSandboxEnabled(), Python: r.pythonEnv(proj), Log: r.log, Tid: tid},
		goal,
		submit,
	}
	// 原书只读工具（片段→原 PDF 页定位、原书页面索引）：修复会话同样要能
	// 回到原页复核——"这条 issue 是不是真的"只能靠原页回答。
	tools = append(tools, r.sourcePageTools()...)

	modelName := r.fixSessionModel()
	client := r.clientFor(modelName)
	modelCfg := r.modelOf(modelName)
	// 会话调优沿用 convert：修复会话和转换会话干的是同一类活（改 .tex/编译）。
	tuning := r.cfg.LatexSession("convert")
	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.ChildFixSystem), tuning, r.outputLang()),
		tools, r.log, tid, "fix:"+base)

	var resumeMsgs []session.ChatMessage
	var seed liveSeed
	if msgs, stats, err := session.LoadTranscriptStats(trPath); err == nil && len(msgs) > 0 {
		// 第二轮起接在同一条转录上继续：上一轮的上下文（它看过什么、改过
		// 什么）比重新解释一遍便宜也准。
		sess.SetMessages(msgs)
		sess.SeedCounters(stats.Rounds, stats.Tools)
		resumeMsgs = msgs
		seed = liveSeed{rounds: stats.Rounds, tools: stats.Tools, active: stats.Active, resumed: true}
		resumeLog(r.log, tid, "fix", base, msgs, stats)
	}
	if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}
	fixHook, fixState, fixClose := r.livePhaseRowState("fix:"+base, "fix:"+base, seed)
	sess.SetProgressHook(fixHook)
	defer fixClose()

	// 锚点（P60）：修复对象是哪一次转换、这一轮改了哪几条 issue。每轮一条，
	// 轮数就从这里数（fixRoundsUsed），不另建账本文件。
	ids := make([]string, 0, len(issues))
	for _, it := range issues {
		ids = append(ids, it.ID)
	}
	_ = sess.AppendTranscriptNote("fix_target", map[string]any{
		"convert":         checkerAnchorName(base, convertAttempt),
		"convert_attempt": convertAttempt,
		"issue_ids":       ids,
	})

	var b strings.Builder
	b.WriteString("Repair " + base + ". It is already converted and checked; the problems below are the only things to change — keep everything else exactly as it is.\n")
	for _, it := range issues {
		b.WriteString("\n- id " + it.ID + "\n  where: " + it.Where + "\n  what: " + it.What + "\n  rule: " + it.Rule)
	}
	b.WriteString("\n\nFix them with minimal edits in your workspace, compile until clean, look at the changed pages and the original page, then submit the same two paths again.")
	initial := resumeUserText(resumeMsgs, b.String())

	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "queued"})
	// 修复会话干的是"改 .tex / 编译"这一类活，和转换会话抢同一批并发槽位：
	// 父章节等其他顶层章节在转换时，这里不能再凭空多开一路请求。
	release := r.acquireConvertSlot()
	fixState(rowStateRunning)
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
	_, runErr := sess.Run(session.RunOptions{UserText: initial})
	release()
	if runErr != nil {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
		return fmt.Errorf("修复会话失败: %w", runErr)
	}
	if !submit.Submitted {
		if !r.remindSubmit(sess, tid, "[fix]", base, "submit", func() bool { return submit.Submitted }) {
			_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
			return fmt.Errorf("修复会话未提交（已提醒）")
		}
	}
	if !fileExists(texPath) {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "error"})
		return fmt.Errorf("修复会话已提交但没有写出 .tex")
	}
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "checking_result"})

	// 交回该子章节原本的核对：同一只读视图、同一转换锚点（P59），等价于
	// "这一支再核一遍"，而不是新开一条核对链。
	//
	// 但**不能沿用原来那条核对转录**：那一轮的 submit 已经收账，续跑逻辑会
	// 直接重放旧结论、模型根本不会再核一遍（实测就是这么"秒过"的）。所以
	// 与手动重核同一个口径：新的一次核对写 `checker_<章>.tryN.jsonl`，锚点
	// 仍然指同一个转换尝试——预览页据此把它挂在同一次转换下面。
	hints := checkHints{Report: reportPath, Notes: submit.Notes}
	if gate != nil {
		if pdf, _, ok := gate.compiledPDF(); ok {
			hints.PDF = pdf
		}
	}
	checkerBase := filepath.Join(t.NodeRoot, "sessions", "checker_"+base+".jsonl")
	checkerTr := attemptTranscriptPath(checkerBase, lastAttemptNumber(checkerBase)+1)
	ok, detail := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints, tid,
		checkerTr, filepath.Join(t.NodeRoot, "checker-view"), true, convertAttempt)
	if !ok {
		_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "issues"})
		return fmt.Errorf("修复后核对仍未通过: %s", detail)
	}
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "pass"})
	_ = os.Remove(texPath + ".checker")
	r.log.Log(tid, "[fix]", base, "修复完成并通过核对")
	return nil
}

/*
 * 第二步：把 dispatch_fix 接进章节转换流程（P60）。
 *
 * 时间点在 convertChapterNode 的细分父节点分支里：子章节全部转换+核对完、
 * 父会话的整合对话之前。为什么必须在这里给父会话一次**额外的对话**：子章节
 * 只要有一个没通过自己的核对，convertChapterChildren 就直接报错、整章不会
 * 整合；所以走到这一步时"所有子章节都过了核对"，没有任何现成信号告诉父会话
 * 哪个孩子还有问题——只能它自己读过报告与产物之后才知道。
 */

// childReviewText 是整合前那次「复查子章节、把还不对的还回去」的用户提示。
// 它是一次性的运行指令（与整合指令同一类），不是常驻提示词。
const childReviewText = "The child chapters are converted and their checkers have run. Before integrating, look at them: read work:_subdivision/children-summary.md, then the report and the .tex/parts of any child that still looks wrong. " +
	"If a child's artifact really still has a problem (missing content, wrong structure, a figure/table that does not match the original page), call dispatch_fix ONCE with every such issue (id, child, where, what, rule) — it only records the plan; the runner runs that child's fix session and re-checks it, then you integrate. " +
	"If nothing needs handing back, do nothing. Do NOT integrate and do NOT submit in this turn: this turn only hands work down."

// convertSystemPrompt 是章节转换会话的系统提示词。
//
// P60 第二步的「把问题子章节还回去」那一段只在 latex.fix.enabled 打开时追加：
// dispatch_fix 也只有那时才注册给会话。关着开关的跑法拿到的提示词必须与引入
// 这段之前逐字节相同（有测试钉住）——否则"默认零行为"就不成立了。
func (r *Runner) convertSystemPrompt(tuning config.SessionTuning) string {
	prompt := renderPrompt(prompts.Must(prompts.ConvertSystem), tuning, r.outputLang())
	if !r.cfg.Latex.FixEnabled() {
		return prompt
	}
	return prompt + "\n\n" + prompts.Must(prompts.ConvertFixDispatch)
}

// reviewChildrenBeforeIntegration 给父会话一次「看子章节结果、按 issue 下派」的
// 对话，返回它写下的计划文件路径（空 = 没有下派任何东西）。
//
// 这一段只是改善手段：对话本身失败不是致命错，按今天的行为直接整合。
func (r *Runner) reviewChildrenBeforeIntegration(sess *session.Session, base string, dispatch *DispatchFixTool, setState func(string), tid int) string {
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "queued"})
	setState(rowStateQueued)
	release := r.acquireConvertSlot()
	setState(rowStateRunning)
	_, err := sess.Run(session.RunOptions{UserText: childReviewText})
	release()
	if err != nil {
		setState(rowStateRunning)
		r.log.LogWarning(tid, "[fix]", base, "复查子章节的对话失败（跳过修复，按原行为整合）:", err)
		return ""
	}
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
	setState(rowStateRunning)
	path := dispatch.PlannedPath()
	if path == "" {
		r.log.Log(tid, "[fix]", base, "没有子章节需要下派修复，直接整合")
	}
	return path
}

// applyChildFixPlan 按计划逐个子章节跑修复会话，然后把修过的产物刷新回父章节。
//
// 任何一步失败都只警告、不中止：今天的行为（父会话照旧整合并提交）必须还是
// 兜底——一整本书不能因为一轮修复失败而失败。
func (r *Runner) applyChildFixPlan(proj, clsName, base, work string, childDepth int, children []ChapterSubdivision, planPath string, tid int) {
	plan, err := ReadFixPlan(proj, base)
	if err != nil {
		r.log.LogWarning(tid, "[fix]", base, "修复计划读取失败（按原行为整合）:", err)
		return
	}
	r.log.Log(tid, "[fix]", base, "按 "+planPath+" 下派 "+strconv.Itoa(len(plan.Children))+" 个子章节的修复")
	var fixed []string
	for _, pc := range plan.Children {
		target := fixTarget{
			Child:    pc.Child,
			Base:     base + "_" + pc.Child,
			NodeRoot: filepath.Join(work, "_subdivision", pc.Child, "node"),
			InputMD:  filepath.Join(work, "_subdivision", pc.Child, "input", "chapter.md"),
			Depth:    childDepth,
		}
		// 顺序跑：计划通常只有一两个子章节，父会话只等这一小段；每一轮都会
		// 拿转换槽位，所以不会额外放大并发。
		if err := r.runChildFix(proj, clsName, target, pc.Issues, tid); err != nil {
			r.log.LogWarning(tid, "[fix]", base, "子章节 "+pc.Child+" 修复失败（按原行为继续整合）:", err)
		}
		// 失败也刷新：修复会话可能已经交了一份（只是核对没过），整合应该看到
		// 子章节在磁盘上的最新状态，而不是更旧的副本。
		fixed = append(fixed, pc.Child)
	}
	if err := refreshFixedChildren(work, base, fixed, children); err != nil {
		r.log.LogWarning(tid, "[fix]", base, "刷新子章节产物失败（整合会读到修复前的副本）:", err)
	}
}

// refreshFixedChildren 把修过的子章节重新收进父章节：`children/` 副本、
// 子章节汇报汇总与继承清单。整合对话读的就是这三样。
func refreshFixedChildren(work, parentBase string, fixed []string, children []ChapterSubdivision) error {
	parts := filepath.Join(work, parentBase, "children")
	for _, child := range fixed {
		childBase := parentBase + "_" + child
		src := filepath.Join(work, "_subdivision", child, "node", "result")
		if err := copyFile(filepath.Join(src, childBase+".tex"), filepath.Join(parts, childBase+".tex")); err != nil {
			return fmt.Errorf("复制 %s.tex 失败: %w", childBase, err)
		}
		// 分片整棵替换：修复删掉的文件不能留在旧副本里。
		if srcParts := filepath.Join(src, childBase); fileExists(srcParts) {
			dst := filepath.Join(parts, childBase)
			_ = os.RemoveAll(dst)
			if err := copyDir(srcParts, dst); err != nil {
				return fmt.Errorf("复制 %s/ 失败: %w", childBase, err)
			}
		}
	}
	if children == nil {
		return nil
	}
	// 继承清单重算：修复会话的 submit 可能已经把某条问题标成 resolved，不重算
	// 就会把它当成"还没解决"继续往上带。
	var inherited []ChapterIssue
	for i := range children {
		id := fmt.Sprintf("child_%02d", i+1)
		store := filepath.Join(work, "_subdivision", id, "node", "work", ".issues", "issues.json")
		inherited = append(inherited, openIssues(readIssueStore(store))...)
	}
	if err := writeIssueStore(filepath.Join(work, "_subdivision", "issues-inherited.json"), openIssues(inherited)); err != nil {
		return fmt.Errorf("重写继承问题清单失败: %w", err)
	}
	// manifest + summary 由报告原文重建：修过的子章节交的是新报告。
	return writeChapterChildrenManifest(work, parentBase, children)
}
