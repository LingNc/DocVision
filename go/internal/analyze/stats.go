package analyze

import (
	"math"
	"sort"
	"time"
)

// Statistics holds every metric computed from a slice of sessions.
type Statistics struct {
	Total       int
	Success     int
	Failed      int
	Incomplete  int
	SuccessRate float64

	ToolCalls        *ToolCallStats
	Elapsed          *ElapsedStats
	ElapsedByToolCalls map[int]*ElapsedGroup
	ErrorDistribution map[string]int
	ThreadStats      map[string]*ThreadStat
	ThreadCount      int

	WallClock float64
	Throughput float64
}

// ToolCallStats summarises the tool-call counts across successful sessions.
type ToolCallStats struct {
	Total       int
	Avg         float64
	Median      float64
	Min         int
	Max         int
	Distribution map[int]int
	Top         []KeyCount
}

// KeyCount pairs a session key with its tool-call count.
type KeyCount struct {
	Key   string
	Count int
}

// ElapsedStats summarises the elapsed seconds of successful sessions.
type ElapsedStats struct {
	Total  float64
	Avg    float64
	Median float64
	Min    float64
	Max    float64
	Stdev  float64
	P      map[int]float64
}

// ElapsedGroup is a per-tool-call-count elapsed summary.
type ElapsedGroup struct {
	Count  int
	Avg    float64
	Median float64
}

// ThreadStat summarises a single thread.
type ThreadStat struct {
	Count        int
	Success      int
	Failed       int
	ElapsedTotal float64
	ElapsedMax   float64
	SuccessRate  float64
	AvgElapsed   float64
}

// ComputeStatistics returns a Statistics value for sessions.
func ComputeStatistics(sessions []Session, percentiles []int) *Statistics {
	if percentiles == nil {
		percentiles = []int{90, 95, 99}
	}

	stats := &Statistics{
		Total:             len(sessions),
		ErrorDistribution: map[string]int{},
		ThreadStats:       map[string]*ThreadStat{},
		ElapsedByToolCalls: map[int]*ElapsedGroup{},
	}

	successes := make([]Session, 0)
	failures := make([]Session, 0)
	for _, s := range sessions {
		switch s.Status {
		case StatusSuccess:
			stats.Success++
			successes = append(successes, s)
		case StatusFailed:
			stats.Failed++
			failures = append(failures, s)
		case StatusIncomplete:
			stats.Incomplete++
		}
	}
	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Success) / float64(stats.Total) * 100
	}

	// ---- Tool-call stats (successful only) ----
	if len(successes) > 0 {
		tcList := make([]int, len(successes))
		dist := map[int]int{}
		for i, s := range successes {
			tcList[i] = s.ToolCalls
			dist[s.ToolCalls]++
		}
		avg := meanFloat(toIntF(tcList))
		med := medianInt(tcList)
		tc := &ToolCallStats{
			Total:       sumInt(tcList),
			Avg:         avg,
			Median:      med,
			Min:         minInt(tcList),
			Max:         maxInt(tcList),
			Distribution: dist,
		}
		// Top-20 most-called images
		keys := make([]KeyCount, len(successes))
		for i, s := range successes {
			keys[i] = KeyCount{Key: s.Key, Count: s.ToolCalls}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].Count > keys[j].Count })
		if len(keys) > 20 {
			keys = keys[:20]
		}
		tc.Top = keys
		stats.ToolCalls = tc
	}

	// ---- Elapsed stats (successful only) ----
	elapsedList := make([]float64, 0)
	for _, s := range successes {
		if s.Elapsed > 0 || s.Status == StatusSuccess {
			elapsedList = append(elapsedList, s.Elapsed)
		}
	}
	if len(elapsedList) > 0 {
		el := &ElapsedStats{
			Total:  sumFloat(elapsedList),
			Avg:    meanFloat(elapsedList),
			Median: medianFloat(elapsedList),
			Min:    minFloat(elapsedList),
			Max:    maxFloat(elapsedList),
			Stdev:  stdevFloat(elapsedList),
			P:      map[int]float64{},
		}
		for _, p := range percentiles {
			el.P[p] = ComputePercentile(elapsedList, p)
		}
		stats.Elapsed = el
	}

	// ---- Elapsed grouped by tool-call count ----
	grouped := map[int][]float64{}
	for _, s := range successes {
		if s.Status == StatusSuccess {
			grouped[s.ToolCalls] = append(grouped[s.ToolCalls], s.Elapsed)
		}
	}
	for tc, list := range grouped {
		stats.ElapsedByToolCalls[tc] = &ElapsedGroup{
			Count:  len(list),
			Avg:    meanFloat(list),
			Median: medianFloat(list),
		}
	}

	// ---- Error distribution ----
	for _, s := range failures {
		et := s.ErrorType
		if et == "" {
			et = "unknown"
		}
		stats.ErrorDistribution[et]++
	}

	// ---- Per-thread stats ----
	for _, s := range sessions {
		ts, ok := stats.ThreadStats[s.TID]
		if !ok {
			ts = &ThreadStat{}
			stats.ThreadStats[s.TID] = ts
		}
		ts.Count++
		if s.Status == StatusSuccess {
			ts.Success++
			ts.ElapsedTotal += s.Elapsed
			if s.Elapsed > ts.ElapsedMax {
				ts.ElapsedMax = s.Elapsed
			}
		} else if s.Status == StatusFailed {
			ts.Failed++
		}
	}
	for _, ts := range stats.ThreadStats {
		if ts.Count > 0 {
			ts.SuccessRate = float64(ts.Success) / float64(ts.Count) * 100
		}
		if ts.Success > 0 {
			ts.AvgElapsed = ts.ElapsedTotal / float64(ts.Success)
		}
	}
	stats.ThreadCount = len(stats.ThreadStats)

	// ---- Wall clock & throughput ----
	earliest, latest, ok := wallClockRange(sessions)
	if ok {
		stats.WallClock = latest.Sub(earliest).Seconds()
		if stats.WallClock > 0 {
			stats.Throughput = float64(stats.Success) / stats.WallClock
		}
	}

	return stats
}

