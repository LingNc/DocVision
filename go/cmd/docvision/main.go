package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mineru-tools/internal/analyze"
	"mineru-tools/internal/config"
	"mineru-tools/internal/img2text"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/mineru"
	"mineru-tools/internal/organize"
	"mineru-tools/internal/split"
	"mineru-tools/internal/splitlog"
	"mineru-tools/pkg/util"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "docvision",
		Short:         "DocVision — PDF/image parsing and AI post-processing pipeline",
		Long:          "DocVision drives the full pipeline: split PDFs, call the MinerU API, organize output, run image-to-text via AI, and analyze logs.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return err
			}
			if configPath == "" {
				return nil
			}
			// Mirror the Python workflow: chdir to the directory containing the
			// config file so relative paths in the YAML resolve from there.
			abs, err := filepath.Abs(configPath)
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			if err := os.Chdir(filepath.Dir(abs)); err != nil {
				return fmt.Errorf("chdir to config dir: %w", err)
			}
			return nil
		},
	}

	root.PersistentFlags().StringP("config", "c", "config.yaml", "配置文件路径")

	// Reusable split flags attached to the root so the workflow command
	// can read them via the same plumbing as `split`.
	root.PersistentFlags().Bool("all", false, "分割目录下所有 PDF（默认从 config.yaml 的 paths.input_dir）")
	root.PersistentFlags().Bool("force", false, "强制重新分割（忽略已有文件）")
	root.PersistentFlags().Int("max-pages", 200, "每部分最大页数")
	root.PersistentFlags().Float64("max-size-mb", 200, "每部分最大大小 MB（0=不限制）")
	root.PersistentFlags().String("output-dir", "", "输出目录（默认从 config.yaml 的 paths.split_dir）")
	root.PersistentFlags().String("input-dir", "", "--all 模式下的输入目录（默认从 config.yaml 的 paths.input_dir）")

	root.AddCommand(
		newWorkflowCmd(),
		newSplitCmd(),
		newMinerUCmd(),
		newOrganizeCmd(),
		newImg2TextCmd(),
		newAnalyzeCmd(),
		newSplitLogCmd(),
		newInitCmd(),
	)

	return root
}

// loadConfigWithFlag is a small helper that respects the --config flag.
func loadConfigWithFlag(cmd *cobra.Command) (*config.Config, error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	return config.LoadConfig(configPath)
}

func newWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "运行完整工作流或单个步骤",
		Long:  "串联 PDF 分割、MinerU API、文件整理、图片转文本、日志分析等步骤。\n使用 --step 指定单个步骤。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}

			step, err := cmd.Flags().GetString("step")
			if err != nil {
				return err
			}

			steps := []string{"split", "mineru", "organize", "img2text", "analyze"}
			if step != "" {
				steps = []string{step}
			}

			overallStart := time.Now()
			for i, s := range steps {
				fmt.Printf("\n=== Step %d: %s ===\n", i+1, stepLabel(s))
				if err := runStep(s, cmd, cfg); err != nil {
					return fmt.Errorf("step %q failed: %w", s, err)
				}
			}

			fmt.Printf("\n%s\n", strings.Repeat("=", 50))
			fmt.Printf("Workflow finished in %s\n", time.Since(overallStart).Truncate(time.Millisecond))
			fmt.Printf("%s\n", strings.Repeat("=", 50))
			return nil
		},
	}
	cmd.Flags().StringP("step", "s", "", "仅运行指定步骤 (split|mineru|organize|img2text|analyze)")
	return cmd
}

func stepLabel(s string) string {
	labels := map[string]string{
		"split":    "Split PDFs",
		"mineru":   "MinerU API",
		"organize": "Organize Files",
		"img2text": "Image to Text",
		"analyze":  "Analyze Logs",
	}
	if l, ok := labels[s]; ok {
		return l
	}
	return s
}

// runStep dispatches a single named step.
func runStep(step string, cmd *cobra.Command, cfg *config.Config) error {
	switch step {
	case "split":
		return runSplitFromConfig(cmd, cfg)
	case "mineru":
		return runMinerUFromConfig(cmd, cfg)
	case "organize":
		return organize.OrganizeFiles(cfg)
	case "img2text":
		return runImg2TextFromConfig(cmd, cfg)
	case "analyze":
		return runAnalyzeFromConfig(cmd, cfg)
	default:
		return fmt.Errorf("unknown step %q", step)
	}
}

func newSplitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "split [pdf_file]",
		Short: "按页数/大小分割 PDF",
		Long:  "将大 PDF 按指定最大页数或文件大小拆分为多个部分。可处理单个文件或 --all 处理目录下所有 PDF。",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			// Positional argument overrides any --all behaviour.
			if len(args) > 0 {
				maxPages, maxSizeMB, outputDir, force := splitOptsFromFlags(cmd, cfg)
				return split.SplitPDF(args[0], maxPages, maxSizeMB, outputDir, force)
			}
			return runSplitFromConfig(cmd, cfg)
		},
	}
	return cmd
}

// splitOptsFromFlags returns (maxPages, maxSizeMB, outputDir, force) by
// reading the corresponding flags and falling back to config defaults
// when a flag was not explicitly set. outputDir defaults to
// cfg.Paths.SplitDir.
func splitOptsFromFlags(cmd *cobra.Command, cfg *config.Config) (int, float64, string, bool) {
	maxPages := cfg.Mineru.MaxPagesPerPart
	if cmd.Flags().Changed("max-pages") {
		if v, _ := cmd.Flags().GetInt("max-pages"); v > 0 {
			maxPages = v
		}
	}
	maxSizeMB := float64(cfg.Mineru.MaxSizeMB)
	if cmd.Flags().Changed("max-size-mb") {
		if v, _ := cmd.Flags().GetFloat64("max-size-mb"); v > 0 {
			maxSizeMB = v
		}
	}
	outputDir := cfg.Paths.SplitDir
	if dir, _ := cmd.Flags().GetString("output-dir"); dir != "" {
		outputDir = dir
	}
	force, _ := cmd.Flags().GetBool("force")
	return maxPages, maxSizeMB, outputDir, force
}

func runSplitFromConfig(cmd *cobra.Command, cfg *config.Config) error {
	maxPages, maxSizeMB, outputDir, force := splitOptsFromFlags(cmd, cfg)
	all, _ := cmd.Flags().GetBool("all")
	inputDir, _ := cmd.Flags().GetString("input-dir")
	if inputDir == "" {
		inputDir = cfg.Paths.InputDir
	}
	if all {
		return split.SplitAll(inputDir, maxPages, maxSizeMB, outputDir, force)
	}
	return fmt.Errorf("split: 请提供 PDF 文件路径或使用 --all")
}

func newMinerUCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mineru",
		Short: "调用 MinerU API 解析文件",
		Long:  "处理 split_dir 中的分割 PDF 和 input_dir 中的非 PDF 文件（图片、Office 文档等）。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			return runMinerUFromConfig(cmd, cfg)
		},
	}
	cmd.Flags().StringP("file", "f", "", "单个文件路径（不指定则处理目录）")
	return cmd
}

func runMinerUFromConfig(cmd *cobra.Command, cfg *config.Config) error {
	var files []string

	if single, _ := cmd.Flags().GetString("file"); single != "" {
		if !util.FileExists(single) {
			return fmt.Errorf("file not found: %s", single)
		}
		files = []string{single}
	} else {
		if util.DirExists(cfg.Paths.SplitDir) {
			more, err := util.ListAllFiles(cfg.Paths.SplitDir, mineru.SupportedExtensions)
			if err != nil {
				return fmt.Errorf("list split_dir: %w", err)
			}
			files = append(files, more...)
		}
		if util.DirExists(cfg.Paths.InputDir) {
			nonPDF := map[string]bool{}
			for ext := range mineru.SupportedExtensions {
				if ext == ".pdf" {
					continue
				}
				nonPDF[ext] = true
			}
			more, err := util.ListAllFiles(cfg.Paths.InputDir, nonPDF)
			if err != nil {
				return fmt.Errorf("list input_dir: %w", err)
			}
			files = append(files, more...)
		}
	}

	client := mineru.NewClient(cfg.Mineru)
	statusDir := filepath.Join(cfg.Paths.MineruOutput, "status")
	_, _, _ = mineru.ProcessFilesConcurrent(client, files, cfg.Paths.MineruOutput, statusDir, cfg.Mineru.MaxConcurrent)
	return nil
}

func newOrganizeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "organize",
		Short: "整理 MinerU 输出文件",
		Long:  "把 MinerU 产出的多个分片目录合并到 subject 目录，整理 images 等。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			return organize.OrganizeFiles(cfg)
		},
	}
}

func newImg2TextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "img2text",
		Short: "使用 AI 将图片转换为文本",
		Long:  "遍历文档图片，调用多模态大模型生成图片描述，写回 Markdown。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			return runImg2TextFromConfig(cmd, cfg)
		},
	}
	cmd.Flags().Bool("test", false, "启用测试模式（随机抽样）")
	cmd.Flags().Int("number", 10, "测试图片数量（默认: 10）")
	cmd.Flags().String("seed", "", "随机种子：'random' 或数字（用于复现）")
	return cmd
}

