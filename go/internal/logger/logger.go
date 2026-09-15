package logger

import (
	"fmt"
	"os"
	"strings"
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
	// liveRows 是"实时进度块"：每行一个所有者（阶段进度、某个子会话）。
	// 它们一起在终端里原地重绘，日志行先擦掉整块再画在下面。**每人一行**
	// 是这套东西存在的理由：曾经只有一个 live 槽位，谁最后装谁的话就显示
	// 谁，于是并发的样式修复会话与它的父阶段每秒互相覆盖（用户看到
	// "[style-feedback] 轮次 33 …" 与 "[style-fix] …" 来回跳、两边时间都在
	// 涨），而阶段收尾时又会把别人的行擦掉（用户看到"进度行重叠"）。
	liveRows []*liveRowEntry
	// liveDrawn 是**当前已经画在终端上**的行数，重绘/擦除要靠它把光标移回去。
	liveDrawn int
	// tty 决定重绘方式：真终端里用 ANSI 光标上移 + 清行原地重绘；管道/
	// 重定向里每次变化只整行追加一次（否则日志里全是裸转义序列）。
	tty   bool
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
	l := &Logger{threadIDWidth: threadIDWidth, tty: stdoutIsTerminal()}

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

// TTYForTest forces the terminal/pipe decision of loggers created after it
// is set (nil = detect from os.Stdout). A test's stdout is a pipe, so without
// this the in-place redraw path could not be exercised at all.
var TTYForTest *bool

// stdoutIsTerminal reports whether os.Stdout is a character device. The
// live block redraws itself with ANSI escapes, which is only meaningful
// there.
func stdoutIsTerminal() bool {
	if TTYForTest != nil {
		return *TTYForTest
	}
	st, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// liveRowEntry is one owner's line inside the live block.
type liveRowEntry struct {
	id   string
	text string
	// printed is the last text flushed as a whole line in non-TTY mode
	// (one line per change; consecutive identical states stay one line).
	printed string
}

// LiveRow is a handle to one line of the live progress block. Rows are
// independent: setting or removing one never touches another owner's line.
type LiveRow struct {
	l   *Logger
	ent *liveRowEntry
}

// LiveRow returns the row with this id, creating it when missing. The id
// should name the owner (e.g. "convert/chapter_002", "style-feedback"), so a
// caller can re-acquire the same row across reconnects and so two concurrent
// owners can never share a line.
func (l *Logger) LiveRow(id string) *LiveRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.liveRows {
		if e.id == id {
			return &LiveRow{l: l, ent: e}
		}
	}
	ent := &liveRowEntry{id: id}
	l.liveRows = append(l.liveRows, ent)
	return &LiveRow{l: l, ent: ent}
}

// Update sets the row text (printf-style) and repaints the block.
func (r *LiveRow) Update(format string, args ...interface{}) {
	if r == nil || r.l == nil || r.ent == nil {
		return
	}
	r.Set(fmt.Sprintf(format, args...))
}

// Set replaces the row text and repaints the block. An empty text keeps the
// row (it renders as a blank line) — use Remove to drop it.
func (r *LiveRow) Set(text string) {
	if r == nil || r.l == nil || r.ent == nil {
		return
	}
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.findRowLocked(r.ent)
	if e == nil {
		return // already removed: late updates must not resurrect a row
	}
	if e.text == text {
		return
	}
	e.text = text
	// 必须先擦再画：块画完后光标停在**最后一行**，直接重画会把第 0 行
	// 盖到最后一行上。
	l.refreshLiveLocked()
}

// Text returns the current text (empty when the row is gone).
func (r *LiveRow) Text() string {
	if r == nil || r.l == nil || r.ent == nil {
		return ""
	}
	r.l.mu.Lock()
	defer r.l.mu.Unlock()
	if e := r.l.findRowLocked(r.ent); e != nil {
		return e.text
	}
	return ""
}

// Remove drops the row from the block. Idempotent.
func (r *LiveRow) Remove() {
	if r == nil || r.l == nil || r.ent == nil {
		return
	}
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, e := range l.liveRows {
		if e == r.ent {
			l.liveRows = append(l.liveRows[:i], l.liveRows[i+1:]...)
			break
		}
	}
	l.eraseLiveLocked()
	l.paintLiveLocked()
}

// Finalize prints `text` as an ordinary line (so it survives in the
// terminal scrollback and in a captured log) and drops the row. In a pipe
// the line is only printed when a repaint has not already flushed that
// exact text — otherwise the phase's final state would appear twice.
func (r *LiveRow) Finalize(text string) {
	if r == nil || r.l == nil || r.ent == nil {
		return
	}
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.findRowLocked(r.ent)
	already := e != nil && text != "" && text == e.printed
	for i, cur := range l.liveRows {
		if cur == r.ent {
			l.liveRows = append(l.liveRows[:i], l.liveRows[i+1:]...)
			break
		}
	}
	// T11：quiet（紧凑控制台）只压**明细日志行**，不压进度块——compact 模式
	// 的意义就是"只看阶段进度"，Finalize 的终态行照打（滚动缓冲里留得住）。
	l.eraseLiveLocked()
	if text != "" && (l.tty || !already) {
		fmt.Fprintln(os.Stdout, text)
	}
	l.paintLiveLocked()
}

// TTY reports whether the live block redraws in place (a real terminal).
func (l *Logger) TTY() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tty
}

