package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mineru-tools/internal/prompts"
	"mineru-tools/internal/session"
)

/*
 * P60：子章节「修复会话」的骨架。
 *
 * 背景（用户口径）：父章节重试代价太大。子会话都已跑过之后，应当能**按修复
 * 任务重新下派**到已经划好的章节与工作区上，用新会话继续改，父会话只做整合；
 * 父章节反复失败到与转换重试同一上限时才整章重开。
 *
 * 这一步（P60 第一步）只落**接口**，不接进运行路径：
 *
 *   - 契约：父会话在整理阶段用 `dispatch_fix` 工具把「哪个子章节、按哪条规则
 *     改」写成 `work/fix/<根章>/plan.json`；工具**只记录计划**、不同步执行。
 *   - 执行：`runChildFix` 已按最终形态写好（复用子章节自己的工作区、工具集 =
 *     转换会话减去 request_chapter_split、提交后跑该子章节原本的核对、锚点
 *     不变），但只有测试会调用它——第一道守卫就是 `latex.fix.enabled`，默认关闭。
 *   - 预算：`latex.fix.max_rounds`（默认 2）；用完就退回老行为（父会话重试）。
 *
 * 为什么不在这批里接进 book.go：接进去就改变了现有跑法的结果，而这一步要的
 * 是"接口先定死、骨架先编译通过、默认零行为"。真正的调度（父会话何时下派、
 * 修完后父会话怎么整合）是 P60 的下一步。
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
}

func (t *DispatchFixTool) Name() string { return "dispatch_fix" }

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
// 形态（P60 第一步已定，调度留给下一步）：
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
	fixState(rowStateRunning)
	_ = sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": "running"})
	_, runErr := sess.Run(session.RunOptions{UserText: initial})
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
	hints := checkHints{Report: reportPath, Notes: submit.Notes}
	if gate != nil {
		if pdf, _, ok := gate.compiledPDF(); ok {
			hints.PDF = pdf
		}
	}
	checkerTr := attemptTranscriptPath(filepath.Join(t.NodeRoot, "sessions", "checker_"+base+".jsonl"), convertAttempt)
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
