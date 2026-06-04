package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mineru-tools/internal/config"
)

// RunOptions configures the analyze command.
type RunOptions struct {
	All          bool
	LogFile      string
	ShowThreads  bool
	Percentiles  []int
	OutputCSV    string
	ProgressOnly bool
}

// Run is the entry point for log analysis + progress checking.
func Run(cfg *config.Config, opts RunOptions) error {
	if opts.Percentiles == nil {
		opts.Percentiles = []int{90, 95, 99}
	}

	inputDir := cfg.Paths.OutputDir
	finallyDir := cfg.Paths.FinallyDir
	progressRoot := filepath.Join(finallyDir, "progress_items")

	if opts.ProgressOnly {
		PrintProgressReport(inputDir, progressRoot, finallyDir)
		return nil
	}

	// Resolve log files to analyse.
	logPaths, err := resolveLogPaths(opts, finallyDir)
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

	// Always print a progress summary footer.
	printProgressFooter(inputDir, progressRoot, finallyDir, logPaths)

	if opts.OutputCSV != "" {
		if err := ExportCSV(allSessions, opts.OutputCSV); err != nil {
			return err
		}
	}
	_ = stats
	return nil
}

// resolveLogPaths picks the log files to analyse based on opts.
func resolveLogPaths(opts RunOptions, finallyDir string) ([]string, error) {
	if opts.LogFile != "" {
		return []string{opts.LogFile}, nil
	}
	if opts.All {
		paths := findAllLogs(finallyDir)
		if len(paths) == 0 {
			return nil, fmt.Errorf("未找到任何日志文件")
		}
		fmt.Printf("找到 %d 个日志文件\n", len(paths))
		return paths, nil
	}
	latest := findLatestLog(finallyDir)
	if latest == "" {
		return nil, fmt.Errorf("未找到日志文件")
	}
	return []string{latest}, nil
}

// findLatestLog returns the path of the newest img2text_*.log under dir.
func findLatestLog(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "img2text_*.log"))
	if len(matches) == 0 {
		return ""
	}
	// Sort by modification time, newest first.
	sort.Slice(matches, func(i, j int) bool {
		ai, _ := os.Stat(matches[i])
		aj, _ := os.Stat(matches[j])
		if ai == nil || aj == nil {
			return false
		}
		return ai.ModTime().After(aj.ModTime())
	})
	return matches[0]
}

// printProgressFooter prints the brief "总计/已完成/无效/剩余" line.
func printProgressFooter(inputDir, progressRoot, finallyDir string, logPaths []string) {
	totalImages := CountImagesInMDFiles(inputDir)
	completed, invalid, _ := checkProgressWithPaths(progressRoot)
	remaining := totalImages - completed - invalid

	sep := strings.Repeat("=", 70)
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("【进度摘要】 总计 %d | 已完成 %d | 无效 %d | 剩余 %d\n",
		totalImages, completed, invalid, remaining)
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
