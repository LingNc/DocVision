package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
	"mineru-tools/internal/retryq"
	"mineru-tools/internal/session"
)

// P58：会话"跑完但没 submit"时的复盘。
//
// 今天的兜底是在**已经退化的同一会话里**再喊一句"你还没提交"：对"网络断在一
// 半、这一轮判断错、工具参数写坏"有效，对"模型把工具调用写成正文（XML / 裸
// JSON / `\submit{…}`）、无故重复前文、直接说干不了"完全无效——只会再退化一次。
// 所以先起一条**只读的复盘会话**（角色 rescue）看目标会话的转录，给出结论：
//
//	retry    —— 会话还健康，照旧发作过的提醒（可带一句 hint）
//	truncate —— 已退化，截断到最后一个干净轮次再继续（from = 那条消息的行号）
//	fail     —— 救不回来，按失败收尾
//
// 复盘会话没有任何可写挂载：它看不到工作区、也不能提交产物，最坏后果是一次
// 截断（源转录与归档尝试都留着）。

type rescueVerdict struct {
	Verdict string `json:"verdict"`
	From    int    `json:"from"`
	Reason  string `json:"reason"`
	Hint    string `json:"hint"`
}

// rescueMaxReviewAttempts 是一次复盘最多起几条复盘会话（P61/B）：1 次正常 +
// 2 次重来。复盘会话**自身**失败（报错终止或跑完也没有 submit）不算"这条目标
// 没救了"——它只是一次没拿到结论的模型调用，换一条新会话再来即可；旧转录丢进
// `work/sessions/.trash/`（复盘会话是可丢弃的）。
const rescueMaxReviewAttempts = 3

// rescueActivityState 是"复盘正在处理这条目标会话"的活动标记（P61/C）。
// 它写进**目标**转录（不是复盘自己的），预览页据此把那条已经报错/没提交的
// 会话显示成「复盘处理中」，而不是一直挂着上一次的状态。
const rescueActivityState = "rescuing"

// rescueTarget 说明"复盘哪条会话"。
type rescueTarget struct {
	ProjDir string // 项目根目录
	Base    string // 章节 ID
	Role    string // convert | checker
	TrPath  string // 目标会话的转录（绝对路径）
	Label   string // 日志标签
	// Sess 是目标会话本体（可空）。复盘要把「复盘处理中」「等待并发槽」这类
	// 状态写回目标转录；有会话时走它自己的 transcript writer——那条 writer 记着
	// 转录行数（P55 的工作区快照按行号索引），另开一个句柄追加一行会让行号错位。
	Sess *session.Session
	// Cause 非空表示这条会话是**报错终止**的（Run 返回了错误），调用方已经
	// 排除致命错误（余额不足/无可用资源包）。它不影响会话、工具与结论处理，
	// 只让复盘者的第一句话看到"停下来的原因是报错"而不是"模型自己收尾了"——
	// 这正是 `retry` 那一档（外部瞬时故障）该被认出来的场合。
	Cause string
	// NoToolNudge 为真表示会话里已经用过一次「你的工具确实可用」的纠正，之后
	// 仍然没有提交——"工具不可用"是幻觉，这是退化的强信号。同样只进复盘者的
	// 第一段话，结论处理一字不变。
	NoToolNudge bool
}

// transcriptLine 是复盘工具看到的一行（物理行号，1 起）。
type transcriptLine struct {
	Line int
	Kind string // meta | msg | usage
	Role string
	TS   string
	// 摘要字段
	Finish   string
	Tools    []string
	Text     string
	Thinking int
}

// loadTranscriptLines 把转录逐物理行读出来；坏行保留占位（行号不能错位，
// 因为 truncate 的 from 就是行号）。
func loadTranscriptLines(path string) ([]transcriptLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.Split(string(data), "\n")
	out := make([]transcriptLine, 0, len(raw))
	for i, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tl := transcriptLine{Line: i + 1}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			tl.Kind = "unreadable"
			tl.Text = line
			out = append(out, tl)
			continue
		}
		tl.Kind, _ = obj["t"].(string)
		tl.Role, _ = obj["role"].(string)
		tl.TS, _ = obj["ts"].(string)
		tl.Finish, _ = obj["finish_reason"].(string)
		tl.Text, _ = obj["text"].(string)
		if rc, ok := obj["reasoning_content"].(string); ok {
			tl.Thinking = len(rc)
		}
		if calls, ok := obj["tool_calls"].([]any); ok {
			for _, c := range calls {
				cm, _ := c.(map[string]any)
				fn, _ := cm["function"].(map[string]any)
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				if name != "" {
					tl.Tools = append(tl.Tools, name+"("+clipOneLine(args, 240)+")")
				}
			}
		}
		out = append(out, tl)
	}
	return out, nil
}

func clipOneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ReadSessionTool 让复盘会话按物理行号翻看目标转录。默认给末尾若干条消息，
// 可以带 from 往前翻；max_chars=0 看全文（配合小的 count）。
type ReadSessionTool struct {
	Path   string
	Lines  []transcriptLine
	Total  int // 物理行数
	MaxRow int // 一次最多返回多少行（默认 40）
}

func (t *ReadSessionTool) Name() string { return "read_session" }

func (t *ReadSessionTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "read_session",
		"description": "Read the reviewed session's own transcript by PHYSICAL LINE NUMBER (the numbers used by submit.from). " +
			"No arguments: the last 12 messages. Pass from=<line> to walk backwards and count=<lines> to widen the window (max 40). " +
			"Each message is clipped to max_chars characters (default 2000); pass max_chars=0 with a small count to read ONE message in full. " +
			"Usage/meta lines are included; they carry the round numbers.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"from":      map[string]any{"type": "integer", "description": "1-based physical line to start at (default: the tail)."},
			"count":     map[string]any{"type": "integer", "description": "how many physical lines to return (1-40, default 12 messages)."},
			"max_chars": map[string]any{"type": "integer", "description": "per-message character clip; 0 = full text (default 2000)."},
		}},
	}}
}

func (t *ReadSessionTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args struct {
		From     int  `json:"from"`
		Count    int  `json:"count"`
		MaxChars *int `json:"max_chars"`
	}
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return session.ToolResult{}, fmt.Errorf("参数不是合法 JSON: %w", err)
		}
	}
	clip := 2000
	if args.MaxChars != nil {
		clip = *args.MaxChars
	}
	maxRow := t.MaxRow
	if maxRow <= 0 {
		maxRow = 40
	}
	count := args.Count
	if count <= 0 {
		count = 12
	}
	if count > maxRow {
		count = maxRow
	}
	from := args.From
	if from <= 0 {
		// 默认：末尾 count 条消息
		msgLines := make([]int, 0, len(t.Lines))
		for _, l := range t.Lines {
			if l.Kind == "msg" {
				msgLines = append(msgLines, l.Line)
			}
		}
		if len(msgLines) == 0 {
			return session.ToolResult{Text: "转录里没有任何消息行。"}, nil
		}
		start := len(msgLines) - count
		if start < 0 {
			start = 0
		}
		from = msgLines[start]
		// 从这条消息所在行开始，往后取 count 条**消息行**（外加它们之间的行）
		last := msgLines[len(msgLines)-1]
		return t.window(from, last, clip, count), nil
	}
	// 显式 from：取 from 起 count 行物理行
	endLine := from
	n := 0
	for _, l := range t.Lines {
		if l.Line < from {
			continue
		}
		endLine = l.Line
		n++
		if n >= count {
			break
		}
	}
	return t.render(from, endLine, clip), nil
}

func (t *ReadSessionTool) window(from, last, clip, msgCount int) session.ToolResult {
	// 从 from 开始按物理行输出，直到取满 msgCount 条消息行
	got := 0
	end := last
	for _, l := range t.Lines {
		if l.Line < from {
			continue
		}
		if l.Kind == "msg" {
			got++
			if got >= msgCount {
				end = l.Line
				break
			}
		}
		end = l.Line
	}
	return t.render(from, end, clip)
}

