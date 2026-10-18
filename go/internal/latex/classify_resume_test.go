package latex

import (
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T112: resuming process on already classified images must not print a stale
// classify 0/N progress line or initialise an unused classifier client.
func TestClassifyPhaseResumeAllClassified(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	r := &Runner{cfg: &config.Config{}, log: log}
	tasks := []*task{{mdName: "book.md", imgPath: "a.jpg"}, {mdName: "book.md", imgPath: "b.jpg"}}
	prog := map[string]*imageProgress{}
	for _, task := range tasks {
		prog[task.key()] = &imageProgress{Class: ClassVector, Status: "classified"}
	}
	out := captureStdout(t, func() { r.classifyPhase(tasks, nil, prog, dir, false) })
	if !strings.Contains(out, "[classify 2/2] 100.00%") || !strings.Contains(out, ") done") || strings.Contains(out, "[classify 0/") {
		t.Fatalf("unexpected classify resume progress: %q", out)
	}
}
