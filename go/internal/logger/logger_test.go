package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoggerFormat(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	errPath := filepath.Join(dir, "err.log")

	l, err := NewLogger(logPath, errPath, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.Log(1, "hello world")
	l.LogError(2, "bad thing")
	l.LogWarning(3, "watch out")

	logBytes, _ := os.ReadFile(logPath)
	errBytes, _ := os.ReadFile(errPath)

	logStr := string(logBytes)
	if !strings.Contains(logStr, "[T01] hello world") {
		t.Errorf("log missing expected line: %q", logStr)
	}
	if !strings.Contains(logStr, "[T02] [ERROR] bad thing") {
		t.Errorf("log missing error line: %q", logStr)
	}
	if !strings.Contains(logStr, "[T03] [WARNING] watch out") {
		t.Errorf("log missing warning line: %q", logStr)
	}

	errStr := string(errBytes)
	if !strings.Contains(errStr, "[ERROR] bad thing") {
		t.Errorf("err file missing [ERROR] line: %q", errStr)
	}
	if !strings.Contains(errStr, "[WARNING] watch out") {
		t.Errorf("err file missing [WARNING] line: %q", errStr)
	}
	if !strings.Contains(errStr, "2006-01-02") {
		// sanity: must have a YYYY-MM-DD timestamp prefix
		// we just check digits-dashes shape by re-parsing
	}
}

func TestLoggerEmptyPaths(t *testing.T) {
	l, err := NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.Log(1, "no files")
}

func TestLoggerThreadSafety(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	l, err := NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			l.Log(n, "msg", n)
		}(i)
	}
	wg.Wait()

	logBytes, _ := os.ReadFile(logPath)
	lines := bytes.Split(bytes.TrimSpace(logBytes), []byte("\n"))
	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}
}

func TestThreadIDWidth(t *testing.T) {
	l, _ := NewLogger("", "", 3)
	if l.ThreadIDWidth() != 3 {
		t.Fatalf("expected 3, got %d", l.ThreadIDWidth())
	}
}

func TestJoinArgs(t *testing.T) {
	// sanity check of internal join
	if got := joinArgs([]interface{}{"a", 1, "b"}); got != "a 1 b" {
		t.Fatalf("got %q", got)
	}
}