func (t *ReadSessionTool) render(from, to, clip int) session.ToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "transcript: %d physical lines, %d message lines (line numbers are the ones submit.from takes)\n", t.Total, countKind(t.Lines, "msg"))
	lastMsg := 0
	for _, l := range t.Lines {
		if l.Kind == "msg" {
			lastMsg = l.Line
		}
	}
	fmt.Fprintf(&b, "last message at line %d; showing lines %d-%d\n\n", lastMsg, from, to)
	shown := 0
	for _, l := range t.Lines {
		if l.Line < from || l.Line > to {
			continue
		}
		shown++
		switch l.Kind {
		case "usage":
			fmt.Fprintf(&b, "#%d usage finish=%s\n", l.Line, l.Finish)
			continue
		case "meta":
			fmt.Fprintf(&b, "#%d meta %s\n", l.Line, clipOneLine(l.Text, 200))
			continue
		}
		role := l.Role
		if role == "" {
			role = "?"
		}
		fmt.Fprintf(&b, "#%d %s", l.Line, role)
		if len(l.Tools) > 0 {
			fmt.Fprintf(&b, " tools=[%s]", strings.Join(l.Tools, " | "))
		}
		if l.Thinking > 0 {
			fmt.Fprintf(&b, " [thinking %d chars]", l.Thinking)
		}
		b.WriteString("\n")
		text := l.Text
		if clip > 0 && len(text) > clip {
			text = text[:clip] + fmt.Sprintf("\n… [clipped %d chars; call read_session with from=%d max_chars=0 to read it in full]", len(l.Text)-clip, l.Line)
		}
		if strings.TrimSpace(text) != "" {
			for _, ln := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				fmt.Fprintf(&b, "  %s\n", ln)
			}
		}
	}
	out := b.String()
	if len(out) > 120000 {
		out = out[:120000] + "\n… [window too large; narrow it with from/count]\n"
	}
	if shown == 0 {
		out += "（这个范围内没有行；转录一共 " + fmt.Sprint(t.Total) + " 行）\n"
	}
	return session.ToolResult{Text: out}
}

func countKind(lines []transcriptLine, kind string) int {
	n := 0
	for _, l := range lines {
		if l.Kind == kind {
			n++
		}
	}
	return n
}

// SessionStatsTool 给出代码算好的事实（不做判断）：轮次、工具调用、每轮
// finish_reason、"工具调用被写成正文"的行号、最长重复块、每轮起点行号。
type SessionStatsTool struct {
	Lines []transcriptLine
	Total int
}

func (t *SessionStatsTool) Name() string { return "session_stats" }

func (t *SessionStatsTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": "session_stats",
		"description": "Facts about the reviewed transcript computed by code (no judgement): rounds and their finish_reason, tool calls/results, " +
			"line numbers where each round starts, assistant messages whose TEXT looks like a tool call written as prose (XML `<tool_call>`/`<parameter`, " +
			"bare JSON, `\\submit{…}`), and the most repeated 120-char block. Use it to find the last clean round quickly.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *SessionStatsTool) Execute(string) (session.ToolResult, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "physical lines: %d | messages: %d | tool results: %d\n", t.Total, countKind(t.Lines, "msg"), countToolResults(t.Lines))
	rounds := 0
	var starts []string
	for _, l := range t.Lines {
		if l.Kind == "usage" {
			rounds++
		}
	}
	fmt.Fprintf(&b, "usage lines (rounds): %d\n", rounds)
	// 每轮起点：assistant 带工具调用或 usage 之后的第一条 user 行
	for _, l := range t.Lines {
		if l.Kind == "msg" && l.Role == "user" {
			starts = append(starts, fmt.Sprintf("#%d", l.Line))
		}
	}
	if len(starts) > 0 {
		fmt.Fprintf(&b, "user-turn start lines: %s\n", strings.Join(starts, " "))
	}
	// 每轮 finish_reason
	var finishes []string
	for _, l := range t.Lines {
		if l.Kind == "usage" {
			finishes = append(finishes, l.Finish)
		}
	}
	if len(finishes) > 0 {
		fmt.Fprintf(&b, "finish_reason per round: %s\n", strings.Join(finishes, ", "))
	}
	// 正文里像工具调用
	var suspected []string
	for _, l := range t.Lines {
		if l.Kind != "msg" || l.Role != "assistant" {
			continue
		}
		if textLooksLikeToolCall(l.Text) {
			suspected = append(suspected, fmt.Sprintf("#%d", l.Line))
		}
	}
	if len(suspected) > 0 {
		fmt.Fprintf(&b, "assistant messages whose text looks like a tool call written as prose (heuristic): %s\n", strings.Join(suspected, " "))
	} else {
		b.WriteString("no assistant message looks like a prose tool call (heuristic)\n")
	}
	// 最长重复块
	if blk, n := mostRepeatedBlock(t.Lines, 120); n > 1 {
		fmt.Fprintf(&b, "most repeated %d-char block: x%d — %q\n", 120, n, clipOneLine(blk, 120))
	}
	return session.ToolResult{Text: b.String()}, nil
}

