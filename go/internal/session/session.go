package session

import (
	"time"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// ToolResult is what a tool hands back to the model. Text is always
// delivered as the tool response; Image (base64, no data: prefix) is
// optionally appended as a follow-up user turn so vision models can
// inspect rendered output (e.g. a rasterised TikZ preview).
type ToolResult struct {
	Text        string
	ImageBase64 string
	ImageMIME   string // default "image/png"
}

// Tool is one pluggable capability of a session. Sessions own a tool
// registry so different pipelines can compose different toolsets
// (compile+preview, submit, grep, sandboxed bash, file IO...) without
// touching the conversation engine.
type Tool interface {
	// Definition returns the full OpenAI tool schema:
	// {"type":"function","function":{name, description, parameters}}.
	Definition() map[string]any
	// Execute runs the tool with the raw JSON arguments string.
	Execute(argsJSON string) (ToolResult, error)
	// Name returns the function name for dispatch and logs.
	Name() string
}

// ToolRoundsSafetyCap is kept only as a reference value for docs; the
// runtime does NOT enforce it (max_tool_rounds: 0 = truly unlimited,
// so a model that never converges CAN run forever).
const ToolRoundsSafetyCap = 200

// Session is a multi-turn AI conversation with tools, a configurable
// context window and automatic AI-driven compaction. A Session is NOT
// safe for concurrent use; each worker builds its own.
type Session struct {
	client   *Client
	modelCfg config.ModelConfig
	tuning   config.SessionTuning
	system   string
	messages []ChatMessage
	tools    []Tool
	toolDefs []map[string]any
	logger   *logger.Logger
	tid      int
	label    string

	// stats for logging / analysis
	APIRequests int
	ToolInvoked int
	Compactions int
	// lastCompactTokens is the estimate right after the last AI summary:
	// a tiny window plus a tail that is itself over the threshold would
	// otherwise buy a new summary every single round (see maybeCompact).
	lastCompactTokens int
	// contextNoteStep is the last growth threshold already logged (0/50/80).
	contextNoteStep int
	Prunes          int

	// 本地估算与厂商实测的标定。textTokens 数不出厂商侧真实开销
	// （reasoning_content/tool_calls/图片像素编码/每轮包装），实测
	// 差到 2~4 倍：现场一次运行里估算说 66k tokens、厂商
	// prompt_tokens 却是 132k~243k，于是 compaction_at=0.85×128k 的
	// 阈值永远够不着，整个运行一次都没压缩过。现在每次响应后拿
	// prompt_tokens / 请求前估算 得到系数，阈值判断用**标定后**的规模。
	lastPromptTokens int     // 最近一次厂商实测的 prompt_tokens
	calibNum         float64 // 标定分子（实测）
	calibDen         float64 // 标定分母（当时估算）
	// estBeforeRequest 是最近一次请求发出**之前**的本地估算，用它当
	// 标定分母（响应回来后才知道厂商实测值）。
	estBeforeRequest int
	// reqTextTokens / reqImages snapshot the request that was just sent: the
	// text-only local estimate and how many images it carried. Both travel on
	// the t="usage" line, which is what lets the preview derive the MEASURED
	// per-image cost from two consecutive prompt_tokens values (vendor number
	// minus the text growth, divided by the images that were added).
	reqTextTokens int
	reqImages     int

	// progressHook, when set, is notified after every API round and
	// every tool execution with (completed rounds, executed tool
	// calls) — used by compact console modes to show a live line.
	progressHook func(rounds, tools int)
	rounds       int
	// user is the stable per-conversation identifier sent as the OpenAI
	// `user` field. Gateways such as new-api can pin routing on it so one
	// conversation always reaches the same upstream channel; vendor prefix
	// caches live per upstream key, so affinity is what makes
	// prompt_tokens_details.cached_tokens reliable.
	user string

	// transcript, when set, receives every appended conversation
	// message as a JSONL line (images as file:// refs) for resume.
	transcript *TranscriptWriter
}

// SetProgressHook registers a callback notified after every API round
// and every tool execution with (completed rounds, executed tool
// calls). Only one hook is supported; pass nil to clear.
func (s *Session) SetProgressHook(fn func(rounds, tools int)) {
	s.progressHook = fn
}

// notifyProgress reports the current counters to the hook (no-op
// without one).
func (s *Session) notifyProgress() {
	if s.progressHook != nil {
		s.progressHook(s.rounds, s.ToolInvoked)
	}
}

// SetTranscript attaches a JSONL transcript writer. Every message
// appended to the conversation afterwards (user turns, assistant
// replies, tool results) is also appended to the transcript file, so
// an interrupted session can be resumed later without burning tokens
// on a fresh context. Images are stored as file://media/... refs.
// HasTranscript reports whether this session writes its messages to a live
// JSONL transcript. Callers that used to dump the whole conversation to the
// same file at the end (style session) must skip that when a transcript is
// attached: the messages are already on disk, and appending them again
// duplicated the history.
func (s *Session) HasTranscript() bool {
	return s.transcript != nil
}

func (s *Session) SetTranscript(w *TranscriptWriter) {
	s.transcript = w
	s.recordPromptMeta()
}

// recordPromptMeta writes the "what was the model told" meta line: the
// system prompt of THIS run plus the tool definitions that go with it.
// Rationale: the transcript deliberately stores no system message (the
// prompt is re-rendered per run, so replaying a stored one would be wrong),
// which left the file unable to explain itself. The meta line fixes that
// without touching replay: LoadTranscript ignores every non-"msg" line.
func (s *Session) recordPromptMeta() {
	if s.transcript == nil {
		return
	}
	tools := make([]ToolSnapshot, 0, len(s.tools))
	for _, t := range s.tools {
		snap := ToolSnapshot{Name: t.Name()}
		if def, ok := t.Definition()["function"].(map[string]any); ok {
			if d, ok := def["description"].(string); ok {
				snap.Description = d
			}
			if params, ok := def["parameters"]; ok {
				if raw, err := json.Marshal(params); err == nil {
					snap.Parameters = raw
				}
			}
		}
		tools = append(tools, snap)
	}
	model := ""
	if s.client != nil {
		model = s.client.Model()
	}
	if s.logger != nil && strings.TrimSpace(s.system) == "" {
		// 历史事故：样式反馈会话曾经以空系统提示词启动，转录里看不出
		// 异常、模型也没被交代规则。这里显式告警，别再靠人去翻。
		s.logger.LogWarning(s.tid, fmt.Sprintf("[session:%s] 系统提示词为空，转录快照将不含任何指令", s.label))
	}
	if err := s.transcript.AppendMeta("system", s.label, model, s.system, tools); err != nil && s.logger != nil {
		s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] 元信息写入失败: %v", s.label, err))
	}
}

