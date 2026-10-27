package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/session"
)

// writeActivityTranscript 造一条只有 meta 行的转录，最后一条是 state。
func writeActivityTranscript(t *testing.T, path string, states ...string) {
	t.Helper()
	w, err := session.NewTranscript(path)
	if err != nil {
		t.Fatalf("NewTranscript(%s): %v", path, err)
	}
	for _, st := range states {
		if err := w.AppendNote("chapter_activity", map[string]any{"state": st}); err != nil {
			t.Fatalf("AppendNote: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// T246：上一轮停在 queued 就没了下文（进程被杀/请求刚被消费就退出），
// 页面上会永远挂一句「等待并发槽」。走「已有产物，跳过」时按磁盘收口。
func TestSettleStaleActivityFromDiskFacts(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "convert_chapter_900.jsonl")
	tex := filepath.Join(dir, "chapter_900.tex")
	if err := os.WriteFile(tex, []byte(`\input{chapter_900/part01}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tex+".checker", []byte("missing section"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeActivityTranscript(t, tr, "checking_result", "queued", "queued")

	if got := settleStaleActivity(tr, tex); got != "issues" {
		t.Fatalf("settleStaleActivity = %q, want issues", got)
	}
	if got := lastActivityState(tr); got != "issues" {
		t.Fatalf("转录最后状态 = %q, want issues", got)
	}
	// 已经收口过一次之后再调就不该再动手（不会刷出第二条 note）。
	if got := settleStaleActivity(tr, tex); got != "" {
		t.Fatalf("第二次 settleStaleActivity = %q, want 空", got)
	}
}

func TestSettleStaleActivityOnlyForInFlightStates(t *testing.T) {
	dir := t.TempDir()
	tex := filepath.Join(dir, "chapter_901.tex")
	if err := os.WriteFile(tex, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(dir, "convert_chapter_901.jsonl")
	// pass 是终态：持有者已经收过口，别再盖一次。
	writeActivityTranscript(t, tr, "queued", "pass")
	if got := settleStaleActivity(tr, tex); got != "" {
		t.Fatalf("pass 之后 settleStaleActivity = %q, want 空", got)
	}
	if got := lastActivityState(tr); got != "pass" {
		t.Fatalf("转录最后状态 = %q, want pass", got)
	}
}

func TestSettleStaleActivityWithoutProduct(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "convert_chapter_902.jsonl")
	writeActivityTranscript(t, tr, "queued")
	// 没有产物：这一章的真相是"还没转出来"，跳过路径本来也不该进来，
	// 这里只保证不会凭空写出一种状态。
	if got := settleStaleActivity(tr, filepath.Join(dir, "chapter_902.tex")); got != "" {
		t.Fatalf("无产物时 settleStaleActivity = %q, want 空", got)
	}
	if got := lastActivityState(tr); got != "queued" {
		t.Fatalf("转录最后状态 = %q, want queued（未改动）", got)
	}
}

// 产物在、但没有 .checker（上一轮核对通过后被删）而状态却停在 running：
// 收口成 checking_result，而不是凭空造一个 pass。
func TestSettleStaleActivityProductWithoutVerdict(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "convert_chapter_903.jsonl")
	tex := filepath.Join(dir, "chapter_903.tex")
	if err := os.WriteFile(tex, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeActivityTranscript(t, tr, "running")
	if got := settleStaleActivity(tr, tex); got != "checking_result" {
		t.Fatalf("settleStaleActivity = %q, want checking_result", got)
	}
}
