package img2text

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// systemPromptTemplate is the body of the system prompt sent to the model.
// {MAX_TOOL_CALLS} and {OUTPUT_LANG} are placeholders substituted in
// BuildSystemPrompt before each call. The wording is the same as the
// Python SYSTEM_PROMPT constant.
const systemPromptTemplate = `You are a document image analyst. Describe images from  technical Chinese textbook as structured, machine-readable content.

## Priority: Correctness > Completeness > Conciseness
First ensure you understand the image content. Then ensure correctness by verifying image content (labels, arrows, values) against surrounding text. If window insufficient or content is unclear, call get_more_context. Then be exhaustive (every visible element). Finally trim redundancy.

## Core rule: describe WHAT is visible, never WHY/HOW.
If ambiguous or overly complex, call get_more_context to resolve; if still unclear, mark [?] and describe only what is certain. No guessing.

## Rules:
1. **Identify image type**: Table, Flowchart, Gantt Chart, Architecture/Network Diagram, Graph/Chart, Formula, Code screenshot, or Simple illustration.
2. **Mermaid** for flowcharts, Gantt charts, sequence diagrams, class diagrams, state diagrams, ER diagrams, mind maps, timeline, Sankey, pie charts, quadrant charts, requirement diagrams. Use ` + "```mermaid code block." + `
3. **Markdown table** for tabular data: ALL rows and columns exactly as shown.
4. **LaTeX** for formulas: $$...$$ block or $...$ inline.
5. **Structured text** for diagrams not suitable for Mermaid: preserve ALL labels, arrows, relationships shown.
6. **Code block** for code screenshots.
7. **Graph description**: key data points, max/min, trends for charts.
8. **Be EXHAUSTIVE**: every visible text, number, label. No summary.

## Output format:
[IMG_TYPE: <type>]
<description / mermaid / table / latex>

## CRITICAL
Your response MUST start exactly with "[IMG_TYPE:" (no extra text before), NEVER omit it. Do NOT include any introductory phrases, conversational text, meta-commentary, or analysis. "Do NOT write \"The image shows\", \"This diagram illustrates\", or any similar analysis."
Never output XML tags like <tool_call> or <function=>.

## Tool: get_more_context
Start with a small context window. To get more, call get_more_context(more_above=N, more_below=M) — N and M are additional lines.
You receive only the delta. Max {MAX_TOOL_CALLS} calls.
Each subsequent call must request STRICTLY MORE lines than the previous call in at least one direction (above or below).

## LANGUAGE
Respond in {OUTPUT_LANG}.
`

// Status constants returned to the runner. They match the Python
// return tuple ("ok", "error", "retry") so the existing log analyser
// can classify them.
const (
	StatusOK         = "ok"
	StatusError      = "error"
	StatusRetry      = "retry"
	sentinelEmpty    = "[IMG_EMPTY_RESPONSE]"
	sentinelInvalid  = "[IMG_INVALID_FORMAT]"
	sentinelRate     = "[IMG_RATE_LIMIT_EXCEEDED]"
	sentinelConnTO   = "[IMG_CONNECTION_TIMEOUT]"
)

// BuildSystemPrompt returns the system prompt with the two placeholders
// filled in. The Python reference substitutes them exactly once before
// the run starts; we mirror that semantics.
func BuildSystemPrompt(maxToolCalls int, lang string) string {
	if lang == "" {
		lang = "Chinese"
	}
	prompt := systemPromptTemplate
	prompt = strings.ReplaceAll(prompt, "{MAX_TOOL_CALLS}", fmt.Sprintf("%d", maxToolCalls))
	prompt = strings.ReplaceAll(prompt, "{OUTPUT_LANG}", lang)
	return prompt
}