// LiveRowTexts returns the rows in display order (tests and the panel owner).
func (l *Logger) LiveRowTexts() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.liveRows))
	for _, e := range l.liveRows {
		out = append(out, e.text)
	}
	return out
}

func (l *Logger) findRowLocked(ent *liveRowEntry) *liveRowEntry {
	for _, e := range l.liveRows {
		if e == ent {
			return e
		}
	}
	return nil
}

// eraseLiveLocked clears the painted block and leaves the cursor on its
// first line, so the next thing printed replaces the block instead of
// landing next to it. No-op for pipes (nothing was ever painted in place).
func (l *Logger) eraseLiveLocked() {
	if !l.tty || l.liveDrawn == 0 {
		l.liveDrawn = 0
		return
	}
	n := l.liveDrawn
	if n > 1 {
		fmt.Fprintf(os.Stdout, "\x1b[%dA", n-1) // to the block's first line
	}
	for i := 0; i < n; i++ {
		fmt.Fprint(os.Stdout, "\r\x1b[K")
		if i < n-1 {
			fmt.Fprint(os.Stdout, "\n")
		}
	}
	if n > 1 {
		fmt.Fprintf(os.Stdout, "\x1b[%dA", n-1) // back to the first line
	}
	l.liveDrawn = 0
}

// paintLiveLocked draws the current rows. In a TTY it reuses the space the
// previous block occupied; for a pipe it appends one line per CHANGE (so a
// captured log keeps a chronological record without每秒重画).
func (l *Logger) paintLiveLocked() {
	// T11：quiet 不再抑制进度块——RunBook 的紧凑模式靠 LiveRow 显示
	// classify/convert/style 进度；quiet 只负责压明细日志行。

	if !l.tty {
		for _, e := range l.liveRows {
			if e.text == "" || e.text == e.printed {
				continue
			}
			e.printed = e.text
			fmt.Fprintln(os.Stdout, e.text)
		}
		return
	}
	for i, e := range l.liveRows {
		if i > 0 {
			fmt.Fprint(os.Stdout, "\n")
		}
		fmt.Fprintf(os.Stdout, "\r\x1b[K%s", e.text)
	}
	l.liveDrawn = len(l.liveRows)
}

// refreshLiveLocked repaints the block in place (row added/updated/removed).
func (l *Logger) refreshLiveLocked() {
	l.eraseLiveLocked()
	l.paintLiveLocked()
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
		// 先擦掉整个实时块（光标停在块首行），日志行写在块原来的位置，
		// 再把块画在日志行下面：日志与进度块永不互相覆盖。旧实现只补一个
		// "\n" 再重画，等于把进度行**永久留在滚动区**，终端里就是同一行
		// 内容出现两次（用户实测的"重叠"）。
		l.eraseLiveLocked()
		fmt.Print(line)
		l.paintLiveLocked()
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

// PrintConsole writes one plain line to the console, ending (and redrawing)
// the live progress line around it. Callers that print their own user-facing
// phase lines must use this instead of fmt.Print* directly, otherwise the
// text lands on top of the progress line exactly like logger output did.
func (l *Logger) PrintConsole(line string) {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.quiet {
		return
	}
	l.eraseLiveLocked()
	fmt.Print(line)
	l.paintLiveLocked()
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
