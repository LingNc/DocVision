package logger

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// screen replays an ANSI byte stream into a simple terminal model and returns
// the visible lines. Modelling the cursor is the only way to prove the claim
// "two live owners never overlap and log lines are never clobbered" — asserting
// on raw bytes cannot tell an in-place repaint from text appended below it.
func screen(seq string) []string {
	type pos struct{ row, col int }
	lines := []string{""}
	cur := pos{}
	ensure := func(row int) {
		for len(lines) <= row {
			lines = append(lines, "")
		}
	}
	put := func(s string) {
		ensure(cur.row)
		line := []rune(lines[cur.row])
		for len(line) < cur.col {
			line = append(line, ' ')
		}
		runes := []rune(s)
		for i, r := range runes {
			if cur.col+i < len(line) {
				line[cur.col+i] = r
			} else {
				line = append(line, r)
			}
		}
		lines[cur.row] = string(line)
		cur.col += len(runes)
	}
	clearToEOL := func() {
		ensure(cur.row)
		line := []rune(lines[cur.row])
		if cur.col < len(line) {
			lines[cur.row] = string(line[:cur.col])
		}
	}
	for i := 0; i < len(seq); {
		c := seq[i]
		switch {
		case c == 0x1b && i+1 < len(seq) && seq[i+1] == '[':
			j := i + 2
			for j < len(seq) && (seq[j] == ';' || (seq[j] >= '0' && seq[j] <= '9')) {
				j++
			}
			if j >= len(seq) {
				break
			}
			arg := seq[i+2 : j]
			switch seq[j] {
			case 'K':
				clearToEOL()
			case 'A':
				n := 1
				if arg != "" {
					n, _ = strconv.Atoi(arg)
				}
				cur.row -= n
				if cur.row < 0 {
					cur.row = 0
				}
			}
			i = j + 1
		case c == '\r':
			cur.col = 0
			i++
		case c == '\n':
			cur.row++
			cur.col = 0
			ensure(cur.row)
			i++
		default:
			j := i
			for j < len(seq) && seq[j] != 0x1b && seq[j] != '\r' && seq[j] != '\n' {
				j++
			}
			put(seq[i:j])
			i = j
		}
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return lines
}

// The user-visible contract (2026-09-11 feedback): concurrent owners each get
// their own line; a finished owner's line disappears; log lines keep their own
// line and are never overwritten by a repaint.
func TestLivePanelNeverOverlapsOwnerRows(t *testing.T) {
	dir := t.TempDir()
	forced := true
	TTYForTest = &forced
	defer func() { TTYForTest = nil }()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 4)
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
	// 现场顺序：convert 聚合行 + 两个章节子会话行同时在跑，期间穿插日志行。
	agg := l.LiveRow("convert")
	a := l.LiveRow("convert:chapter_001")
	b := l.LiveRow("convert:chapter_002")
	agg.Set("[convert] 1/4 25.00% (done: 1, errors: 0, running: 2, 12.0s)")
	a.Set("[convert:chapter_001] 轮次 7 · 工具调用 14 · 已用 12.0s")
	b.Set("[convert:chapter_002] 轮次 3 · 工具调用 5 · 已用 9.0s")
	l.Log(1, "[checker:chapter_001] 开始核对")
	b.Set("[convert:chapter_002] 轮次 4 · 工具调用 6 · 已用 15.0s")
	a.Remove() // chapter_001 跑完 → 它那一行消失
	l.Log(1, "[checker:chapter_001] 核对完成")
	_ = w.Close()
	os.Stdout = old
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, rerr := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	_ = r.Close()

	lines := screen(string(buf))
	joined := strings.Join(lines, "\n")
	t.Logf("终端最终画面:\n%s", joined)

	// 两个 checker 日志行必须完整存在（不被任何重绘覆盖/截断）。
	for _, want := range []string{"[checker:chapter_001] 开始核对", "[checker:chapter_001] 核对完成"} {
		found := false
		for _, ln := range lines {
			if strings.Contains(ln, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("日志行被覆盖/截断了: %q\n画面:\n%s", want, joined)
		}
	}
	// 已结束的 chapter_001 子会话行必须消失，仍在跑的 chapter_002 与聚合行必须还在。
	if strings.Contains(joined, "chapter_001] 轮次") {
		t.Errorf("跑完的子会话行没有消失:\n%s", joined)
	}
	if !strings.Contains(joined, "chapter_002] 轮次 4") {
		t.Errorf("仍在跑的子会话行丢了:\n%s", joined)
	}
	if !strings.Contains(joined, "[convert] 1/4 25.00%") {
		t.Errorf("聚合进度行丢了:\n%s", joined)
	}
	// 任何一行里都不该出现两个实时行拼在一起（"重叠"的具体形态）。
	for _, ln := range lines {
		if strings.Count(ln, "[convert") > 1 {
			t.Errorf("同一行里出现多个实时行（重叠）: %q", ln)
		}
	}
}

// captureConsole runs fn with os.Stdout replaced by a pipe and returns the
// raw bytes (the live block is console output; only the byte stream can tell
// an in-place redraw from a fresh line).
func captureConsole(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	_ = w.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, rerr := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	_ = r.Close()
	return string(buf)
}

// maxCursorUp 取字节流里最大的光标上移量（"\x1b[NA"）：它必须小于块行数，
// 否则终端会把光标夹在屏幕顶部，擦除就变成"把上面的历史滚掉"。
func maxCursorUp(seq string) int {
	max := 0
	for i := 0; i+2 < len(seq); {
		if seq[i] != 0x1b || seq[i+1] != '[' {
			i++
			continue
		}
		j := i + 2
		for j < len(seq) && seq[j] >= '0' && seq[j] <= '9' {
			j++
		}
		if j < len(seq) && seq[j] == 'A' && j > i+2 {
			if n, _ := strconv.Atoi(seq[i+2 : j]); n > max {
				max = n
			}
		}
		i = j
	}
	return max
}

// 一屏放不下的时候，块必须自己截断（T226）：否则写满几十行的块比终端还高，
// 每秒一次的"上移光标 + 清行"会把滚动缓冲里的历史一并滚掉，用户往上翻什么
// 都没有。截断按优先级——运行中的常驻，排队的最先折进一行提示。
func TestLiveBlockBoundedByTerminalHeight(t *testing.T) {
	dir := t.TempDir()
	forcedTTY := true
	TTYForTest = &forcedTTY
	rows := 12 // cap = 12-6 = 6 行
	TermRowsForTest = &rows
	defer func() {
		TTYForTest = nil
		TermRowsForTest = nil
	}()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	l.tty = true

	out := captureConsole(t, func() {
		for i := 0; i < 20; i++ {
			row := l.LiveRow("checker:queued_" + strconv.Itoa(i))
			row.SetWithPriority("[checker:queued_"+strconv.Itoa(i)+"] 排队 · 轮次 3", 3)
		}
		l.LiveRow("run:a").SetWithPriority("[convert:run_a] 运行 · 轮次 9", 0)
		l.LiveRow("run:b").SetWithPriority("[convert:run_b] 运行 · 轮次 4", 0)
	})

	lines := screen(out)
	liveLines := 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "[") || strings.HasPrefix(ln, "……") {
			liveLines++
		}
	}
	if liveLines > 6 {
		t.Errorf("实时块画了 %d 行，超过终端能给的 6 行:\n%s", liveLines, strings.Join(lines, "\n"))
	}
	if !strings.Contains(out, "…… 另有 17 个会话") {
		t.Errorf("被折叠的 17 行必须有一行提示，实际:\n%s", strings.Join(lines, "\n"))
	}
	for _, want := range []string{"[convert:run_a]", "[convert:run_b]"} {
		if !strings.Contains(out, want) {
			t.Errorf("运行中的会话必须常驻（优先级 0），缺 %q", want)
		}
	}
	if strings.Contains(out, "queued_19") {
		t.Errorf("排队第 20 个不该占屏幕（该折起来）:\n%s", strings.Join(lines, "\n"))
	}
	if up := maxCursorUp(out); up > 5 {
		t.Errorf("光标上移了 %d 行，超过块高减一（会把历史滚掉）: %q", up, out)
	}
}

