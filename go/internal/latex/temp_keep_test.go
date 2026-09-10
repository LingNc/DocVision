package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

func keepTestRunner(t *testing.T, probes ...*bool) *Runner {
	t.Helper()
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	log.SetQuiet(true)
	cfg := &config.Config{}
	if len(probes) >= 1 && probes[0] != nil {
		cfg.Latex.KeepTempDirs = probes[0]
	}
	if len(probes) >= 2 && probes[1] != nil {
		cfg.Latex.KeepSessionRecords = probes[1]
	}
	return &Runner{cfg: cfg, log: log}
}

// TestTempDirLivesInProject: temporary workspaces are created INSIDE the
// project (<proj>/work/temp/...) instead of /tmp, so a run can be
// inspected; deletion is the default.
func TestTempDirLivesInProject(t *testing.T) {
	proj := t.TempDir()
	r := keepTestRunner(t)
	dir, cleanup, err := r.tempDir(proj, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(proj, "work", "temp", "chapters"); dir != want {
		t.Fatalf("temp dir = %q, want %q", dir, want)
	}
	if err := os.WriteFile(filepath.Join(dir, "book.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir must be removed by default, stat err = %v", err)
	}
	// fresh dir again, now kept
	keep := true
	r2 := keepTestRunner(t, &keep, nil)
	dir2, cleanup2, err := r2.tempDir(proj, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir2, "book.md"), []byte("x"), 0o644)
	cleanup2()
	if _, err := os.Stat(filepath.Join(dir2, "book.md")); err != nil {
		t.Errorf("keep_temp_dirs must keep the temp dir: %v", err)
	}
}

// TestDebugAlwaysKeeps: debug logging overrides both switches.
func TestDebugAlwaysKeeps(t *testing.T) {
	off := false
	r := keepTestRunner(t, &off, &off)
	r.log.SetLevel(logger.LevelDebug)
	if !r.keepTemp() || !r.keepRecords() {
		t.Fatalf("debug must keep temps and records (keepTemp=%v keepRecords=%v)", r.keepTemp(), r.keepRecords())
	}
	proj := t.TempDir()
	tr := filepath.Join(proj, "work", "sessions", "convert_chapter_01.jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.keepSessionFile(tr)
	if _, err := os.Stat(tr); err != nil {
		t.Errorf("debug must keep the transcript: %v", err)
	}
	// and without debug the default removes it
	r2 := keepTestRunner(t)
	r2.keepSessionFile(tr)
	if _, err := os.Stat(tr); !os.IsNotExist(err) {
		t.Errorf("transcript must be removed by default, stat err = %v", err)
	}
}

// TestAlignmentProblemsDetectsDrift: when the OCR index and the real PDF
// disagree about a part's page count, the mismatch is reported (global
// page numbers may drift) — part-based lookups stay exact.
func TestAlignmentProblemsDetectsDrift(t *testing.T) {
	view := &pdfView{Files: []pdfViewFile{
		{Name: "book_part1.pdf", Part: "book_part1", First: 1, Count: 200},
		{Name: "book_part2.pdf", Part: "book_part2", First: 201, Count: 200},
	}}
	pages := &pageIndex{total: 396, srcs: []pageSrc{
		{pdf: "/m/book_part1/x_origin.pdf", first: 1, count: 200},
		{pdf: "/m/book_part2/y_origin.pdf", first: 201, count: 196},
	}}
	msgs := view.alignmentProblems(pages)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "book_part2") {
		t.Fatalf("expected one drift report for book_part2, got %v", msgs)
	}
	if _, _, err := view.LocatePart("book_part2", 196); err != nil {
		t.Errorf("part-based lookup must stay exact: %v", err)
	}
	if _, _, err := view.LocatePart("", 1); err == nil {
		t.Errorf("empty part must never match")
	}
}

// TestChapterViewIsPrivate: the conversion session's work view exposes
// ONLY its own chapter, and writing through the (possibly dangling) link
// creates the real file in the canonical chapter tree.
func TestChapterViewIsPrivate(t *testing.T) {
	proj := t.TempDir()
	// a sibling chapter that already exists
	sib := filepath.Join(proj, "work", "chapters", "chapter_02.tex")
	if err := os.MkdirAll(filepath.Dir(sib), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sib, []byte("sibling"), 0o644); err != nil {
		t.Fatal(err)
	}
	view := ensureChapterView(proj, "chapter_01")
	if view == "" {
		t.Fatal("no view")
	}
	// own main file does not exist yet => the tool must be able to create it
	if _, err := os.Stat(filepath.Join(view, "chapters", "chapter_01.tex")); err == nil {
		t.Errorf("own file should not exist yet (dangling link expected)")
	}
	if _, err := os.Stat(filepath.Join(view, "chapters", "chapter_02.tex")); err == nil {
		t.Errorf("sibling chapter must NOT be visible in the private view")
	}
	write := &WriteWorkFileTool{Root: view, Prefixes: []string{"chapters/chapter_01.tex", "chapters/chapter_01/"}}
	res, err := write.Execute(`{"path":"chapters/chapter_01.tex","content":"\\section{one}"}`)
	if err != nil || strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("writing own chapter through the view failed: %v %s", err, res.Text)
	}
	data, err := os.ReadFile(filepath.Join(proj, "work", "chapters", "chapter_01.tex"))
	if err != nil || string(data) != "\\section{one}" {
		t.Fatalf("real chapter file not written through the view: %v %q", err, data)
	}
	// asset folder round trip
	sub, err := write.Execute(`{"path":"chapters/chapter_01/table1.tex","content":"rows"}`)
	if err != nil || strings.Contains(sub.Text, "REJECTED") {
		t.Fatalf("writing own asset failed: %v %s", err, sub.Text)
	}
	if _, err := os.Stat(filepath.Join(proj, "work", "chapters", "chapter_01", "table1.tex")); err != nil {
		t.Errorf("asset not written into the canonical tree: %v", err)
	}
}
