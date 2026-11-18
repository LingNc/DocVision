// Package prep 是 PREP 阶段的外部工具编排（split/mineru/organize）。
// 纯 subprocess 管道，不含判据——判据全在 verify/dedupe 等包；本包只做
// "调用什么工具、参数怎么给、产物在哪"（行为契约见 docs/latex.md 各阶段）。
// 所有外部命令可注入（CommandRunner），测试用假工具驱动。
package prep

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Runner 执行一条外部命令；默认实现 exec.Command。测试注入假 runner。
type Runner func(name string, args ...string) (output []byte, err error)

// DefaultRunner 是默认外部命令执行器。
func DefaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// SplitConfig 是 PDF 拆分的参数。
type SplitConfig struct {
	PagesPerPart int // 每个 part 的页数（旧线档位1按内存/时长权衡定，如 100）
}

// Split 把整本 PDF 按固定页数拆成 <stem>_part1..N.pdf（poppler 工具链：
// pdfinfo 数页 → pdfseparate 拆单页 → pdfunite 并成 part）。
// 幂等：已存在的 part 不重建（单页临时文件用后即删）。
func Split(pdfPath, outDir string, cfg SplitConfig, run Runner) ([]string, error) {
	if cfg.PagesPerPart <= 0 {
		cfg.PagesPerPart = 100
	}
	if run == nil {
		run = DefaultRunner
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, fmt.Errorf("PDF 不存在: %s", pdfPath)
	}
	stem := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	// 页数（pdfinfo）
	out, err := run("pdfinfo", pdfPath)
	if err != nil {
		return nil, fmt.Errorf("pdfinfo 失败（poppler-utils 装了？）: %v %.200s", err, out)
	}
	pages := 0
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "Pages:") {
			pages, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(ln, "Pages:")))
		}
	}
	if pages <= 0 {
		return nil, fmt.Errorf("pdfinfo 未给出页数")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp(outDir, ".split-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	var parts []string
	pageFmt := filepath.Join(tmpDir, "page-%d.pdf")
	for start := 1; start <= pages; start += cfg.PagesPerPart {
		end := start + cfg.PagesPerPart - 1
		if end > pages {
			end = pages
		}
		partName := fmt.Sprintf("%s_part%d.pdf", stem, len(parts)+1)
		partPath := filepath.Join(outDir, partName)
		if _, err := os.Stat(partPath); err == nil {
			parts = append(parts, partName)
			continue
		}
		// 拆出本 part 覆盖的单页
		var singles []string
		for p := start; p <= end; p++ {
			single := fmt.Sprintf(pageFmt, p)
			if _, err := os.Stat(single); err != nil {
				if out, err := run("pdfseparate", "-f", strconv.Itoa(p), "-l", strconv.Itoa(p), pdfPath, pageFmt); err != nil {
					return parts, fmt.Errorf("pdfseparate 第 %d 页失败: %v %.200s", p, err, out)
				}
			}
			singles = append(singles, single)
		}
		args := append([]string{}, singles...)
		args = append(args, partPath)
		if out, err := run("pdfunite", args...); err != nil {
			return parts, fmt.Errorf("pdfunite %s 失败: %v %.200s", partName, err, out)
		}
		parts = append(parts, partName)
	}
	return parts, nil
}
