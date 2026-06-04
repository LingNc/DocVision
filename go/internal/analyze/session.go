// Package analyze parses img2text logs and computes processing statistics.
package analyze

import "regexp"

// Status constants for session lifecycle.
const (
	StatusPending    = "pending"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusIncomplete = "incomplete"
)

// Session represents a single image's processing session in the log.
type Session struct {
	Key       string  // e.g. "ch1.md::images/fig1.jpg"
	TID       string  // thread ID
	StartTS   string  // HH:MM:SS
	ToolCalls int     // number of tool calls observed
	Status    string  // pending/success/failed/incomplete
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
	PatternWarning   = regexp.MustCompile(`\[WARNING\]`)
	PatternError     = regexp.MustCompile(`\[ERROR\]`)
)

// ErrorPattern regex constants for error classification.
// Patterns use (?i) to mirror the Python re.I flag for case-insensitive matching.
var (
	ErrorConnectionTimeout = regexp.MustCompile(`(?i)connection|timeout|timed out`)
	ErrorRateLimit         = regexp.MustCompile(`(?i)429|rate.*limit`)
	ErrorImgMissing        = regexp.MustCompile(`(?i)IMG_MISSING`)
	ErrorImgError          = regexp.MustCompile(`(?i)IMG_ERROR`)
	ErrorAPIError          = regexp.MustCompile(`(?i)IMG_API_ERROR|IMG_PROCESS_ERROR`)
	ErrorWorkerFatal       = regexp.MustCompile(`(?i)IMG_WORKER_FATAL`)
	ErrorEmptyResponse     = regexp.MustCompile(`(?i)IMG_EMPTY_RESPONSE`)
	ErrorInvalidFormat     = regexp.MustCompile(`(?i)IMG_INVALID_FORMAT`)
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
