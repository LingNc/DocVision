package analyze

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// PrintReport writes a human-readable summary of stats to stdout.
func PrintReport(stats *Statistics, logPath string, showThreads bool) {
	sep := strings.Repeat("=", 70)
	fmt.Println(sep)
	fmt.Printf("日志分析报告: %s\n", logPath)
	fmt.Println(sep)

	fmt.Println("\n【基础统计】")
	fmt.Printf("  总图片数:      %d\n", stats.Total)
	fmt.Printf("  成功:          %d (%.1f%%)\n", stats.Success, stats.SuccessRate)
	fmt.Printf("  失败:          %d\n", stats.Failed)
	fmt.Printf("  未完成:        %d\n", stats.Incomplete)
	fmt.Printf("  线程数:        %d\n", stats.ThreadCount)

	if stats.ToolCalls != nil {
		tc := stats.ToolCalls
		fmt.Println("\n【工具调用统计】")
		fmt.Printf("  总调用次数:    %d\n", tc.Total)
		fmt.Printf("  平均调用:      %.2f 次/图片\n", tc.Avg)
		fmt.Printf("  中位数:        %.0f\n", tc.Median)
		fmt.Printf("  范围:          %d - %d\n", tc.Min, tc.Max)
		fmt.Println("\n  调用次数分布:")
		keys := make([]int, 0, len(tc.Distribution))
		for k := range tc.Distribution {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			num := tc.Distribution[k]
			pct := 0.0
			if stats.Success > 0 {
				pct = float64(num) / float64(stats.Success) * 100
			}
			fmt.Printf("    %d 次: %5d 张 (%5.1f%%)\n", k, num, pct)
		}
		fmt.Println("\n  调用最多的前10张图片:")
		limit := 10
		if len(tc.Top) < limit {
			limit = len(tc.Top)
		}
		for i := 0; i < limit; i++ {
			item := tc.Top[i]
			short := item.Key
			if idx := strings.LastIndex(short, "::"); idx >= 0 {
				short = short[idx+2:]
			}
			if len(short) > 50 {
				short = short[:50]
			}
			fmt.Printf("    %2d 次 - %s\n", item.Count, short)
		}
	}

	if stats.Elapsed != nil {
		el := stats.Elapsed
		fmt.Println("\n【耗时统计】（仅成功）")
		fmt.Printf("  累计耗时:      %.1fs\n", el.Total)
		if stats.WallClock > 0 {
			fmt.Printf("  实际耗时:      %.1fs\n", stats.WallClock)
		}
		fmt.Printf("  平均耗时:      %.2fs\n", el.Avg)
		fmt.Printf("  中位数:        %.2fs\n", el.Median)
		fmt.Printf("  最小/最大:     %.2fs / %.2fs\n", el.Min, el.Max)
		fmt.Printf("  标准差:        %.2fs\n", el.Stdev)
		ps := make([]int, 0, len(el.P))
		for p := range el.P {
			ps = append(ps, p)
		}
		sort.Ints(ps)
		for _, p := range ps {
			fmt.Printf("  P%02d:           %.2fs\n", p, el.P[p])
		}
		fmt.Printf("\n  吞吐量:        %.2f 张/秒\n", stats.Throughput)

		if len(stats.ElapsedByToolCalls) > 0 {
			fmt.Println("\n  按工具调用次数分组的平均耗时:")
			keys := make([]int, 0, len(stats.ElapsedByToolCalls))
			for k := range stats.ElapsedByToolCalls {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				g := stats.ElapsedByToolCalls[k]
				fmt.Printf("    %d 次调用: %.2fs (n=%d)\n", k, g.Avg, g.Count)
			}
		}
	}

	if len(stats.ErrorDistribution) > 0 {
		fmt.Println("\n【错误分类统计】")
		type kv struct {
			k string
			v int
		}
		pairs := make([]kv, 0, len(stats.ErrorDistribution))
		for k, v := range stats.ErrorDistribution {
			pairs = append(pairs, kv{k, v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
		for _, p := range pairs {
			fmt.Printf("  %-20s: %4d\n", p.k, p.v)
		}
	}

	if showThreads && len(stats.ThreadStats) > 0 {
		fmt.Println("\n【线程详细统计】")
		tids := make([]string, 0, len(stats.ThreadStats))
		for tid := range stats.ThreadStats {
			tids = append(tids, tid)
		}
		sort.Slice(tids, func(i, j int) bool {
			a, _ := strconv.Atoi(tids[i])
			b, _ := strconv.Atoi(tids[j])
			return a < b
		})
		for _, tid := range tids {
			ts := stats.ThreadStats[tid]
			fmt.Printf("  T%2s: %4d张 成功%4d (%5.1f%%) 平均%6.2fs 最大%6.2fs\n",
				tid, ts.Count, ts.Success, ts.SuccessRate, ts.AvgElapsed, ts.ElapsedMax)
		}
	}
}

// ExportCSV writes per-session data to outputPath.
func ExportCSV(sessions []Session, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"key", "thread_id", "tool_calls", "status", "elapsed_sec",
		"error_type", "error_msg", "img_type",
	}); err != nil {
		return err
	}

	for _, s := range sessions {
		elapsed := ""
		if s.Elapsed != 0 {
			elapsed = strconv.FormatFloat(s.Elapsed, 'f', -1, 64)
		}
		if err := w.Write([]string{
			s.Key, s.TID, strconv.Itoa(s.ToolCalls), s.Status, elapsed,
			s.ErrorType, s.ErrorMsg, s.ImgType,
		}); err != nil {
			return err
		}
	}
	fmt.Printf("\nCSV已导出: %s\n", outputPath)
	return nil
}
