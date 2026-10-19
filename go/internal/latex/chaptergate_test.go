package latex

import (
	"os"
	"path/filepath"
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
	if msg := g.check(); msg != "" {
		t.Fatalf("compile + one page viewed must pass, got: %s", msg)
	}
	// 编译之后又改了 parts/：旧印记作废，必须重编。
	if err := os.WriteFile(filepath.Join(work, base, "part1.tex"), []byte("p1 changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := g.check(); msg == "" {
		t.Fatal("editing the chapter after a compile must invalidate the stamp")
	}
}
