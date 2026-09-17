package img2text

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// P12：就地修复轮（processor 的 repairBudget，同一对话里追加 fix 消息）用完后的
// **升级修复会话**。给它一个虚拟工作区：出错的完整响应放在 submit.md 里，模型用
// write_file / grep / view_image 反复编辑，submit 时跑 mmdc 检查（要求不高：能过
// 语法检查就行）。编译错误累计到上限（默认 3）就清上下文、换备选模型（配置
// tools.mermaid.fallback_model，未配置则原模型）从头再来；提交/检查次数总共受
// session_rounds（默认 6）约束。会话全程走 internal/session——JSONL 转录、
// 工具轮次软上限、压缩照常；文本日志统一打 [ToolCall] 行，分析侧照常统计。

const (
	// mermaidSessionRoundsDefault 是 tools.mermaid.session_rounds 未设置时的
	// 检查次数上限。≤ 0 = 关闭升级会话（维持旧的跳过+下轮重试）。
	mermaidSessionRoundsDefault = 6
	// mermaidSessionErrorsDefault 是 tools.mermaid.session_errors 未设置时的
	// 编译错误累计上限。
	mermaidSessionErrorsDefault = 3
	// mermaidFixSubmitFile 是工作区里模型要修的文件名（计划里的 submit.md）。
	mermaidFixSubmitFile = "submit.md"
	// mermaidFixErrorFile 是最近一次 mmdc 失败输出的落盘文件（grep 的另一半、
	// 升级时"出错的结果文件保留"的那份）。
	mermaidFixErrorFile = "compile_error.log"
	// mermaidSessionToolRoundsGrace 是提交检查轮之外给模型的机动工具轮
	//（read/grep/write 也要占轮次）。
	mermaidSessionToolRoundsGrace = 6
)

// resolveMermaidSessionRounds 归一化 session_rounds：nil → 默认 6；≤0 → 关闭。
func resolveMermaidSessionRounds(cfg *int) int {
	if cfg == nil {
		return mermaidSessionRoundsDefault
	}
	return *cfg
}

// resolveMermaidSessionErrors 归一化 session_errors：nil/非正 → 默认 3。
func resolveMermaidSessionErrors(cfg *int) int {
	if cfg == nil || *cfg <= 0 {
		return mermaidSessionErrorsDefault
	}
	return *cfg
}

// MermaidFixConfig 汇总升级会话需要的全部配置（runner 一次性解析好）。
type MermaidFixConfig struct {
	Rounds        int                // 提交/检查次数上限（session_rounds，总共）
	ErrorLimit    int                // 编译错误累计上限（session_errors）
	FallbackModel string             // 备选模型条目名（空 = 原模型清上下文重来）
	Command       string             // mermaid CLI（mmdc）
	Timeout       time.Duration      // 单次检查超时
	Primary       config.ModelConfig // 当前的 img2text 模型
	Fallback      config.ModelConfig // 备选模型的完整配置（FallbackModel 非空时有意义）
	ImagesDir     string             // 原图所在目录（view_image 用）
	ImgPath       string             // 原图相对路径（md 引用形式）
	WorkspaceRoot string             // 工作区根（每个图键一个子目录；空 = 临时目录）
}

// mermaidFixSessionState 是一次升级会话里工具共享的可变状态。
type mermaidFixSessionState struct {
	cfg       *MermaidFixConfig
	ws        string
	log       *logger.Logger
	tid       int
	errors    int    // 累计编译错误
	checks    int    // 累计提交/检查次数
	escalated bool   // 已切备选模型（切过就不再切）
	submitted bool   // submit 通过
	final     string // 提交通过后的最终响应
}

// MermaidFixSession 在就地修复轮用完后运行。返回修复后的完整响应与会话是否成功。
// validationError 是就地修复阶段的最后一条 mmdc 错误（任务描述的一部分）。
// 日志与转录都落在 logger / 工作区 JSONL 里，分析侧按 [ToolCall] 行统计。
func MermaidFixSession(cfg *MermaidFixConfig, prevResult, validationError string, log *logger.Logger, tid int) (string, bool) {
	return mermaidFixSessionInDir(cfg, "", prevResult, validationError, log, tid)
}