// appendTranscript writes one message to the transcript if attached.
// Errors are logged (debug) but never fail the session itself.
func (s *Session) appendTranscript(msg ChatMessage) {
	if s.transcript == nil {
		return
	}
	if err := s.transcript.Append(msg); err != nil && s.logger != nil {
		s.logger.Debug(s.tid, "[session:"+s.label+"] transcript 写入失败:", err)
	}
	// P7: the full message is on disk now — the streaming snapshot is stale.
	s.transcript.ClearPartial()
}

// recordUsage appends one t="usage" line for the request that just finished,
// so the transcript alone is enough to compute token counts, prefix-cache hit
// rate, latency, time-to-first-token and output speed for a session.
func (s *Session) recordUsage(resp *ChatResponse, round int, kind string) {
	if resp == nil {
		return
	}
	// 标定必须在 transcript 判空**之前**：没有转录的会话（没挂转录文件的
	// 一次性会话）同样要按真实 prompt 规模判断压缩阈值。
	if u := resp.Usage; u != nil {
		s.observePromptSize(s.estBeforeRequest, u.PromptTokens)
	}
	if s.transcript == nil {
		return
	}
	rec := UsageRecord{
		Model:    s.client.Model(),
		Stream:   resp.Streamed,
		Round:    round,
		Kind:     kind,
		Duration: resp.Elapsed,
		TTFT:     resp.TTFT,
		Finish:   resp.FinishReason,
		// The local half of the request (text estimate + image count) is what
		// makes the vendor's prompt_tokens delta readable as a per-image cost.
		TextTokens: s.reqTextTokens,
		Images:     s.reqImages,
	}
	if u := resp.Usage; u != nil {
		rec.PromptTokens = u.PromptTokens
		rec.CachedTokens = u.CachedTokens()
		rec.Completion = u.CompletionTokens
		if u.CompletionTokensDetails != nil {
			rec.Reasoning = u.CompletionTokensDetails.ReasoningTokens
		}
	}
	if err := s.transcript.AppendUsage(rec); err != nil && s.logger != nil {
		s.logger.Debug(s.tid, "[session:"+s.label+"] usage 写入失败:", err)
	}
}

// NewSession creates a session. system is the system prompt; tools may
// be nil. The conversation keeps the system prompt out of the
// compaction scope so it is never rewritten.
func NewSession(
	client *Client,
	modelCfg config.ModelConfig,
	tuning config.SessionTuning,
	system string,
	tools []Tool,
	log *logger.Logger,
	tid int,
	label string,
) *Session {
	defs := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, t.Definition())
	}
	s := &Session{
		client:   client,
		modelCfg: modelCfg,
		tuning:   tuning,
		system:   system,
		tools:    tools,
		toolDefs: defs,
		logger:   log,
		tid:      tid,
		label:    label,
	}
	if system != "" {
		s.messages = append(s.messages, ChatMessage{Role: "system", Content: system})
	}
	s.user = affinityUser(label, tid)
	return s
}

// affinityUser builds the per-conversation identifier sent as `user`.
// One session keeps the same value on every request (that is what makes the
// gateway route it to a stable upstream channel), while different sessions
// get different values so the gateway can still spread them out.
func affinityUser(label string, tid int) string {
	if label == "" {
		return ""
	}
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, label)
	if len(clean) > 60 {
		clean = clean[:60]
	}
	return fmt.Sprintf("docvision-%s-T%d", clean, tid)
}

// Label returns the session label used in log lines.
func (s *Session) Label() string { return s.label }

// SetLabel updates the session label.
func (s *Session) SetLabel(l string) { s.label = l }

