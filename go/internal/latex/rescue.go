package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// rescueTarget 说明"复盘哪条会话"。
type rescueTarget struct {
	ProjDir string // 项目根目录
	Base    string // 章节 ID
	Role    string // convert | checker
	TrPath  string // 目标会话的转录（绝对路径）
	Label   string // 日志标签
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
var (
	rescueMu   sync.Mutex
	rescueSeen = map[string]int{}
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

// rescueReview 起一条只读复盘会话，返回它的结论。ok=false 表示没能拿到结论。
func (r *Runner) rescueReview(t rescueTarget, tid int) (rescueVerdict, bool) {
	lines, err := loadTranscriptLines(t.TrPath)
	if err != nil || len(lines) == 0 {
		r.log.LogWarning(tid, "[rescue]", t.Base, "读不到转录，跳过复盘:", err)
		return rescueVerdict{}, false
	}
	name := r.rescueModelName()
	client := r.clientFor(name)
	modelCfg := r.modelOf(name)
	// 上下文：默认 128K（配置可覆盖）；会话与模型两侧取 min，所以两边都设。
	limit := r.cfg.Latex.RescueContextLimit()
	modelCfg.ContextLimit = limit
	tuning := r.cfg.LatexSession("checker") // 输出预算与 checker 同配置
	if v := r.cfg.Latex.RescueMaxTokens(); v > 0 {
		tuning.MaxTokens = v
	}
	tuning.ContextLimit = limit
	tuning.MaxToolRounds = r.cfg.Latex.RescueMaxRounds()

	readTool := &ReadSessionTool{Path: t.TrPath, Lines: lines, Total: len(lines)}
	statsTool := &SessionStatsTool{Lines: lines, Total: len(lines)}
	submit := &RescueSubmitTool{}
	tools := []session.Tool{readTool, statsTool, submit}
	sess := session.NewSession(client, modelCfg, tuning,
		renderPrompt(prompts.Must(prompts.RescueSystem), tuning, r.outputLang()),
		tools, r.log, tid, "rescue:"+t.Base)
	if r.rescueKeepRecords(tid) {
		if tr, terr := session.NewTranscript(r.rescueTranscriptPath(t)); terr == nil {
			sess.SetTranscript(tr)
			defer tr.Close()
		}
	}
	userText := fmt.Sprintf("The %s session for chapter %s ended its turn without calling submit. Its transcript has %d physical lines (%d message lines). Review it and submit one verdict.",
		t.Role, t.Base, len(lines), countKind(lines, "msg"))
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

// rescueKeepRecords：默认不存复盘转录，--debug / keep_session_records 才存。
func (r *Runner) rescueKeepRecords(tid int) bool {
	if r.cfg.Latex.RescueKeepRecords() {
		return true
	}
	return r.log != nil && r.log.Level() >= 4
}

// rescueTranscriptPath 是复盘会话的转录落点（同一个项目里按章节+时间区分）。
func (r *Runner) rescueTranscriptPath(t rescueTarget) string {
	name := fmt.Sprintf("rescue_%s_%s.jsonl", t.Base, time.Now().Format("20060102-150405"))
	return filepath.Join(t.ProjDir, "work", "sessions", name)
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