// CallAIWithTools is the core multi-round tool-calling loop. It mirrors
// the Python reference precisely:
//
//   - customUserText mode: one-shot call with no tools, used for the
//     format-fix retry. Skips tool calling entirely.
//   - normal mode: up to maxToolRounds+1 chat-completion calls. While
//     rounds < maxToolRounds the model may call get_more_context to
//     expand the window. On the final round tools are disabled so the
//     model is forced to commit to an answer.
//   - Rate-limit backoff: 2*2^n seconds, capped at 60. Unlimited retries
//     when rateLimitRetries == 0.
//   - Connection-timeout backoff: 5*2^n seconds, capped at 60.
//
// Returns the final assistant text and a status string. The status
// matches the Python return tuple (result, status) where "ok" means a
// usable response was produced and "error" / "retry" indicate a
// sentinel-style failure.
func CallAIWithTools(
	client *AIClient,
	imgBase64 string,
	lines []string,
	imgLineIdx int,
	logger *logger.Logger,
	tid int,
	opts config.OptionsConfig,
	customUserText string,
) (string, string) {
	rateLimitLimit := 100
	if opts.RateLimitRetries > 0 {
		rateLimitLimit = opts.RateLimitRetries
	}
	maxAPIRetries := opts.APIMaxRetries
	if maxAPIRetries <= 0 {
		maxAPIRetries = 3
	}
	maxRounds := opts.MaxRetries
	if maxRounds <= 0 {
		maxRounds = 3
	}

	// Mode 1: format-fix / custom user text — single shot, no tools.
	if customUserText != "" {
		req := &ChatRequest{
			Messages: []ChatMessage{
				{Role: "system", Content: BuildSystemPrompt(opts.MaxRetries, opts.OutputLanguage)},
				{Role: "user", Content: []map[string]interface{}{
					{"type": "text", "text": customUserText},
					{"type": "image_url", "image_url": map[string]string{
						"url": "data:image/jpeg;base64," + imgBase64,
					}},
				}},
			},
			MaxTokens:      opts.MaxTokens,
			Temperature:    opts.Temperature,
			Stream:         false,
			EnableThinking: boolPtr(client.EnableThinking()),
		}
		content, status := doCallWithRetry(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
		if status != "" {
			return content, status
		}
		return content, StatusOK
	}

	// Mode 2: incremental context expansion with tool calls.
	tools := BuildTools(opts.MaxWindowUp, opts.MaxWindowDown)
	curUp := opts.MaxContextLinesUp
	curDown := opts.MaxContextLinesDown
	if curUp <= 0 {
		curUp = 3
	}
	if curDown <= 0 {
		curDown = 10
	}

	parts := GetContextLines(lines, imgLineIdx, curUp, curDown)
	ctxText := strings.Join(parts, "\n")

	messages := []ChatMessage{
		{Role: "system", Content: BuildSystemPrompt(opts.MaxRetries, opts.OutputLanguage)},
		{Role: "user", Content: []map[string]interface{}{
			{"type": "text", "text": fmt.Sprintf(
				"The image to describe is at line %d. "+
					"Context: [%d to %d] (%d↑, %d↓).\n"+
					"```\n%s\n```\n"+
					"Understand image before output. If confused, call get_more_context(↑N, ↓M) — max per call: ↑%d, ↓%d.",
				imgLineIdx, imgLineIdx-curUp, imgLineIdx+curDown, curUp, curDown,
				ctxText, opts.MaxWindowUp, opts.MaxWindowDown,
			)},
			{"type": "image_url", "image_url": map[string]string{
				"url": "data:image/jpeg;base64," + imgBase64,
			}},
		}},
	}

	for round := 0; round < maxRounds+1; round++ {
		req := &ChatRequest{
			Messages:       messages,
			MaxTokens:      opts.MaxTokens,
			Temperature:    opts.Temperature,
			Stream:         false,
			EnableThinking: boolPtr(client.EnableThinking()),
		}
		if round < maxRounds {
			req.Tools = tools
			req.ToolChoice = "auto"
		} else {
			// Forced final round: no tools, force commitment.
			req.ToolChoice = "none"
		}

		resp, errSentinel, status := doCallWithRetryFull(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
		if status != "" {
			return errSentinel, status
		}
		if len(resp.Choices) == 0 {
			return sentinelEmpty, StatusError
		}
		choice := resp.Choices[0]

		if len(choice.Message.ToolCalls) == 0 {
			return contentString(choice.Message), StatusOK
		}

		// Append the assistant turn (with tool_calls) verbatim.
		messages = append(messages, choice.Message)

		// Walk every tool call in this turn. Multiple get_more_context
		// calls in one turn are possible (the Python reference handles
		// them by appending one tool response per call).
		for _, tc := range choice.Message.ToolCalls {
			if tc.Function.Name != "get_more_context" {
				continue
			}
			var args struct {
				MoreAbove int `json:"more_above"`
				MoreBelow int `json:"more_below"`
			}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				logger.LogWarning(tid, "  [ToolCall] invalid arguments:", err)
				continue
			}

			// Per-request cap; no cumulative cap across rounds.
			actualUp := args.MoreAbove
			if actualUp > opts.MaxWindowUp {
				actualUp = opts.MaxWindowUp
			}
			actualDown := args.MoreBelow
			if actualDown > opts.MaxWindowDown {
				actualDown = opts.MaxWindowDown
			}
			newUp := curUp + actualUp
			newDown := curDown + actualDown

			logger.Log(tid, fmt.Sprintf(
				"  [ToolCall] AI wants +%dup/-%ddown -> window %d/%d>%d/%d (per-request max=%d/%d)",
				args.MoreAbove, args.MoreBelow,
				curUp, curDown, newUp, newDown,
				opts.MaxWindowUp, opts.MaxWindowDown,
			))

			delta := GetDeltaLines(lines, imgLineIdx, curUp, curDown, newUp, newDown)

			var resultText string
			if len(delta) > 0 {
				resultText = fmt.Sprintf(
					"Added %d lines above and %d lines below. "+
						"Window is now [%d to %d].\n\nNEW content (delta only):\n\n%s",
					actualUp, actualDown,
					imgLineIdx-newUp, imgLineIdx+newDown,
					strings.Join(delta, "\n"),
				)
			} else {
				resultText = fmt.Sprintf(
					"No new lines could be added. Window remains [%d to %d]. "+
						"Please proceed with your best analysis.",
					imgLineIdx-curUp, imgLineIdx+curDown,
				)
			}
			curUp, curDown = newUp, newDown

			toolMsg := ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    resultText,
			}
			messages = append(messages, toolMsg)
		}
	}

	// Forced final attempt: out of tool rounds. Ask for a best-effort
	// response without tools.
	messages = append(messages, ChatMessage{
		Role:    "user",
		Content: "Provide your best analysis now.",
	})
	req := &ChatRequest{
		Messages:       messages,
		MaxTokens:      opts.MaxTokens,
		Temperature:    opts.Temperature,
		Stream:         false,
		EnableThinking: boolPtr(client.EnableThinking()),
	}
	resp, errSentinel, status := doCallWithRetryFull(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
	if status != "" {
		return errSentinel, status
	}
	if len(resp.Choices) == 0 {
		return sentinelEmpty, StatusError
	}
	content := contentString(resp.Choices[0].Message)
	if content == "" {
		return sentinelEmpty, StatusError
	}
	return content, StatusOK
}

// doCallWithRetry issues a single chat completion and returns the
// assistant text plus an empty status on success. On a transport-level
// error it performs the bounded exponential backoff loop from the
// Python reference; when retries are exhausted it returns the
// appropriate sentinel and a non-empty status.
//
// The status is "" on success; otherwise it is StatusError and the
// returned text is a sentinel ([IMG_RATE_LIMIT_EXCEEDED],
// [IMG_CONNECTION_TIMEOUT], [IMG_API_ERROR: ...], [IMG_EMPTY_RESPONSE]).
func doCallWithRetry(
	client *AIClient,
	req *ChatRequest,
	maxAPIRetries, rateLimitLimit int,
	logger *logger.Logger,
	tid int,
) (string, string) {
	resp, sentinel, status := doCallWithRetryFull(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
	if status != "" {
		return sentinel, status
	}
	if len(resp.Choices) == 0 {
		return sentinelEmpty, StatusError
	}
	return contentString(resp.Choices[0].Message), ""
}

// doCallWithRetryFull is the loop body used by both call sites. It
// returns the full ChatResponse on success, or a sentinel string +
// StatusError when retries are exhausted.
func doCallWithRetryFull(
	client *AIClient,
	req *ChatRequest,
	maxAPIRetries, rateLimitLimit int,
	logger *logger.Logger,
	tid int,
) (*ChatResponse, string, string) {
	retry := 0
	rateLimitRetry := 0
	for {
		resp, err := client.ChatCompletion(req)
		if err == nil {
			return resp, "", ""
		}

		errStr := err.Error()
		errLower := strings.ToLower(errStr)

		// Rate limit: 429, "rate" in message.
		if strings.Contains(errStr, "429") || strings.Contains(errLower, "rate") {
			if rateLimitRetry < rateLimitLimit {
				wait := time.Duration(1<<rateLimitRetry) * 2 * time.Second
				if wait > 60*time.Second {
					wait = 60 * time.Second
				}
				logger.LogWarning(tid, "  [RateLimit] waiting", wait)
				time.Sleep(wait)
				rateLimitRetry++
				continue
			}
			return nil, sentinelRate, StatusError
		}

		// Connection/timeout: 5s, 10s, 20s, ..., max 60s.
		if containsAny(errLower, "connect", "timeout", "handshake", "timed out") {
			if retry < maxAPIRetries {
				wait := time.Duration(1<<retry) * 5 * time.Second
				if wait > 60*time.Second {
					wait = 60 * time.Second
				}
				logger.LogWarning(tid, "  [ConnRetry] waiting", wait)
				time.Sleep(wait)
				retry++
				continue
			}
			return nil, sentinelConnTO, StatusError
		}

		// Generic API error: short fixed backoff, up to maxAPIRetries.
		if retry < maxAPIRetries {
			time.Sleep(2 * time.Second)
			retry++
			continue
		}
		return nil, fmt.Sprintf("[IMG_API_ERROR: %s]", truncate(errStr, 200)), StatusError
	}
}

// contentString extracts the assistant text from a ChatMessage,
// supporting both the simple string case and the structured content
// slice used for multimodal turns.
func contentString(m ChatMessage) string {
	switch c := m.Content.(type) {
	case string:
		return c
	case nil:
		return ""
	default:
		// Fall back to JSON encoding; the model's content is normally
		// a string but other providers may return a structured body.
		raw, err := json.Marshal(c)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ProcessOneImage loads the image, calls CallAIWithTools, and validates
// the [IMG_TYPE:] prefix. On missing prefix the format-fix path is
// attempted up to formatFixAttempts times. Returns the (result, status)
// tuple matching the Python reference.
func ProcessOneImage(
	client *AIClient,
	imagesDir, imgPath string,
	lines []string,
	imgLineIdx int,
	logger *logger.Logger,
	tid int,
	opts config.OptionsConfig,
) (string, string) {
	imgFile, err := resolveImageFile(imagesDir, imgPath)
	if err != nil {
		return fmt.Sprintf("[IMG_MISSING: %s]", imgPath), StatusError
	}
	imgBase64, err := ImageToBase64(imgFile, 1280)
	if err != nil {
		return fmt.Sprintf("[IMG_ERROR: %s - %v]", imgPath, err), StatusError
	}

	result, status := CallAIWithTools(
		client, imgBase64, lines, imgLineIdx,
		logger, tid, opts, "",
	)
	if status != StatusOK {
		return result, status
	}
	result = strings.TrimSpace(result)

	// Look for the [IMG_TYPE: prefix anywhere in the result. If the
	// model added leading prose we drop it and warn (Python does the
	// same with log_warning).
	if idx := strings.Index(result, "[IMG_TYPE:"); idx >= 0 {
		if idx > 0 {
			prefix := result[:idx]
			logger.LogWarning(tid,
				"Unexpected prefix before '[IMG_TYPE:' in", imgPath+":",
				strings.TrimSpace(prefix)[:min(80, len(strings.TrimSpace(prefix)))])
		}
		return strings.TrimSpace(result[idx:]), StatusOK
	}

	// Missing [IMG_TYPE:. If the response is already a system error
	// sentinel, return it as-is.
	if strings.HasPrefix(result, "[IMG_") {
		return result, StatusError
	}

	// Try the format fix.
	if opts.FormatFixAttempts > 0 {
		logger.LogWarning(tid, "Format fix triggered for", imgPath)
		fixMsg := fmt.Sprintf(
			`Your previous response was REJECTED because it did NOT start with "[IMG_TYPE: <type>]".`+"\n"+
				"Here is your previous response (for reference only):\n"+
				"---\n%s\n---\n\n"+
				"Start EXACTLY with \"[IMG_TYPE:\" followed by the type, then the pure description (mermaid/table/latex/text). "+
				"Do NOT write \"The image shows\", \"This diagram illustrates\", or any similar analysis.",
			result,
		)
		fixed, fixStatus := CallAIWithTools(
			client, imgBase64, nil, 0,
			logger, tid, opts, fixMsg,
		)
		if fixStatus != StatusOK && !strings.HasPrefix(fixed, "[IMG_") {
			return fixed, fixStatus
		}
		fixed = strings.TrimSpace(fixed)
		if idx := strings.Index(fixed, "[IMG_TYPE:"); idx >= 0 {
			if idx > 0 {
				logger.LogWarning(tid, "Format fix had extra prefix in", imgPath)
			}
			return strings.TrimSpace(fixed[idx:]), StatusOK
		}
		prefix := result
		if len(prefix) > 100 {
			prefix = prefix[:100]
		}
		logger.LogError(tid,
			"Format fix still missing [IMG_TYPE:] for", imgPath+":", prefix)
		return sentinelInvalid, StatusRetry
	}

	prefix := result
	if len(prefix) > 100 {
		prefix = prefix[:100]
	}
	logger.LogError(tid, "No '[IMG_TYPE:' found in result from", imgPath+":", prefix)
	return sentinelInvalid, StatusRetry
}

// resolveImageFile maps an "images/..." reference from the markdown into
// an absolute path under imagesDir. The Python reference splits the path
// on the first "/" and joins with images_dir; we mirror that semantics.
func resolveImageFile(imagesDir, imgPath string) (string, error) {
	rel := imgPath
	if i := strings.Index(imgPath, "/"); i >= 0 {
		rel = imgPath[i+1:]
	}
	full := filepath.Join(imagesDir, rel)
	if _, err := os.Open(full); err != nil {
		return "", err
	}
	return full, nil
}

// boolPtr is a tiny helper to take the address of a bool literal/value.
func boolPtr(b bool) *bool { return &b }
