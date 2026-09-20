package img2text

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/prompts"
)

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

// unsafeWorkspaceRe 把文件系统不安全/跨平台的字符压掉（工作区子目录名）。
var unsafeWorkspaceRe = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

// MermaidFixFunc 是就地修复轮用完后的**升级修复会话**钩子（P12）：接收最后一份
// 出错响应与校验错误，返回修复后的完整响应与会话是否成功。nil = 不升级（维持
// 旧的"跳过、下轮重试"）。
type MermaidFixFunc func(prevResult, validationError string) (string, bool)

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
	prompt := prompts.Must(prompts.Img2TextSystem)
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
	fixSession MermaidFixFunc,
) (string, string, string) {
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
		}
		content, status := doCallWithRetry(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
		if status != "" {
			return content, status, content
		}
		return content, StatusOK, content
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
	// 首次用户提示词统一取自 internal/prompts（模板 img2text.user）；
	// 只把本次的坐标/上下文/窗口上限作为数据传进去。
	prompt := prompts.Render(prompts.Img2TextUser, map[string]string{
		"LINE":     strconv.Itoa(imgLineIdx),
		"UP_START": strconv.Itoa(imgLineIdx - curUp),
		"DOWN_END": strconv.Itoa(imgLineIdx + curDown),
		"UP":       strconv.Itoa(curUp),
		"DOWN":     strconv.Itoa(curDown),
		"CONTEXT":  ctxText,
		"MAX_UP":   strconv.Itoa(opts.MaxWindowUp),
		"MAX_DOWN": strconv.Itoa(opts.MaxWindowDown),
	})
	messages := []ChatMessage{
		{Role: "system", Content: mainSystem},
		{Role: "user", Content: []map[string]interface{}{
			{"type": "text", "text": prompt},
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
		}
		if includeTools {
			req.Tools = tools
			req.ToolChoice = "auto"
		} else {
			req.ToolChoice = "none"
		}

		resp, errSentinel, status := doCallWithRetryFull(client, req, maxAPIRetries, rateLimitLimit, logger, tid)
		if status != "" {
			return errSentinel, status, ""
		}
		if len(resp.Choices) == 0 {
			return sentinelEmpty, StatusError, ""
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
				ToolChoice:  "none",
			}
			finalResp, finalSentinel, finalStatus := doCallWithRetryFull(client, finalReq, maxAPIRetries, rateLimitLimit, logger, tid)
			if finalStatus != "" {
				return finalSentinel, finalStatus, ""
			}
			if len(finalResp.Choices) == 0 {
				return sentinelEmpty, StatusError, ""
			}
			finalChoice := finalResp.Choices[0]
			messages = append(messages, finalChoice.Message)
			content := contentString(finalChoice.Message)
			if content == "" {
				return sentinelEmpty, StatusError, ""
			}
			// The prompt asks for Mermaid only. A ```latex / ```tikz
			// drawing block is a response this pipeline cannot use: it is
			// neither compile-checked nor embedded any more, so reject it
			// even when Mermaid validation is switched off.
			if leftoverDrawingBlock(content) {
				logRejectedDrawingBlock(logger, tid, content, lines, imgLineIdx)
				return sentinelInvalid, StatusRetry, content
			}
			if validator == nil {
				return content, StatusOK, content
			}
			currentResult = content
		} else {
			// Normal final-of-round reply: stash it for the Mermaid
			// check and append it to the transcript so subsequent
			// turns can reference it.
			messages = append(messages, choice.Message)
			content := contentString(choice.Message)
			if content == "" {
				return sentinelEmpty, StatusError, ""
			}
			if leftoverDrawingBlock(content) {
				logRejectedDrawingBlock(logger, tid, content, lines, imgLineIdx)
				return sentinelInvalid, StatusRetry, content
			}
			if validator == nil {
				return content, StatusOK, content
			}
			currentResult = content
		}

		// Mermaid validation: classify the action.
		validation := validator(currentResult)
		action := decideMermaidAction(validation, strings.ToLower(strings.TrimSpace(opts.MermaidValidation)))
		switch action.kind {
		case actionAccept:
			return currentResult, StatusOK, currentResult
		case actionSentinel:
			logger.LogError(tid,
				"Mermaid validation unavailable for", imgPathFromIdx(lines, imgLineIdx),
				":", validation.Error)
			return sentinelMermaid, StatusRetry, currentResult
		case actionRepair:
			if repairAttempts >= repairBudget {
				if repairBudget == 0 {
					// T37：不做就地修复轮，首次失败直接升级会话。
					logger.LogError(tid, fmt.Sprintf(
						"Mermaid validation failed on first output for %s: %s. Raw output: %s",
						imgPathFromIdx(lines, imgLineIdx), validation.Error, snippet(currentResult),
					))
				} else {
					logger.LogError(tid, fmt.Sprintf(
						"Mermaid validation failed after %d repair round(s) for %s: %s. Raw output: %s",
						repairBudget, imgPathFromIdx(lines, imgLineIdx), validation.Error, snippet(currentResult),
					))
				}
				// P12：就地修复轮用完 → 升级修复会话（虚拟工作区 + submit +
				// mmdc 检查 + 错误累计切备选模型）。未配置（fixSession 为 nil
				// 或 rounds≤0）时维持旧的"跳过、下轮重试"。
				if fixSession != nil {
					if repairBudget == 0 {
						logger.Log(tid, "  [mermaid-fix] 首次校验失败，直接启动升级修复会话")
					} else {
						logger.Log(tid, "  [mermaid-fix] 就地修复轮用尽，启动升级修复会话")
					}
					if fixed, ok := fixSession(currentResult, validation.Error); ok {
						return fixed, StatusOK, fixed
					}
				}
				return sentinelMermaid, StatusRetry, currentResult
			}
			if repairPromptBuilder == nil {
				// No builder wired (e.g. tests use the validator without
				// exercising fix). Treat as terminal so we do not
				// loop forever sending identical fix messages.
				return sentinelMermaid, StatusRetry, currentResult
			}
			logger.LogWarning(tid, fmt.Sprintf(
				"Mermaid validation failed (%d/%d) for %s: %s",
				repairAttempts+1, repairBudget, imgPathFromIdx(lines, imgLineIdx), validation.Error,
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
			return sentinelMermaid, StatusRetry, currentResult
		}
	}
}

// logRejectedDrawingBlock records a response that used a LaTeX/TikZ
// drawing block, naming the actual problem, the image it belongs to, the
// expected shape and the model's own text.
func logRejectedDrawingBlock(l *logger.Logger, tid int, content string, lines []string, imgLineIdx int) {
	l.LogWarning(tid, fmt.Sprintf(
		"Invalid response for %s: model returned a LaTeX/TikZ drawing block, but img2text only accepts Mermaid. %s. Raw output: %s",
		imgPathFromIdx(lines, imgLineIdx), expectedFormatHint, snippet(content),
	))
}

// resolveMermaidRepairBudget normalises the user-facing
// MermaidFixAttempts field into a concrete retry budget:
//
//	nil or <0 -> 0 rounds: T37 — the first validation failure goes
//	           straight to the upgraded fix session (no in-place
//	           "fix this syntax" rounds at all)
//	>0        -> the configured number of in-place repair rounds
//	0         -> explicit unlimited, clamped by mermaidFixSafetyCap
func resolveMermaidRepairBudget(cfg *int) int {
	if cfg == nil {
		return 0
	}
	if *cfg > 0 {
		return *cfg
	}
	if *cfg == 0 {
		return mermaidFixSafetyCap
	}
	return 0
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

// imgLineRefRe extracts the image reference from the markdown line the
// current image sits on.
var imgLineRefRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// imgPathFromIdx is a small helper used purely for log messages. The
// img2text state machine (CallAIWithTools) only knows the markdown lines
// and the image's line index, so this recovers the image path from that
// line. The "line:<idx>" fallback stays for synthetic call sites (tests)
// whose line array carries no image reference.
func imgPathFromIdx(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return fmt.Sprintf("line:%d", idx)
	}
	if m := imgLineRefRe.FindStringSubmatch(lines[idx]); m != nil {
		return m[1]
	}
	if line := strings.TrimSpace(lines[idx]); line != "" {
		if len(line) > 80 {
			line = line[:80] + "..."
		}
		return line
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
	emptyRetry := 0
	round := 0
	for {
		round++
		if logger.DebugEnabled() {
			logger.Debug(tid, fmt.Sprintf("[api] request #%d (%s messages=%d tools=%d tool_choice=%v)",
				round, client.RequestSummary(req), len(req.Messages), len(req.Tools), req.ToolChoice))
			debugDumpMessages(logger, tid, req.Messages)
		}
		resp, err := client.ChatCompletion(req)
		if err == nil {
			// T59：厂商偶发退化空响应（0 choices / 全空 content 无工具调用）
			// 不是模型的"回答"，按可重试错误处理——原样重发（请求字节
			// 一致保住前缀缓存），次数与 API 错误共用 api_max_retries
			// （默认 3），仍空才算 EMPTY_RESPONSE。
			if emptyRetry < maxAPIRetries && isDegenerateResponse(resp) {
				emptyRetry++
				logger.LogWarning(tid, "  [EmptyResponse] 空响应，重发", emptyRetry, "/", maxAPIRetries)
				time.Sleep(time.Duration(emptyRetry) * 2 * time.Second)
				continue
			}
			if logger.DebugEnabled() {
				content, reasoning, tools := 0, resp.ReasoningChars, 0
				if len(resp.Choices) > 0 {
					content = len(contentString(resp.Choices[0].Message))
					tools = len(resp.Choices[0].Message.ToolCalls)
				}
				logger.Debug(tid, fmt.Sprintf("[api] response #%d (%.1fs stream=%v finish=%s content=%d chars reasoning=%d chars tools=%d %s)",
					round, resp.Elapsed.Seconds(), resp.Streamed, dash(resp.FinishReason),
					content, reasoning, tools, resp.Usage.String()))
				if len(resp.Choices) > 0 {
					if text := strings.TrimSpace(contentString(resp.Choices[0].Message)); text != "" {
						logger.Debug(tid, fmt.Sprintf("[api] response #%d content:", round), truncate(text, 4000))
					}
				}
			}
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
// tuple matching the Python reference, plus the last raw model response
// so the runner can quote it when the item is discarded as invalid (a
// run that only says "invalid" is undiagnosable).
func ProcessOneImage(
	client *AIClient,
	imagesDir, imgPath, subject string,
	lines []string,
	imgLineIdx int,
	logger *logger.Logger,
	tid int,
	opts config.OptionsConfig,
	fixCfg *MermaidFixConfig,
) (string, string, string) {
	imgFile, err := resolveImageFile(imagesDir, imgPath, subject)
	if err != nil {
		return fmt.Sprintf("[IMG_MISSING: %s]", imgPath), StatusError, ""
	}
	imgBase64, err := ImageToBase64(imgFile, 1280)
	if err != nil {
		return fmt.Sprintf("[IMG_ERROR: %s - %v]", imgPath, err), StatusError, ""
	}
	// lastRaw keeps the model's most recent reply so a discarded item can
	// be explained in the log instead of only marked "invalid".
	lastRaw := ""

	// Mermaid validator: nil when the operator disabled validation.
	// The closure re-uses the resolved MermaidCommand / timeout so each
	// fix round sees the same configuration.
	mode := strings.ToLower(strings.TrimSpace(opts.MermaidValidation))
	var validator MermaidValidatorFunc
	var repairBuilder MermaidRepairPromptBuilder
	if mode != "" && mode != "off" {
		timeout := time.Duration(opts.MermaidTimeout) * time.Second
		command := opts.MermaidCommand
		validator = func(response string) MermaidValidationResult {
			// img2text 只校验 Mermaid；LaTeX/TikZ 绘图块由下方的
			// leftoverDrawingBlock 判为无效响应。
			v := ValidateMermaid(context.Background(), response, command, timeout)
			debugValidation(logger, tid, "mermaid", v)
			return v
		}
		repairBuilder = buildMermaidRepairMessage
	}

	var fixSession MermaidFixFunc
	if fixCfg != nil && fixCfg.Rounds > 0 {
		// 每图一个工作区子目录（并发 worker 互不共享）：键 = subject + 图片名，
		// 非法字符压成 '_'。同一图下轮重试时续用同一目录（出错现场保留）。
		key := subject + "_" + filepath.Base(imgPath)
		key = unsafeWorkspaceRe.ReplaceAllString(key, "_")
		ws := filepath.Join(fixCfg.WorkspaceRoot, key)
		fixSession = func(prevResult, validationError string) (string, bool) {
			// fixCfg 是全部 worker 共享的——复制一份按本图设置 ImgPath，
			// 否则 view_image 拿到空路径、解析出 images 目录本身而 decode
			// 失败（T53 真实事故：模型在修复会话里从没看到过原图）。
			fc := *fixCfg
			fc.ImgPath = imgPath
			return mermaidFixSessionInDir(&fc, ws, prevResult, validationError, logger, tid)
		}
	}

	result, status, lastRaw := CallAIWithTools(
		client, imgBase64, lines, imgLineIdx,
		logger, tid, opts, "",
		validator, repairBuilder, fixSession,
	)
	if status != StatusOK {
		// result is a sentinel; lastRaw (the model's own text) is what
		// makes the failure diagnosable downstream.
		return result, status, lastRaw
	}
	result = strings.TrimSpace(result)
	if lastRaw == "" {
		lastRaw = result
	}

	// Look for the [IMG_TYPE: prefix anywhere in the result. If the
	// model added leading prose we drop it and warn (Python does the
	// same with log_warning). The dropped prose is quoted in the log so
	// a run can tell WHAT the model emitted instead of just "invalid".
	if idx := strings.Index(result, "[IMG_TYPE:"); idx >= 0 {
		if idx > 0 {
			prefix := strings.TrimSpace(result[:idx])
			logger.LogWarning(tid,
				"Unexpected prefix before '[IMG_TYPE:' in", imgPath+":",
				truncate(prefix, 200),
				fmt.Sprintf("(前缀 %d 字符；完整响应 %d 字符，已丢弃前缀)", len([]rune(prefix)), len([]rune(result))))
		}
		return strings.TrimSpace(result[idx:]), StatusOK, result
	}

	// Missing [IMG_TYPE:. If the response is already a system error
	// sentinel, return it as-is.
	if strings.HasPrefix(result, "[IMG_") {
		return result, StatusError, result
	}

	// Try the format fix.
	if opts.FormatFixAttempts > 0 {
		logger.LogWarning(tid, "Format fix triggered for", imgPath, "— raw output:", snippet(result))
		fixMsg := fmt.Sprintf(
			`Your previous response was REJECTED because it did NOT start with "[IMG_TYPE: <type>]".`+"\n"+
				"Here is your previous response (for reference only):\n"+
				"---\n%s\n---\n\n"+
				"Start EXACTLY with \"[IMG_TYPE:\" followed by the type, then the pure description (mermaid/table/text/code/formula/flowchart/...). "+
				"Do NOT write \"The image shows\", \"This diagram illustrates\", or any similar analysis.",
			result,
		)
		// Format-fix path runs without Mermaid validation: the model is
		// being asked to repair the [IMG_TYPE:] prefix, not syntax.
		// The validator stays in scope on the *initial* call so the
		// original response still benefits from validation.
		fixed, fixStatus, fixRaw := CallAIWithTools(
			client, imgBase64, nil, 0,
			logger, tid, opts, fixMsg,
			nil, nil, nil,
		)
		if fixRaw != "" {
			fixed = strings.TrimSpace(fixed)
			if !strings.HasPrefix(fixed, "[IMG_") {
				fixed = strings.TrimSpace(fixRaw)
			}
			fixed = strings.TrimSpace(fixed)
		}
		if fixStatus != StatusOK && !strings.HasPrefix(fixed, "[IMG_") {
			return fixed, fixStatus, fixed
		}
		fixed = strings.TrimSpace(fixed)
		if idx := strings.Index(fixed, "[IMG_TYPE:"); idx >= 0 {
			if idx > 0 {
				logger.LogWarning(tid, "Format fix had extra prefix in", imgPath)
			}
			return strings.TrimSpace(fixed[idx:]), StatusOK, fixed
		}
		logger.LogError(tid, fmt.Sprintf(
			"Format fix still missing [IMG_TYPE:] for %s. %s. Raw output: %s",
			imgPath, expectedFormatHint, snippet(fixed),
		))
		return sentinelInvalid, StatusRetry, fixed
	}

	logger.LogError(tid, fmt.Sprintf(
		"No '[IMG_TYPE:' found in result from %s. %s. Raw output: %s",
		imgPath, expectedFormatHint, snippet(result),
	))
	return sentinelInvalid, StatusRetry, result
}

// resolveImageFile maps an "images/..." reference from the markdown into
// an absolute path under imagesDir. The Python reference splits the path
// on the first "/" and joins with images_dir; we mirror that semantics.
func resolveImageFile(imagesDir, imgPath, subject string) (string, error) {
	if strings.TrimSpace(imgPath) == "" {
		return "", os.ErrNotExist // 空路径会 join 出 imagesDir 自身（是目录）
	}
	rel := imgPath
	if i := strings.Index(imgPath, "/"); i >= 0 {
		rel = imgPath[i+1:]
	}
	if full := filepath.Join(imagesDir, rel); fileExists(full) {
		return full, nil
	}
	// Fallback: the markdown may reference a bare "images/foo.jpg" while
	// the collected images live under images/<subject>/foo.jpg. This
	// happens when organize's incremental path kept an unnormalised
	// markdown on a rerun (see organize.step3CollectImages).
	if subject != "" {
		if alt := filepath.Join(imagesDir, subject, filepath.Base(imgPath)); fileExists(alt) {
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

// fileExists reports whether p exists and is a regular file (a directory
// passing an existence check used to be returned as an "image" and only
// failed at decode time with a confusing message).
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// dash renders an empty string as "-" for log lines.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// debugValidation writes one [validate:<kind>] line to the debug log so
// a run's syntax checks are visible without dumping the full tool
// output. The error text goes through extractValidationError so the
// line carries the real failure (TeX `! …` / `l.NNN`, mmdc's parser
// error) instead of the engine's version banner.
func debugValidation(logger *logger.Logger, tid int, kind string, v MermaidValidationResult) {
	if !logger.DebugEnabled() {
		return
	}
	errText := "-"
	if v.Error != "" {
		errText = truncate(extractValidationError(sanitizeMermaidError(v.Error)), errorLineMaxBytes)
	}
	logger.Debug(tid, fmt.Sprintf("[validate:%s] has_blocks=%v valid=%v available=%v error=%s",
		kind, v.HasMermaid, v.Valid, v.Available, errText))
}

// debugDumpMessages writes the full prompt of every request to the log
// file (debug mode only). Image parts are summarised, never dumped.
func debugDumpMessages(logger *logger.Logger, tid int, msgs []ChatMessage) {
	for i, m := range msgs {
		logger.Debug(tid, fmt.Sprintf("[api]   msg[%d] role=%s content=%s",
			i, m.Role, debugContent(m.Content)))
	}
}

func debugContent(content any) string {
	switch c := content.(type) {
	case string:
		return truncate(c, 8000)
	case []map[string]interface{}:
		var b strings.Builder
		for _, part := range c {
			switch t, _ := part["type"].(string); t {
			case "text":
				if s, ok := part["text"].(string); ok {
					b.WriteString(truncate(s, 8000))
				}
			case "image_url":
				b.WriteString("[image]")
			}
		}
		return b.String()
	case nil:
		return ""
	default:
		raw, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprint(c)
		}
		return truncate(string(raw), 2000)
	}
}

// isDegenerateResponse reports the vendor-degenerate shapes T50/T59 cover:
// zero choices, or a choice with no content, no reasoning and no tool calls
// (finish=length/0-completion junk). Such a reply carries no model intent,
// so it is retried like a transport error instead of being judged.
func isDegenerateResponse(resp *ChatResponse) bool {
	if resp == nil || len(resp.Choices) == 0 {
		return true
	}
	m := resp.Choices[0].Message
	return strings.TrimSpace(contentString(m)) == "" && len(m.ToolCalls) == 0
}