func countToolResults(lines []transcriptLine) int {
	n := 0
	for _, l := range lines {
		if l.Kind == "msg" && l.Role == "tool" {
			n++
		}
	}
	return n
}

func textLooksLikeToolCall(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return false
	}
	for _, pat := range []string{"<tool_call", "<parameter", "<function=", `"function":`, `\submit{`, `\submit {`} {
		if strings.Contains(s, pat) {
			return true
		}
	}
	// 整段只有一个 JSON 对象
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && len(s) < 8000 {
		var v map[string]any
		if json.Unmarshal([]byte(s), &v) == nil {
			return true
		}
	}
	return false
}

func mostRepeatedBlock(lines []transcriptLine, size int) (string, int) {
	counts := map[string]int{}
	for _, l := range lines {
		if l.Kind != "msg" {
			continue
		}
		for i := 0; i+size <= len(l.Text); i += size {
			counts[l.Text[i:i+size]]++
		}
	}
	best, bestN := "", 0
	for k, n := range counts {
		if n > bestN {
			best, bestN = k, n
		}
	}
	return best, bestN
}

// RescueSubmitTool 是复盘会话的结论出口。
type RescueSubmitTool struct {
	Submitted bool
	Verdict   string
	From      int
	Reason    string
	Hint      string
}

func (t *RescueSubmitTool) Name() string { return "submit" }

func (t *RescueSubmitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit",
		"description": "Submit the verdict for the reviewed session. retry = it is healthy, send one more reminder (hint says what to do); truncate = it degenerated, cut everything after `from` and start again there; fail = nothing salvageable.",
		"parameters": map[string]any{"type": "object", "properties": map[string]any{
			"verdict": map[string]any{"type": "string", "enum": []string{"retry", "truncate", "fail"}},
			"from":    map[string]any{"type": "integer", "description": "truncate only: 1-based physical line of the LAST GOOD message; everything after it is dropped."},
			"reason":  map[string]any{"type": "string", "description": "one or two sentences for the operator log"},
			"hint":    map[string]any{"type": "string", "description": "retry only: one sentence to append to the reminder"},
		}, "required": []string{"verdict", "reason"}},
	}}
}

func (t *RescueSubmitTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args struct {
		Verdict string `json:"verdict"`
		From    int    `json:"from"`
		Reason  string `json:"reason"`
		Hint    string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return session.ToolResult{}, fmt.Errorf("参数不是合法 JSON: %w", err)
	}
	v := strings.TrimSpace(strings.ToLower(args.Verdict))
	switch v {
	case "retry", "truncate", "fail":
	default:
		return session.ToolResult{Text: "REJECTED: verdict must be retry, truncate or fail"}, nil
	}
	if v == "truncate" && args.From < 1 {
		return session.ToolResult{Text: "REJECTED: truncate needs from = the line number of the last good message"}, nil
	}
	if strings.TrimSpace(args.Reason) == "" {
		return session.ToolResult{Text: "REJECTED: reason is required"}, nil
	}
	t.Submitted, t.Verdict, t.From, t.Reason, t.Hint = true, v, args.From, strings.TrimSpace(args.Reason), strings.TrimSpace(args.Hint)
	label := map[string]string{"retry": "REMINDER WILL BE SENT", "truncate": fmt.Sprintf("WILL TRUNCATE AT LINE %d AND CONTINUE", t.From), "fail": "THE RUNNER WILL STOP"}[v]
	return session.ToolResult{Text: "VERDICT ACCEPTED: " + label + ". Reason: " + t.Reason, Terminal: true}, nil
}

// rescueMu 保护 rescueSeen（同一章同一次尝试最多复盘 max_per_attempt 次）。
// rescuePathMu 保护"定转录名 + 建文件"这一段：同一个章节的转换与核对可能在各自
// 的槽位里同时复盘，两边都只看磁盘上已有的最大号就会拿到同一个名字。
var (
	rescueMu     sync.Mutex
	rescueSeen   = map[string]int{}
	rescuePathMu sync.Mutex
)

// rescueBudget 记一次复盘并判断是否还在预算内。
func (r *Runner) rescueBudget(key string) bool {
	rescueMu.Lock()
	defer rescueMu.Unlock()
	if rescueSeen[key] >= r.cfg.Latex.RescueMaxPerAttempt() {
		return false
	}
	rescueSeen[key]++
	return true
}