// ComputePercentile returns the p-th percentile of a sorted slice using
// linear interpolation, matching the Python implementation.
func ComputePercentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	vals := append([]float64(nil), sorted...)
	sort.Float64s(vals)
	k := float64(len(vals)-1) * float64(p) / 100.0
	f := int(math.Floor(k))
	c := f + 1
	if c >= len(vals) {
		c = f
	}
	if f == c {
		return vals[f]
	}
	return vals[f]*(float64(c)-k) + vals[c]*(k-float64(f))
}

// wallClockRange finds earliest START and latest end (start + elapsed) among
// sessions. Returns ok=false if no parseable timestamps were found.
func wallClockRange(sessions []Session) (earliest, latest time.Time, ok bool) {
	earliest = time.Time{}
	latest = time.Time{}
	haveAny := false
	for _, s := range sessions {
		st, ok := parseTimestamp(s.StartTS)
		if !ok {
			continue
		}
		if !haveAny || st.Before(earliest) {
			earliest = st
			haveAny = true
		}
		if s.Elapsed > 0 {
			end := st.Add(time.Duration(s.Elapsed * float64(time.Second)))
			if end.After(latest) {
				latest = end
			}
		}
	}
	if !haveAny {
		return time.Time{}, time.Time{}, false
	}
	if latest.IsZero() {
		latest = earliest
	}
	return earliest, latest, true
}

// parseTimestamp parses HH:MM:SS into a time.Time anchored to today's date,
// matching the Python helper (which uses datetime.now().date()).
func parseTimestamp(ts string) (time.Time, bool) {
	t, err := time.Parse("15:04:05", ts)
	if err != nil {
		return time.Time{}, false
	}
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location()), true
}

// ---- numeric helpers ----

func toIntF(in []int) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

func sumInt(in []int) int {
	s := 0
	for _, v := range in {
		s += v
	}
	return s
}

func sumFloat(in []float64) float64 {
	s := 0.0
	for _, v := range in {
		s += v
	}
	return s
}

func meanFloat(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	return sumFloat(in) / float64(len(in))
}

func minInt(in []int) int {
	if len(in) == 0 {
		return 0
	}
	m := in[0]
	for _, v := range in[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxInt(in []int) int {
	if len(in) == 0 {
		return 0
	}
	m := in[0]
	for _, v := range in[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func minFloat(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	m := in[0]
	for _, v := range in[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxFloat(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	m := in[0]
	for _, v := range in[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func medianInt(in []int) float64 {
	if len(in) == 0 {
		return 0
	}
	cp := append([]int(nil), in...)
	sort.Ints(cp)
	return ComputePercentile(toIntF(cp), 50)
}

func medianFloat(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	cp := append([]float64(nil), in...)
	sort.Float64s(cp)
	return ComputePercentile(cp, 50)
}

func stdevFloat(in []float64) float64 {
	if len(in) < 2 {
		return 0
	}
	m := meanFloat(in)
	sum := 0.0
	for _, v := range in {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(in)-1))
}
