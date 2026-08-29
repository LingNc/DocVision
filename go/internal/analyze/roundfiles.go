package analyze

import (
	"fmt"
	"sort"
	"strings"
)

// RoundFileStat aggregates this round's sessions per source markdown file.
type RoundFileStat struct {
	Total      int
	Success    int
	Failed     int
	Incomplete int
}

// GroupSessionsByFile buckets sessions by the markdown file part of the
// session key (key format "<md>.md::images/...").
func GroupSessionsByFile(sessions []Session) map[string]*RoundFileStat {
	stats := map[string]*RoundFileStat{}
	for _, s := range sessions {
		name := s.Key
		if i := strings.Index(s.Key, "::"); i >= 0 {
			name = s.Key[:i]
		}
		if name == "" {
			name = "(unknown)"
		}
		st, ok := stats[name]
		if !ok {
			st = &RoundFileStat{}
			stats[name] = st
		}
		st.Total++
		switch s.Status {
		case StatusSuccess:
			st.Success++
		case StatusFailed:
			st.Failed++
		case StatusIncomplete:
			st.Incomplete++
		}
	}
	return stats
}

// PrintRoundFileSummary reports which final files this log round wrote
// (any md with at least one successful image is rebuilt in finally/),
// plus a per-file and overall success summary.
func PrintRoundFileSummary(logName string, sessions []Session) {
	sep := strings.Repeat("=", 70)
	fmt.Println(sep)
	fmt.Printf("本轮输出文件统计: %s\n", logName)
	fmt.Println(sep)

	stats := GroupSessionsByFile(sessions)
	names := make([]string, 0, len(stats))
	for n := range stats {
		names = append(names, n)
	}
	sort.Strings(names)

	var totImg, totOK, totFail, totIncomplete int
	for _, n := range names {
		st := stats[n]
		totImg += st.Total
		totOK += st.Success
		totFail += st.Failed
		totIncomplete += st.Incomplete
	}
	rate := 0.0
	if totImg > 0 {
		rate = float64(totOK) / float64(totImg) * 100
	}

	fmt.Println("\n【本轮摘要】")
	fmt.Printf("  涉及文件数:   %d\n", len(names))
	fmt.Printf("  图片任务数:   %d\n", totImg)
	fmt.Printf("  成功:         %d (%.1f%%)\n", totOK, rate)
	fmt.Printf("  失败:         %d\n", totFail)
	fmt.Printf("  未完成:       %d\n", totIncomplete)

	fmt.Println("\n【已写出/更新的最终文件（本轮成功 >= 1）】")
	written := 0
	for _, n := range names {
		st := stats[n]
		if st.Success <= 0 {
			continue
		}
		written++
		r := 0.0
		if st.Total > 0 {
			r = float64(st.Success) / float64(st.Total) * 100
		}
		fmt.Printf("  %s\n     总 %d | 成功 %d | 失败 %d | 成功率 %.1f%%\n",
			n, st.Total, st.Success, st.Failed, r)
	}
	if written == 0 {
		fmt.Println("  （本轮没有任何图片成功，finally/ 未被本轮写入/更新）")
	}

	fmt.Println("\n【全部文件明细】")
	for _, n := range names {
		st := stats[n]
		fmt.Printf("  %s\n     总 %d | 成功 %d | 失败 %d | 未完成 %d\n",
			n, st.Total, st.Success, st.Failed, st.Incomplete)
	}
}
