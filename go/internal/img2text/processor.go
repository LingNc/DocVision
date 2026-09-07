package img2text

import (
	"context"
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
4b. **LaTeX vector graphics** for figures that need precise vector rendering and no Mermaid type fits: TikZ, pgfplots (function/coordinate plots), tabular/array, or any LaTeX approach that reproduces the structure faithfully — use a latex code block (three-backtick latex fence) with a standalone-compatible body. Division of labour: Mermaid for the listed diagram types, LaTeX for everything else (geometry, plots, complex tables, mixed structures).
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
	StatusOK        = "ok"
	StatusError     = "error"
	StatusRetry     = "retry"
	sentinelEmpty   = "[IMG_EMPTY_RESPONSE]"
	sentinelInvalid = "[IMG_INVALID_FORMAT]"
	sentinelRate    = "[IMG_RATE_LIMIT_EXCEEDED]"
	sentinelConnTO  = "[IMG_CONNECTION_TIMEOUT]"
	sentinelMermaid = "[IMG_MERMAID_INVALID]"
)

// mermaidFixSafetyCap is the upper bound applied when the user opts
// into "unlimited" mermaid repair attempts by setting
// MermaidFixAttempts to 0. It prevents an infinite loop on a model that
// never produces valid Mermaid syntax.
const mermaidFixSafetyCap = 100

// mermaidDefaultFixBudget is used when the operator leaves
// MermaidFixAttempts unset or sets it to a negative value.
const mermaidDefaultFixBudget = 3

// MermaidValidatorFunc runs Mermaid validation against a candidate
// assistant response. Callers wire it from ProcessOneImage using
// ValidateMermaid + the resolved MermaidCommand/timeout. Returning
// nil skips validation entirely (used for the format-fix path).
type MermaidValidatorFunc func(response string) MermaidValidationResult

// MermaidRepairPromptBuilder builds the user-turn prompt that asks the
// model to fix its previous Mermaid syntax. It receives the current
// assistant response and the validator's error message so the prompt
// can quote the mmdc failure inline while keeping the previous
// response compact (we no longer inline the full previous response —
// the conversation already contains it).
type MermaidRepairPromptBuilder func(currentResult, validationError string) string

// systemPromptWithExtra appends the caller-provided working-memory
// instruction (e.g. the latex watermark memory) to the base prompt.
func systemPromptWithExtra(opts config.OptionsConfig) string {
	sys := BuildSystemPrompt(opts.MaxRetries, opts.OutputLanguage)
	if opts.ExtraInstruction != "" {
		sys += "\n\n" + opts.ExtraInstruction
	}
	return sys
}

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