// Messages returns a copy of the conversation so callers can persist
// or inspect it.
func (s *Session) Messages() []ChatMessage {
	out := make([]ChatMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

// SetMessages replaces the conversation history with a previously
// persisted context (Messages / saveSessionContext). The system prompt
// is expected to be part of msgs; the tool registry stays as built.
// SetMessages replaces the conversation, e.g. with a transcript loaded
// from disk when a previous run was interrupted. The session's own
// system prompt is authoritative: transcripts do not carry it, so it is
// re-inserted at the head (and an old copy inside msgs is replaced) —
// otherwise a resumed session would run without any system prompt.
func (s *Session) SetMessages(msgs []ChatMessage) {
	rest := msgs
	if len(rest) > 0 && rest[0].Role == "system" {
		rest = rest[1:]
	}
	out := make([]ChatMessage, 0, len(rest)+1)
	if s.system != "" {
		out = append(out, ChatMessage{Role: "system", Content: s.system})
	}
	out = append(out, rest...)
	s.messages = out
}

// RunOptions describes one logical user turn.
type RunOptions struct {
	// UserText is the user message body.
	UserText string
	// Images is a list of base64-encoded images (no data: prefix)
	// attached to the user turn.
	Images []string
	// ForceNoTools disables the tool registry for this turn (single
	// shot completion).
	ForceNoTools bool
}

// visionTurn is one tool-produced image waiting to be handed back to the model.
// The call id and the tool name ride along so the handle text can say **which
// call** the image belongs to: tool-role content is text-only in the OpenAI
// schema, so an image can only travel on a user message, and a user message has
// nowhere to put a tool_call_id. Ownership therefore lives in the text handle,
// never in a wire field (DSH does the same: its internal model keeps the image
// as an attachment of the tool-result block and only flattens it into
// text + image_url at request time, with an image handle text in front).
type visionTurn struct {
	mime   string
	b64    string
	callID string
	name   string
}

// toolImageTextPrefix opens every tool-image handle. It is load-bearing: the
// viewer recognises tool image turns in **old** transcripts by this prefix, so
// it must never change.
const toolImageTextPrefix = "Tool image output"

// toolImageHandleText renders the text handle that precedes a tool image on its
// user turn, e.g.
//
//	Tool image output from view_pdf (call call_abc123) (for your visual review):
//
// The attribution part is omitted when the call is unknown, which keeps the
// handle readable and the prefix stable.
func toolImageHandleText(v visionTurn) string {
	if v.callID == "" && v.name == "" {
		return toolImageTextPrefix + " (for your visual review):"
	}
	handle := toolImageTextPrefix
	if v.name != "" {
		handle += " from " + v.name
	}
	if v.callID != "" {
		handle += " (call " + v.callID + ")"
	}
	return handle + " (for your visual review):"
}

// toolImageContent is the user message that carries one tool image back to the
// model: a text part (the handle, which names the owning call) plus the image
// as a data URI. It is deliberately a plain user message — no extra fields.
func toolImageContent(v visionTurn) []map[string]interface{} {
	return []map[string]interface{}{
		{"type": "text", "text": toolImageHandleText(v)},
		{"type": "image_url", "image_url": map[string]string{
			"url": "data:" + v.mime + ";base64," + v.b64,
		}},
	}
}

// appendToolImage appends that user turn to the live request as well as to the
// transcript, so a reader of the transcript sees the same thing the model saw.
// The image's local token estimate travels on the message (never on the wire),
// so the compaction threshold counts the picture and not just its text handle.
func (s *Session) appendToolImage(v visionTurn) {
	content := toolImageContent(v)
	tokens := []int{ImageTokensOfBase64ForModel(s.modelName(), v.b64)}
	s.messages = append(s.messages, ChatMessage{Role: "user", Content: content, ImageTokens: tokens})
	s.appendTranscript(ChatMessage{Role: "user", Content: toolImageContent(v)})
}

// Run processes one logical user turn: sends the message, executes any
// tool calls, feeds results back, and repeats until the model produces
// a final text answer or the tool budget is exhausted (at which point
// it forces a no-tools final reply). Returns the final assistant text.
func (s *Session) Run(opts RunOptions) (string, error) {
	// The context guard lives INSIDE the request loop (see below): one
	// Run() is an entire agentic session here, so checking only here
	// meant checking once, at the smallest the conversation ever is.

	userMsg := ChatMessage{Role: "user"}
	if len(opts.Images) > 0 {
		parts := []map[string]interface{}{}
		if strings.TrimSpace(opts.UserText) != "" {
			parts = append(parts, map[string]interface{}{"type": "text", "text": opts.UserText})
		}
		for _, img := range opts.Images {
			parts = append(parts, map[string]interface{}{
				"type":      "image_url",
				"image_url": map[string]string{"url": "data:image/jpeg;base64," + img},
			})
			userMsg.ImageTokens = append(userMsg.ImageTokens, ImageTokensOfBase64ForModel(s.modelName(), img))
		}
		userMsg.Content = parts
	} else {
		userMsg.Content = opts.UserText
	}
	s.messages = append(s.messages, userMsg)
	s.appendTranscript(userMsg)

	// Debug tracing: full prompts and every tool exchange land in the
	// log file (never the console) when debug mode is on.
	if s.logger.DebugEnabled() {
		s.logger.Debug(s.tid, "[session:"+s.label+"] === 新会话轮 ===")
		s.logger.Debug(s.tid, "[session:"+s.label+"] system prompt:", s.system)
		if opts.UserText != "" {
			s.logger.Debug(s.tid, "[session:"+s.label+"] user prompt:", opts.UserText)
		}
		if len(opts.Images) > 0 {
			s.logger.Debug(s.tid, "[session:"+s.label+"] attached images:", len(opts.Images))
		}
	}

	toolRounds := 0
	// maxRounds <= 0 means unlimited (user opted out of any cap).
	maxRounds := s.tuning.MaxToolRounds
	// Soft round budget: from warnFrom the session starts reporting how
	// many tool rounds are left, and the cap itself only ends the tool
	// phase after `grace` extra rounds — a hard cut-off in the middle of
	// real work loses more than it saves.
	warnFrom := 0
	hardStop := 0
	if maxRounds > 0 {
		ratio := s.tuning.ToolRoundsWarnRatio
		if ratio <= 0 || ratio > 1 {
			ratio = 0.7
		}
		warnFrom = int(math.Ceil(float64(maxRounds) * ratio))
		if warnFrom < 1 {
			warnFrom = 1
		}
		hardStop = maxRounds + s.tuning.ToolRoundsGraceRounds()
	}
	reminderIdx := -1 // index of the replaceable round-budget reminder
	remind := func(text string) {
		// Replace the previous reminder in place: the history stays short
		// and the prefix before it is untouched, so prompt caching keeps
		// working while the countdown stays accurate.
		msg := ChatMessage{Role: "user", Content: text}
		if reminderIdx >= 0 && reminderIdx < len(s.messages) {
			s.messages[reminderIdx] = msg
			return
		}
		reminderIdx = len(s.messages)
		s.messages = append(s.messages, msg)
		s.appendTranscript(msg)
	}

	for {
		// Context guard INSIDE the loop, i.e. before EVERY model request.
		//
		// It used to be called once per Run(), and for these sessions one
		// Run() IS the whole agentic session (the latex sessions hand the
		// initial task to Run and stay inside this loop for hundreds of
		// rounds) — so the guard only ever looked at a conversation of
		// system prompt + first message and never fired again. Real
		// consequence: a run reached prompt=323,966 tokens with
		// context_limit 128K and compaction_at 0.85 and nothing happened,
		// no "COMPRESSED SESSION CONTEXT" line ever appeared in the
		// transcript, and the bill ended in an exhausted account.
		if err := s.maybeCompact(); err != nil {
			return "", fmt.Errorf("compaction failed: %w", err)
		}
		s.snapshotRequest("")
		req := &ChatRequest{
			Model:       s.client.Model(),
			Messages:    s.messages,
			MaxTokens:   s.tuning.MaxTokens,
			Temperature: s.tuning.Temperature,
			User:        s.user,
		}
		if maxRounds > 0 && toolRounds >= warnFrom {
			switch {
			case toolRounds < maxRounds:
				remind(fmt.Sprintf("ROUND BUDGET: %d of %d tool rounds used, %d left. Use them sparingly, finish the work and call submit with your best result.",
					toolRounds, maxRounds, maxRounds-toolRounds))
			case toolRounds < hardStop:
				remind(fmt.Sprintf("ROUND BUDGET EXCEEDED: the soft limit of %d tool rounds is used up. You have %d extra rounds left; after that no more tool calls are possible, so wrap up now and submit your best result.",
					maxRounds, hardStop-toolRounds))
			}
		}
		useTools := len(s.tools) > 0 && !opts.ForceNoTools && (maxRounds <= 0 || toolRounds < hardStop)
		if len(s.tools) > 0 && !opts.ForceNoTools {
			// ALWAYS send the tool definitions. Providers build their prompt
			// prefix from (system, tools, messages): dropping the tool block
			// on the final round shifts everything after it and throws away
			// the whole prefix cache (measured: prompt 37114 -> 35362, i.e.
			// the entire 1752-token tool schema). Whether tools may actually
			// be called is decided by tool_choice alone.
			req.Tools = s.toolDefs
		}
		if useTools {
			req.ToolChoice = "auto"
		} else if len(s.tools) > 0 && !opts.ForceNoTools {
			// Budget exhausted: force a text-only final answer.
			req.ToolChoice = "none"
		}

		if s.logger.DebugEnabled() {
			s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] round %d: api request (%s messages=%d est_tokens=%d tools=%v)",
				s.label, toolRounds+1, s.client.RequestSummary(req), len(s.messages), s.EstimatedTokens(), useTools))
		}
		// P7: progressive snapshots land in <transcript>.partial while this
		// request streams; every transcript append clears it again.
		if s.transcript != nil {
			req.StreamHook = func(phase, text string) {
				_ = s.transcript.WritePartial(PartialRecord{Phase: phase, Text: text, Ts: time.Now().UnixMilli()})
			}
		}
		resp, sentinel, status := s.client.CallWithRetry(req)
		s.APIRequests++
		s.rounds++
		s.notifyProgress()
		if status != "" {
			return "", fmt.Errorf("api error: %s", sentinel)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response (no choices)")
		}
		choice := resp.Choices[0]
		if s.logger.DebugEnabled() {
			s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] round %d: api response (%.1fs, stream=%v finish=%s content=%d chars reasoning=%d chars tools=%d %s)",
				s.label, toolRounds+1, resp.Elapsed.Seconds(), resp.Streamed, dash(resp.FinishReason),
				len(ContentString(choice.Message)), resp.ReasoningChars, len(choice.Message.ToolCalls), resp.Usage.String()))
			if text := strings.TrimSpace(ContentString(choice.Message)); text != "" {
				s.logger.Debug(s.tid, "[session:"+s.label+"] round "+fmt.Sprint(toolRounds+1)+" content:", truncateForLog(text, 4000))
			}
		}

		s.recordUsage(resp, toolRounds+1, "")

		if len(choice.Message.ToolCalls) > 0 && useTools {
			NormalizeToolCallTypes(&choice.Message)
			s.messages = append(s.messages, choice.Message)
			s.appendTranscript(choice.Message)

			// Tool-produced images are delivered AFTER every tool message:
			// the OpenAI schema requires the messages directly following an
			// assistant tool_calls turn to be the matching tool responses,
			// so an interleaved user turn makes the next request fail with
			// "insufficient tool messages following tool_calls message".
			var vision []visionTurn

			for _, tc := range choice.Message.ToolCalls {
				if s.logger.DebugEnabled() {
					s.logger.Debug(s.tid, "[session:"+s.label+"] tool call:", tc.Function.Name, tc.Function.Arguments)
				}
				result := s.executeTool(tc)
				if s.logger.DebugEnabled() {
					out := result.Text
					if len(out) > 2000 {
						out = out[:2000] + fmt.Sprintf("...(truncated, %d bytes total)", len(result.Text))
					}
					if result.ImageBase64 != "" {
						out += fmt.Sprintf(" [image: %s, %d bytes b64]", result.ImageMIME, len(result.ImageBase64))
					}
					s.logger.Debug(s.tid, "[session:"+s.label+"] tool result:", tc.Function.Name, out)
				}

				s.messages = append(s.messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result.Text,
				})
				s.appendTranscript(ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result.Text,
				})
				if result.ImageBase64 != "" {
					mime := result.ImageMIME
					if mime == "" {
						mime = "image/png"
					}
					vision = append(vision, visionTurn{
						mime: mime, b64: result.ImageBase64,
						callID: tc.ID, name: tc.Function.Name,
					})
				}
			}
			// Vision feedback: tool images ride a user turn (tool role
			// content is text-only in the OpenAI schema), appended once all
			// tool responses are in place.
			for _, v := range vision {
				s.appendToolImage(v)
			}
			toolRounds++
			continue
		}

		// Tool calls with the tool budget exhausted (some models ignore
		// tool_choice: none): execute them anyway so the transcript stays
		// valid, then force a text-only answer next round.
		if len(choice.Message.ToolCalls) > 0 && len(s.tools) > 0 && !opts.ForceNoTools {
			if s.logger.DebugEnabled() {
				s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] round %d: %d tool call(s) after the tool budget — executing them to keep the transcript valid",
					s.label, toolRounds+1, len(choice.Message.ToolCalls)))
			}
			NormalizeToolCallTypes(&choice.Message)
			s.messages = append(s.messages, choice.Message)
			s.appendTranscript(choice.Message)
			for _, tc := range choice.Message.ToolCalls {
				result := s.executeTool(tc)
				s.messages = append(s.messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result.Text,
				})
				s.appendTranscript(ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result.Text,
				})
				if result.ImageBase64 != "" {
					mime := result.ImageMIME
					if mime == "" {
						mime = "image/png"
					}
					s.appendToolImage(visionTurn{
						mime: mime, b64: result.ImageBase64,
						callID: tc.ID, name: tc.Function.Name,
					})
				}
			}
			toolRounds++
			continue
		}

		// Final text reply for this turn.
		s.messages = append(s.messages, choice.Message)
		s.appendTranscript(choice.Message)
		content := ContentString(choice.Message)
		if strings.TrimSpace(content) == "" {
			// One nudge before giving up (mirrors img2text behaviour).
			s.messages = append(s.messages, ChatMessage{
				Role:    "user",
				Content: "Provide your final answer now.",
			})
			s.appendTranscript(ChatMessage{
				Role:    "user",
				Content: "Provide your final answer now.",
			})
			req2 := *req
			req2.Messages = s.messages
			// Keep the tool definitions (the provider builds its prompt
			// prefix from tools + messages, so dropping them would throw
			// away the whole prefix cache); tool_choice=none is enough to
			// force a text answer.
			req2.ToolChoice = "none"
			if s.logger.DebugEnabled() {
				s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] round %d: nudge request (empty reply, tools disabled)",
					s.label, toolRounds+1))
			}
			resp2, sentinel, status := s.client.CallWithRetry(&req2)
			s.APIRequests++
			if status != "" {
				return "", fmt.Errorf("api error: %s", sentinel)
			}
			if len(resp2.Choices) == 0 {
				return "", fmt.Errorf("empty response after nudge")
			}
			if s.logger.DebugEnabled() {
				s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] nudge response (%.1fs, stream=%v finish=%s content=%d chars %s)",
					s.label, resp2.Elapsed.Seconds(), resp2.Streamed, dash(resp2.FinishReason),
					len(ContentString(resp2.Choices[0].Message)), resp2.Usage.String()))
			}
			s.recordUsage(resp2, toolRounds+1, "nudge")
			s.messages = append(s.messages, resp2.Choices[0].Message)
			s.appendTranscript(resp2.Choices[0].Message)
			content = ContentString(resp2.Choices[0].Message)
			if strings.TrimSpace(content) == "" {
				return "", fmt.Errorf("empty final answer")
			}
		}
		return content, nil
	}
}