// mermaidFixSessionInDir 是 MermaidFixSession 的可测内核：workspace 显式传入
// （空 = 按 cfg.WorkspaceRoot + 临时目录）。生产路径的每图子目录由调用方拼好。
func mermaidFixSessionInDir(cfg *MermaidFixConfig, workspace, prevResult, validationError string, log *logger.Logger, tid int) (string, bool) {
	if cfg == nil || cfg.Rounds <= 0 {
		return "", false
	}
	ws, cleanup, err := mermaidFixWorkspace(cfg, workspace)
	if err != nil {
		log.LogError(tid, "  [mermaid-fix] 创建工作区失败:", err)
		return "", false
	}
	defer cleanup()

	// 出错的完整响应原样放进 submit.md——模型基于它改，提交时整体校验。
	submitPath := filepath.Join(ws, mermaidFixSubmitFile)
	if err := os.WriteFile(submitPath, []byte(prevResult), 0o644); err != nil {
		log.LogError(tid, "  [mermaid-fix] 写 submit.md 失败:", err)
		return "", false
	}

	st := &mermaidFixSessionState{cfg: cfg, ws: ws, log: log, tid: tid}
	modelCfg := cfg.Primary
	for stage := 0; stage < 2; stage++ {
		if stage == 1 {
			// 没有配置可解析的 fallback_model 时备选段没有模型可用
			// （cfg.Fallback 是零值），直接结束，不空跑一个坏客户端。
			if !hasFallbackModel(cfg) {
				break
			}
			// 备选段：清上下文（全新会话），工作区文件（submit.md +
			// compile_error.log + 前一段转录）原样保留。
			modelCfg = cfg.Fallback
			log.LogError(tid, "  [mermaid-fix] 备选模型会话开始（上下文已清空，工作区文件保留）")
		}
		if st.runOneStage(modelCfg, ws, stage, validationError) {
			return st.final, true
		}
		if !st.escalated {
			// 没有触发升级：轮次耗尽或会话失败，没有第二段。
			break
		}
	}
	log.LogError(tid, "  [mermaid-fix] 升级会话未能修复（跳过、下轮重试）；工作区保留:", ws)
	return "", false
}

// mermaidFixWorkspace 准备工作区，返回路径与清理函数。调用方自带的工作区不清理
// （出错现场保留供诊断与下轮续用）；临时目录用完即删。
func mermaidFixWorkspace(cfg *MermaidFixConfig, workspace string) (string, func(), error) {
	if workspace != "" {
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			return "", nil, err
		}
		return workspace, func() {}, nil
	}
	ws, err := os.MkdirTemp("", "docvision-mermaid-fix-")
	if err != nil {
		return "", nil, err
	}
	return ws, func() { _ = os.RemoveAll(ws) }, nil
}

