package analyze

import (
	"bufio"
	"os"
	"strconv"
)

// AnalyzeLog parses a log file into Sessions using a per-thread state machine.
//
// Rules (matching the Python implementation):
//   - START creates a new session for the thread; any open session on the
//     same thread is closed as incomplete.
//   - DONE / FAILED closes the current session with the parsed status.
//   - [ToolCall] increments the counter on the active session.
//   - At EOF, any still-open sessions are marked incomplete.
func AnalyzeLog(logPath string) ([]Session, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sessions []Session
	current := make(map[string]*Session)

	scanner := bufio.NewScanner(f)
	// Allow long lines (e.g. base64-ish content).
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		ts, tid, _, ok := ParseLogLine(line)
		if !ok || tid == "" {
			continue
		}

		// START line
		if m := PatternStart.FindStringSubmatch(line); m != nil {
			if old, ok := current[tid]; ok {
				old.Status = StatusIncomplete
				sessions = append(sessions, *old)
			}
			key := m[1]
			startTS := ts
			if startTS == "" {
				startTS = "00:00:00"
			}
			s := Session{
				Key:     key,
				TID:     tid,
				StartTS: startTS,
				Status:  StatusPending,
			}
			current[tid] = &s
			continue
		}

		// No active session on this thread: skip remaining checks.
		sess, ok := current[tid]
		if !ok {
			continue
		}

		// Tool call
		if PatternToolCall.MatchString(line) {
			sess.ToolCalls++
			continue
		}

		// DONE
		if m := PatternDone.FindStringSubmatch(line); m != nil {
			sess.Elapsed = parseFloat(m[1])
			sess.Status = StatusSuccess
			if len(m) > 2 {
				sess.ImgType = m[2]
			}
			sessions = append(sessions, *sess)
			delete(current, tid)
			continue
		}

		// FAILED
		if m := PatternFailed.FindStringSubmatch(line); m != nil {
			sess.Elapsed = parseFloat(m[1])
			sess.Status = StatusFailed
			sess.ErrorMsg = trimRight(m[2])
			sess.ErrorType = ClassifyError(sess.ErrorMsg)
			sessions = append(sessions, *sess)
			delete(current, tid)
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return sessions, err
	}

	// Close any sessions that never received DONE/FAILED.
	for _, sess := range current {
		sess.Status = StatusIncomplete
		sessions = append(sessions, *sess)
	}
	return sessions, nil
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