// executeTool dispatches one tool call, logging and error-wrapping.
func (s *Session) executeTool(tc ToolCall) ToolResult {
	s.ToolInvoked++
	s.notifyProgress()
	for _, t := range s.tools {
		if t.Name() != tc.Function.Name {
			continue
		}
		res, err := t.Execute(tc.Function.Arguments)
		if err != nil {
			// A tool error is handed back to the model (which usually
			// recovers), so it is not a session failure: say so in the log.
			s.logf("[tool:%s] error (已返回模型，会话继续): %v", tc.Function.Name, err)
			return ToolResult{Text: "TOOL ERROR: " + err.Error()}
		}
		s.logf("[tool:%s] ok (%d chars result)", tc.Function.Name, len(res.Text))
		return res
	}
	s.logf("[tool:%s] unknown tool", tc.Function.Name)
	return ToolResult{Text: "TOOL ERROR: unknown tool " + tc.Function.Name}
}

func (s *Session) logf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Log(s.tid, "["+s.label+"] "+fmt.Sprintf(format, args...))
}

// EstimatedTokens reports the current request size estimate. The system
// prompt and the tool definitions are part of every request and they are
// NOT small (a full LaTeX tool set costs ~1.8k tokens), so leaving them out
// underestimated the real prompt by tens of thousands of tokens.
func (s *Session) EstimatedTokens() int {
	total := s.promptTextTokens()
	for _, m := range s.messages {
		total += messageImageTokens(s.modelName(), m)
	}
	return total
}