// rescueModelFor 解析复盘会话的模型（空 = checker，再空 = 转换/文本模型）。
func (r *Runner) rescueModelName() string {
	name := strings.TrimSpace(r.cfg.Latex.Rescue.Model)
	if name == "" {
		// P58：模板里预置了 models.rescue 条目（默认与 checker 同底座），
		// 用户想单独调就改它；没配这个条目时跟随 checker。
		if _, ok := r.cfg.Models["rescue"]; ok {
			name = "rescue"
		}
	}
	if name == "" {
		name = strings.TrimSpace(r.cfg.Latex.CheckerModel)
	}
	if name == "" {
		name = strings.TrimSpace(r.cfg.Latex.ConvertModel)
	}
	return name
}

// rescueReview 起只读复盘会话，返回它的结论。ok=false 表示没能拿到结论。
//
// P61/B：复盘会话自己也是会话——它可能报错终止，也可能跑完不调 submit（没有
// 结论）。那种情况下**不复盘它自己**（rescue-of-rescue 一律禁止），而是把这条
// 没结论的转录丢掉、重开一条新会话，最多 `rescueMaxReviewAttempts` 次：每次
// 记一行警告，全都没结论时再记一行"放弃"。放弃不是"目标会话没救"——调用方
// 退回引入复盘之前的路径（照常提醒，仍不提交就判失败）。
func (r *Runner) rescueReview(t rescueTarget, tid int) (rescueVerdict, bool) {
	lines, err := loadTranscriptLines(t.TrPath)
	if err != nil || len(lines) == 0 {
		r.log.LogWarning(tid, "[rescue]", t.Base, "读不到转录，跳过复盘:", err)
		return rescueVerdict{}, false
	}
	for attempt := 1; attempt <= rescueMaxReviewAttempts; attempt++ {
		// 转录名在**开跑之前**定下来：每一条复盘都用自己的一号（见
		// rescueTranscriptPath），拿不到结论时退掉的必须正是这一次那份。
		trPath := r.allocateRescueTranscript(t, tid)
		v, ok := r.rescueReviewOnce(t, lines, tid, trPath)
		if ok {
			return v, true
		}
		if attempt >= rescueMaxReviewAttempts {
			r.log.LogWarning(tid, "[rescue]", t.Base, fmt.Sprintf(
				"复盘会话连续 %d 次没有给出结论，放弃复盘：目标按引入复盘之前的行为收尾（照常提醒/判失败）", attempt))
			r.retireRescueTranscript(tid, t, trPath, "连续三次没有结论")
			return rescueVerdict{}, false
		}
		r.log.LogWarning(tid, "[rescue]", t.Base, fmt.Sprintf(
			"复盘会话第 %d 次没有给出结论：丢弃这条复盘转录，重开一条（第 %d/%d 次）",
			attempt, attempt+1, rescueMaxReviewAttempts))
		r.retireRescueTranscript(tid, t, trPath, "这一次没有结论")
	}
	return rescueVerdict{}, false
}