// runOneStage 跑一段会话（stage 0 = 原模型，stage 1 = 备选模型）。
// 返回该段是否以提交成功收尾。
func (st *mermaidFixSessionState) runOneStage(modelCfg config.ModelConfig, ws string, stage int, validationError string) bool {
	client := session.NewClient(modelCfg)
	client.SetLogger(st.log)
	tuning := config.SessionTuning{
		MaxTokens:     modelCfg.MaxTokens,
		Temperature:   modelCfg.Temperature,
		MaxToolRounds: st.cfg.Rounds + mermaidSessionToolRoundsGrace,
		ContextLimit:  131072,
	}
	sess := session.NewSession(client, modelCfg, tuning, st.systemPrompt(), st.tools(), st.log, st.tid, "mermaid-fix")
	trPath := filepath.Join(ws, "session-stage"+fmt.Sprint(stage)+".jsonl")
	if tr, err := session.NewTranscript(trPath); err == nil {
		sess.SetTranscript(tr)
		defer tr.Close()
	}

	_, runErr := sess.Run(session.RunOptions{UserText: st.taskPrompt(stage > 0, validationError)})
	if runErr != nil {
		st.log.LogError(st.tid, "  [mermaid-fix] 会话错误:", runErr)
	}
	if !st.submitted && runErr == nil {
		// 与 tikz 会话同款：模型漏了 submit 就提醒一次。
		st.log.LogWarning(st.tid, "  [mermaid-fix] 会话结束但未提交，发送提交提醒")
		_, _ = sess.Run(session.RunOptions{
			UserText: "You have NOT called submit yet. Fix " + mermaidFixSubmitFile + " (write_file), then call submit.",
		})
	}
	if !st.submitted && !st.escalated && hasFallbackModel(st.cfg) {
		// T50：会话失败（模型退化空响应、API 错误）或提醒后仍未提交，
		// 都没有经过编译错误累计——原先这类失败直接放弃，备选模型形同
		// 虚设。同样升级备选段：清上下文、工作区文件保留，更强的兜底
		// 模型拿同一份 submit.md 再修。
		st.escalated = true
		st.log.LogError(st.tid, "  [mermaid-fix] 本段会话未能完成，进入备选模型（fallback=", st.cfg.FallbackModel, "）；上下文清空、工作区文件保留")
	}
	return st.submitted
}

// hasFallbackModel 报告备选段是否有模型可用（配置了能解析的
// fallback_model；未配置时 cfg.Fallback 是零值 ModelConfig）。
func hasFallbackModel(cfg *MermaidFixConfig) bool {
	return cfg != nil && (cfg.Fallback.Model != "" || cfg.Fallback.BaseURL != "")
}

// systemPrompt 是修复会话的系统提示词。只有这一个使用点、一次成文，不进 prompts
// 注册表（该表收的是跨会话复用的模板；MustMention 黑名单也不涉及）。
func (st *mermaidFixSessionState) systemPrompt() string {
	return "You are fixing a Mermaid diagram that failed syntax validation. " +
		"The broken document is in your workspace as " + mermaidFixSubmitFile + ". " +
		"Edit it with write_file until it passes, then call submit. " +
		"Requirements: keep it a Mermaid diagram that renders the SAME content as before — " +
		"fix syntax, do not invent or drop nodes; do not switch to LaTeX/TikZ or any other format; " +
		"use view_image to look at the original figure whenever unsure. " +
		"When submit reports OK you are done; do not call submit again after success."
}

// taskPrompt 是会话的首条 user 消息（备选段清空上下文后，这条就是全部任务）。
func (st *mermaidFixSessionState) taskPrompt(fallback bool, validationError string) string {
	var b strings.Builder
	if fallback {
		b.WriteString(fmt.Sprintf("The previous fix session also failed after %d compile errors. ", st.cfg.ErrorLimit))
		b.WriteString("The context was cleared; the workspace files (submit.md, compile_error.log) are unchanged.\n\n")
	}
	b.WriteString("The document " + mermaidFixSubmitFile + " in your workspace failed Mermaid validation:\n")
	b.WriteString(validationError + "\n\n")
	b.WriteString("Fix it (write_file rewrites " + mermaidFixSubmitFile + "), use grep to re-read the current file or the last error, view_image for the original figure, then submit. ")
	b.WriteString(fmt.Sprintf("At most %d submit/check attempts in total; each failed submit counts one compile error and %d errors force a fallback-model escalation.\n",
		st.cfg.Rounds-st.checks, st.cfg.ErrorLimit))
	return b.String()
}

// tools 组装会话工具：write_file / grep / view_image / submit。
func (st *mermaidFixSessionState) tools() []session.Tool {
	ws := st.ws
	return []session.Tool{
		&mermaidWriteTool{st: st, path: filepath.Join(ws, mermaidFixSubmitFile)},
		&mermaidGrepTool{st: st, dir: ws},
		&mermaidViewTool{st: st},
		&mermaidSubmitTool{st: st, path: filepath.Join(ws, mermaidFixSubmitFile)},
	}
}