func runImg2TextFromConfig(cmd *cobra.Command, cfg *config.Config) error {
	testMode, _ := cmd.Flags().GetBool("test")
	number, _ := cmd.Flags().GetInt("number")
	seed, _ := cmd.Flags().GetString("seed")

	if err := os.MkdirAll(cfg.Paths.FinallyDir, 0o755); err != nil {
		return fmt.Errorf("create finally dir: %w", err)
	}
	ts := time.Now().Format("20060102_150405")
	logPath := filepath.Join(cfg.Paths.FinallyDir, fmt.Sprintf("img2text_%s.log", ts))
	errLogPath := filepath.Join(cfg.Paths.FinallyDir, fmt.Sprintf("img2text_error_%s.log", ts))
	threadIDWidth := len(strconv.Itoa(cfg.Options.Concurrency))
	if threadIDWidth < 2 {
		threadIDWidth = 2
	}
	log, err := logger.NewLogger(logPath, errLogPath, threadIDWidth)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer log.Close()

	opts := img2text.RunOptions{
		TestMode: testMode,
		Number:   number,
		Seed:     seed,
	}
	return img2text.Run(cfg, log, opts)
}

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "分析 img2text 处理日志",
		Long:  "统计工具调用、耗时、成功率、百分位等指标，支持按线程细分。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			return runAnalyzeFromConfig(cmd, cfg)
		},
	}
	cmd.Flags().Bool("all", false, "汇总所有历史日志进行分析")
	cmd.Flags().String("logfile", "", "指定单个日志文件路径")
	cmd.Flags().Bool("threads", false, "显示线程详细统计")
	cmd.Flags().String("percentiles", "90,95,99", "自定义百分位数，逗号分隔 (默认: 90,95,99)")
	cmd.Flags().StringP("output", "o", "", "导出 CSV 文件路径")
	cmd.Flags().Bool("progress", false, "仅显示进度摘要（完成率/良品率）")
	return cmd
}

func runAnalyzeFromConfig(cmd *cobra.Command, cfg *config.Config) error {
	all, _ := cmd.Flags().GetBool("all")
	logFile, _ := cmd.Flags().GetString("logfile")
	showThreads, _ := cmd.Flags().GetBool("threads")
	percentilesStr, _ := cmd.Flags().GetString("percentiles")
	outputCSV, _ := cmd.Flags().GetString("output")
	progressOnly, _ := cmd.Flags().GetBool("progress")

	percentiles, err := parsePercentiles(percentilesStr)
	if err != nil {
		return err
	}

	opts := analyze.RunOptions{
		All:          all,
		LogFile:      logFile,
		ShowThreads:  showThreads,
		Percentiles:  percentiles,
		OutputCSV:    outputCSV,
		ProgressOnly: progressOnly,
	}
	return analyze.Run(cfg, opts)
}

// parsePercentiles converts a comma-separated list of integers into a
// sorted, deduplicated slice. Empty input returns nil so the analyze
// package falls back to its default percentiles.
func parsePercentiles(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	seen := make(map[int]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
			return nil, fmt.Errorf("invalid percentile %q: %w", p, err)
		}
		if n <= 0 || n >= 100 {
			return nil, fmt.Errorf("invalid percentile %d (must be 1-99)", n)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

func newSplitLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "splitlog",
		Short: "按线程 ID 分割日志文件",
		Long:  "默认处理 finally 目录中最新的 img2text_*.log，按线程前缀拆分为多个文件。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			logFile, _ := cmd.Flags().GetString("logfile")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			return splitlog.Run(cfg, logFile, outputDir)
		},
	}
	cmd.Flags().String("logfile", "", "指定日志文件路径（不指定则取最新的 img2text_*.log）")
	cmd.Flags().String("output-dir", "", "输出目录（默认放在日志文件同目录的 <logstem>/）")
	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "生成模板 config.yaml",
		Long:  "在当前目录或 --output 指定的路径写入一份带注释的 config.yaml 模板。",
		RunE: func(cmd *cobra.Command, _ []string) error {
			output, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}
			if _, err := os.Stat(output); err == nil {
				fmt.Printf("Warning: %s already exists, overwriting\n", output)
			}
			if err := os.WriteFile(output, []byte(config.ConfigTemplate), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			fmt.Printf("Wrote config template to %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "config.yaml", "模板输出路径")
	return cmd
}