// rescueReviewOnce 起一条复盘会话并等它的结论（不做重来，见 rescueReview）。
// trPath 是这一次的复盘转录落点（已完成编号，见 rescueTranscriptPath）。
func (r *Runner) rescueReviewOnce(t rescueTarget, lines []transcriptLine, tid int, trPath string) (rescueVerdict, bool) {
	name := r.rescueModelName()
	client := r.clientFor(name)
	modelCfg := r.modelOf(name)
	// 会话参数全部来自 latex.sessions.rescue（没写的字段继承 sessions.checker：
	// 输出预算与 checker 同配置；上下文默认 128K、轮次默认 12 由
	// config.LatexSession("rescue") 给）。模型那边一并不动——限制设在这里。
	tuning := r.cfg.LatexSession("rescue")

	readTool := &ReadSessionTool{Path: t.TrPath, Lines: lines, Total: len(lines)}
	statsTool := &SessionStatsTool{Lines: lines, Total: len(lines)}
	submit := &RescueSubmitTool{}
	tools := []session.Tool{readTool, statsTool, submit}
	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.RescueSystem), tuning, r.outputLang()),
		tools, r.log, tid, "rescue:"+t.Base)
	if r.rescueKeepRecords(tid) {
		if tr, terr := session.NewTranscript(trPath); terr == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
			// 复盘**自己**的转录先写下"它复盘的是哪一条会话"（在会话开跑之前）：
			// ① 预览页靠它把老命名（`rescue_<章>_<时间戳>.jsonl`）的复盘补出章节
			// 身份（`rescue_<章>` 的形状能解析，剩下部分不是章节 ID 也没关系）；
			// ② 历史页签靠它说清「复盘了 转换#1」，同一章不同目标的复盘才能各成
			// 一条链。字段口径与 `checker_target` 的锚点一致：转录名不含扩展名。
			_ = sess.AppendTranscriptNote("rescue_target", map[string]any{
				"chapter":             t.Base,
				"reviewed_role":       t.Role,
				"reviewed_transcript": transcriptStemName(t.TrPath),
				"reviewed_attempt":    transcriptAttemptOf(t.TrPath),
			})
		}
	}
	// P58 扩展：三种触发（没提交 / 报错终止 / 纠正过"工具不可用"）共用同一条
	// 复盘会话、同一套工具与同一套结论处理；差别只在第一段话把"为什么停下"说准，
	// 否则复盘者会把外部瞬时故障当成模型自己收尾（反之亦然）。
	userText := fmt.Sprintf("The %s session for chapter %s ended its turn without calling submit. Its transcript has %d physical lines (%d message lines). Review it and submit one verdict.",
		t.Role, t.Base, len(lines), countKind(lines, "msg"))
	if t.Cause != "" {
		userText = fmt.Sprintf("The %s session for chapter %s ended with an error (%s) instead of a submit call. Its transcript has %d physical lines (%d message lines). Review it and submit one verdict.",
			t.Role, t.Base, clipOneLine(t.Cause, 400), len(lines), countKind(lines, "msg"))
	}
	if t.NoToolNudge {
		userText += " Note: this session claimed that its tools were not available; the runner already corrected that once (the tools are always sent with every request) and it still ended without submit."
	}
	// 复盘也是一次模型调用，占一个章节槽位（与提醒轮同一条规矩），免得在
	// 并发已满时凭空多开一条会话。
	release := r.acquireChapterSlot()
	_, runErr := sess.Run(session.RunOptions{UserText: userText})
	release()
	if runErr != nil {
		r.log.LogWarning(tid, "[rescue]", t.Base, "复盘会话失败:", runErr)
	}
	if !submit.Submitted {
		r.log.LogWarning(tid, "[rescue]", t.Base, "复盘会话没有给出结论")
		return rescueVerdict{}, false
	}
	v := rescueVerdict{Verdict: submit.Verdict, From: submit.From, Reason: submit.Reason, Hint: submit.Hint}
	r.rescueAudit(t, v, len(lines))
	r.log.LogInfo(tid, "[rescue]", t.Base, fmt.Sprintf("复盘结论 %s（from=%d）：%s", v.Verdict, v.From, v.Reason))
	return v, true
}

// markRescueQueued 记下"这条转录已被复盘排进重试队列"（P61/C）。
func (r *Runner) markRescueQueued(trPath string) {
	if trPath == "" {
		return
	}
	r.rescueQueuedMu.Lock()
	defer r.rescueQueuedMu.Unlock()
	if r.rescueQueued == nil {
		r.rescueQueued = map[string]bool{}
	}
	r.rescueQueued[trPath] = true
}

// clearRescueQueued 丢掉这条转录的"已排队"标记（下次复盘开始前清一次，
// 上一轮的标记不会漏到这一轮）。
func (r *Runner) clearRescueQueued(trPath string) {
	if trPath == "" {
		return
	}
	r.rescueQueuedMu.Lock()
	defer r.rescueQueuedMu.Unlock()
	delete(r.rescueQueued, trPath)
}

// rescueQueuedFor 问"这条转录是不是刚被复盘排进队列了"（P61/C）：是的话调用方
// 的失败终态要写成 queued 而不是 error——它马上会被重跑，界面不该停在"会话错误"。
func (r *Runner) rescueQueuedFor(trPath string) bool {
	if trPath == "" {
		return false
	}
	r.rescueQueuedMu.Lock()
	defer r.rescueQueuedMu.Unlock()
	return r.rescueQueued[trPath]
}

// isRescueTranscript 判断一条转录是不是复盘会话自己的（P61/B）：`rescue_` 前缀。
func isRescueTranscript(path string) bool {
	return strings.HasPrefix(filepath.Base(path), "rescue_")
}