// promptTextTokens is the text-only half of EstimatedTokens: system prompt,
// tool definitions and every message without its images. The usage line stores
// it so the preview can subtract the text growth from a prompt_tokens delta and
// get the measured per-image cost.
func (s *Session) promptTextTokens() int {
	total := textTokens(s.system)
	if len(s.toolDefs) > 0 {
		if raw, err := json.Marshal(s.toolDefs); err == nil {
			total += textTokens(string(raw))
		}
	}
	for _, m := range s.messages {
		total += messageTextTokens(m)
	}
	return total
}

// modelName is the wire model id this session talks to; it selects the
// per-model image estimate. Empty when the session has no client (hand-built
// sessions in tests) and then falls back to the global estimate.
func (s *Session) modelName() string {
	if s.client == nil {
		return ""
	}
	return s.client.Model()
}

// snapshotRequest records what the request about to be sent contains: the local
// size estimate (for the compaction threshold) plus the text-only estimate and
// the image count that the usage line carries. extraText is request-only
// content that is not in s.messages (the compaction instruction).
func (s *Session) snapshotRequest(extraText string) {
	extra := textTokens(extraText)
	s.estBeforeRequest = s.EstimatedTokens() + extra
	s.reqTextTokens = s.promptTextTokens() + extra
	s.reqImages = 0
	for _, m := range s.messages {
		s.reqImages += messageImageCount(m)
	}
}

