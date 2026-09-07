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
	"mineru-tools/pkg/util"
)

// newLatexCmd wires the LaTeX output pipeline (level 1 / level 2).
func newLatexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "latex [书.pdf|书.docx|书.md ...]",
		Short: "LaTeX 输出管线（档位2：图片矢量化 / 档位1：全书 LaTeX）",
		Long: `LaTeX 输出（自带工作流：前置流程 + 按档位处理 + 日志分析）。

输入约定与 workflow 一致——源头是 paths.input_dir（files/）：
  docvision latex             无参数：自动跑 split→mineru→organize（跳过已处理）
                              一路到 output/，然后按档位处理全部 md，最后日志分析
  docvision latex 书.pdf      单文件：自动复制进 files/ 并跑前置，只处理该文件
  docvision latex 书.docx
  docvision latex 书.md       自备 md：复制进 files/ 后整理进 output/ 直接处理
                              （output/ 里的 md 直接选用，不做拒绝）

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

			// 输入约定：与 workflow 相同——源头是 paths.input_dir（files/）。
			// - 无参数：像 workflow 一样跑前置（split→mineru→organize，
			//   自动跳过已处理文件）一路到 output/，然后按档位处理全部 md。
			// - PDF/DOCX 参数：先复制进 files/，跑前置（只处理新文件），
			//   再对产出的 md 继续 LaTeX。
			// - md 参数：视为用户自备的输入文件（放在 files/ 中），整理
			//     整理进 output/ 后直接 LaTeX（不做硬拒绝）。
			selected, err := prepareLatexInputs(cmd, cfg, args)
			if err != nil {
				return err
			}

			log, closeLog, err := newLatexLogger(cfg)
			if err != nil {
				return err
			}
			defer closeLog()

			runner := latex.NewRunner(cfg, log)
			if cfg.Latex.Level == 1 {
				fmt.Println("=== LaTeX 档位 1：全书转换 ===")
				if err := runner.RunBook(latex.BookOptions{
					Step: step, SourceDir: sourceDir, Restart: false,
					TestMode: testMode, Number: number, Seed: seed,
					Files: selected,
				}); err != nil {
					return err
				}
			} else {
				fmt.Println("=== LaTeX 档位 2：图片矢量化 ===")
				if err := runner.RunImages(latex.ImagesOptions{
					Step: step, TestMode: testMode, Number: number, Seed: seed,
					SourceDir: sourceDir, Files: selected,
				}); err != nil {
					return err
				}
			}
			// LaTeX 自带工作流属性：结束后自动做日志分析。
			fmt.Println("\n=== 日志分析 ===")
			return runAnalyzeFromConfig(cmd, cfg, "")
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

// prepareLatexInputs applies the files/-centred input contract and
// returns the selected markdown names (with .md suffix) for the latex
// runner:
//   - PDF/DOCX args are copied into paths.input_dir (unless already
//     there) and the split → mineru → organize prerequisites run with
//     their normal skip-done behaviour (like workflow).
//   - md args are treated as user-supplied input from files/: staged
//     into output/ (+ adjacent images) and processed directly.
//   - No args: full prerequisites then every md in output/.
func prepareLatexInputs(cmd *cobra.Command, cfg *config.Config, args []string) ([]string, error) {
	var sources, mds []string
	for _, arg := range args {
		switch strings.ToLower(filepath.Ext(arg)) {
		case ".pdf", ".docx":
			sources = append(sources, arg)
		case ".md", ".markdown":
			mds = append(mds, arg)
		default:
			return nil, fmt.Errorf("不支持的输入 %q：请传 files/ 中的 PDF/DOCX（或自备 md）", arg)
		}
	}

	// PDF/DOCX：归位到 files/，再跑前置（已处理过的自动跳过）。
	if len(sources) > 0 {
		if err := os.MkdirAll(cfg.Paths.InputDir, 0o755); err != nil {
			return nil, err
		}
		for _, src := range sources {
			abs, err := filepath.Abs(src)
			if err != nil {
				return nil, err
			}
			dst := filepath.Join(cfg.Paths.InputDir, filepath.Base(abs))
			if !util.FileExists(dst) {
				if err := copyFile(dst, abs); err != nil {
					return nil, err
				}
			}
		}
		for _, stepName := range []string{"split", "mineru", "organize"} {
			fmt.Printf("\n=== LaTeX 前置: %s ===\n", stepLabel(stepName))
			if _, err := runStep(stepName, cmd, cfg, ""); err != nil {
				return nil, fmt.Errorf("前置步骤 %s 失败: %w", stepName, err)
			}
		}
	}

	// md：output/ 里的直接选用（本就是整理产物）；其他位置的复制进
	// files/ 后再整理进 output/。不做任何硬拒绝——用户路径里带 output
	// 之类的名字完全正常。
	var selected []string
	for _, md := range mds {
		abs, err := filepath.Abs(md)
		if err != nil {
			return nil, err
		}
		outAbs, _ := filepath.Abs(cfg.Paths.OutputDir)
		if outAbs != "" && (abs == outAbs || strings.HasPrefix(abs, outAbs+string(filepath.Separator))) {
			selected = append(selected, filepath.Base(abs))
			continue
		}
		inDir, _ := filepath.Abs(cfg.Paths.InputDir)
		if inDir == "" || (!strings.HasPrefix(abs, inDir+string(filepath.Separator)) && filepath.Dir(abs) != inDir) {
			if err := os.MkdirAll(cfg.Paths.InputDir, 0o755); err != nil {
				return nil, err
			}
			copied := filepath.Join(cfg.Paths.InputDir, filepath.Base(abs))
			if !util.FileExists(copied) {
				if err := copyFile(copied, abs); err != nil {
					return nil, err
				}
			}
			abs = copied
		}
		if err := stageUserMD(cfg, abs); err != nil {
			return nil, err
		}
		selected = append(selected, filepath.Base(abs))
	}

	// 无参数：确认 output/ 里确实有内容（前置没跑过会误导）。
	if len(args) == 0 && len(sources) == 0 {
		exists, _ := filepath.Glob(filepath.Join(cfg.Paths.OutputDir, "*.md"))
		if len(exists) == 0 {
			return nil, fmt.Errorf("output/ 中没有 markdown。请先把 PDF/DOCX 放到 files/ 并运行本命令（会自动跑前置流程）")
		}
		fmt.Println("处理 output/ 中全部 markdown:", len(exists), "个")
	}
	return selected, nil
}

// stageUserMD copies a user-supplied markdown (and its adjacent images
// folder, if any) into output/, mimicking what organize produces, so
// the latex pipeline can treat it like any other input.
func stageUserMD(cfg *config.Config, mdPath string) error {
	if err := os.MkdirAll(cfg.Paths.OutputDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(cfg.Paths.OutputDir, filepath.Base(mdPath))
	if !util.FileExists(dst) {
		if err := copyFile(dst, mdPath); err != nil {
			return err
		}
	}
	stem := strings.TrimSuffix(filepath.Base(mdPath), filepath.Ext(mdPath))
	for _, imgDir := range []string{
		filepath.Join(filepath.Dir(mdPath), stem, "images"),
		filepath.Join(filepath.Dir(mdPath), "images"),
	} {
		entries, err := os.ReadDir(imgDir)
		if err != nil {
			continue
		}
		dest := filepath.Join(cfg.Paths.ImagesDir, stem)
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			from := filepath.Join(imgDir, e.Name())
			to := filepath.Join(dest, e.Name())
			if !util.FileExists(to) {
				if err := copyFile(to, from); err != nil {
					return err
				}
			}
		}
		fmt.Println("已随 md 整理图片:", imgDir, "->", dest)
		break
	}
	return nil
}
