package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/pkg/util"

	"github.com/spf13/cobra"
)

func runAdHocWorkflow(cmd *cobra.Command, cfg *config.Config, target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", target, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("路径不存在: %s", target)
	}
	var inputs []string
	destDir := abs
	if info.IsDir() {
		pdfs, err := util.ListFiles(abs, ".pdf")
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		docxs, err := util.ListFiles(abs, ".docx")
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		for _, f := range append(pdfs, docxs...) {
			if !strings.HasPrefix(filepath.Base(f), ".") {
				inputs = append(inputs, f)
			}
		}
		if len(inputs) == 0 {
			return fmt.Errorf("目录中没有 PDF/DOCX 文件: %s", abs)
		}
	} else {
		ext := strings.ToLower(filepath.Ext(abs))
		if ext != ".pdf" && ext != ".docx" {
			return fmt.Errorf("仅支持 PDF/DOCX 文件: %s", abs)
		}
		inputs = []string{abs}
		destDir = filepath.Dir(abs)
	}
	if strings.Contains(cfg.Mineru.Token, "YOUR-") || strings.TrimSpace(cfg.Mineru.Token) == "" ||
		strings.Contains(cfg.Models["text"].APIKey, "YOUR-") || strings.TrimSpace(cfg.Models["text"].APIKey) == "" {
		return fmt.Errorf("配置中的 mineru.token / ai.api_key 还是占位符，请先运行 docvision setup 配置")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	job := filepath.Join(home, ".docvision", "jobs", base+"-"+time.Now().Format("20060102_150405"))
	for _, d := range []string{"files", "split_files", "mineru_output", "output", "output/images", "finally", "logs", "files/done"} {
		if err := os.MkdirAll(filepath.Join(job, d), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	for _, in := range inputs {
		if err := copyFile(filepath.Join(job, "files", filepath.Base(in)), in); err != nil {
			return fmt.Errorf("copy %s: %w", in, err)
		}
	}
	fmt.Printf("工作目录: %s\n", job)
	fmt.Printf("输入文件: %d 个\n", len(inputs))
	cfg.Paths.InputDir = filepath.Join(job, "files")
	cfg.Paths.SplitDir = filepath.Join(job, "split_files")
	cfg.Paths.MineruOutput = filepath.Join(job, "mineru_output")
	cfg.Paths.OutputDir = filepath.Join(job, "output")
	cfg.Paths.ImagesDir = filepath.Join(job, "output/images")
	cfg.Paths.FinallyDir = filepath.Join(job, "finally")
	cfg.Paths.LogsDir = filepath.Join(job, "logs")
	cfg.Paths.DoneDir = filepath.Join(job, "files/done")
	oldWd, wdErr := os.Getwd()
	if wdErr == nil {
		defer os.Chdir(oldWd)
	}
	os.Chdir(job)
	steps := []string{"split", "mineru", "organize", "img2text", "analyze"}
	overallStart := time.Now()
	var currentImg2TextLog string
	for i, s := range steps {
		fmt.Printf("\n=== Step %d: %s ===\n", i+1, stepLabel(s))
		stepLog, err := runStep(s, cmd, cfg, currentImg2TextLog)
		if err != nil {
			return fmt.Errorf("step %q failed: %w", s, err)
		}
		if s == "img2text" {
			currentImg2TextLog = stepLog
		}
	}
	fmt.Printf("\n%s\n", strings.Repeat("=", 50))
	fmt.Printf("Workflow finished in %s\n", time.Since(overallStart).Truncate(time.Millisecond))
	fmt.Printf("%s\n", strings.Repeat("=", 50))
	mds, err := filepath.Glob(filepath.Join(cfg.Paths.FinallyDir, "*.md"))
	if err != nil || len(mds) == 0 {
		fmt.Println("警告: finally 中没有生成 .md")
		return nil
	}
	for _, md := range mds {
		dst := filepath.Join(destDir, filepath.Base(md))
		if err := copyFile(dst, md); err != nil {
			return fmt.Errorf("copy %s: %w", dst, err)
		}
		fmt.Printf("输出: %s\n", dst)
	}
	return nil
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
