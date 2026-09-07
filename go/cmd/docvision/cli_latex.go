package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mineru-tools/internal/config"
	"mineru-tools/internal/latex"
	"mineru-tools/internal/logger"
)

// newLatexCmd wires the LaTeX output pipeline (level 1 / level 2).
func newLatexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "latex [file.md ...]",
		Short: "LaTeX 输出管线（档位2：图片矢量化 / 档位1：全书 LaTeX）",
		Long: `基于 output/ 中 MinerU 整理后的 Markdown 构建 LaTeX 输出。

档位 2（latex.level: 2）:
  专用分类 AI 逐图标记 text（艺术字文本）/ vector（可用 TikZ 重绘）/ raster（保留原图）。
  text   -> 复用图片解释 AI，纯内容嵌入文本流（无 [AI]/[IMG_TYPE] 标记）。
  vector -> 专用作图 AI 会话写 TikZ -> 自动编译 -> 栅格化 PNG 回给模型视觉核对
            -> 确认提交；编译产物 PDF 以矢量图嵌入 Markdown。
  raster -> 保留原图链接（insert_image_description 开启时嵌入可读解释文本）。

档位 1（latex.level: 1）:
  在档位 2 图片处理之上：样式分析 AI 产出 book.cls + 使用手册 + 案例（自动试编译），
  章节划分 AI（grep + 最小 bash 沙箱）切分章节，转换 AI 并发逐章转 .tex，
  最终汇总编译全书 PDF 并生成单文件 standalone.tex。

所有 AI 会话支持：独立模型配置（models: 注册表）、上下文窗口配置、自动压缩、
可分离工具注册。进度自动断点续传。`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			level, _ := cmd.Flags().GetInt("level")
			if level != 0 {
				cfg.Latex.Level = level
			}
			if cfg.Latex.Level != 1 && cfg.Latex.Level != 2 {
				return fmt.Errorf("latex.level 必须为 1 或 2（当前 %d）", cfg.Latex.Level)
			}
			step, _ := cmd.Flags().GetString("step")
			testMode, _ := cmd.Flags().GetBool("test")
			number, _ := cmd.Flags().GetInt("number")
			seed, _ := cmd.Flags().GetString("seed")
			sourceDir, _ := cmd.Flags().GetString("source-dir")

			// Self-service: PDF/DOCX/目录 参数会自动补跑前置流程
			// （split → mineru → organize，隔离在 ~/.docvision/jobs 作业目录），
			// 然后对产出的 markdown 继续 LaTeX 流程；md 名字参数则直接
			// 选中 output/ 中的对应文件。不带参数 = 批量处理全部 md。
			jobs, mdFiles := partitionLatexArgs(args)
			for _, arg := range jobs {
				if err := runLatexPrerequisites(cmd, cfg, arg); err != nil {
					return err
				}
			}
			selected := append(mdFiles, latexSelectedFiles...)
			if latexSelectedSourceDir != "" && sourceDir == "" {
				sourceDir = latexSelectedSourceDir
			}

			log, closeLog, err := newLatexLogger(cfg)
			if err != nil {
				return err
			}
			defer closeLog()

			runner := latex.NewRunner(cfg, log)
			if cfg.Latex.Level == 1 {
				fmt.Println("=== LaTeX 档位 1：全书转换 ===")
				return runner.RunBook(latex.BookOptions{
					Step: step, SourceDir: sourceDir, Restart: false,
					TestMode: testMode, Number: number, Seed: seed,
					Files: selected,
				})
			}
			fmt.Println("=== LaTeX 档位 2：图片矢量化 ===")
			return runner.RunImages(latex.ImagesOptions{
				Step: step, TestMode: testMode, Number: number, Seed: seed,
				SourceDir: sourceDir, Files: selected,
			})
		},
	}
	cmd.Flags().Int("level", 0, "覆盖配置的档位（1=全书 LaTeX，2=图片矢量化）")
	cmd.Flags().String("step", "", "仅运行指定阶段（档位2: classify|process；档位1: images|style|chapters|convert|assemble）")
	cmd.Flags().Bool("test", false, "测试模式（随机抽样图片）")
	cmd.Flags().Int("number", 10, "测试图片数量")
	cmd.Flags().String("seed", "", "随机种子")
	cmd.Flags().String("source-dir", "", "覆盖输入 markdown 目录（默认 paths.output_dir）")
	return cmd
}

