package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAttemptTranscriptPath pins the per-attempt transcript naming: the first
// attempt keeps the plain name, later attempts get `.tryN` so every
// re-conversion keeps its own conversation instead of overwriting the one
// that explains why it went wrong.
func TestAttemptTranscriptPath(t *testing.T) {
	base := "/p/work/sessions/convert_chapter_042.jsonl"
	if got := attemptTranscriptPath(base, 1); got != base {
		t.Errorf("attempt 1 = %q, want %q", got, base)
	}
	if got, want := attemptTranscriptPath(base, 2), "/p/work/sessions/convert_chapter_042.try2.jsonl"; got != want {
		t.Errorf("attempt 2 = %q, want %q", got, want)
	}
	if got, want := attemptTranscriptPath("/p/x/checker_chapter_042.jsonl", 3), "/p/x/checker_chapter_042.try3.jsonl"; got != want {
		t.Errorf("attempt 3 = %q, want %q", got, want)
	}
	// T171：拿已经带 .try2 的名字再派生一次，不能套成 .try2.try3（一次运行里
	// 连续重试就是这个路径，曾经在磁盘上留下 convert_<章>.try2.try3.jsonl）。
	if got, want := attemptTranscriptPath("/p/s/convert_chapter_042.try2.jsonl", 3), "/p/s/convert_chapter_042.try3.jsonl"; got != want {
		t.Errorf("nested attempt = %q, want %q", got, want)
	}
	if got, want := attemptTranscriptPath("/p/s/convert_chapter_042.try2.try3.jsonl", 4), "/p/s/convert_chapter_042.try4.jsonl"; got != want {
		t.Errorf("legacy nested attempt = %q, want %q", got, want)
	}
	if got, want := attemptTranscriptPath("/p/s/convert_chapter_042", 2), "/p/s/convert_chapter_042.try2"; got != want {
		t.Errorf("no extension = %q, want %q", got, want)
	}
}