// normalizeToolCallTypes fills in an empty Type on echoed assistant
// tool_calls with "function". Some providers return tool_call entries
// without a type field; replaying them verbatim makes strict upstream
// validators reject the follow-up request with HTTP 400
// ("Input should be 'function'").
func normalizeToolCallTypes(msg *ChatMessage) {
	for i := range msg.ToolCalls {
		if msg.ToolCalls[i].Type == "" {
			msg.ToolCalls[i].Type = "function"
		}
	}
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
//   - Optional Mermaid validation/repair: when validator is non-nil
//     every final assistant text is checked; syntax failures send the
//     model a compact fix message inside the same tool loop (the
//     conversation keeps its prior turns, get_more_context remains
//     available until toolRounds is exhausted, and toolRounds /
//     repairAttempts are tracked independently).
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
	validator MermaidValidatorFunc,
	repairPromptBuilder MermaidRepairPromptBuilder,
) (string, string) {
	// Request/retry controls now live on the client (resolved from
	// the model config); legacy options.* act as fallbacks there.
	rateLimitLimit := client.RateLimitRetries
	maxAPIRetries := client.MaxRetries
	maxRounds := opts.MaxRetries
	if maxRounds <= 0 {
		maxRounds = 3
	}

	// Mode 1: format-fix / custom user text — single shot, no tools.
	if customUserText != "" {
		req := &ChatRequest{
			Model: client.Model(),
			Messages: []ChatMessage{
				{Role: "system", Content: systemPromptWithExtra(opts)},
				{Role: "user", Content: []map[string]interface{}{
					{"type": "text", "text": customUserText},
					{"type": "image_url", "image_url": map[string]string{
						"url": "data:image/jpeg;base64," + imgBase64,
					}},
				}},
			},
			MaxTokens:   opts.MaxTokens,
			Temperature: opts.Temperature,
			Stream:      false,
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

	mainSystem := BuildSystemPrompt(opts.MaxRetries, opts.OutputLanguage)
	if opts.ExtraInstruction != "" {
		mainSystem += "\n\n" + opts.ExtraInstruction
	}
	messages := []ChatMessage{
		{Role: "system", Content: mainSystem},
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

	repairBudget := resolveMermaidRepairBudget(opts.MermaidFixAttempts)
	toolRounds := 0
	repairAttempts := 0
	currentResult := ""

	for {
		// toolRounds counts how many get_more_context rounds we've spent
		// so far. The Python reference caps tool calls at maxRounds;
		// after that we send the request without tools so the model is
		// forced to commit. The forced-final "Provide your best analysis
		// now." path is handled below once toolRounds == maxRounds and
		// the next reply still returns no tool_calls.
		includeTools := toolRounds < maxRounds
		req := &ChatRequest{
			Model:       client.Model(),
			Messages:    messages,
			MaxTokens:   opts.MaxTokens,
			Temperature: opts.Temperature,
			Stream:      false,
		}
		if includeTools {
			req.Tools = tools
			req.ToolChoice = "auto"
		} else {
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

		if len(choice.Message.ToolCalls) > 0 {
			// Tool round: append assistant verbatim, execute each
			// get_more_context, append the matching tool responses,
			// then continue the loop. toolRounds advances by one
			// regardless of how many tool calls the model issued in
			// this turn (matches the Python reference semantics).
			normalizeToolCallTypes(&choice.Message)
			messages = append(messages, choice.Message)

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
			toolRounds++
			continue
		}

		// No tool_calls this turn. The forced-final path fires when the
		// model has spent all its tool rounds AND we still have not
		// received a usable answer; we append the same "Provide your
		// best analysis now." nudge the legacy code used.
		if !includeTools {
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: "Provide your best analysis now.",
			})
			// Re-issue one last no-tools request so the nudge is part
			// of the model-visible transcript (and so Mermaid
			// validation runs on this final reply).
			finalReq := &ChatRequest{
				Model:       client.Model(),
				Messages:    messages,
				MaxTokens:   opts.MaxTokens,
				Temperature: opts.Temperature,
				Stream:      false,
				ToolChoice:  "none",
			}
			finalResp, finalSentinel, finalStatus := doCallWithRetryFull(client, finalReq, maxAPIRetries, rateLimitLimit, logger, tid)
			if finalStatus != "" {
				return finalSentinel, finalStatus
			}
			if len(finalResp.Choices) == 0 {
				return sentinelEmpty, StatusError
			}
			finalChoice := finalResp.Choices[0]
			messages = append(messages, finalChoice.Message)
			content := contentString(finalChoice.Message)
			if content == "" {
				return sentinelEmpty, StatusError
			}
			if validator == nil {
				return content, StatusOK
			}
			currentResult = content
		} else {
			// Normal final-of-round reply: stash it for the Mermaid
			// check and append it to the transcript so subsequent
			// turns can reference it.
			messages = append(messages, choice.Message)
			content := contentString(choice.Message)
			if content == "" {
				return sentinelEmpty, StatusError
			}
			if validator == nil {
				return content, StatusOK
			}
			currentResult = content
		}

		// Mermaid validation: classify the action.
		validation := validator(currentResult)
		action := decideMermaidAction(validation, strings.ToLower(strings.TrimSpace(opts.MermaidValidation)))
		switch action.kind {
		case actionAccept:
			return currentResult, StatusOK
		case actionSentinel:
			logger.LogError(tid,
				"Mermaid validation unavailable for", imgPathFromIdx(lines, imgLineIdx),
				":", validation.Error)
			return sentinelMermaid, StatusRetry
		case actionRepair:
			if repairAttempts >= repairBudget {
				logger.LogError(tid,
					"Mermaid validation failed after repairs for",
					imgPathFromIdx(lines, imgLineIdx)+":", validation.Error)
				return sentinelMermaid, StatusRetry
			}
			if repairPromptBuilder == nil {
				// No builder wired (e.g. tests use the validator without
				// exercising fix). Treat as terminal so we do not
				// loop forever sending identical fix messages.
				return sentinelMermaid, StatusRetry
			}
			logger.LogWarning(tid, fmt.Sprintf(
				"Mermaid validation failed (%d/%d): %s",
				repairAttempts+1, repairBudget, validation.Error,
			))
			fixMsg := repairPromptBuilder(currentResult, validation.Error)
			messages = append(messages, ChatMessage{Role: "user", Content: fixMsg})
			repairAttempts++
			// After a fix message we still want to allow the model to
			// call get_more_context if it needs more lines. toolRounds
			// is unchanged so the loop's includeTools gate continues
			// to work from its current position.
			continue
		default:
			return sentinelMermaid, StatusRetry
		}
	}
}

// resolveMermaidRepairBudget normalises the user-facing
// MermaidFixAttempts field into a concrete retry budget:
//
//	nil  -> default 3
//	>0   -> the configured value
//	0    -> explicit unlimited, clamped by mermaidFixSafetyCap
//	<0   -> misconfiguration, default 3
func resolveMermaidRepairBudget(cfg *int) int {
	if cfg == nil {
		return mermaidDefaultFixBudget
	}
	if *cfg > 0 {
		return *cfg
	}
	if *cfg == 0 {
		return mermaidFixSafetyCap
	}
	return mermaidDefaultFixBudget
}

// mermaidAction enumerates the decisions decideMermaidAction can make.
type mermaidAction struct {
	kind   int
	reason string
}

const (
	actionAccept   = iota // No Mermaid, valid Mermaid, or auto-mode unavailable.
	actionSentinel        // strict mode + Mermaid validator unavailable.
	actionRepair          // Syntax failure with budget remaining.
)

// decideMermaidAction maps a MermaidValidationResult + mode into a
// high-level action. The validation contract is the same one the
// pre-refactor validateAndRepairMermaid enforced:
//
//   - no Mermaid block          -> accept
//   - valid Mermaid             -> accept
//   - validator unavailable     -> accept (auto) | sentinel (strict)
//   - syntax failure            -> repair (caller enforces budget)
func decideMermaidAction(v MermaidValidationResult, mode string) mermaidAction {
	if !v.HasMermaid || v.Valid {
		return mermaidAction{kind: actionAccept}
	}
	if !v.Available {
		if mode == "strict" {
			return mermaidAction{kind: actionSentinel, reason: v.Error}
		}
		// auto (or any non-strict mode) tolerates unavailable
		// validators and treats the response as acceptable.
		return mermaidAction{kind: actionAccept, reason: v.Error}
	}
	return mermaidAction{kind: actionRepair, reason: v.Error}
}

// mermaidErrorStackCap is the byte budget applied to the mmdc error
// string before it is inlined into the Mermaid repair prompt. The
// previous behaviour truncated to 16384 bytes; sanitizeMermaidError
// keeps that contract as a safety net while also stripping the
// puppeteer internals that pad every mmdc failure with hundreds of
// unhelpful stack frames.
const mermaidErrorStackCap = 16384

// mermaidErrorCutMarkers are the substrings we look for in the
// post-"Error:" summary to decide where the useful error ends and the
// runtime/puppeteer stack begins. The first match wins; when neither
// is present we keep the whole summary.
var mermaidErrorCutMarkers = []string{
	"Parser.parseError",
	"\n    at ",
}

// sanitizeMermaidError turns a noisy mmdc / ValidateMermaid error
// string into a compact, model-friendly summary that can be safely
// embedded in the Mermaid repair prompt.
//
// The input format is something like:
//
//	Generating single mermaid chart
//
//	block 1: Error: Parse error on line 17:
//	... end    Cup1 -->|P(A) = 3/5| Cup2
//	---------------------^
//	Expecting 'SQE', 'DOUBLECIRCLEEND', ... got 'PS'
//	Parser.parseError (https://mermaid-cli-intercept.invalid/.../node_modules/...)
//	    at #evaluate (...)
//	    at processTicksAndRejections (...)
//	    ... (hundreds of puppeteer frames)
//
// The only lines that actually help the model are the "Error:" line,
// the caret underline, and the "Expecting ... got ..." line; everything
// from "Parser.parseError" (or, when missing, the first `\n    at `)
// downwards is puppeteer internals and is dropped. The "block N: "
// wrapper added by ValidateMermaid and the "Generating single mermaid
// chart" preamble are also removed by anchoring the summary at the
// first "Error:" (case-insensitive). When no "Error:" is present we
// keep the trimmed input verbatim.
//
// The result is finally trimmed and bounded by mermaidErrorStackCap
// bytes (with a trailing "...") so the prompt stays capped even on
// inputs that lack any stack marker.
func sanitizeMermaidError(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	summary := trimmed
	if idx := strings.Index(lower, "error:"); idx >= 0 {
		// Drop "block N: " (added by ValidateMermaid) and any
		// "Generating single mermaid chart" preamble in one shot
		// by anchoring at the first "Error:" occurrence.
		summary = trimmed[idx:]
	}
	// Truncate at the first stack marker. We only scan the summary
	// we just extracted, so words like "Error: ...catch" that appear
	// in the useful lines are not mistaken for a stack frame.
	cut := -1
	for _, marker := range mermaidErrorCutMarkers {
		if i := strings.Index(summary, marker); i >= 0 {
			if cut == -1 || i < cut {
				cut = i
			}
		}
	}
	if cut > 0 {
		summary = summary[:cut]
	}
	summary = strings.TrimSpace(summary)
	if len(summary) > mermaidErrorStackCap {
		summary = summary[:mermaidErrorStackCap] + "..."
	}
	return summary
}

// buildMermaidRepairMessage returns the user-turn prompt that asks the
// model to fix its previous Mermaid syntax. We deliberately do NOT
// inline the full previous response — it is already part of the
// conversation, so duplicating it would balloon the request without
// adding information. The mmdc error is sanitised (stack frames and
// the "block N: " wrapper stripped) and quoted inline (truncated to
// ~16KB) so the model can target the failing construct without being
// distracted by hundreds of puppeteer frames.
func buildMermaidRepairMessage(currentResult, validationError string) string {
	trimmedError := sanitizeMermaidError(validationError)
	return strings.Join([]string{
		"Your previous response contains invalid Mermaid syntax. Fix only the Mermaid syntax.",
		"Validator output: " + trimmedError,
		"",
		"Preserve the [IMG_TYPE: <type>] prefix and all non-Mermaid content.",
		"If a Mermaid block is present, keep it fenced with ```mermaid and make its syntax valid.",
		"Do not add explanations outside the response.",
		"",
		"Special-character rules inside the Mermaid block:",
		"- Wrap node labels containing `( ) < > & | { } [ ]` in double quotes, e.g. `A[\"x (y)\"]` or `A[\"a<b\"]`.",
		"- Do not use unescaped HTML such as `<br/>`, `<b>`, etc.; either escape with `&lt;br/&gt;` or replace with spaces.",
		"- Do not use the math operator `~` outside of explicit math contexts; prefer text labels instead.",
		"- Use only ASCII quotes (\"...\"); never use Chinese/typographic quotes like “ ” ‘ ’, and avoid full-width punctuation （ ） ， ： inside the diagram.",
	}, "\n")
}

// imgPathFromIdx is a small helper used purely for log messages. It
// returns a short tag derived from the image line index so logs do not
// require the caller to thread the image path through every step of
// the state machine. Used only when callers do not pass an explicit
// image path (e.g. the format-fix path which reuses the same
// validator closure from ProcessOneImage).
func imgPathFromIdx(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return fmt.Sprintf("line:%d", idx)
	}
	line := strings.TrimSpace(lines[idx])
	if len(line) > 80 {
		line = line[:80] + "..."
	}
	return fmt.Sprintf("line:%d", idx)
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
				logger.LogInfo(tid, "  [RateLimit] waiting", wait)
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
				logger.LogInfo(tid, "  [ConnRetry] waiting", wait)
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
	imagesDir, imgPath, subject string,
	lines []string,
	imgLineIdx int,
	logger *logger.Logger,
	tid int,
	opts config.OptionsConfig,
) (string, string) {
	imgFile, err := resolveImageFile(imagesDir, imgPath, subject)
	if err != nil {
		return fmt.Sprintf("[IMG_MISSING: %s]", imgPath), StatusError
	}
	imgBase64, err := ImageToBase64(imgFile, 1280)
	if err != nil {
		return fmt.Sprintf("[IMG_ERROR: %s - %v]", imgPath, err), StatusError
	}

	// Mermaid validator: nil when the operator disabled validation.
	// The closure re-uses the resolved MermaidCommand / timeout so each
	// fix round sees the same configuration.
	mode := strings.ToLower(strings.TrimSpace(opts.MermaidValidation))
	var validator MermaidValidatorFunc
	var repairBuilder MermaidRepairPromptBuilder
	if mode != "" && mode != "off" {
		timeout := time.Duration(opts.MermaidTimeout) * time.Second
		command := opts.MermaidCommand
		tikzMode := strings.ToLower(strings.TrimSpace(opts.LatexValidation))
		tikzEngine := opts.LatexEngine
		validator = func(response string) MermaidValidationResult {
			// TikZ responses route to the LaTeX compile check; Mermaid
			// keeps its own CLI validation. Pure text/math/table answers
			// hit ValidateMermaid's no-block fast path (valid).
			if tikzMode != "off" && len(ExtractTikZBlocks(response)) > 0 && !hasMermaidBlock(response) {
				return ValidateTikZ(context.Background(), response, tikzEngine, timeout)
			}
			return ValidateMermaid(context.Background(), response, command, timeout)
		}
		repairBuilder = func(current, validationError string) string {
			if len(ExtractTikZBlocks(current)) > 0 && !hasMermaidBlock(current) {
				return buildTikzRepairMessage(current, validationError)
			}
			return buildMermaidRepairMessage(current, validationError)
		}
	}

	result, status := CallAIWithTools(
		client, imgBase64, lines, imgLineIdx,
		logger, tid, opts, "",
		validator, repairBuilder,
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
				"Start EXACTLY with \"[IMG_TYPE:\" followed by the type, then the pure description (mermaid/tikz/table/latex/text/flowchart/...). "+
				"Do NOT write \"The image shows\", \"This diagram illustrates\", or any similar analysis.",
			result,
		)
		// Format-fix path runs without Mermaid validation: the model is
		// being asked to repair the [IMG_TYPE:] prefix, not syntax.
		// The validator stays in scope on the *initial* call so the
		// original response still benefits from validation.
		fixed, fixStatus := CallAIWithTools(
			client, imgBase64, nil, 0,
			logger, tid, opts, fixMsg,
			nil, nil,
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
func resolveImageFile(imagesDir, imgPath, subject string) (string, error) {
	rel := imgPath
	if i := strings.Index(imgPath, "/"); i >= 0 {
		rel = imgPath[i+1:]
	}
	if full := filepath.Join(imagesDir, rel); pathExists(full) {
		return full, nil
	}
	// Fallback: the markdown may reference a bare "images/foo.jpg" while
	// the collected images live under images/<subject>/foo.jpg. This
	// happens when organize's incremental path kept an unnormalised
	// markdown on a rerun (see organize.step3CollectImages).
	if subject != "" {
		if alt := filepath.Join(imagesDir, subject, filepath.Base(imgPath)); pathExists(alt) {
			return alt, nil
		}
	}
	return "", os.ErrNotExist
}

// pathExists reports whether p exists on disk.
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