// logTool 统一打 [ToolCall] 行（analyze 的 PatternToolCall 按行统计工具调用）。
func (st *mermaidFixSessionState) logTool(name, summary string) {
	st.log.Log(st.tid, "  [ToolCall]", name, "·", summary)
}

// ---------- write_file ----------

type mermaidWriteTool struct {
	st   *mermaidFixSessionState
	path string
}

func (t *mermaidWriteTool) Name() string { return "write_file" }

func (t *mermaidWriteTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "write_file",
		"description": "Overwrite " + mermaidFixSubmitFile + " in the workspace with the full corrected document. This is the only writable file.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "The complete new document content."},
			},
			"required": []string{"content"},
		},
	}}
}

func (t *mermaidWriteTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return session.ToolResult{}, err
	}
	if strings.TrimSpace(args.Content) == "" {
		return session.ToolResult{}, fmt.Errorf("content 为空")
	}
	if err := os.WriteFile(t.path, []byte(args.Content), 0o644); err != nil {
		return session.ToolResult{}, err
	}
	t.st.logTool("write_file", fmt.Sprintf("%d 字节 → %s", len(args.Content), mermaidFixSubmitFile))
	return session.ToolResult{Text: fmt.Sprintf("OK: %s rewritten (%d bytes). Call submit when ready.", mermaidFixSubmitFile, len(args.Content))}, nil
}

// ---------- grep ----------

type mermaidGrepTool struct {
	st  *mermaidFixSessionState
	dir string
}

func (t *mermaidGrepTool) Name() string { return "grep" }

func (t *mermaidGrepTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "grep",
		"description": "Search the workspace files (" + mermaidFixSubmitFile + ", " + mermaidFixErrorFile + ") for a regular expression. Returns matching lines with line numbers.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Go regular expression."},
			},
			"required": []string{"pattern"},
		},
	}}
}

func (t *mermaidGrepTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return session.ToolResult{}, err
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return session.ToolResult{}, fmt.Errorf("pattern 为空")
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return session.ToolResult{}, fmt.Errorf("正则无效: %v", err)
	}
	var b strings.Builder
	for _, name := range []string{mermaidFixSubmitFile, mermaidFixErrorFile} {
		data, err := os.ReadFile(filepath.Join(t.dir, name))
		if err != nil {
			fmt.Fprintf(&b, "%s: (missing)\n", name)
			continue
		}
		lines := strings.Split(string(data), "\n")
		hits := 0
		for i, ln := range lines {
			if re.MatchString(ln) {
				hits++
				if hits <= 40 {
					fmt.Fprintf(&b, "%s:%d: %s\n", name, i+1, mermaidTruncateStr(ln, 200))
				}
			}
		}
		fmt.Fprintf(&b, "%s: %d match(es)\n", name, hits)
	}
	t.st.logTool("grep", "/"+args.Pattern+"/")
	return session.ToolResult{Text: b.String()}, nil
}

// ---------- view_image ----------

type mermaidViewTool struct {
	st *mermaidFixSessionState
}

func (t *mermaidViewTool) Name() string { return "view_image" }

func (t *mermaidViewTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_image",
		"description": "Look at the original figure this Mermaid diagram must reproduce.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *mermaidViewTool) Execute(argsJSON string) (session.ToolResult, error) {
	imgFile, err := resolveImageFile(t.st.cfg.ImagesDir, t.st.cfg.ImgPath, "")
	if err != nil {
		return session.ToolResult{Text: "original image not found: " + t.st.cfg.ImgPath}, nil
	}
	b64, err := ImageToBase64(imgFile, 1280)
	if err != nil {
		return session.ToolResult{Text: "image decode failed: " + err.Error()}, nil
	}
	t.st.logTool("view_image", filepath.Base(imgFile))
	return session.ToolResult{Text: "Original figure attached.", ImageBase64: b64, ImageMIME: "image/jpeg"}, nil
}

