package analyze

import (
	"fmt"
	"path/filepath"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logfind"
)

// RunOptions configures the analyze command.
type RunOptions struct {
	All          bool
	LogFile      string
	RoundSpec    string // -r/--round: 轮次选择，如 "1"（上一次）、"1-3"；0=最新
	TimeSpec     string // -l/--last: 时间范围，如 "+2d"、"2026Y9M1D-2026Y9M2D"
	ShowThreads  bool
	Percentiles  []int
	OutputCSV    string
	ProgressOnly bool
	// LatexOutDir: when set (latex command), the progress footer
	// summarises the latex progress_items instead of the img2text ones.
	LatexOutDir string
}

// Run is the entry point for log analysis + progress checking.
func Run(cfg *config.Config, opts RunOptions) error {
	if opts.Percentiles == nil {
		opts.Percentiles = []int{90, 95, 99}
	}

	inputDir := cfg.Paths.OutputDir
	logsDir := cfg.Paths.LogsDir
	finallyDir := cfg.Paths.FinallyDir
	progressRoot := filepath.Join(finallyDir, "progress_items")

	if opts.ProgressOnly {
		PrintProgressReport(inputDir, progressRoot, logsDir, finallyDir)
		return nil
	}

	// Resolve log files to analyse.
	logPaths, err := resolveLogPaths(opts, logsDir, finallyDir)
	if err != nil {
		return err
	}

	allSessions := make([]Session, 0)
	for _, lp := range logPaths {
		sessions, err := AnalyzeLog(lp)
		if err != nil {
			return fmt.Errorf("analyze %s: %w", lp, err)
		}
		allSessions = append(allSessions, sessions...)
		if len(logPaths) > 1 {
			fmt.Printf("  %s: %d 条记录\n", filepath.Base(lp), len(sessions))
		}
	}

	if len(allSessions) == 0 {
		fmt.Println("日志中未解析到图片处理会话（可能所有任务之前已完成）")
	}

	var stats *Statistics
	if len(allSessions) > 0 {
		stats = ComputeStatistics(allSessions, opts.Percentiles)
		reportPath := logPaths[0]
		if len(logPaths) == 1 {
			reportPath = logPaths[0]
		}
		PrintReport(stats, filepath.Base(reportPath), opts.ShowThreads)
	}

	// Round output-file summary (always shown): which md files had images
	// processed and written this round, plus overall success statistics.
	if len(allSessions) > 0 {
		PrintRoundFileSummary(filepath.Base(logPaths[0]), allSessions)
	}

	// Always print a progress summary footer.
	if opts.LatexOutDir != "" {
		PrintLatexProgressFooter(opts.LatexOutDir)
	} else {
		printProgressFooter(inputDir, progressRoot, finallyDir, logPaths)
		// P12：mermaid 升级修复会话的触发统计（有才打印）。
		PrintMermaidFixNote(logPaths)
	}

	if opts.OutputCSV != "" {
		if err := ExportCSV(allSessions, opts.OutputCSV); err != nil {
			return err
		}
	}
	_ = stats
	return nil
}

// resolveLogPaths picks the log files to analyse based on opts. When
// LogFile is empty, the lookup walks logsDir first and falls back to
// finallyDir so legacy logs produced before the T7 split keep working.
// RoundSpec (-r) and TimeSpec (-l) narrow the candidate list; the chosen
// files are printed before analysis so the user sees what will be read.
func resolveLogPaths(opts RunOptions, logsDir, finallyDir string) ([]string, error) {
	if opts.LogFile != "" {
		return []string{opts.LogFile}, nil
	}
	if opts.All {
		paths, err := logfind.FindAllWithFallback(logsDir, finallyDir)
		if err != nil {
			return nil, err
		}
		fmt.Printf("找到 %d 个日志文件\n", len(paths))
		return paths, nil
	}

	// Candidates: all primary logs (newest first); -r/-l narrow the set.
	all, err := logfind.FindAllWithFallback(logsDir, finallyDir)
	if err != nil {
		return nil, err
	}
	sel, desc, err := SelectLogs(all, opts.RoundSpec, opts.TimeSpec)
	if err != nil {
		return nil, err
	}
	if desc != "" {
		fmt.Printf("日志筛选: %s\n", desc)
	}
	fmt.Printf("本次分析 %d 个日志文件:\n", len(sel))
	for _, p := range sel {
		fmt.Printf("  %s\n", filepath.Base(p))
	}
	return sel, nil
}

// printProgressFooter prints the brief "总计/已完成/无效/剩余" line.
func printProgressFooter(inputDir, progressRoot, finallyDir string, logPaths []string) {
	totalImages := CountImagesInMDFiles(inputDir)
	completed, invalid, _ := checkProgressWithPaths(progressRoot)
	remaining := totalImages - completed - invalid
	// Progress items can outnumber the current md refs (renamed/deleted
	// books, older formats): never print a negative "remaining".
	stale := remaining < 0
	if stale {
		remaining = 0
	}

	sep := strings.Repeat("=", 70)
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("【进度摘要】 总计 %d | 已完成 %d | 无效 %d | 剩余 %d\n",
		totalImages, completed, invalid, remaining)
	if invalid > 0 {
		fmt.Printf("  注: “无效”是校验未通过/格式不符而被跳过的条目，下轮 img2text 会自动重试（不是失败）\n")
	}
	if stale {
		fmt.Printf("  注: 进度条目多于当前 md 引用（可能含历史/已删除书目或旧格式条目），剩余按 0 计\n")
	}
	if totalImages > 0 {
		fmt.Printf("  完成率: %.2f%%\n", float64(completed)/float64(totalImages)*100)
	}
	if completed > 0 {
		problem := map[string]struct{}{}
		for _, lf := range logPaths {
			for _, p := range GetProblematicImages(lf) {
				problem[p] = struct{}{}
			}
		}
		good := completed
		if len(problem) > 0 {
			// Without the completed paths, fall back to a conservative
			// estimate (assume the worst case for the footer).
			if len(problem) > completed {
				good = 0
			} else {
				good = completed - len(problem)
			}
		}
		fmt.Printf("  良品率: %.2f%% (%d/%d)\n",
			float64(good)/float64(completed)*100, good, completed)
	}
}

// checkProgressWithPaths mirrors CheckProgressItems but also returns the set
// of completed image paths. We only need the count for the footer, so the
// third return value is unused by callers.
func checkProgressWithPaths(progressRoot string) (completed, invalid int, _ []string) {
	completed, invalid = CheckProgressItems(progressRoot)
	return completed, invalid, nil
}