// observePromptSize 用厂商返回的真实 prompt_tokens 校准本地估算。
// estBefore 是发请求前的 EstimatedTokens()。厂商侧还有估算看不见的
// 开销（图片按像素编码、每轮包装、reasoning 回传），实测普遍是估算的
// 2~4 倍；不校准就会像现场那样"估算 66k、实际 132k"，压缩阈值永远
// 够不着。系数夹在 [1, 10] 之间，避免单次异常值把阈值推到天上。
func (s *Session) observePromptSize(estBefore, actual int) {
	if actual <= 0 {
		return
	}
	s.lastPromptTokens = actual
	if estBefore <= 0 {
		return
	}
	ratio := float64(actual) / float64(estBefore)
	if ratio < 1 {
		ratio = 1
	}
	if ratio > 10 {
		ratio = 10
	}
	s.calibNum, s.calibDen = ratio, 1
}

// CurrentTokens 是压缩阈值判断使用的规模，取三者最大：
// ① 本地估算；② 标定后的估算（估算 × 实测/估算比）；③ 最近一次厂商实测的
// prompt_tokens。③ 是"下一次请求至少这么大"的硬下限——本地估算漏算的
// 开销（图片按像素编码、每轮包装）再多，厂商的数字不会骗人。裁剪或压缩
// 之后历史真的变小了，这两个值都会被清掉/重算，所以不会把旧的大值钉住。
func (s *Session) CurrentTokens() int {
	est := s.EstimatedTokens()
	best := est
	if s.calibDen > 0 && s.calibNum > 0 {
		if c := int(float64(est) * s.calibNum / s.calibDen); c > best {
			best = c
		}
	}
	if s.lastPromptTokens > best {
		best = s.lastPromptTokens
	}
	return best
}

// forgetMeasuredPromptSize 在历史真的被缩小（本地裁剪 / AI 摘要）之后清掉
// 实测值：那一轮请求的规模已经不代表现在的上下文，留着会让阈值判断虚高。
func (s *Session) forgetMeasuredPromptSize() {
	s.lastPromptTokens = 0
	s.calibNum, s.calibDen = 0, 0
}

// noteContextGrowth logs once at half and once at 80% of the configured
// window, so "the session is getting huge" is visible without --debug.
func (s *Session) noteContextGrowth(est, limit int) {
	if limit <= 0 || est <= 0 {
		return
	}
	pct := est * 100 / limit
	measured := ""
	if s.lastPromptTokens > 0 {
		measured = fmt.Sprintf("（上次请求厂商实测 prompt=%d）", s.lastPromptTokens)
	}
	if s.contextNoteStep < 50 && pct >= 50 {
		s.contextNoteStep = 50
		s.logf("[context] 估算 %d tokens%s = 窗口(%d) 的 %d%%", est, measured, limit, pct)
		return
	}
	if s.contextNoteStep < 80 && pct >= 80 {
		s.contextNoteStep = 80
		s.logf("[context] 估算 %d tokens%s = 窗口(%d) 的 %d%%，接近压缩阈值", est, measured, limit, pct)
	}
}

// pruneHistory shortens the conversation WITHOUT calling the model: long
// tool results are cut to head+tail and older images are replaced by a text
// placeholder (the record of what was inspected survives, the pixels do
// not). It returns how many messages it changed. This is the first of two
// compaction stages — an AI summary is only paid for when local pruning is
// not enough.
func (s *Session) pruneHistory() int {
	maxChars := s.tuning.PruneToolCharsLimit()
	keepImages := s.tuning.KeepImagesCount()
	if maxChars <= 0 && keepImages < 0 {
		return 0
	}
	// Which image-bearing messages are recent enough to keep?
	// (Long tool results are cut to half-head + quarter-tail of the
	// configured budget, so a just-over-threshold result still loses a
	// quarter of its length and a 64 KB read_file loses ~95%.)
	// (Long tool results are cut to half-head + quarter-tail of the
	// configured budget below, so a just-over-threshold result still loses
	// a quarter of its length and a 64 KB read_file loses ~95%.)
	keepFrom := 0
	if keepImages >= 0 {
		seen := 0
		for i := len(s.messages) - 1; i >= 0; i-- {
			if !hasImage(s.messages[i]) {
				continue
			}
			seen++
			if seen > keepImages {
				keepFrom = i + 1
				break
			}
		}
	}
	changed := 0
	for i := range s.messages {
		m := &s.messages[i]
		if maxChars > 0 {
			if text, ok := m.Content.(string); ok && m.Role == "tool" && len(text) > maxChars {
				head := maxChars / 2
				tail := maxChars / 4
				m.Content = fmt.Sprintf("%s\n[... %d characters pruned by docvision (local, no model call) ...]\n%s",
					text[:head], len(text)-head-tail, text[len(text)-tail:])
				changed++
			}
		}
		if keepImages >= 0 && i < keepFrom && hasImage(*m) {
			parts, ok := m.Content.([]map[string]interface{})
			if !ok {
				continue
			}
			var b strings.Builder
			imgs := 0
			for _, part := range parts {
				if t, ok := part["text"].(string); ok && t != "" {
					b.WriteString(t)
					b.WriteString("\n")
					continue
				}
				if _, ok := part["image_url"].(map[string]string); ok {
					imgs++
				}
			}
			note := fmt.Sprintf("[%d image(s) attached here are no longer carried in the context; call view_image/view_pdf again if you need to look at them.]", imgs)
			m.Content = strings.TrimSpace(b.String() + "\n" + note)
			changed++
		}
	}
	return changed
}

// hasImage reports whether a message carries an attached image.
func hasImage(m ChatMessage) bool {
	parts, ok := m.Content.([]map[string]interface{})
	if !ok {
		return false
	}
	for _, part := range parts {
		if _, ok := part["image_url"].(map[string]string); ok {
			return true
		}
	}
	return false
}

