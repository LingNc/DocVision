package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
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
		fmt.Print(line)
	}
	if file != nil {
		_, _ = file.WriteString(line)
	}
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