// TestAttemptNumberOnDisk pins the cross-run attempt numbering: a transcript
// that ended (run_end) gets a NEW attempt number on rerun, while an interrupted
// one is resumed in place — otherwise two runs append into the same file.
func TestAttemptNumberOnDisk(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "convert_chapter_007.jsonl")

	path, num, ended := attemptNumberOnDisk(base)
	if num != 0 || ended || path != "" {
		t.Fatalf("empty dir = %q %d %v", path, num, ended)
	}

	// 第 1 次结束（有 run_end）、第 2 次被中断（没有）→ 续跑第 2 次。
	os.WriteFile(filepath.Join(dir, "convert_chapter_007.jsonl"),
		[]byte(`{"t":"meta","kind":"run_end","ok":true}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "convert_chapter_007.try2.jsonl"),
		[]byte(`{"t":"msg","role":"assistant"}`+"\n"), 0o644)
	path, num, ended = attemptNumberOnDisk(base)
	if num != 2 || ended || path != filepath.Join(dir, "convert_chapter_007.try2.jsonl") {
		t.Fatalf("interrupted = %q %d %v", path, num, ended)
	}

	// 第 2 次也结束 → 下一次是 3，且不能和旧文件同名。
	os.WriteFile(filepath.Join(dir, "convert_chapter_007.try2.jsonl"),
		[]byte(`{"t":"meta","kind":"run_end"}`+"\n"), 0o644)
	path, num, ended = attemptNumberOnDisk(base)
	if num != 2 || !ended || path != filepath.Join(dir, "convert_chapter_007.try2.jsonl") {
		t.Fatalf("ended = %q %d %v", path, num, ended)
	}

	// 旧版本的套娃名也算数，取最大号并指向真实文件。
	os.WriteFile(filepath.Join(dir, "convert_chapter_007.try2.try3.jsonl"),
		[]byte(`{"t":"msg","role":"assistant"}`+"\n"), 0o644)
	path, num, ended = attemptNumberOnDisk(base)
	if num != 3 || ended || path != filepath.Join(dir, "convert_chapter_007.try2.try3.jsonl") {
		t.Fatalf("legacy nested = %q %d %v", path, num, ended)
	}

	// 别的章的文件不能算进来。
	os.WriteFile(filepath.Join(dir, "convert_chapter_0071.jsonl"), []byte("{}"), 0o644)
	if _, num, _ := attemptNumberOnDisk(base); num != 3 {
		t.Fatalf("sibling chapter leaked: %d", num)
	}
}

// TestCheckerAttemptBudget pins "top-level 6, one less per level, never below
// 3" — including the user-configured base.
func TestCheckerAttemptBudget(t *testing.T) {
	cases := []struct{ base, depth, want int }{
		{0, 0, 6}, {0, 1, 5}, {0, 2, 4}, {0, 3, 3}, {0, 9, 3},
		{6, 0, 6}, {6, 3, 3},
		{8, 0, 8}, {8, 3, 5},
		{4, 0, 4}, {4, 1, 3}, {4, 3, 3},
		{1, 0, 3}, // 配得再小也不低于 3
	}
	for _, c := range cases {
		if got := checkerAttemptBudget(c.base, c.depth); got != c.want {
			t.Errorf("budget(base=%d, depth=%d) = %d, want %d", c.base, c.depth, got, c.want)
		}
	}
}

// TestCheckerBudgetWarning pins the "config too small for the recursion depth"
// warning: 6 >= 3+3 时不说，4 就必须警告。
func TestCheckerBudgetWarning(t *testing.T) {
	if w := checkerBudgetWarning(6); w != "" {
		t.Errorf("6 should not warn: %s", w)
	}
	if w := checkerBudgetWarning(0); w != "" {
		t.Errorf("0 (=default) should not warn: %s", w)
	}
	if w := checkerBudgetWarning(4); w == "" {
		t.Error("4 cannot feed depth 3 with 3 attempts — must warn")
	}
}

// TestChapterGateRequiresCompileAndView pins the submission gate: compile ->
// view -> submit, and any edit after the compile invalidates it.
func TestChapterGateRequiresCompileAndView(t *testing.T) {
	work := t.TempDir()
	base := "chapter_009"
	if err := os.WriteFile(filepath.Join(work, base+".tex"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, base), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, base, "part1.tex"), []byte("p1"), 0o644); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join(work, base+"_wrapper.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := &chapterGate{WorkRoot: work, Base: base}

	if msg := g.check(); msg == "" {
		t.Fatal("submit must be rejected before any compile")
	}
	if err := g.markCompiled(pdf); err != nil {
		t.Fatal(err)
	}
	if msg := g.check(); msg == "" {
		t.Fatal("a compile without looking at the PDF must not pass")
	}
	g.markViewed(pdf, 1)
	// T168：看过自己的产物还不够，还必须和原书对过（至少 2 页）。
	if msg := g.check(); msg == "" || !strings.Contains(msg, "原书") {
		t.Fatalf("compile + own view without comparing the original must be rejected, got: %q", msg)
	}
	src := filepath.Join(t.TempDir(), "book_part1.pdf")
	if err := os.WriteFile(src, []byte("%PDF-1.4 src"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.markSourceViewed(src, 12)
	if msg := g.check(); msg == "" || !strings.Contains(msg, "原书") {
		t.Fatalf("one original page is not enough (need 2), got: %q", msg)
	}
	g.markSourceViewed(src, 13)
	if msg := g.check(); msg != "" {
		t.Fatalf("compile + own view + two original pages must pass, got: %s", msg)
	}
	// 编译之后又改了 parts/：旧印记作废，必须重编。
	if err := os.WriteFile(filepath.Join(work, base, "part1.tex"), []byte("p1 changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := g.check(); msg == "" {
		t.Fatal("editing the chapter after a compile must invalidate the stamp")
	}
}
