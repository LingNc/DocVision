package session

import (
	"encoding/json"
	"fmt"
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

	// progressHook, when set, is notified after every API round and
	// every tool execution with (completed rounds, executed tool
	// calls) — used by compact console modes to show a live line.
	progressHook func(rounds, tools int)
	rounds       int

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
func (s *Session) SetTranscript(w *TranscriptWriter) { s.transcript = w }

// appendTranscript writes one message to the transcript if attached.
// Errors are logged (debug) but never fail the session itself.
func (s *Session) appendTranscript(msg ChatMessage) {
	if s.transcript == nil {
		return
	}
	if err := s.transcript.Append(msg); err != nil && s.logger != nil {
		s.logger.Debug(s.tid, "[session:"+s.label+"] transcript 写入失败:", err)
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
	return s
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

// Run processes one logical user turn: sends the message, executes any
// tool calls, feeds results back, and repeats until the model produces
// a final text answer or the tool budget is exhausted (at which point
// it forces a no-tools final reply). Returns the final assistant text.
func (s *Session) Run(opts RunOptions) (string, error) {
	// Context window guard BEFORE growing the conversation further.
	if err := s.maybeCompact(); err != nil {
		return "", fmt.Errorf("compaction failed: %w", err)
	}

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

	for {
		req := &ChatRequest{
			Model:       s.client.Model(),
			Messages:    s.messages,
			MaxTokens:   s.tuning.MaxTokens,
			Temperature: s.tuning.Temperature,
		}
		useTools := len(s.tools) > 0 && !opts.ForceNoTools && (maxRounds <= 0 || toolRounds < maxRounds)
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

		if len(choice.Message.ToolCalls) > 0 && useTools {
			NormalizeToolCallTypes(&choice.Message)
			s.messages = append(s.messages, choice.Message)
			s.appendTranscript(choice.Message)

			// Tool-produced images are delivered AFTER every tool message:
			// the OpenAI schema requires the messages directly following an
			// assistant tool_calls turn to be the matching tool responses,
			// so an interleaved user turn makes the next request fail with
			// "insufficient tool messages following tool_calls message".
			type visionTurn struct{ mime, b64 string }
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
					vision = append(vision, visionTurn{mime: mime, b64: result.ImageBase64})
				}
			}
			// Vision feedback: tool images ride a user turn (tool role
			// content is text-only in the OpenAI schema), appended once all
			// tool responses are in place.
			for _, v := range vision {
				s.messages = append(s.messages, ChatMessage{
					Role: "user",
					Content: []map[string]interface{}{
						{"type": "text", "text": "Tool image output (for your visual review):"},
						{"type": "image_url", "image_url": map[string]string{
							"url": "data:" + v.mime + ";base64," + v.b64,
						}},
					},
				})
				s.appendTranscript(ChatMessage{
					Role: "user",
					Content: []map[string]interface{}{
						{"type": "text", "text": "Tool image output (for your visual review):"},
						{"type": "image_url", "image_url": map[string]string{
							"url": "data:" + v.mime + ";base64," + v.b64,
						}},
					},
				})
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
					s.messages = append(s.messages, ChatMessage{
						Role: "user",
						Content: []map[string]interface{}{
							{"type": "text", "text": "Tool image output (for your visual review):"},
							{"type": "image_url", "image_url": map[string]string{
								"url": "data:" + mime + ";base64," + result.ImageBase64,
							}},
						},
					})
					s.appendTranscript(ChatMessage{
						Role: "user",
						Content: []map[string]interface{}{
							{"type": "text", "text": "Tool image output (for your visual review):"},
							{"type": "image_url", "image_url": map[string]string{
								"url": "data:" + mime + ";base64," + result.ImageBase64,
							}},
						},
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

// EstimatedTokens reports the current conversation size estimate.
func (s *Session) EstimatedTokens() int {
	total := 0
	for _, m := range s.messages {
		total += messageTokens(m)
	}
	return total
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
	if s.EstimatedTokens() < int(float64(limit)*at) {
		return nil
	}
	if len(s.messages) <= 2 {
		return nil // nothing to compact beyond the system prompt
	}
	if err := s.compact(); err != nil {
		return err
	}
	s.Compactions++
	s.logf("[compact] history compacted; now ~%d tokens", s.EstimatedTokens())
	return nil
}

// compactedMarker prefixes the replacement note written by compact; it is
// also used to collapse a transcript on resume (see LoadTranscript).
const compactedMarker = "=== COMPRESSED SESSION CONTEXT"

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
func (s *Session) compact() error {
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
		return nil // nothing between task and tail worth summarising
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
		return fmt.Errorf("summary request failed: %s", sentinel)
	}
	if len(resp.Choices) == 0 {
		return fmt.Errorf("empty summary response")
	}
	summary := ContentString(resp.Choices[0].Message)
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("empty summary content")
	}
	if s.logger.DebugEnabled() {
		s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] [compact] summary response (%.1fs, %d chars %s)",
			s.label, resp.Elapsed.Seconds(), len(summary), resp.Usage.String()))
	}

	note := compactedMarker + " (auto-generated; the earlier turns were summarised) ===\n" + summary
	rebuilt := make([]ChatMessage, 0, headEnd+1+len(tail))
	rebuilt = append(rebuilt, s.messages[:headEnd]...)
	rebuilt = append(rebuilt, ChatMessage{Role: "user", Content: note})
	rebuilt = append(rebuilt, tail...)
	s.messages = rebuilt
	s.appendTranscript(ChatMessage{Role: "user", Content: note})
	return nil
}

// imageTokens is the flat cost assigned to each attached image.
const imageTokens = 1100

// messageTokens estimates the token cost of one message: CJK runes
// count roughly one token each, other text four characters per token,
// plus a flat cost per attached image.
func messageTokens(m ChatMessage) int {
	switch c := m.Content.(type) {
	case string:
		return textTokens(c)
	case []map[string]interface{}:
		total := 0
		for _, part := range c {
			if t, ok := part["type"].(string); ok && t == "image_url" {
				total += imageTokens
				continue
			}
			if t, ok := part["text"].(string); ok {
				total += textTokens(t)
			}
		}
		return total
	default:
		raw, err := json.Marshal(m.Content)
		if err != nil {
			return 0
		}
		return textTokens(string(raw))
	}
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
