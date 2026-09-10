package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Log levels. info is the default (progress + warnings/errors); debug
// adds per-request summaries, prompts and tool results; trace adds the
// noisy per-chunk stream traffic and raw dumps.
const (
	LevelInfo  = 0
	LevelDebug = 1
	LevelTrace = 2
)

// Logger is a thread-safe logger that writes timestamped, thread-tagged
// messages to console and (optionally) to a log file and an error log file.
//
// Format matches the Python reference: "[HH:MM:SS][T<tid>] message".
// The thread ID is zero-padded to threadIDWidth so multi-thread output aligns.
type Logger struct {
	mu            sync.Mutex
	logFile       *os.File // may be nil if logPath is empty
	errorFile     *os.File // may be nil if errLogPath is empty
	threadIDWidth int
	quiet         bool // when true, suppress console output; still writes to log files
	// live renders the "live" progress line (the one that is redrawn in
	// place with \r while a phase runs). While it is set, every console
	// line is preceded by a newline and the live line is redrawn after it,
	// so log lines never overwrite/garble the progress line.
	live  func()
	level int // LevelInfo / LevelDebug / LevelTrace
}

// NewLogger creates a Logger.
//   - logPath:     path to the main log file; if empty, only console output is used.
//   - errLogPath:  path to the error log file; if empty, error file output is disabled.
//   - threadIDWidth: width used to zero-pad the thread id in the log prefix.
//
// Files are opened in append mode and created if missing. Returns the first
// open error encountered; on error any files already opened are closed.
func NewLogger(logPath, errLogPath string, threadIDWidth int) (*Logger, error) {
	if threadIDWidth <= 0 {
		threadIDWidth = 2
	}
	l := &Logger{threadIDWidth: threadIDWidth}

	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_ = l.Close()
			return nil, err
		}
		l.logFile = f
	}
	if errLogPath != "" {
		f, err := os.OpenFile(errLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			_ = l.Close()
			return nil, err
		}
		l.errorFile = f
	}
	return l, nil
}

// Log writes a timestamped message to the console and to the main log file.
// Format: "[HH:MM:SS][T<tid>] message". Failures writing to the log file are
// swallowed so logging never disrupts the caller.
func (l *Logger) Log(tid int, args ...interface{}) {
	l.write(l.logFile, tid, "", args...)
}

// SetDebug toggles debug logging (written to the log file only, never
// to the console, so long prompts/tool dumps stay out of the way).
func (l *Logger) SetDebug(on bool) {
	if on {
		l.SetLevel(LevelDebug)
		return
	}
	l.SetLevel(LevelInfo)
}

