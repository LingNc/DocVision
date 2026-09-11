package latex

import (
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

// newTestLiveProgress 造一条进度的实时行：logger 的 tty 由参数强制
// （测试的 stdout 是管道，靠探测永远走不到终端那条路）。
func newTestLiveProgress(t *testing.T, text func() string, tty bool) (*liveProgress, *logger.Logger) {
	t.Helper()
	dir := t.TempDir()
	forced := tty
	logger.TTYForTest = &forced // 必须在 NewLogger 之前：tty 是建 logger 时定的
	log, err := logger.NewLogger(dir+"/run.log", dir+"/err.log", 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.TTYForTest = nil; _ = log.Close() })
	return newLiveProgressRow(log, "test", text), log
}

// TestLiveProgressTTYCloseRepaints: 终端里进度行是"原地覆写"的一行，
// Close 把最终状态定格成普通行（否则阶段之间会多出空白行，或者最终状态
// 随实时块一起消失）。只换一次行。
func TestLiveProgressTTYCloseRepaints(t *testing.T) {
	out := captureStdout(t, func() {
		p, _ := newTestLiveProgress(t, func() string { return "[classify 8/8] 100.00% (failed: 0, running: 0)" }, true)
		p.Close()
	})
	if !strings.Contains(out, "[classify 8/8] 100.00% (failed: 0, running: 0)") {
		t.Fatalf("终端下必须画出并定格最终状态: %q", out)
	}
	if !strings.HasSuffix(out, "100.00% (failed: 0, running: 0)\n") {
		t.Errorf("定格行必须以换行结束: %q", out)
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
		p, _ := newTestLiveProgress(t, func() string { return "[process 2/8] 25.00% (done: 2, running: 4)" }, false)
		p.Close()
	})
	want := "[process 2/8] 25.00% (done: 2, running: 4)\n"
	if out != want {
		t.Errorf("管道下进度行字节 = %q, want %q", out, want)
	}

	// 提示词回调返回空串（verbose 或 total==0）：一行都不该出现。
	out = captureStdout(t, func() {
		p, _ := newTestLiveProgress(t, func() string { return "" }, true)
		p.Close()
	})
	if out != "" {
		t.Errorf("空进度行输出了 %q，want 空", out)
	}

	// 两个阶段首尾相接：整体只应有一行空行都没有。
	out = captureStdout(t, func() {
		a, _ := newTestLiveProgress(t, func() string { return "[classify 8/8] 100.00%" }, false)
		a.Close()
		b, _ := newTestLiveProgress(t, func() string { return "[process 2/8] 25.00%" }, false)
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
		p, _ := newTestLiveProgress(t, func() string { return convertProgressText("[convert]", 3, 2, 0, 3, 0) }, false)
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
	forced := true
	logger.TTYForTest = &forced // 必须在 NewLogger 之前：tty 是建 logger 时定的
	defer func() { logger.TTYForTest = nil }()
	log, err := logger.NewLogger(dir+"/run.log", dir+"/err.log", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	note := r.phaseNote()
	// 装一条实时行（模拟 chapters 会话正在进行）。
	row := log.LiveRow("chapters")
	row.Set("LIVE")
	out := captureStdout(t, func() { note("[chapters] 划分完成: %d 章", 3) })
	if !strings.Contains(out, "[chapters] 划分完成: 3 章\n") {
		t.Errorf("阶段提示必须整行输出: %q", out)
	}
	if strings.Contains(out, "LIVE[chapters]") || strings.Contains(out, "章LIVE") {
		t.Errorf("阶段提示与实时行粘连了: %q", out)
	}
	if !strings.HasSuffix(out, "LIVE") {
		t.Errorf("打印后应重画实时行: %q", out)
	}
}

// TestLiveProgressNoDuplicateFinalLine: 用户实测 classify 与 process 各出现
// 两行完全相同的进度——阶段末尾那段 `progress(); Fprintln(os.Stdout)` 先
// 整行 + 换行，deferred Close 又重画一遍同一行。现在定格只由 Close 负责，
// 且管道里连续相同的状态不重复整行：每个状态**恰好一行**。
func TestLiveProgressNoDuplicateFinalLine(t *testing.T) {
	states := []string{
		"[classify 0/8] 0.00% (failed: 0, running: 5)",
		"[classify 8/8] 100.00% (failed: 0, running: 0)",
	}
	i := 0
	out := captureStdout(t, func() {
		p, _ := newTestLiveProgress(t, func() string { return states[i] }, false)
		i = 1
		p.render() // 状态变了 → 一行
		p.render() // 状态没变 → 不再重复
		p.Close()  // 文本已整行输出过 → 什么都不打
	})
	want := states[0] + "\n" + states[1] + "\n"
	if out != want {
		t.Errorf("管道输出 = %q, want %q（每个状态恰好一行）", out, want)
	}
	if strings.Count(out, states[1]) != 1 {
		t.Errorf("最终行出现了 %d 次，want 1", strings.Count(out, states[1]))
	}

	// 终端里则相反：Close 必须重画最终一行（会话实时行可能把它清掉了），
	// 且只换一次行。
	out = captureStdout(t, func() {
		i = 0
		p, _ := newTestLiveProgress(t, func() string { return states[0] }, true)
		i = 0
		p.Close()
	})
	if countByte(out, '\n') != 1 {
		t.Errorf("终端里换行 %d 次，want 1: %q", countByte(out, '\n'), out)
	}
	if strings.Count(out, states[0]) != 2 {
		t.Errorf("终端里最终行应重画一次（共 2 次原地写）: %q", out)
	}
}