// 被折叠掉的会话跑完时，它的定格行照样落到滚动区：屏幕上省了地方，账不能少。
func TestLiveBlockHiddenRowStillFinalizes(t *testing.T) {
	dir := t.TempDir()
	forcedTTY := true
	TTYForTest = &forcedTTY
	rows := 8 // cap = 3
	TermRowsForTest = &rows
	defer func() {
		TTYForTest = nil
		TermRowsForTest = nil
	}()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	l.tty = true

	var hidden *LiveRow
	out := captureConsole(t, func() {
		for i := 0; i < 6; i++ {
			row := l.LiveRow("chapter_" + strconv.Itoa(i))
			row.SetWithPriority("[convert:chapter_"+strconv.Itoa(i)+"] 排队", 3)
			if i == 5 {
				hidden = row
			}
		}
		hidden.Finalize("[convert:chapter_5] 轮次 11 · 工具调用 20 · 已用 2m01s")
	})
	lines := screen(out)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "[convert:chapter_5] 轮次 11 · 工具调用 20 · 已用 2m01s") {
		t.Errorf("折叠掉的行完成后必须留下定格行:\n%s", joined)
	}
}

// 每秒的刷新只重写"变了的那几行"（T226 的频闪）：没变的行一个字都不重写。
func TestLiveBlockRepaintsOnlyChangedLines(t *testing.T) {
	dir := t.TempDir()
	forcedTTY := true
	TTYForTest = &forcedTTY
	defer func() { TTYForTest = nil }()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	l.tty = true

	a := l.LiveRow("a")
	b := l.LiveRow("b")
	c := l.LiveRow("c")
	out := captureConsole(t, func() {
		a.Set("[convert:chapter_a] 轮次 3 · 已用 1m00s")
		b.Set("[convert:chapter_b] 轮次 3 · 已用 2m00s")
		c.Set("[convert:chapter_c] 轮次 3 · 已用 3m00s")
		// 只有 b 的计时在动：a/c 一个字都不该重写。
		b.Set("[convert:chapter_b] 轮次 3 · 已用 2m01s")
		b.Set("[convert:chapter_b] 轮次 4 · 已用 2m02s")
	})
	if got := strings.Count(out, "[convert:chapter_a]"); got != 1 {
		t.Errorf("没变的行被重写了 %d 次（频闪来源）: %q", got, out)
	}
	if got := strings.Count(out, "[convert:chapter_c]"); got != 1 {
		t.Errorf("没变的行被重写了 %d 次（频闪来源）: %q", got, out)
	}
	if got := strings.Count(out, "已用 2m01s"); got != 1 {
		t.Errorf("变化行应恰好写一次，实际 %d 次: %q", got, out)
	}
}

// 管道/重定向不截断：日志里必须留下每一个会话的每一行（屏幕上省，账不省）。
func TestLiveBlockPipeKeepsAllRows(t *testing.T) {
	dir := t.TempDir()
	rows := 8
	TermRowsForTest = &rows
	defer func() { TermRowsForTest = nil }()
	l, err := NewLogger(filepath.Join(dir, "t.log"), "", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	out := captureConsole(t, func() {
		for i := 0; i < 10; i++ {
			l.LiveRow("r"+strconv.Itoa(i)).SetWithPriority("[convert:pipe_"+strconv.Itoa(i)+"] 排队", 3)
		}
	})
	for i := 0; i < 10; i++ {
		want := "[convert:pipe_" + strconv.Itoa(i) + "]"
		if !strings.Contains(out, want) {
			t.Errorf("管道里少了 %q（不截断）: %q", want, out)
		}
	}
	if strings.Contains(out, "另有") {
		t.Errorf("管道里不该出现折叠提示: %q", out)
	}
}