// SetLevel sets the log level (LevelInfo / LevelDebug / LevelTrace).
func (l *Logger) SetLevel(level int) {
	if level < LevelInfo {
		level = LevelInfo
	}
	if level > LevelTrace {
		level = LevelTrace
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Level returns the current log level.
func (l *Logger) Level() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// DebugEnabled reports whether debug (or trace) logging is on.
func (l *Logger) DebugEnabled() bool { return l.Level() >= LevelDebug }

// TraceEnabled reports whether trace logging is on.
func (l *Logger) TraceEnabled() bool { return l.Level() >= LevelTrace }

// Debug writes a [DEBUG] entry to the main log file only. No-op unless
// debug or trace logging was enabled.
func (l *Logger) Debug(tid int, args ...interface{}) {
	if !l.DebugEnabled() {
		return
	}
	l.writeFileOnly(l.logFile, tid, "[DEBUG] ", args...)
}

// Trace writes a [TRACE] entry to the main log file only. Trace is the
// deepest level: per-chunk stream traffic and raw dumps live here so a
// normal debug log stays readable.
func (l *Logger) Trace(tid int, args ...interface{}) {
	if !l.TraceEnabled() {
		return
	}
	l.writeFileOnly(l.logFile, tid, "[TRACE] ", args...)
}

// LogError writes "[ERROR] ..." tagged message to the console, the main log
// file, and the error log file. The error log entry is prefixed with a
// full date+time to match the Python reference: "YYYY-MM-DD HH:MM:SS [ERROR] msg".
func (l *Logger) LogError(tid int, args ...interface{}) {
	l.write(l.logFile, tid, "[ERROR] ", args...)
	if l.errorFile != nil {
		ts := time.Now().Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf("%s [ERROR] %s\n", ts, joinArgs(args))
		l.append(l.errorFile, msg)
	}
}

// LogWarning writes "[WARNING] ..." tagged message to the console, the main
// log file, and the error log file. The error log entry is prefixed with a
// full date+time, matching the Python reference.
// LogInfo records a transient/recoverable event (rate-limit waits,
// auto-retried connection issues) in the console and the MAIN log
// only - it must not pollute the error log, which the analyser and
// operators treat as a list of things needing attention.
func (l *Logger) LogInfo(tid int, args ...interface{}) {
	l.write(l.logFile, tid, "[INFO] ", args...)
}
func (l *Logger) LogWarning(tid int, args ...interface{}) {
	l.write(l.logFile, tid, "[WARNING] ", args...)
	if l.errorFile != nil {
		ts := time.Now().Format("2006-01-02 15:04:05")
		msg := fmt.Sprintf("%s [WARNING] %s\n", ts, joinArgs(args))
		l.append(l.errorFile, msg)
	}
}

// Close releases the underlying log files. Safe to call multiple times.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var firstErr error
	if l.logFile != nil {
		if err := l.logFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		l.logFile = nil
	}
	if l.errorFile != nil {
		if err := l.errorFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		l.errorFile = nil
	}
	return firstErr
}

// ThreadIDWidth returns the configured width used to format thread IDs.
func (l *Logger) ThreadIDWidth() int {
	return l.threadIDWidth
}

// SetQuiet enables or disables console output. When quiet=true, messages
// are still written to the log file but not printed to the console.
// Quiet reports whether console output is currently suppressed. Callers
// that temporarily need the console (e.g. a phase that prints its own
// progress) must restore the PREVIOUS value instead of hard-coding false:
// doing the latter leaked every session log line of the following phases
// into the terminal.
func (l *Logger) Quiet() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.quiet
}

// SetLiveLine installs (fn != nil) or clears (fn == nil) the live progress
// line renderer. See the field comment for the interleaving guarantee.
func (l *Logger) SetLiveLine(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.live = fn
}

func (l *Logger) SetQuiet(quiet bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.quiet = quiet
}

// write formats the message and writes it to console and (if non-nil) the
// given file under a single lock. Tag is prepended inside the message body
// (e.g. "[ERROR] "); tag may be empty.
func (l *Logger) write(file *os.File, tid int, tag string, args ...interface{}) {
	ts := time.Now().Format("15:04:05")
	tidStr := fmt.Sprintf("%0*d", l.threadIDWidth, tid)
	body := joinArgs(args)
	if tag != "" {
		body = tag + body
	}
	line := fmt.Sprintf("[%s][T%s] %s\n", ts, tidStr, body)

	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.quiet {
		if l.live != nil {
			// Finish the live progress line before the log line, then
			// redraw it below: without this the log text landed on top of
			// the progress line and the two interleaved.
			fmt.Print("\n")
		}
		fmt.Print(line)
		if l.live != nil {
			l.live()
		}
	}
	if file != nil {
		_, _ = file.WriteString(line)
	}
}

// writeFileOnly writes an entry to the log file WITHOUT touching the
// console. Debug/trace detail (prompts, tool dumps, per-request summaries)
// belongs in the file: printing it next to the live progress line garbles
// the terminal, which is why only info/warning/error reach the console.
func (l *Logger) writeFileOnly(file *os.File, tid int, tag string, args ...interface{}) {
	if file == nil {
		return
	}
	ts := time.Now().Format("15:04:05")
	tidStr := fmt.Sprintf("%0*d", l.threadIDWidth, tid)
	body := joinArgs(args)
	if tag != "" {
		body = tag + body
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = file.WriteString(fmt.Sprintf("[%s][T%s] %s\n", ts, tidStr, body))
}

// append writes a pre-formatted line to the given file under the logger's
// mutex. Used for the error file which uses a different timestamp format.
func (l *Logger) append(file *os.File, line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = file.WriteString(line)
}

func joinArgs(args []interface{}) string {
	// Match the Python " ".join(str(a) for a in args) behaviour.
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