// maybeCompact triggers AI-driven compaction when the conversation
// approaches the configured context window. The system prompt is kept;
// the rest of the transcript is replaced by one summary message.
func (s *Session) maybeCompact() error {
	limit := s.tuning.ContextLimit
	if limit <= 0 {
		limit = 131072
	}
	at := s.tuning.CompactionAt
	if at <= 0 || at > 1 {
		at = 0.85
	}
	now := s.CurrentTokens()
	// Surface context growth in the PLAIN log (no --debug needed). The bug
	// this whole guard had was invisible precisely because every number
	// about the request size lived in debug output: the run reached 324k
	// tokens with a 128K window and the log said nothing at all.
	s.noteContextGrowth(now, limit)
	if now < int(float64(limit)*at) {
		return nil
	}
	// 一张小窗口 + 一条本身就超阈值的尾巴（系统提示词/原始任务/最近 8 条）
	// 会让"压完还是超阈值"成为常态。此时若每轮都再花钱摘要一次，就是持续
	// 重复付费且几乎压不动——只有在上次压缩之后又涨了 20% 才值得再压。
	if s.lastCompactTokens > 0 && now < s.lastCompactTokens*6/5 {
		return nil
	}
	// Stage 1: local pruning (free).
	before := now
	if n := s.pruneHistory(); n > 0 {
		now := s.CurrentTokens()
		s.logf("[compact] 本地裁剪：%d 条消息，估算 %d → %d tokens（未调用模型）", n, before, now)
		if now < int(float64(limit)*at) {
			s.Prunes++
			// 注意：这里**不**清掉厂商实测值。裁剪只动本地历史，而实测
			// 值是"上一次请求厂商真的发了多少"——清掉它就会退回"只看
			// 估算"，又变成估算说够小、实则超窗。下一次响应的实测值会
			// 自动刷新成裁剪后的真实规模。
			return nil
		}
	}
	if len(s.messages) <= 2 {
		return nil // nothing to compact beyond the system prompt
	}
	// Stage 2: the conversation is still too large — pay for an AI summary.
	did, err := s.compact()
	if err != nil {
		return err
	}
	if !did {
		// 没有可摘要的中段（任务与保留的最近 N 条之间是空的）：这次不是
		// 一次压缩，不能记账、也不能把 lastCompactTokens 当成压缩后规模。
		s.logf("[compact] 无需摘要（任务与最近 %d 条之间没有内容）；上下文仍偏大", compactKeepTail)
		return nil
	}
	s.Compactions++
	s.forgetMeasuredPromptSize()
	s.lastCompactTokens = s.CurrentTokens()
	s.logf("[compact] history compacted; now ~%d tokens", s.lastCompactTokens)
	return nil
}

// compactedMarker prefixes the replacement note written by compact; it is
// also used to collapse a transcript on resume (see LoadTranscript) — keep
// it as the note's FIRST line so both new and old notes stay detectable.
const compactedMarker = "=== COMPRESSED SESSION CONTEXT"

// checkpointNote renders the compaction replacement note (T21): DSH 式检查点
// ——说明文字 + XML 标签包裹，给继续工作的模型明确语义（这是背景、不要复述、
// 直接接着干），不再只是裸摘要。首行 marker 供 LoadTranscript 的 HasPrefix 折叠。
func checkpointNote(summary string) string {
	return compactedMarker + " ===\n<compacted-summary>\n" +
		"This is an automatically generated checkpoint condensing an earlier span of the conversation to free up context. " +
		"Treat the captured context as established background and build on it without restating it. " +
		"Continue the task directly from the messages that follow, without acknowledging this checkpoint.\n\n" +
		"<summary>\n" + summary + "\n</summary>\n</compacted-summary>"
}

// compactKeepTail is how many of the most recent messages stay verbatim
// across a compaction; everything between the original task and this tail
// is summarised. Keeping real recent turns (instead of a summary only) is
// what makes a resumed session actually continue working.
const compactKeepTail = 8

