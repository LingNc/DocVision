package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T39: persist → current; any chapter change breaks the fingerprint.
func TestFinalReviewState_RoundTrip(t *testing.T) {
	proj := t.TempDir()
	build := t.TempDir()
	mk := func(dir, name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// workspace: two chapters + generated main.tex
	mk(proj, "work/chapters/chapter_001.tex", "A")
	mk(proj, "work/chapters/chapter_002.tex", "B")
	mk(proj, "work/chapters/parts/x.tex", "P")
	mk(proj, "build/main.tex", "GEN")
	// build tree after final review: chapter_001 revised, frontmatter added,
	// main.tex hooked
	mk(build, "chapters/chapter_001.tex", "A-fixed")
	mk(build, "chapters/chapter_002.tex", "B")
	mk(build, "chapters/parts/x.tex", "P")
	mk(build, "main.tex", "GEN+frontmatter-hook")
	mk(build, "frontmatter.tex", "FM")

	if finalReviewStateCurrent(proj) {
		t.Fatal("state must not be current before persist")
	}
	if err := persistFinalReviewState(proj, build); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(proj, "work/chapters/chapter_001.tex")); string(got) != "A-fixed" {
		t.Fatalf("chapter not written back: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(proj, "work/final_review/frontmatter.tex")); string(got) != "FM" {
		t.Fatal("frontmatter not persisted")
	}
	if !finalReviewStateCurrent(proj) {
		t.Fatal("state must be current right after persist")
	}
	// apply onto a fresh build dir overwrites the generated main.tex
	fresh := t.TempDir()
	mk(fresh, "main.tex", "GEN")
	if err := applyFinalReviewState(proj, fresh); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, "main.tex")); string(got) != "GEN+frontmatter-hook" {
		t.Fatalf("main.tex not overlaid: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, "frontmatter.tex")); string(got) != "FM" {
		t.Fatal("frontmatter not overlaid")
	}
	// a chapter change invalidates the state
	mk(proj, "work/chapters/chapter_002.tex", "B-changed")
	if finalReviewStateCurrent(proj) {
		t.Fatal("state must be invalid after a chapter change")
	}
}

// T39: archivePrevTranscript keeps the old transcript under *_prev.
func TestArchivePrevTranscript(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "final_review.jsonl")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	archivePrevTranscript(p)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("original must be gone")
	}
	prev := filepath.Join(dir, "final_review_prev.jsonl")
	if got, _ := os.ReadFile(prev); string(got) != "x" {
		t.Fatal("prev must keep content")
	}
	// second archive overwrites prev, no error on missing original
	os.WriteFile(p, []byte("y"), 0o644)
	archivePrevTranscript(p)
	if got, _ := os.ReadFile(prev); string(got) != "y" {
		t.Fatal("prev must be replaced")
	}
	archivePrevTranscript(filepath.Join(dir, "nope.jsonl")) // must not panic
}

func TestArchivePrevTranscript_NoTexSuffixIgnored(t *testing.T) {
	if strings.HasSuffix(stateHashFile, ".tex") {
		t.Fatal("state hash file must not be treated as a .tex input")
	}
}
