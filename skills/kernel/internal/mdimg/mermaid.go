// Package mdimg 是档位2（img2text 模式）的 kernel 侧：旧线 go/internal/img2text
// 移植。vision API 会话半不移植（新架构由宿主 agent 看图）；这里保留**规则与
// 组装**——mermaid 块抽取/校验、md 图片按决策重组装、页码标记（下批）。
package mdimg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var mermaidBlockRe = regexp.MustCompile("(?is)```[ \t]*mermaid[ \t]*\\r?\\n(.*?)```")

// MermaidValidationResult 描述文本中全部 mermaid 块的校验结果（旧线逐字）。
type MermaidValidationResult struct {
	HasMermaid bool
	Valid      bool
	Available  bool
	Error      string
}

// ExtractMermaidBlocks 返回全部 mermaid 围栏代码块的内容（旧线逐字）。
func ExtractMermaidBlocks(text string) []string {
	matches := mermaidBlockRe.FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			blocks = append(blocks, strings.TrimSpace(match[1]))
		}
	}
	return blocks
}

// ValidateMermaid 用 mmdc 逐块校验 mermaid（旧线逐字；缺 mmdc 报 Available=false
// 由调用方按宽松/严格策略处置）。
func ValidateMermaid(ctx context.Context, text, command string, timeout time.Duration) MermaidValidationResult {
	blocks := ExtractMermaidBlocks(text)
	if len(blocks) == 0 {
		return MermaidValidationResult{Valid: true}
	}
	if command == "" {
		command = "mmdc"
	}
	if _, err := exec.LookPath(command); err != nil {
		return MermaidValidationResult{
			HasMermaid: true,
			Available:  false,
			Error:      fmt.Sprintf("Mermaid validator %q not found: %v", command, err),
		}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	workDir, err := os.MkdirTemp("", "docvision-mermaid-")
	if err != nil {
		return MermaidValidationResult{HasMermaid: true, Available: true, Error: err.Error()}
	}
	defer os.RemoveAll(workDir)

	for i, block := range blocks {
		inputPath := filepath.Join(workDir, fmt.Sprintf("diagram-%d.mmd", i))
		outputPath := filepath.Join(workDir, fmt.Sprintf("diagram-%d.svg", i))
		if err := os.WriteFile(inputPath, []byte(block+"\n"), 0o600); err != nil {
			return MermaidValidationResult{HasMermaid: true, Available: true, Error: err.Error()}
		}

		callCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(callCtx, command, "-i", inputPath, "-o", outputPath)
		output, runErr := cmd.CombinedOutput()
		cancel()
		if runErr != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = runErr.Error()
			}
			return MermaidValidationResult{
				HasMermaid: true,
				Available:  true,
				Error:      fmt.Sprintf("block %d: %s", i+1, message),
			}
		}
	}
	return MermaidValidationResult{HasMermaid: true, Available: true, Valid: true}
}
