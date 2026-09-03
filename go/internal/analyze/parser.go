package analyze

import (
	"bufio"
	"os"
	"sort"
	"strconv"
)

// startEvent is one ▶ START line.
type startEvent struct {
	ts      int64       // seconds since midnight (from HH:MM:SS)
	key     string      // image key "<md>.md::images/..."
	matched bool        // already claimed by a close line
	close   *closeEvent // set when matched
}

// closeEvent is one ✓ DONE / ✗ FAILED line.
type closeEvent struct {
	ts      int64
	elapsed float64
	failed  bool
	imgType string
	errMsg  string
}

// threadLog collects the per-thread start and close events in file order.
type threadLog struct {
	starts []startEvent
	closes []closeEvent
	order  []string // "s" / "c" markers in file order
}

// midnightSeconds converts "HH:MM:SS" to seconds. Lines crossing midnight
// (rare, batch runs do not) would wrap; we handle by clamping to >= 0 via
// monotonic comparison in matchCloses.
func midnightSeconds(hhmmss string) int64 {
	var h, m, s int
	if len(hhmmss) >= 8 {
		h = int(hhmmss[0]-'0')*10 + int(hhmmss[1]-'0')
		m = int(hhmmss[3]-'0')*10 + int(hhmmss[4]-'0')
		s = int(hhmmss[6]-'0')*10 + int(hhmmss[7]-'0')
	}
	return int64(h*3600 + m*60 + s)
}

// tolerance seconds for matching a close to its start: elapsed has 2 decimals
// and timestamps 1s resolution; ToolCall delays are inside elapsed. Anything
// beyond this is treated as not belonging.
const matchTolerance = 15

// AnalyzeLog parses a log file into Sessions.
//
// Lines are attributed by matching each ✓/✗ close line to the most recent
// unmatched ▶ START on the same thread whose start time is close to
// (closeTime - elapsed). This survives old logs where thread ids were reused
// while the previous task was still in flight (close lines appearing after
// the next START), which made the previous per-thread state machine mark
// running tasks as incomplete.
//
// At EOF, unmatched starts become incomplete sessions; unmatched closes are
// dropped (their start lies before the captured window).
func AnalyzeLog(logPath string) ([]Session, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

threads := make(map[string]*threadLog)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		ts, tid, _, ok := ParseLogLine(line)
		if !ok || tid == "" {
			continue
		}
		tl := threads[tid]
		if tl == nil {
			tl = &threadLog{}
			threads[tid] = tl
		}
		tsSec := midnightSeconds(ts)

		if m := PatternStart.FindStringSubmatch(line); m != nil {
			tl.starts = append(tl.starts, startEvent{ts: tsSec, key: m[1]})
			tl.order = append(tl.order, "s")
			continue
		}
		if m := PatternDone.FindStringSubmatch(line); m != nil {
			c := closeEvent{ts: tsSec, elapsed: parseFloat(m[1])}
			if len(m) > 2 {
				c.imgType = m[2]
			}
			tl.closes = append(tl.closes, c)
			tl.order = append(tl.order, "c")
			continue
		}
		if m := PatternFailed.FindStringSubmatch(line); m != nil {
			msg := trimRight(m[2])
			tl.closes = append(tl.closes, closeEvent{
				ts: tsSec, elapsed: parseFloat(m[1]), failed: true, errMsg: msg,
			})
			tl.order = append(tl.order, "c")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sessions := make([]Session, 0)
	for tid, tl := range threads {
		matchThread(tl)
		// Emit in file order: starts (matched→result, unmatched→incomplete).
		si, ci := 0, 0
		for _, kind := range tl.order {
			switch kind {
			case "s":
				se := tl.starts[si]
				si++
				s := Session{
					Key:     se.key,
					TID:     tid,
					StartTS: fmtSeconds(se.ts),
				}
				if se.matched {
					ce := *se.close
					s.Status = StatusSuccess
					s.Elapsed = ce.elapsed
					s.ImgType = ce.imgType
					if ce.failed {
						s.Status = StatusFailed
						s.ErrorMsg = ce.errMsg
						s.ErrorType = ClassifyError(s.ErrorMsg)
					}
				} else {
					s.Status = StatusIncomplete
				}
				sessions = append(sessions, s)
			case "c":
				ci++
			}
		}
	}
	// Stable ordering: keep per-thread emission; sort by key then TID for
	// deterministic output across runs (map iteration order varies).
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Key != sessions[j].Key {
			return sessions[i].Key < sessions[j].Key
		}
		return sessions[i].TID < sessions[j].TID
	})
	return sessions, nil
}

// matchThread pairs each close with its owning start on the same thread.
func matchThread(tl *threadLog) {
	for ci := range tl.closes {
		ce := &tl.closes[ci]
		target := int64(ce.ts - int64(ce.elapsed)) // approx start second
		best := -1
		bestDist := int64(1 << 62)
		for si := range tl.starts {
			se := &tl.starts[si]
			if se.matched {
				continue
			}
			dist := se.ts - target
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist = dist
				best = si
			}
		}
		if best >= 0 && bestDist <= matchTolerance {
			tl.starts[best].matched = true
			tl.starts[best].close = &tl.closes[ci]
		}
	}
}

// fmtSeconds renders seconds-since-midnight back to "HH:MM:SS".
func fmtSeconds(sec int64) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	var buf [8]byte
	buf[0] = byte('0' + h/10)
	buf[1] = byte('0' + h%10)
	buf[2] = ':'
	buf[3] = byte('0' + m/10)
	buf[4] = byte('0' + m%10)
	buf[5] = ':'
	buf[6] = byte('0' + s/10)
	buf[7] = byte('0' + s%10)
	return string(buf[:])
}

// parseFloat parses a float, returning 0 on error.
func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