// ---------- submit ----------

type mermaidSubmitTool struct {
	st   *mermaidFixSessionState
	path string
}

func (t *mermaidSubmitTool) Name() string { return "submit" }

func (t *mermaidSubmitTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "submit",
		"description": "Validate " + mermaidFixSubmitFile + " with the Mermaid CLI. On success the session ends; on failure the compile error is returned and counted.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *mermaidSubmitTool) Execute(argsJSON string) (session.ToolResult, error) {
	st := t.st
	if st.submitted {
		return session.ToolResult{Text: "Already submitted successfully."}, nil
	}
	if st.checks >= st.cfg.Rounds {
		return session.ToolResult{Text: "REJECTED: no check attempts left."}, nil
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return session.ToolResult{Text: "NOT FOUND: " + mermaidFixSubmitFile + "（先 write_file 写入）"}, nil
	}
	st.checks++
	res := ValidateMermaid(context.Background(), string(data), st.cfg.Command, st.cfg.Timeout)
	if !res.HasMermaid {
		return st.recordError("the document contains no ```mermaid block"), nil
	}
	if !res.Available {
		// 校验器缺失：没有裁决手段，按环境问题处理——不计错误预算。
		return session.ToolResult{Text: "VALIDATOR UNAVAILABLE: " + res.Error + " — fix the document as best you can and submit again."}, nil
	}
	if !res.Valid {
		return st.recordError(res.Error), nil
	}
	st.submitted = true
	st.final = finalResponseFromDocument(string(data))
	st.logTool("submit", "OK")
	st.log.Log(st.tid, "  [mermaid-fix] 提交通过（", st.checks, "次检查 /", st.errors, "次编译错误）")
	return session.ToolResult{Text: "OK: the Mermaid diagram passed validation. You are done."}, nil
}

// recordError 记一次编译错误：落盘、计数、到上限时标记升级（备选段已升级过就不再切）。
func (st *mermaidFixSessionState) recordError(errText string) session.ToolResult {
	st.errors++
	if ws := st.ws; ws != "" {
		_ = os.WriteFile(filepath.Join(ws, mermaidFixErrorFile), []byte(errText), 0o644)
	}
	st.logTool("submit", "FAIL("+fmt.Sprint(st.errors)+"/"+fmt.Sprint(st.cfg.ErrorLimit)+")")
	if st.errors >= st.cfg.ErrorLimit {
		if st.escalated {
			return session.ToolResult{Text: "COMPILE FAILED (already on the fallback model, no escalation left): " + errText}
		}
		st.escalated = true
		st.log.LogError(st.tid, "  [mermaid-fix] 编译错误累计 ", st.errors, "/", st.cfg.ErrorLimit,
			"，进入备选模型（fallback=", st.cfg.FallbackModel, "）；上下文清空、工作区文件保留")
		return session.ToolResult{Text: "COMPILE FAILED: " + errText + "\n" +
			"ERROR BUDGET SPENT — a fresh session with a stronger fallback model takes over now. This session ends."}
	}
	return session.ToolResult{Text: "COMPILE FAILED: " + errText}
}

// mermaidTruncateStr 按 rune 截断（与 latex.truncateStr 同职责；img2text 侧
// 原本没有这个帮手，grep 结果行用它限长）。
func mermaidTruncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// finalResponseFromDocument 把修复后的文档还原成 img2text 的标准响应
// （[IMG_TYPE: mermaid] 前缀 + 正文；正文里的 ```mermaid 块由 embedBlockFor 原样嵌入）。
func finalResponseFromDocument(doc string) string {
	doc = strings.TrimSpace(doc)
	if strings.HasPrefix(doc, "[IMG_TYPE:") {
		return doc
	}
	return "[IMG_TYPE: mermaid]\n" + doc
}
