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
	"mineru-tools/internal/organize"
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

档位 2（latex.level: 2）——产物是"可以直接当 markdown 读"的干净文本：
  专用分类 AI 逐图标记 text（样式化文本）/ vector（可用 LaTeX 重绘）/ raster（保留原图）。
  text   -> 只提取图中可见文本（公式→LaTeX、表格→Markdown；无文字则保留原图），
            纯文本嵌入正文（无 [AI]/[IMG_TYPE] 标记）。
  vector -> 专用作图 AI 会话 write_file 写代码 -> compile 编译（只回日志/产物名/页数）
            -> view_pdf 看自己编译出的 PDF 视觉核对 -> submit；
            产物转 SVG 后以 ![label](figures/*.svg) 嵌入（SVG 不可用退 PNG/PDF 链接）。
  raster -> 保留原图链接（insert_image_description 开启时才嵌入可读解释文本）。
  档位2 输出**不含任何 <!-- DOCVISION-* --> 注释**，失败只记日志与 progress.json。

档位 1（latex.level: 1）——全书 LaTeX，产物是 cls + 分章 .tex + 编译好的 PDF：
  images   图片处理（classify → 矢量代码块/文本/raster 按骨架嵌入 md，
           注释首行闭合，CONTENT:/LINK: 字段在注释外）
  style    样式分析 AI 读全书 md + 原书扫描页，产出 book.cls + 使用手册 + 案例，
           在工作区里反复 compile/view_pdf 自查后 submit
  chapters 章节划分 AI（grep + read_file + bash + 记忆缓冲区）切分章节
  convert  转换 AI 并发逐章转 .tex：每章有**私有工作视图**（只能读写自己那章），
           别人的成品只能经只读通道 project:converted/、project:reports/ 参考；
           工具 compile/view_pdf/view_image/doc_search；每章产物交 checker 小模型核对，
           硬性问题回同一会话最多 3 轮，仍不过才换新会话重转换一次
  feedback 多数章节报 cls/手册问题时打回原样式会话；样式包更新后只对
           「报问题」或「新 cls 下编译不过」的章节并发跑样式修复子会话（不重转换）
  assemble 汇总编译全书 PDF（latexmk 多遍），失败进入修复会话；
           成功后进入终审会话逐页核对成品 PDF 并整理，最后写 standalone.tex；
           交付把整棵 build 树复制到 out/（book.pdf = main.pdf 别名）

所有 AI 会话支持：独立模型配置（models: 注册表）、上下文窗口配置、自动压缩、
可分离工具注册、JSONL 转录断点续传。每个会话有独立命名空间（挂载表：work 可写，
project/source 只读），会话 bash 默认跑在 bubblewrap 沙箱里（tools.bash.sandbox），
临时工作区落在 <项目>/work/temp（latex.keep_temp_dirs 可保留，debug 下必定保留）。`,
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
			verbose, _ := cmd.Flags().GetBool("verbose")
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

			log, logPath, closeLog, err := newLatexLogger(cfg)
			applyLogLevel(cmd, cfg, log)
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
					Files: selected, Verbose: verbose,
				}); err != nil {
					return err
				}
			} else {
				fmt.Println("=== LaTeX 档位 2：图片矢量化 ===")
				if err := runner.RunImages(latex.ImagesOptions{
					Step: step, TestMode: testMode, Number: number, Seed: seed,
					SourceDir: sourceDir, Files: selected,
					Verbose: verbose,
				}); err != nil {
					return err
				}
			}
			// LaTeX 自带工作流属性：结束后自动做日志分析（默认只分析
			// 本次运行的日志；显式 --all 才汇总全部历史）。
			analyzeLog := logPath
			if all, _ := cmd.Flags().GetBool("all"); all {
				analyzeLog = ""
			}
			fmt.Println("\n=== 日志分析 ===")
			// 进度摘要指向 latex 输出目录（档位1: <latex_project>/source，
			// 档位2: latex.output_dir），避免误读 img2text 的历史进度。
			latexOut := cfg.Paths.LatexOutput
			if cfg.Latex.Level == 1 {
				latexOut = filepath.Join(cfg.Paths.LatexProject, "source")
			}
			return runAnalyzeFromConfigDir(cmd, cfg, analyzeLog, latexOut)
		},
	}
	cmd.Flags().Int("level", 0, "覆盖配置的档位（1=全书 LaTeX，2=图片矢量化）")
	cmd.Flags().String("step", "", "仅运行指定阶段（档位2: classify|process；档位1: images|style|chapters|convert|assemble）")
	cmd.Flags().Bool("test", false, "测试模式（随机抽样图片）")
	cmd.Flags().Int("number", 10, "测试图片数量")
	cmd.Flags().String("seed", "", "随机种子")
	cmd.Flags().String("source-dir", "", "覆盖输入 markdown 目录（默认 paths.output_dir）")
	cmd.Flags().Bool("all", false, "日志分析汇总全部历史日志（默认只分析本次运行）")
	cmd.Flags().Bool("debug", false, "调试模式：记录请求参数/提示词/工具调用/响应统计到日志文件")
	cmd.Flags().Bool("trace", false, "深度调试：在 debug 基础上再记录流式分片等细节")
	cmd.Flags().Bool("verbose", false, "详细控制台输出（默认仅显示 img2text 风格的进度行，详情写日志文件）")
	return cmd
}

// newVerifyCmd wires the AI verification pass (default off in config).
func newVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "AI 核对输出内容与原图是否一致（仅显式运行，自动流程不调用）",
		Long: `用配置的视觉模型逐项核对每张图片与其嵌入内容（描述/矢量代码）是否一致，
给出问题与修改意见，写入 verify_report.md。不会修改输出本身。

本核对**只在显式运行本命令时执行**：workflow 与 latex 自动流程都不会调用它
（v1.3 起 workflow 已不含 verify 步骤）。verify.enabled 只作为提示信息，
控制台会告知当前取值，不影响本命令是否运行。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigWithFlag(cmd)
			if err != nil {
				return err
			}
			log, _, closeLog, err := newLatexLogger(cfg)
			if err != nil {
				return err
			}
			defer closeLog()
			applyLogLevel(cmd, cfg, log)
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
	cmd.Flags().Bool("debug", false, "调试模式：记录请求参数/提示词/响应统计到日志文件")
	cmd.Flags().Bool("trace", false, "深度调试：在 debug 基础上再记录流式分片等细节")
	return cmd
}

// newLatexLogger creates the shared latex/verify logger.
func newLatexLogger(cfg *config.Config) (*logger.Logger, string, func(), error) {
	if err := os.MkdirAll(cfg.Paths.LogsDir, 0o755); err != nil {
		return nil, "", nil, fmt.Errorf("create logs dir: %w", err)
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
		return nil, "", nil, err
	}
	return log, logPath, func() { log.Close() }, nil
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
	// 前置步骤复用 split 的参数体系（--all/--force/max-pages 等）。
	if cmd.Flags().Lookup("all") == nil {
		addSplitFlags(cmd)
	}
	var sources, mds []string
	var selected []string
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

	// PDF/DOCX：归位到 files/，只拆这些文件，再跑 mineru → organize。
	if len(sources) > 0 {
		if err := os.MkdirAll(cfg.Paths.InputDir, 0o755); err != nil {
			return nil, err
		}
		maxPages, maxSizeMB, splitOut, force := splitOptsFromFlags(cmd, cfg)
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
			fmt.Printf("\n=== LaTeX 前置: Split Documents (%s) ===\n", filepath.Base(dst))
			if err := splitOne(dst, maxPages, maxSizeMB, splitOut, force); err != nil {
				return nil, fmt.Errorf("前置步骤 split 失败: %w", err)
			}
		}
		// 只处理指定源对应的分片与 subject，不全量扫描 files/。
		fmt.Printf("\n=== LaTeX 前置: %s ===\n", stepLabel("mineru"))
		if err := processMinerUFiles(cfg, mineruFilesForSources(cfg, splitOut, sources)); err != nil {
			return nil, fmt.Errorf("前置步骤 mineru 失败: %w", err)
		}
		fmt.Printf("\n=== LaTeX 前置: %s ===\n", stepLabel("organize"))
		var subjects []string
		for _, src := range sources {
			subjects = append(subjects, strings.TrimSuffix(filepath.Base(src), filepath.Ext(filepath.Base(src))))
		}
		if err := organize.OrganizeFiles(cfg, subjects...); err != nil {
			return nil, fmt.Errorf("前置步骤 organize 失败: %w", err)
		}
		// 前置产出的 md（<stem>.md）就是本次 LaTeX 的处理范围。
		for _, src := range sources {
			base := filepath.Base(src)
			selected = append(selected, strings.TrimSuffix(base, filepath.Ext(base))+".md")
		}
	}

	// md：output/ 里的直接选用（本就是整理产物）；其他位置的复制进
	// files/ 后再整理进 output/。不做任何硬拒绝——用户路径里带 output
	// 之类的名字完全正常。
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

	// 无参数：正常走前置流程（split --all → mineru → organize，均自动
	// 跳过已处理内容），然后处理 output/ 中的全部 markdown。
	if len(args) == 0 && len(sources) == 0 {
		for _, stepName := range []string{"split", "mineru", "organize"} {
			fmt.Printf("\n=== LaTeX 前置: %s ===\n", stepLabel(stepName))
			if _, err := runStep(stepName, cmd, cfg, ""); err != nil {
				return nil, fmt.Errorf("前置步骤 %s 失败: %w", stepName, err)
			}
		}
		exists, _ := filepath.Glob(filepath.Join(cfg.Paths.OutputDir, "*.md"))
		if len(exists) == 0 {
			return nil, fmt.Errorf("前置流程完成后 output/ 中没有 markdown：请确认 files/ 中有 PDF/DOCX")
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
