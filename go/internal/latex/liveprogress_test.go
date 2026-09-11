package latex

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// captureStdout runs fn with os.Stdout replaced by a pipe and returns what
// was written (the progress line is console output, so this is the only way
// to assert what a phase leaves on the terminal).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = old
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// TestLiveProgressTTYCloseRepaints: 终端里进度行是"原地覆写"的一行，而会话
// 实时行（book.go livePhaseLine）结束时会用 \r\x1b[K 把这一行清掉；此时若
// Close 只补一个 \n，阶段之间就会多出一行空白（实测就是
// [classify 8/8] … 与 [process 2/8] … 之间的空行）。Close 必须重画最终
// 状态再换行。
func TestLiveProgressTTYCloseRepaints(t *testing.T) {
	out := captureStdout(t, func() {
		p := newLiveProgressTTY(func() string { return "[classify 8/8] 100.00% (failed: 0, running: 0)" }, true)
		p.Close()
	})
	want := "\r\x1b[K[classify 8/8] 100.00% (failed: 0, running: 0)\r\x1b[K[classify 8/8] 100.00% (failed: 0, running: 0)\n"
	if out != want {
		t.Errorf("终端下进度行字节 = %q\nwant %q", out, want)
	}
	// 关键属性：只换一次行（没有裸 \n 留下的空行）。
	if n := countByte(out, '\n'); n != 1 {
		t.Errorf("换行 %d 次，want 1（多出来的就是那行空白）", n)
	}
}

// TestLiveProgressPipeNoTrailingBlank: 管道/重定向里每次 render 都是整行
// （自带换行），Close 再打一次就是重复行，而且未渲染过时不能凭空多打空行。
func TestLiveProgressPipeNoTrailingBlank(t *testing.T) {
	out := captureStdout(t, func() {
		p := newLiveProgressTTY(func() string { return "[process 2/8] 25.00% (done: 2, running: 4)" }, false)
		p.Close()
	})
	want := "[process 2/8] 25.00% (done: 2, running: 4)\n"
	if out != want {
		t.Errorf("管道下进度行字节 = %q, want %q", out, want)
	}

	// 提示词回调返回空串（verbose 或 total==0）：一行都不该出现。
	out = captureStdout(t, func() {
		p := newLiveProgressTTY(func() string { return "" }, true)
		p.Close()
	})
	if out != "" {
		t.Errorf("空进度行输出了 %q，want 空", out)
	}

	// 两个阶段首尾相接：整体只应有一行空行都没有。
	out = captureStdout(t, func() {
		a := newLiveProgressTTY(func() string { return "[classify 8/8] 100.00%" }, false)
		a.Close()
		b := newLiveProgressTTY(func() string { return "[process 2/8] 25.00%" }, false)
		b.Close()
	})
	want = "[classify 8/8] 100.00%\n[process 2/8] 25.00%\n"
	if out != want {
		t.Errorf("两阶段衔接 = %q, want %q", out, want)
	}
}

func countByte(s string, b byte) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			n++
		}
	}
	return n
}

// TestConvertProgressTextNoStrayBracket: [convert] 进度行的格式串曾经写成
// "%s %d/%d]"（label 本身已带方括号），实际输出 "[convert] 0/3] 0.00% …"；
// 用户还看到两次重绘叠在一起（"[convert] 0/3] 0.00% (done: 0,[convert] 3/3] …"），
// 因为那一行是自己 fmt.Fprintf(os.Stdout, "\r…")，管道里裸 CR 不会覆写。
func TestConvertProgressTextNoStrayBracket(t *testing.T) {
	got := convertProgressText("[convert]", 0, 0, 0, 3, 0)
	want := "[convert] 0/3 0.00% (done: 0, errors: 0, running: 0, " + fmtDuration(0) + ")"
	if got != want {
		t.Errorf("起始行 = %q, want %q", got, want)
	}
	got = convertProgressText("[convert#2]", 3, 2, 0, 3, 200*time.Minute+54*time.Second)
	want = "[convert#2] 3/3 100.00% (done: 1, errors: 2, running: 0, 200m54s)"
	if got != want {
		t.Errorf("结束行 = %q, want %q", got, want)
	}
	// total==0 时不能除零，也不能编出 100%。
	if got := convertProgressText("[convert]", 0, 0, 0, 0, 0); !strings.Contains(got, "0.00%") {
		t.Errorf("total=0 应为 0.00%%: %q", got)
	}
	// 管道里同一行渲染两次 = 两整行，绝不出现裸 CR。
	out := captureStdout(t, func() {
		p := newLiveProgressTTY(func() string { return convertProgressText("[convert]", 3, 2, 0, 3, 0) }, false)
		p.Close()
	})
	if strings.Contains(out, "\r") {
		t.Errorf("管道输出里不该有裸 CR: %q", out)
	}
	if countByte(out, '\n') != 1 {
		t.Errorf("管道里应恰好一行: %q", out)
	}
}

// TestPhaseNoteDoesNotOverwriteLiveLine: 阶段提示必须走 logger.PrintConsole
// （先结束实时行、打印、再重画），直接写 stdout 会得到
// "[chapters] 轮次 6 · 工具调用 9 · 已用 1m18s[chapters] 划分完成: 3 章"。
func TestPhaseNoteDoesNotOverwriteLiveLine(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(dir+"/run.log", dir+"/err.log", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	note := r.phaseNote()
	// 装一条"实时行"（模拟 chapters 会话正在进行）。
	log.SetLiveLine(func() { fmt.Fprint(os.Stdout, "LIVE") })
	out := captureStdout(t, func() { note("[chapters] 划分完成: %d 章", 3) })
	log.SetLiveLine(nil)
	if !strings.Contains(out, "\n[chapters] 划分完成: 3 章\n") {
		t.Errorf("阶段提示没有先结束实时行: %q", out)
	}
	if strings.Contains(out, "LIVE[chapters]") {
		t.Errorf("阶段提示被写在实时行上: %q", out)
	}
	if !strings.HasSuffix(out, "LIVE") {
		t.Errorf("打印后应重画实时行: %q", out)
	}
}
