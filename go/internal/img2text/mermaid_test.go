package img2text

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExtractMermaidBlocks(t *testing.T) {
	text := "before\n```mermaid\ngraph TD\n A-->B\n```\nafter\n```mermaid\nsequenceDiagram\n A->>B: hi\n```"
	blocks := ExtractMermaidBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("got %d Mermaid blocks, want 2", len(blocks))
	}
	if blocks[0] != "graph TD\n A-->B" {
		t.Fatalf("unexpected first block: %q", blocks[0])
	}
}

func TestValidateMermaidWithoutBlocks(t *testing.T) {
	result := ValidateMermaid(context.Background(), "plain text", "missing-command", time.Second)
	if result.HasMermaid || !result.Valid {
		t.Fatalf("plain text result = %+v, want valid without Mermaid", result)
	}
}

func TestValidateMermaidWithFakeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a shell command")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "mmdc")
	script := "#!/bin/sh\ncase \"$*\" in *invalid*) exit 1;; esac\nexit 0\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	valid := ValidateMermaid(context.Background(), "```mermaid\ngraph TD\nA-->B\n```", "mmdc", time.Second)
	if !valid.HasMermaid || !valid.Available || !valid.Valid {
		t.Fatalf("valid result = %+v", valid)
	}
}

func TestValidateMermaidCommandUnavailable(t *testing.T) {
	result := ValidateMermaid(context.Background(), "```mermaid\ngraph TD\nA-->B\n```", "definitely-not-docvision-mmdc", time.Second)
	if !result.HasMermaid || result.Available || result.Valid || result.Error == "" {
		t.Fatalf("unavailable result = %+v", result)
	}
}