// newVerifyCmd wires the AI verification pass (default off in config).
func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "AI 核对输出内容与原图是否一致（verify.enabled 默认关闭）",
		Long: `用配置的视觉模型逐项核对每张图片与其嵌入内容（描述/TikZ）是否一致，
给出问题与修改意见，写入 verify_report.md。不会修改输出本身。
显式运行本命令不受 verify.enabled 开关限制；workflow 自动流程中则仅在
verify.enabled: true 时执行。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			log, closeLog, err := newLatexLogger(cfg)
			if err != nil {
				return err
			}
			defer closeLog()
			if !cfg.Verify.Enabled {
				fmt.Println("提示: verify.enabled 当前为 false（本命令为显式运行，照常执行）")
			}
			progressDir, _ := cmd.Flags().GetString("progress-dir")
			report, _ := cmd.Flags().GetString("report")
			runner := latex.NewRunner(cfg, log)
			return runner.RunVerify(latex.VerifyOptions{
				ProgressDir: progressDir,
				ReportPath:  report,
			})
		},
	}
}

// newLatexLogger creates the shared latex/verify logger.
func newLatexLogger(cfg *config.Config) (*logger.Logger, func(), error) {
	if err := os.MkdirAll(cfg.Paths.LogsDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create logs dir: %w", err)
	}
	ts := time.Now().Format("20060102_150405")
	logPath := filepath.Join(cfg.Paths.LogsDir, "latex_"+ts+".log")
	errLogPath := filepath.Join(cfg.Paths.LogsDir, "latex_error_"+ts+".log")
	width := len(strconv.Itoa(cfg.Latex.Concurrency))
	if width < 2 {
		width = 2
	}
	log, err := logger.NewLogger(logPath, errLogPath, width)
	if err != nil {
		return nil, nil, err
	}
	return log, func() { log.Close() }, nil
}

// runLatexFromConfig is the workflow --step latex entry: it runs the
// configured latex level over every md in output_dir (or the files
// selected via latex.files when provided).
func runLatexFromConfig(cfg *config.Config) error {
	if cfg.Latex.Level != 1 && cfg.Latex.Level != 2 {
		return fmt.Errorf("latex.level 必须为 1或 2（当前 %d）", cfg.Latex.Level)
	}
	log, closeLog, err := newLatexLogger(cfg)
	if err != nil {
		return err
	}
	defer closeLog()
	runner := latex.NewRunner(cfg, log)
	if cfg.Latex.Level == 1 {
		return runner.RunBook(latex.BookOptions{})
	}
	return runner.RunImages(latex.ImagesOptions{})
}

// runVerifyFromConfig is the workflow --step verify entry. It respects
// verify.enabled (skips with a notice when disabled).
func runVerifyFromConfig(cfg *config.Config) error {
	if !cfg.Verify.Enabled {
		fmt.Println("verify.enabled 为 false，跳过 AI 核对步骤（如需启用请修改 config.yaml）")
		return nil
	}
	log, closeLog, err := newLatexLogger(cfg)
	if err != nil {
		return err
	}
	defer closeLog()
	return latex.NewRunner(cfg, log).RunVerify(latex.VerifyOptions{})
}

// latexSelectedFiles collects the markdown selections derived from the
// latex command arguments (explicit md names, or md files produced by
// auto-run prerequisites for PDF/DOCX inputs).
var latexSelectedFiles []string

// latexSelectedSourceDir overrides the latex source dir when
// prerequisites produced a dedicated output directory.
var latexSelectedSourceDir string

// partitionLatexArgs splits the positional arguments into source
// documents (PDF/DOCX files or directories containing them, which need
// the MinerU prerequisites) and markdown selections (files in the
// output dir).
func partitionLatexArgs(args []string) (sources []string, mdFiles []string) {
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err == nil && !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(arg))
			if ext == ".pdf" || ext == ".docx" {
				sources = append(sources, arg)
				continue
			}
		}
		if err == nil && info.IsDir() {
			pdfs, _ := filepath.Glob(filepath.Join(arg, "*.pdf"))
			docxs, _ := filepath.Glob(filepath.Join(arg, "*.docx"))
			if len(pdfs)+len(docxs) > 0 {
				sources = append(sources, arg)
				continue
			}
		}
		mdFiles = append(mdFiles, arg)
	}
	return sources, mdFiles
}

// runLatexPrerequisites runs split → mineru → organize for one source
// document inside an isolated job directory (same layout as the
// ad-hoc workflow), then registers the produced markdown files so the
// latex runner picks them up.
func runLatexPrerequisites(cmd *cobra.Command, cfg *config.Config, source string) error {
	abs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	var inputs []string
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		pdfs, _ := filepath.Glob(filepath.Join(abs, "*.pdf"))
		docxs, _ := filepath.Glob(filepath.Join(abs, "*.docx"))
		inputs = append(pdfs, docxs...)
	} else {
		inputs = []string{abs}
	}
	base := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	job := filepath.Join(home, ".docvision", "jobs", base+"-latex-"+time.Now().Format("20060102_150405"))
	for _, d := range []string{"files", "split_files", "mineru_output", "output", "output/images", "logs", "files/done"} {
		if err := os.MkdirAll(filepath.Join(job, d), 0o755); err != nil {
			return err
		}
	}
	for _, in := range inputs {
		if err := copyFile(filepath.Join(job, "files", filepath.Base(in)), in); err != nil {
			return err
		}
	}
	fmt.Printf("LaTeX 前置流程工作目录: %s\n", job)

	jobCfg := *cfg
	jobCfg.Paths.InputDir = filepath.Join(job, "files")
	jobCfg.Paths.SplitDir = filepath.Join(job, "split_files")
	jobCfg.Paths.MineruOutput = filepath.Join(job, "mineru_output")
	jobCfg.Paths.OutputDir = filepath.Join(job, "output")
	jobCfg.Paths.ImagesDir = filepath.Join(job, "output/images")
	jobCfg.Paths.LogsDir = filepath.Join(job, "logs")
	jobCfg.Paths.DoneDir = filepath.Join(job, "files/done")

	oldWd, wdErr := os.Getwd()
	if wdErr == nil {
		defer os.Chdir(oldWd)
	}
	os.Chdir(job)
	for _, s := range []string{"split", "mineru", "organize"} {
		fmt.Printf("\n=== LaTeX 前置: %s ===\n", stepLabel(s))
		if _, err := runStep(s, cmd, &jobCfg, ""); err != nil {
			return fmt.Errorf("前置步骤 %s 失败: %w", s, err)
		}
	}
	// The latex pipeline itself must see the job's mineru_output
	// (original pages for style analysis) and output dir.
	cfg.Paths.MineruOutput = jobCfg.Paths.MineruOutput
	cfg.Paths.ImagesDir = jobCfg.Paths.ImagesDir
	mds, _ := filepath.Glob(filepath.Join(jobCfg.Paths.OutputDir, "*.md"))
	for _, md := range mds {
		latexSelectedFiles = append(latexSelectedFiles, filepath.Base(md))
	}
	latexSelectedSourceDir = jobCfg.Paths.OutputDir
	return nil
}
