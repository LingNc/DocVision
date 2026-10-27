package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeReminderTranscript 写一条带 n 次「没调用 submit」提醒的核对转录。
func writeReminderTranscript(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	lines := []map[string]any{{"t": "msg", "role": "system", "text": "You are a checker."}}
	for i := 0; i < n; i++ {
		lines = append(lines,
			map[string]any{"t": "msg", "role": "assistant", "text": "looks fine"},
			map[string]any{"t": "msg", "role": "user", "text": checkerSubmitReminderMark + " Use the `submit` TOOL."},
		)
	}
	for _, l := range lines {
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNextCheckerAttemptRotatesSpentTranscript：打满提醒的那条要换下一条全新的，
// 而不是把同一条死转录再推一遍（T232）。
func TestNextCheckerAttemptRotatesSpentTranscript(t *testing.T) {
	nodeTr := filepath.Join(t.TempDir(), "checker_chapter_004.jsonl")
	writeReminderTranscript(t, attemptTranscriptPath(nodeTr, 1), checkerNoVerdictAttempts)

	if got := nextCheckerAttempt(nodeTr, 1, 3); got != 2 {
		t.Fatalf("打满的尝试应换到 try2，实际 try%d", got)
	}
	// try2 是干净文件：停在 try2。
	if got := nextCheckerAttempt(nodeTr, 2, 3); got != 2 {
		t.Fatalf("干净尝试应原地不动，实际 try%d", got)
	}
}

// TestNextCheckerAttemptStopsAtBumpLimit：连续几条都打满时不要把尝试号推到天上，
// 交回入口守卫如实报「没结论」。
func TestNextCheckerAttemptStopsAtBumpLimit(t *testing.T) {
	nodeTr := filepath.Join(t.TempDir(), "checker_chapter_005.jsonl")
	for _, n := range []int{1, 2, 3} {
		writeReminderTranscript(t, attemptTranscriptPath(nodeTr, n), checkerNoVerdictAttempts)
	}
	if got := nextCheckerAttempt(nodeTr, 1, 3); got != 1 {
		t.Fatalf("三条都打满应保持原尝试号，实际 try%d", got)
	}
}

// writeLongTranscript 写一条 n 行的消息转录（每一行都是一条消息，切点校验要求）。
func writeLongTranscript(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		data, err := json.Marshal(map[string]any{"t": "msg", "role": "assistant", "text": "step"})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRescueCutRefusal：截断只在"确实能接着跑"时做（T238）。留下来的前缀太少
// 等于重跑，同一个位置再切一次还是同一个死法——两种都该被拦住，让这一章如实
// 停下而不是继续换尝试。
func TestRescueCutRefusal(t *testing.T) {
	r := &Runner{}
	dir := t.TempDir()
	tr := filepath.Join(dir, "convert_chapter_006.jsonl")
	writeLongTranscript(t, tr, 100)
	target := func(trPath, role string) rescueTarget {
		return rescueTarget{Base: "chapter_006", Role: role, TrPath: trPath}
	}

	if why := r.rescueCutRefusal(target(tr, "convert"), 8); why == "" {
		t.Fatal("只留下前 8/100 行等于重跑，应当拦住")
	}
	if why := r.rescueCutRefusal(target(tr, "convert"), 30); why != "" {
		t.Fatalf("留下 30/100 行应当放行，却被拦住：%s", why)
	}
	// 换一条尝试（新转录）但同一章同一角色、切口就在上次附近：还是同一个位置。
	next := filepath.Join(dir, "convert_chapter_006.try2.jsonl")
	writeLongTranscript(t, next, 100)
	if why := r.rescueCutRefusal(target(next, "convert"), 33); why == "" {
		t.Fatal("同一章同一角色切过的位置附近，应当拦住")
	}
	if why := r.rescueCutRefusal(target(next, "convert"), 70); why != "" {
		t.Fatalf("换了位置的切口应当放行，却被拦住：%s", why)
	}
	// 别的角色是另一条会话，不该被这一章的转换切口挡住。
	if why := r.rescueCutRefusal(target(next, "checker"), 33); why != "" {
		t.Fatalf("另一个角色应当放行，却被拦住：%s", why)
	}
}
