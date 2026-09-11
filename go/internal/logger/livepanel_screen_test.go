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