// retireRescueTranscript 退掉一份**没拿到结论**的复盘转录（P61/B）：挪进
// `work/sessions/.trash/`，挪不动就直接删。只有"这一次没有结论"会走这里——
// 有结论的复盘（哪怕后来又被复盘一次）是真实发生过的记录，一条都不删，
// 因为它已经写在自己的 `.tryN` 名字上（见 rescueTranscriptPath）。
func (r *Runner) retireRescueTranscript(tid int, t rescueTarget, trPath, why string) {
	if trPath == "" || !fileExists(trPath) {
		return // latex.rescue.keep_records 关闭时根本没有落盘
	}
	trash := filepath.Join(t.ProjDir, "work", "sessions", ".trash")
	if err := os.MkdirAll(trash, 0o755); err == nil {
		stem := strings.TrimSuffix(filepath.Base(trPath), ".jsonl")
		stamp := time.Now().Format("20060102-150405")
		dst := filepath.Join(trash, fmt.Sprintf("%s.%s.jsonl", stem, stamp))
		// 一秒内丢两次会撞名（重试循环很快）；撞名时加序号，别把上一份覆盖掉。
		for n := 2; fileExists(dst); n++ {
			dst = filepath.Join(trash, fmt.Sprintf("%s.%s-%d.jsonl", stem, stamp, n))
		}
		if movePath(trPath, dst) == nil {
			r.log.LogInfo(tid, "[rescue]", t.Base, "复盘转录（"+why+"）已挪进 work/sessions/.trash/:", filepath.Base(dst))
			return
		}
	}
	if err := os.Remove(trPath); err != nil {
		r.log.LogWarning(tid, "[rescue]", t.Base, "复盘转录（"+why+"）删除失败:", err)
	}
}

