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
			Stream:      false,
		}
		useTools := len(s.tools) > 0 && !opts.ForceNoTools && (maxRounds <= 0 || toolRounds < maxRounds)
		if useTools {
			req.Tools = s.toolDefs
			req.ToolChoice = "auto"
		} else if len(s.tools) > 0 && !opts.ForceNoTools {
			// Budget exhausted: force a text-only final answer.
			req.ToolChoice = "none"
		}

		if s.logger.DebugEnabled() {
			s.logger.Debug(s.tid, fmt.Sprintf("[session:%s] round %d: api request (messages=%d, est_tokens=%d, tools=%v)",
				s.label, toolRounds+1, len(s.messages), s.EstimatedTokens(), useTools))
		}
		resp, sentinel, status := s.client.CallWithRetry(req)
		s.APIRequests++
		if status != "" {
			return "", fmt.Errorf("api error: %s", sentinel)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response (no choices)")
		}
		choice := resp.Choices[0]

		if len(choice.Message.ToolCalls) > 0 && useTools {
			NormalizeToolCallTypes(&choice.Message)
			s.messages = append(s.messages, choice.Message)

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
				// Vision feedback: deliver tool-produced images as an
				// attached user turn (tool role content is text-only in
				// the OpenAI schema).
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
				}
			}
			toolRounds++
			continue
		}

		// Final text reply for this turn.
		s.messages = append(s.messages, choice.Message)
		content := ContentString(choice.Message)
		if strings.TrimSpace(content) == "" {
			// One nudge before giving up (mirrors img2text behaviour).
			s.messages = append(s.messages, ChatMessage{
				Role:    "user",
				Content: "Provide your final answer now.",
			})
			req2 := *req
			req2.Messages = s.messages
			req2.ToolChoice = "none"
			req2.Tools = nil
			resp2, sentinel, status := s.client.CallWithRetry(&req2)
			s.APIRequests++
			if status != "" {
				return "", fmt.Errorf("api error: %s", sentinel)
			}
			if len(resp2.Choices) == 0 {
				return "", fmt.Errorf("empty response after nudge")
			}
			s.messages = append(s.messages, resp2.Choices[0].Message)
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
	for _, t := range s.tools {
		if t.Name() != tc.Function.Name {
			continue
		}
		res, err := t.Execute(tc.Function.Arguments)
		if err != nil {
			s.logf("[tool:%s] error: %v", tc.Function.Name, err)
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

// compact rewrites the transcript: everything after the system prompt
// is summarised by the model itself into a compact context note.
func (s *Session) compact() error {
	var sb strings.Builder
	for _, m := range s.messages {
		if m.Role == "system" {
			continue
		}
		switch m.Role {
		case "user":
			sb.WriteString("## USER\n")
		case "assistant":
			sb.WriteString("## ASSISTANT\n")
		case "tool":
			sb.WriteString("## TOOL RESULT\n")
		}
		text := ContentString(m)
		if len(text) > 4000 {
			text = text[:2000] + "\n...[truncated]...\n" + text[len(text)-1500:]
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	summaryPrompt := strings.Join([]string{
		"You are compressing a long AI working session so it can continue within a limited context window.",
		"Produce a COMPACT but COMPLETE continuation note. Rules:",
		"- Keep: the task definition, all confirmed decisions, final artefacts (code, data, results that will still be needed), current progress and remaining steps.",
		"- Drop: intermediate tool dumps, failed drafts, verbose reasoning, duplicate content.",
		"- Keep any exact strings (code, identifiers, paths) that are still referenced.",
		"Respond with the note only, no preamble.",
		"",
		"=== SESSION TRANSCRIPT ===",
		sb.String(),
	}, "\n")

	req := &ChatRequest{
		Model: s.client.Model(),
		Messages: []ChatMessage{
			{Role: "user", Content: summaryPrompt},
		},
		MaxTokens:   8192,
		Temperature: 0.1,
		Stream:      false,
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

	note := "=== COMPRESSED SESSION CONTEXT (auto-generated; earlier turns were summarised) ===\n" + summary
	s.messages = []ChatMessage{}
	if s.system != "" {
		s.messages = append(s.messages, ChatMessage{Role: "system", Content: s.system})
	}
	s.messages = append(s.messages, ChatMessage{Role: "user", Content: note})
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
