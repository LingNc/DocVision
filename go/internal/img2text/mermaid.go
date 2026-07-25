package img2text

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

// MermaidValidationResult describes validation of all Mermaid blocks in text.
type MermaidValidationResult struct {
	HasMermaid bool
	Valid      bool
	Available  bool
	Error      string
}

// ExtractMermaidBlocks returns the contents of fenced Mermaid code blocks.
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

// ValidateMermaid validates every Mermaid fenced block using the configured
// Mermaid CLI command. A missing command is reported as unavailable so the
// caller can apply the configured auto/strict policy.
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
