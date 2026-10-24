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

// TestRescueCutRepeatedOnlyTwice：同一个切口只允许截一次，重复判定要被拦住
// （T232）；换一个切口或换一条转录都算新情况。
func TestRescueCutRepeatedOnlyTwice(t *testing.T) {
	r := &Runner{}
	tr := filepath.Join(t.TempDir(), "checker_chapter_006.try4.jsonl")

	if r.rescueCutRepeated(tr, 69) {
		t.Fatal("第一次截断不该被拦")
	}
	if !r.rescueCutRepeated(tr, 69) {
		t.Fatal("同一个切口第二次应当被拦")
	}
	if r.rescueCutRepeated(tr, 40) {
		t.Fatal("换了切口应当放行")
	}
	if r.rescueCutRepeated(tr+"x", 40) {
		t.Fatal("换一条转录应当放行")
	}
}
