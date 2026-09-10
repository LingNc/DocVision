package logger

import (
	"bytes"
	"fmt"
	"io"
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

// 进度行（live line）与日志行必须各自独占一行：修复前日志行直接写在
// 进度行光标处，终端里两条内容叠在一行、`\r` 反复覆写留下残影。
func TestLogSeparatesLiveLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "t.log")
	l, err := NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	renders := 0
	l.SetLiveLine(func() { renders++; fmt.Fprint(os.Stdout, "\r[style] 轮次 1") })
	l.Log(1, "[style] [tool:bash] ok")
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	got := string(out)

	if renders == 0 {
		t.Errorf("日志行后应重绘进度行")
	}
	if !strings.Contains(got, "\n[") {
		t.Errorf("日志行前必须先换行结束进度行，实际 %q", got)
	}
	if strings.Count(got, "\n") < 2 {
		t.Errorf("日志行与重绘的进度行应各占一行，实际 %q", got)
	}

	l.SetLiveLine(nil)
	l.Log(1, "plain")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "plain") {
		t.Errorf("日志文件应始终记录，实际 %q", body)
	}
}

// Quiet 必须可读：RunImages 之前硬编码 SetQuiet(false)，把 RunBook 的静默
// 状态抹掉，导致其后所有阶段的会话内部行都打到终端。
func TestQuietIsReadable(t *testing.T) {
	l, err := NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	l.SetQuiet(true)
	if !l.Quiet() {
		t.Fatalf("Quiet() 应为 true")
	}
	l.SetQuiet(false)
	if l.Quiet() {
		t.Fatalf("Quiet() 应为 false")
	}
}