// compact rewrites the transcript at a turn boundary: the system prompt and
// the original task stay verbatim, the most recent turns stay verbatim, and
// everything in between is replaced by one AI-written continuation note.
//
// The summary request is append-only — it reuses the current conversation
// as-is and adds one instruction — so the provider's prefix cache stays
// warm. The previous implementation embedded the whole transcript into a
// single fresh user message, which paid full price for a new (often
// 100k+ token) prefix and re-sent every base64 image.
// compact 用一次 AI 摘要替换"任务与最近若干条之间"的中段历史。返回值
// 表示**真的压了**（false = 会话还短、没有可摘要的中段，调用方不得按
// 已压缩记账）。
func (s *Session) compact() (bool, error) {
	head := 0
	if len(s.messages) > 0 && s.messages[0].Role == "system" {
		head = 1
	}
	// Keep the original task turn verbatim right after the system prompt.
	headEnd := head
	if headEnd < len(s.messages) && s.messages[headEnd].Role == "user" {
		headEnd++
	}
	tailStart := len(s.messages) - compactKeepTail
	if tailStart < headEnd {
		tailStart = headEnd
	}
	// Never start the kept tail on a tool result: its assistant tool_calls
	// turn must come with it, otherwise the replayed history is invalid.
	for tailStart > headEnd && s.messages[tailStart].Role == "tool" {
		tailStart--
	}
	if tailStart-headEnd <= 0 {
		// 会话还短（任务与"保留的最近 N 条"之间没有内容）：不做摘要。
		// 返回 false 而不是 nil——调用方曾无条件 Compactions++，于是
		// "压缩次数"统计里混进了根本没发生的压缩。
		return false, nil
	}
	tail := make([]ChatMessage, len(s.messages)-tailStart)
	copy(tail, s.messages[tailStart:])

	instruction := strings.Join([]string{
		"=== CONTEXT COMPACTION REQUEST ===",
		"Summarise the work done so far in this session as a COMPACT but COMPLETE continuation note",
		fmt.Sprintf("covering everything up to (but NOT including) the last %d messages.", len(tail)),
		"Rules:",
		"- Keep: the task definition, all confirmed decisions, final artefacts (the exact code, commands, file paths, ids, numbers already produced), current progress and the remaining steps.",
		"- Drop: intermediate tool dumps, failed drafts, verbose reasoning, duplicate content.",
		"- Keep exact strings (code, identifiers, paths) that are still referenced later.",
		"- Do NOT summarise or repeat the last messages: they stay in the conversation as-is.",
		"Respond with the note only, no preamble.",
	}, "\n")

	s.snapshotRequest(instruction)
	req := &ChatRequest{
		Model:       s.client.Model(),
		Messages:    append(append([]ChatMessage{}, s.messages...), ChatMessage{Role: "user", Content: instruction}),
		MaxTokens:   8192,
		Temperature: 0.1,
	}
	if s.logger.DebugEnabled() {
		s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] [compact] summary request (append-only, %s messages=%d keep_tail=%d)",
			s.label, s.client.RequestSummary(req), len(req.Messages), len(tail)))
	}
	resp, sentinel, status := s.client.CallWithRetry(req)
	if status != "" {
		return false, fmt.Errorf("summary request failed: %s", sentinel)
	}
	if len(resp.Choices) == 0 {
		return false, fmt.Errorf("empty summary response")
	}
	summary := ContentString(resp.Choices[0].Message)
	if strings.TrimSpace(summary) == "" {
		return false, fmt.Errorf("empty summary content")
	}
	if s.logger.DebugEnabled() {
		s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] [compact] summary response (%.1fs, %d chars %s)",
			s.label, resp.Elapsed.Seconds(), len(summary), resp.Usage.String()))
	}

	s.recordUsage(resp, s.rounds, "compact")
	note := checkpointNote(summary)
	rebuilt := make([]ChatMessage, 0, headEnd+1+len(tail))
	rebuilt = append(rebuilt, s.messages[:headEnd]...)
	rebuilt = append(rebuilt, ChatMessage{Role: "user", Content: note})
	rebuilt = append(rebuilt, tail...)
	s.messages = rebuilt
	s.appendTranscript(ChatMessage{Role: "user", Content: note})
	// On disk the tail arrived BEFORE this note (messages are appended live
	// as they happen), and LoadTranscript drops everything older than the
	// newest note — so a resumed session lost exactly the two things
	// compaction promised to keep: the original task and the recent turns.
	// Re-append both AFTER the note so the replayed history equals the
	// in-memory one (the system prompt is excluded: SetMessages re-inserts
	// it at the front, and a system line mid-conversation would break it).
	for _, m := range s.messages[:headEnd] {
		if m.Role == "system" {
			continue
		}
		s.appendTranscript(m)
	}
	for _, m := range tail {
		s.appendTranscript(m)
	}
	return true, nil
}

// messageTokens estimates what one message costs under the rule that applies to
// `model`: its text plus one estimate per attached image. The image half needs
// the model because vision endpoints bill images differently (fixed per image
// vs. scaling with the pixel grid), so the rule is per model.
//
// messageTokens 必须覆盖请求体里真正发出去的一切：除了 Content，
// 历史里回传的 reasoning_content（GLM 保留式思考）与 tool_calls 的
// 函数名/参数 JSON 同样占 token——它们曾经完全不计入，是估算偏低
// 数倍的主因之一。
func messageTokens(m ChatMessage, model string) int {
	return messageTextTokens(m) + messageImageTokens(model, m)
}

// messageTextTokens is the text half of one message.
func messageTextTokens(m ChatMessage) int {
	total := 0
	switch c := m.Content.(type) {
	case string:
		total += textTokens(c)
	case []map[string]interface{}:
		for _, part := range c {
			if t, ok := part["type"].(string); ok && t == "image_url" {
				continue // images are counted by messageImageTokens
			}
			if t, ok := part["text"].(string); ok {
				total += textTokens(t)
			}
		}
	default:
		if raw, err := json.Marshal(m.Content); err == nil {
			total += textTokens(string(raw))
		}
	}
	total += textTokens(m.ReasoningContent)
	for _, tc := range m.ToolCalls {
		total += textTokens(tc.Function.Name) + textTokens(tc.Function.Arguments)
	}
	return total
}

// messageImageCount is how many images one message carries.
func messageImageCount(m ChatMessage) int {
	n := 0
	if parts, ok := m.Content.([]map[string]interface{}); ok {
		for _, part := range parts {
			if t, ok := part["type"].(string); ok && t == "image_url" {
				n++
			}
		}
	}
	return n
}

// messageImageTokens is the local estimate of every image of one message under
// the rule that applies to `model`: the per-image value computed when the image
// was attached (from its dimensions), or the rule's constant when the message
// carries none (hand-built messages, transcripts from before the field existed).
func messageImageTokens(model string, m ChatMessage) int {
	est := EstimateForModel(model)
	total := 0
	idx := 0
	if parts, ok := m.Content.([]map[string]interface{}); ok {
		for _, part := range parts {
			if t, ok := part["type"].(string); ok && t == "image_url" {
				if idx < len(m.ImageTokens) && m.ImageTokens[idx] > 0 {
					total += m.ImageTokens[idx]
				} else {
					total += est.PerImage(0, 0)
				}
				idx++
			}
		}
	}
	return total
}

func textTokens(s string) int {
	cjk := 0
	other := 0
	for _, r := range s {
		if r > 0x2E00 {
			cjk++
		} else {
			other++
		}
	}
	return cjk + other/4 + 8
}

// EffectiveToolRounds returns the concrete tool-round budget for a
// tuning block. Values <= 0 mean UNLIMITED (the user explicitly opted
// out of a cap; there is no hidden safety limit).
func EffectiveToolRounds(t config.SessionTuning) int {
	return t.MaxToolRounds
}

// dash renders an empty string as "-" for log lines.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// truncateForLog bounds a string for debug log lines.
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("...(truncated, %d bytes total)", len(s))
}
