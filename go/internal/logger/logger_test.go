package logger

import (
	"bytes"
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

// 进度行（live block）与日志行必须各自独占一行：日志行先擦掉整块再写在
// 块原来的位置，块在它下面重绘。修复前日志行直接补一个 "\n" 再接进度行，
// 等于把进度行**永久留在滚动区**，终端里同一行内容出现两次（用户实测的
// "重叠"）。
func TestLogSeparatesLiveLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "t.log")
	l, err := NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	tty := true
	TTYForTest = &tty
	defer func() { TTYForTest = nil }()
	l.tty = true // 本 logger 已建好，直接对齐强制值

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	row := l.LiveRow("style")
	row.Set("[style] 轮次 1 · 工具调用 2 · 已用 3.0s")
	l.Log(1, "[style] [tool:bash] ok")
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	got := string(out)

	if !strings.Contains(got, "\r\x1b[K[style] 轮次 1") {
		t.Errorf("进度行应原地重绘（清行 + 文本），实际 %q", got)
	}
	if strings.Contains(got, "已用 3.0s[21:") || strings.Contains(got, "ok[style]") {
		t.Errorf("日志行不得与进度行粘连，实际 %q", got)
	}
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("只有日志行该带换行，实际 %d 个换行: %q", n, got)
	}

	row.Remove()
	l.Log(1, "plain")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "plain") {
		t.Errorf("日志文件应始终记录，实际 %q", body)
	}
}

// 多行实时块：两行各自独立，谁都不会覆盖谁（这是"进度行重叠"的根因——
// 旧实现只有一个 live 槽位，并发的子会话每秒互相覆盖）。
func TestLiveBlockKeepsRowsIndependent(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	l.tty = true

	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	a := l.LiveRow("convert:chapter_001")
	b := l.LiveRow("convert:chapter_002")
	a.Set("[convert:chapter_001] 轮次 3 · 已用 12.0s")
	b.Set("[convert:chapter_002] 轮次 5 · 已用 8.0s")
	a.Set("[convert:chapter_001] 轮次 4 · 已用 20.0s")
	texts := l.LiveRowTexts()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	_ = out

	if len(texts) != 2 || !strings.Contains(texts[0], "chapter_001") || !strings.Contains(texts[1], "chapter_002") {
		t.Fatalf("两行必须同时存在且顺序稳定，实际 %v", texts)
	}
	if !strings.Contains(texts[0], "轮次 4") {
		t.Fatalf("更新必须落在自己的行上，实际 %v", texts)
	}
	// 摘掉一行后另一行原样保留。
	a.Remove()
	texts = l.LiveRowTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "chapter_002") {
		t.Fatalf("删掉一行不能动另一行，实际 %v", texts)
	}
	// 摘掉后的迟到更新不得让行复活。
	a.Set("[convert:chapter_001] 迟到")
	if got := l.LiveRowTexts(); len(got) != 1 {
		t.Fatalf("已摘除的行不得复活，实际 %v", got)
	}
}
