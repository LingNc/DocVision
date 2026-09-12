// Package analyze parses img2text logs and computes processing statistics.
package analyze

import "regexp"

// Status constants for session lifecycle.
//
// StatusWarning is a fourth, distinct outcome: the item did NOT produce
// a usable result, but the runner deliberately discarded it and it will
// be retried next run ("Skipped invalid response … will retry next
// run"), so the progress line counts it under `warns`, not `errors`.
// Reporting it as a failure is what made `docvision analyze` print
// "失败 39 / 成功率 51.9%" for a run whose own progress line said
// "errors: 0, warns: 39".
const (
	StatusPending    = "pending"
	StatusSuccess    = "success"
	StatusWarning    = "warning"
	StatusFailed     = "failed"
	StatusIncomplete = "incomplete"
)

// retryableErrorTypes are the classified error types the runner treats
// as "skipped, will retry": a validation failure (Mermaid syntax) or a
// response that did not match the required format. Both come back as
// __INVALID_RESPONSE__ and increment the warn counter, never the error
// counter — see runWorkers' writer goroutine.
var retryableErrorTypes = map[string]bool{
	"mermaid_invalid": true,
	"invalid_format":  true,
}

// isRetryableError reports whether an error type is one the runner
// retries on the next run.
func isRetryableError(errType string) bool {
	return retryableErrorTypes[errType]
}

// Session represents a single image's processing session in the log.
type Session struct {
	Key       string  // e.g. "ch1.md::images/fig1.jpg"
	TID       string  // thread ID
	StartTS   string  // HH:MM:SS
	ToolCalls int     // number of tool calls observed
	Status    string  // pending/success/warning/failed/incomplete
	Elapsed   float64 // seconds (0 if not yet closed)
	ErrorType string  // classified error type, empty if success
	ErrorMsg  string  // raw error message
	ImgType   string  // extracted from DONE line
}

// LogPattern regex constants for parsing log lines.
var (
	PatternTimestamp = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2})\]`)
	PatternThreadID  = regexp.MustCompile(`\[T(\d+)\]`)
	PatternStart     = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\]\[T\d+\]\s*▶\s*START\s+(.+)$`)
	PatternDone      = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\]\[T\d+\]\s*✓\s*\[(\d+\.?\d*)s\]\s*DONE(?:\s+\[IMG_TYPE:\s*([^\]]+)\])?`)
	PatternFailed    = regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}\]\[T\d+\].*?✗\s*\[(\d+\.?\d*)s\]\s*FAILED\s*(.+)$`)
	PatternToolCall  = regexp.MustCompile(`\[ToolCall\]`)
	// PatternSessionTool matches the session engine's per-tool log lines
	// ("[tool:view_image] ok (145 chars result)" / "... error: ..."), so
	// latex logs contribute tool-call counts too.
	PatternSessionTool = regexp.MustCompile(`\[tool:[A-Za-z0-9_]+\]\s+(?:ok|error)\b`)
	PatternWarning     = regexp.MustCompile(`\[WARNING\]`)
	PatternError       = regexp.MustCompile(`\[ERROR\]`)
)

// ErrorPattern regex constants for error classification.
// Patterns use (?i) to mirror the Python re.I flag for case-insensitive matching.
var (
	ErrorConnectionTimeout = regexp.MustCompile(`(?i)connection|timeout|timed out`)
	ErrorRateLimit         = regexp.MustCompile(`(?i)429|rate.*limit`)
	ErrorImgMissing        = regexp.MustCompile(`(?i)IMG_MISSING`)
	ErrorImgError          = regexp.MustCompile(`(?i)IMG_ERROR`)
	ErrorAPIError          = regexp.MustCompile(`(?i)IMG_API_ERROR|IMG_PROCESS_ERROR|SESSION_API_ERROR`)
	ErrorSessionError      = regexp.MustCompile(`(?i)SESSION_`)
	ErrorWorkerFatal       = regexp.MustCompile(`(?i)IMG_WORKER_FATAL`)
	ErrorEmptyResponse     = regexp.MustCompile(`(?i)IMG_EMPTY_RESPONSE`)
	ErrorInvalidFormat     = regexp.MustCompile(`(?i)IMG_INVALID_FORMAT`)
	ErrorMermaidInvalid    = regexp.MustCompile(`(?i)IMG_MERMAID_INVALID`)
)

// errorPatternList is checked in order by ClassifyError.
var errorPatternList = []struct {
	Name    string
	Pattern *regexp.Regexp
}{
	{"connection_timeout", ErrorConnectionTimeout},
	{"rate_limit", ErrorRateLimit},
	{"img_missing", ErrorImgMissing},
	{"img_error", ErrorImgError},
	{"api_error", ErrorAPIError},
	{"worker_fatal", ErrorWorkerFatal},
	{"empty_response", ErrorEmptyResponse},
	{"invalid_format", ErrorInvalidFormat},
	{"mermaid_invalid", ErrorMermaidInvalid},
	{"session_error", ErrorSessionError},
}

// ParseLogLine parses one log line and returns timestamp, thread id, content.
// Returns ok=false when the line has no timestamp prefix.
func ParseLogLine(line string) (timestamp, threadID, content string, ok bool) {
	tsMatch := PatternTimestamp.FindStringSubmatch(line)
	if tsMatch == nil {
		return "", "", line, false
	}
	timestamp = tsMatch[1]

	tidMatch := PatternThreadID.FindStringSubmatch(line)
	if tidMatch != nil {
		threadID = tidMatch[1]
	} else {
		threadID = "0"
	}

	// Strip the timestamp prefix (e.g. "[19:26:30]") from the line.
	tsEnd := len(tsMatch[0])
	if tsEnd < len(line) && line[tsEnd] == ' ' {
		tsEnd++ // also consume the single space after the bracket
	}
	content = line[tsEnd:]
	content = trimRight(content)
	return timestamp, threadID, content, true
}

// ClassifyError matches an error message against ERROR_PATTERNS.
// Returns the first matching category name, or "unknown".
func ClassifyError(msg string) string {
	for _, p := range errorPatternList {
		if p.Pattern.MatchString(msg) {
			return p.Name
		}
	}
	return "unknown"
}

// trimRight strips trailing whitespace.
func trimRight(s string) string {
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		end--
	}
	return s[:end]
}