// appendTranscriptActivity 往任意转录末尾追加一条 chapter_activity 标记（P61/C）。
// 只有在拿不到目标会话本体（rescueTarget.Sess == nil）时才走这里：没有 writer
// 可以借用，只能自己开一个追加句柄。文件不存在时什么都不做——不凭空造一条转录。
func appendTranscriptActivity(trPath, state string) error {
	if trPath == "" {
		return nil
	}
	if !fileExists(trPath) {
		return nil
	}
	f, err := os.OpenFile(trPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(map[string]any{
		"t": "meta", "kind": "chapter_activity", "state": state,
		"ts": time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// markTargetActivity 把**目标**会话的活动状态写成 state，并把这一步记进日志
// （P61/C：状态的写入时机与"什么时候排队/提醒"的先后要能在日志里对上）。
func (r *Runner) markTargetActivity(tid int, tag, label string, t rescueTarget, state, why string) {
	var err error
	if t.Sess != nil {
		err = t.Sess.AppendTranscriptNote("chapter_activity", map[string]any{"state": state})
	} else {
		err = appendTranscriptActivity(t.TrPath, state)
	}
	if err != nil {
		r.log.LogWarning(tid, tag, label, "写目标会话状态 "+state+" 失败（界面会停在旧状态）:", err)
		return
	}
	r.log.LogInfo(tid, tag, label, "目标会话状态 → "+state+"（"+why+"）")
}

// rescueKeepRecords：默认不存复盘转录，--debug（logger 的 debug 级别）或
// latex.rescue.keep_records: true 才存。日志级别是 Info=0 / Debug=1 / Trace=2，
// 所以判定用 logger.LevelDebug —— 早先写成"≥4"等于永不落盘，用户在 --debug 下
// 也看不到复盘会话（P58 实测踩到，已修）。
func (r *Runner) rescueKeepRecords(tid int) bool {
	if r.cfg.Latex.RescueKeepRecords() {
		return true
	}
	return r.log != nil && r.log.Level() >= logger.LevelDebug
}

// rescueTranscriptPath 是复盘会话的转录落点（P61/E，P58 补改编号）：
//
//	work/sessions/rescue_<章>[.tryN].jsonl
//
// 名字与转换/核对同一种口径（`<角色>_<章>[.tryN]`），所以预览页用同一套规则就能
// 推出章节身份（`chapterRole: "rescue"`，章节 ID/父章/深度与 `checker_` 完全相同），
// 复盘会话于是能挂在它复盘的章节下面显示。
//
// `.tryN` 是**这条目标被复盘的第几次**（与 convert 的尝试号同一个推导方式：磁盘上
// 已有的最大号 + 1，见 nextAttemptNumber）。它以前是"目标转录的尝试号"，于是同一
// 目标第二次复盘会复用同一个文件名、把上一份（已经收尾的）转录退掉——那是一段
// 真实发生过的复盘记录，不该被后来的那次覆盖。现在每次复盘都拿自己的一号，
// 已完成的转录一条都不删；只有**没拿到结论**的那一次才会被退进
// `work/sessions/.trash/`（见 rescueReview）。
func (r *Runner) rescueTranscriptPath(t rescueTarget) string {
	base := filepath.Join(t.ProjDir, "work", "sessions", "rescue_"+t.Base+".jsonl")
	return attemptTranscriptPath(base, nextAttemptNumber(base))
}

// allocateRescueTranscript 定下这一次的落点，并在**同一把锁**里把文件占出来：
// 只算名字不建文件的话，同一章的转换与核对同时复盘会拿到同一个名字，两段对话
// 追加进同一条转录（NewTranscript 是 O_APPEND）。落不落盘由 latex.rescue.keep_records
// 决定——不落盘时没有文件可占，也就不需要锁（没有任何写入）。
func (r *Runner) allocateRescueTranscript(t rescueTarget, tid int) string {
	if !r.rescueKeepRecords(tid) {
		return r.rescueTranscriptPath(t)
	}
	rescuePathMu.Lock()
	defer rescuePathMu.Unlock()
	path := r.rescueTranscriptPath(t)
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644); err == nil {
		_ = f.Close()
	}
	return path
}

// nextAttemptNumber 是"这一处下一次该用的号"——与 convert 的 lastAttemptNumber
// 同一口径（磁盘上已有的最大号 + 1），只是它给的是**新**号而不是要续写的那一个。
// 一个都没有时是 1（第一次沿用不带后缀的原名）。attemptNumberOnDisk 的第二个
// 返回值是"最后一份是否已收尾"，与新号无关，这里不看它。
func nextAttemptNumber(transcriptBase string) int {
	if n, _ := attemptNumberOnDisk(transcriptBase); n >= 1 {
		return n + 1
	}
	return 1
}

// transcriptStemName 是转录名去掉目录与扩展名（`convert_<章>.try2`）——写进
// 锚点/复盘目标备注时的口径，与核对会话的 `checker_target.convert` 一致。
func transcriptStemName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// transcriptAttemptOf 从转录名里取尝试号（`convert_<章>.try2.jsonl` → 2；
// 没有后缀 → 1）。文件名是唯一来源。
func transcriptAttemptOf(path string) int {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if i := strings.LastIndex(stem, ".try"); i > 0 {
		if n, err := strconv.Atoi(stem[i+len(".try"):]); err == nil && n > 1 {
			return n
		}
	}
	return 1
}

// rescueAudit 追一行审计记录（默认不存对话，也要能看到"哪一章被判过什么"）。
func (r *Runner) rescueAudit(t rescueTarget, v rescueVerdict, lines int) {
	dir := filepath.Join(t.ProjDir, "work", "rescue")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	rec := map[string]any{
		"ts": time.Now().Format(time.RFC3339), "chapter": t.Base, "role": t.Role,
		"verdict": v.Verdict, "from": v.From, "reason": v.Reason, "hint": v.Hint,
		"transcript_lines": lines, "model": r.rescueModelName(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "log.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// rescueQueueTruncate 把"截断到 from 再继续"排进重试队列（与页面的 ⤓ 同一条
// 通路：源转录保留、新尝试写 .tryN+1、有工作区快照就一起回退）。
func (r *Runner) rescueQueueTruncate(t rescueTarget, from int) error {
	rel, err := filepath.Rel(filepath.Join(t.ProjDir, "work"), t.TrPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("转录不在本项目 work/ 之下: %s", t.TrPath)
	}
	role := t.Role
	if role == "" {
		role = "convert"
	}
	if role == "convert" {
		role = "continue" // 转换会话：从这条记录继续
	}
	_, err = retryq.Queue(t.ProjDir, retryq.Request{
		Project: filepath.Base(t.ProjDir), Chapter: t.Base, Role: role,
		From: from, Path: filepath.ToSlash(rel), Source: "rescue",
		Reason: fmt.Sprintf("复盘判为退化，从第 %d 条记录继续", from),
	})
	return err
}

var _ = logger.LevelInfo
