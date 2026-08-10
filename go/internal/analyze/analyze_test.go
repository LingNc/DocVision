package analyze

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLog creates a log file under dir/name with optional age offset.
func writeLog(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if age > 0 {
		past := time.Now().Add(-age)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	return path
}

func TestResolveLogPaths_ExplicitLogFileWins(t *testing.T) {
	logsDir := t.TempDir()
	finallyDir := t.TempDir()
	// Put primary logs in both directories; the explicit override must
	// bypass both and be returned unchanged.
	writeLog(t, logsDir, "img2text_20250110_000000.log", time.Hour)
	legacy := writeLog(t, finallyDir, "img2text_20250111_000000.log", time.Hour)

	explicit := filepath.Join(t.TempDir(), "custom.log")
	if err := os.WriteFile(explicit, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := resolveLogPaths(RunOptions{LogFile: explicit}, logsDir, finallyDir)
	if err != nil {
		t.Fatalf("resolveLogPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != explicit {
		t.Fatalf("explicit logfile ignored, got %v", paths)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy log missing: %v", err)
	}
}

func TestResolveLogPaths_FallbackToFinally(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "nope")
	finallyDir := t.TempDir()
	legacy := writeLog(t, finallyDir, "img2text_20250111_000000.log", time.Hour)

	// All=false → FindLatestWithFallback
	paths, err := resolveLogPaths(RunOptions{}, logsDir, finallyDir)
	if err != nil {
		t.Fatalf("resolveLogPaths (latest): %v", err)
	}
	if len(paths) != 1 || paths[0] != legacy {
		t.Fatalf("expected legacy fallback, got %v", paths)
	}

	// All=true → FindAllWithFallback
	paths, err = resolveLogPaths(RunOptions{All: true}, logsDir, finallyDir)
	if err != nil {
		t.Fatalf("resolveLogPaths (all): %v", err)
	}
	if len(paths) != 1 || paths[0] != legacy {
		t.Fatalf("expected legacy fallback for --all, got %v", paths)
	}
}

func TestResolveLogPaths_PrimaryWinsOverFinally(t *testing.T) {
	logsDir := t.TempDir()
	finallyDir := t.TempDir()
	primary := writeLog(t, logsDir, "img2text_20250110_000000.log", time.Hour)
	writeLog(t, finallyDir, "img2text_20250111_000000.log", time.Hour)

	paths, err := resolveLogPaths(RunOptions{}, logsDir, finallyDir)
	if err != nil {
		t.Fatalf("resolveLogPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != primary {
		t.Fatalf("primary wins; got %v", paths)
	}
}

func TestResolveLogPaths_NoLogsAnywhere(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "nope")
	finallyDir := filepath.Join(t.TempDir(), "nope2")
	if _, err := resolveLogPaths(RunOptions{}, logsDir, finallyDir); err == nil {
		t.Fatalf("expected error when neither directory has logs")
	}
}
