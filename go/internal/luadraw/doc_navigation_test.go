package luadraw

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func navigationDocs(t *testing.T) *Docs {
	t.Helper()
	root := t.TempDir()
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "doc", "src", "body-en"), 0700); err != nil {
		t.Fatal(err)
	}
	return &Docs{root: root, cache: cache, path: filepath.Join(cache, "luadraw-doc-index-v4.json")}
}

func TestSourceSearchUsesCanonicalParagraphBeforePaging(t *testing.T) {
	d := navigationDocs(t)
	long := strings.Repeat("continuation words ", 100)
	text := "-- alpha comment\nfunction alpha()\n  local a = \"needle\"\n  local b = \"needle\"\n  " + long + "\nend\n\nfunction beta()\n  return \"needle\"\nend\n"
	idx := &docIndex{Key: "source-fixture", Docs: []docEntry{{Path: "doc/src/body-en/sample.tex", Kind: "source", Text: text}}}
	hits := d.search(idx, "needle", 1, 0)
	if len(hits) != 1 || hits[0].line != 1 || hits[0].end != 6 {
		t.Fatalf("search should return the full canonical source paragraph: %#v", hits)
	}
	short, _ := snippetForHit(idx, hits[0], 64)
	longSnippet, _ := snippetForHit(idx, hits[0], 1200)
	if len([]rune(short)) > 65 || len([]rune(short)) < 64 || len([]rune(longSnippet)) <= len([]rune(short)) {
		t.Fatalf("snippet_chars was not applied to the untruncated paragraph: short=%d long=%d", len([]rune(short)), len([]rune(longSnippet)))
	}
	second := d.search(idx, "needle", 1, 1)
	if len(second) != 1 || second[0].line != 8 || second[0].line == hits[0].line {
		t.Fatalf("paragraph pagination repeated or lost a canonical hit: %#v", second)
	}
}

func TestReadWindowsRecordTheSameCanonicalSourceEntry(t *testing.T) {
	d := navigationDocs(t)
	text := "-- stable comment\nfunction stable()\n  local value = \"needle\"\n  return value\nend\n\nfunction next()\n  return 2\nend\n"
	path := filepath.Join(d.root, "doc", "src", "body-en", "stable.tex")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	readIdx := &docIndex{Key: "source-fixture"}
	for _, line := range []int{3, 4} {
		if _, err := readSourceText(d, readIdx, map[string]any{"doc": "doc/src/body-en/stable.tex", "start_line": line, "end_line": line}); err != nil {
			t.Fatal(err)
		}
	}
	idx := &docIndex{Key: "source-fixture", Docs: []docEntry{{Path: "doc/src/body-en/stable.tex", Kind: "source", Text: text}}}
	for _, hit := range d.search(idx, "needle", 10, 0) {
		for _, segment := range hit.segments {
			d.recordFrequency(idx.Key, segment.doc, segment.start, segment.end, segment.text, "search", 1)
		}
	}
	entries, err := d.frequencySnapshot("source-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != frequencyID("doc/src/body-en/stable.tex", 1, 5) || entries[0].Start != 1 || entries[0].End != 5 || entries[0].Hits != 3 {
		t.Fatalf("different read windows should coalesce on one full paragraph identity: %#v", entries)
	}
	if len([]rune(entries[0].Text)) > 2048 || !strings.Contains(entries[0].Text, "function stable()") {
		t.Fatalf("frequency entry must save the complete bounded paragraph: %#v", entries[0])
	}
}

func TestPDFReadAndSearchSharePageQualifiedBlockIDs(t *testing.T) {
	d := navigationDocs(t)
	pdfPath := filepath.Join(d.root, "doc", "manual.pdf")
	if err := os.WriteFile(pdfPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	b1 := bbox{10, 10, 110, 20}
	b2 := bbox{12, 22, 112, 32}
	b3 := bbox{300, 10, 390, 20}
	pdf := docEntry{Path: "doc/manual.pdf", Kind: "pdf", Pages: []page{{Number: 1, Width: 400, Height: 500, Blocks: []block{{Text: "needle first line", BBox: b1}, {Text: "needle second line", BBox: b2}, {Text: "needle other column", BBox: b3}}}}}
	idx := &docIndex{Key: "pdf-fixture", Docs: []docEntry{pdf}}
	text, err := readPDFText(d, idx, map[string]any{"doc": "doc/manual.pdf#page=1", "start_line": 1, "end_line": 999})
	if err != nil || !strings.Contains(text, "blocks 1-3/3") {
		t.Fatalf("PDF page selector/read cap failed: %v %s", err, text)
	}
	hits := d.search(idx, "needle", 10, 0)
	if len(hits) != 2 || len(hits[0].segments) < 2 {
		t.Fatalf("PDF adjacent blocks should visually merge but retain paragraph IDs: %#v", hits)
	}
	for _, h := range hits {
		for _, segment := range h.segments {
			d.recordFrequency(idx.Key, segment.doc+"#page="+itoa(segment.page), segment.start, segment.end, segment.text, "search", 1)
		}
	}
	entries, err := d.frequencySnapshot("pdf-fixture")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{frequencyID("doc/manual.pdf#page=1", 1, 1): false, frequencyID("doc/manual.pdf#page=1", 2, 2): false, frequencyID("doc/manual.pdf#page=1", 3, 3): false}
	for _, entry := range entries {
		if _, ok := want[entry.ID]; ok {
			want[entry.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("read/search omitted stable page/block frequency ID %s: %#v", id, entries)
		}
	}
}

func TestNavigationPathScopeListingAndBudgets(t *testing.T) {
	d := navigationDocs(t)
	base := filepath.Join(d.root, "doc", "src", "body-en")
	if err := os.MkdirAll(filepath.Join(base, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"alpha.tex": "literal [needle]\\nneedle 123\\n", "nested/beta.tex": "needle 456\\n", "section-1-Introduction.tex": "intro\\n", "section-3-Graphics-Methods.tex": "graphics\\n"} {
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(name)), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plain, err := d.listFiles("doc/src/body-en", false)
	if err != nil || len(plain) != 4 || !strings.Contains(strings.Join(plain, "\n"), "nested/") || strings.Contains(strings.Join(plain, "\n"), "nested/beta.tex") {
		t.Fatalf("nonrecursive list must show only immediate children: %v %v", plain, err)
	}
	recursive, err := d.listFiles("doc/src/body-en", true)
	if err != nil || len(recursive) != 5 || recursive[0] > recursive[len(recursive)-1] {
		t.Fatalf("recursive listing should be stable and bounded: %v %v", recursive, err)
	}
	found, err := d.findFiles("doc/src/body-en", "section-?.tex")
	if err != nil || len(found) != 0 {
		t.Fatalf("glob should match actual filenames, not invented names: %v %v", found, err)
	}
	found, err = d.findFiles("doc/src/body-en", "section-*-*.tex")
	if err != nil || len(found) != 2 || found[0] > found[1] {
		t.Fatalf("find should be scoped, globbed, and sorted: %v %v", found, err)
	}
	literal, err := d.grepFiles("doc/src/body-en", "[needle]", false, 20, 0, 256)
	if err != nil || !strings.Contains(literal, "alpha.tex") || strings.Contains(literal, "nested/beta.tex") || len([]rune(literal)) > 256 {
		t.Fatalf("literal grep should be scoped and budgeted: %v %q", err, literal)
	}
	regex, err := d.grepFiles("doc/src/body-en", `needle [0-9]+`, true, 20, 0, 256)
	if err != nil || !strings.Contains(regex, "nested/beta.tex") {
		t.Fatalf("regex grep failed: %v %q", err, regex)
	}
	if _, _, err := d.rootPath("/etc/passwd"); err == nil {
		t.Fatal("absolute root path accepted")
	}
	for _, unsafe := range []string{"../etc/passwd", `doc\\..\\outside`} {
		if _, _, err := d.rootPath(unsafe); err == nil {
			t.Fatalf("parent segment accepted: %q", unsafe)
		}
	}
	badPath := filepath.Join(base, "section-1-The-graph3d-Class.tex")
	_, _, _, err = d.sourcePath(filepath.ToSlash(strings.TrimPrefix(badPath, d.root+string(filepath.Separator))))
	if err == nil || !strings.Contains(err.Error(), "section-1-Introduction.tex") || !strings.Contains(err.Error(), "section-3-Graphics-Methods.tex") {
		t.Fatalf("missing path should suggest nearest real files: %v", err)
	}
}

func TestReadEndLineCapsAt400AndRecordsOnlyIntersectingParagraphs(t *testing.T) {
	d := navigationDocs(t)
	var lines []string
	for i := 1; i <= 500; i++ {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}
	path := filepath.Join(d.root, "doc", "src", "body-en", "long.tex")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	idx := &docIndex{Key: "cap-fixture"}
	read, err := readSourceText(d, idx, map[string]any{"doc": "doc/src/body-en/long.tex", "start_line": 1, "end_line": 999})
	if err != nil || !strings.Contains(read, "lines 1-400/500") || !strings.Contains(read, "400: line-400") || strings.Contains(read, "401: line-401") {
		t.Fatalf("read must report and enforce the actual 400-line cap: %v %s", err, read)
	}
	entries, err := d.frequencySnapshot("cap-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("only five intersecting 80-line canonical paragraphs should be recorded, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.End > 400 {
			t.Fatalf("paragraph beyond actual read range was recorded: %#v", entry)
		}
	}
}

func TestNavigationModesRecordOnlyEmittedExcerpts(t *testing.T) {
	d := navigationDocs(t)
	path := filepath.Join(d.root, "doc", "src", "body-en", "nav.tex")
	if err := os.WriteFile(path, []byte("needle paragraph\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"mode": "list", "doc": "doc/src/body-en"},
		{"mode": "find", "doc": "doc/src/body-en", "query": "nav.tex"},
		{"mode": "grep", "doc": "doc/src/body-en", "query": "needle"},
		{"mode": "find", "doc": "doc/src/body-en", "query": "missing-file"},
	} {
		if _, _, err := d.Execute(context.Background(), args, t.TempDir()); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := d.frequencySnapshot(d.index.Key)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if len([]rune(entry.Text)) > 2048 {
			t.Fatalf("navigation frequency entry exceeded saved-text budget: %d", len([]rune(entry.Text)))
		}
		seen[entry.Source] = true
		if strings.Contains(entry.Doc, "missing-file") {
			t.Fatal("empty find result must not be recorded")
		}
	}
	for _, source := range []string{"list", "find", "grep"} {
		if !seen[source] {
			t.Fatalf("missing frequency entry for emitted %s result: %#v", source, entries)
		}
	}
}

func TestPDFParagraphMergingKeepsColumnsSeparate(t *testing.T) {
	lines := []block{{Text: "first", BBox: bbox{10, 10, 110, 20}}, {Text: "continuation", BBox: bbox{12, 22, 112, 32}}, {Text: "other column", BBox: bbox{300, 10, 390, 20}}}
	got := pdfParagraphs(lines, 400)
	if len(got) != 2 || !strings.Contains(got[0].Text, "first\ncontinuation") {
		t.Fatalf("paragraph merge should use geometry without crossing columns: %#v", got)
	}
}

func TestPDFParagraphsDoNotCollapseLongPageAsGapAccumulates(t *testing.T) {
	var lines []block
	y := 10.0
	for i := 0; i < 36; i++ {
		if i > 0 {
			y += 1.2 + float64(i%5)*.15
		}
		lines = append(lines, block{Text: fmt.Sprintf("line %d content", i+1), BBox: bbox{10, y, 180, y + 8}})
	}
	got := pdfParagraphs(lines, 400)
	if len(got) < 3 {
		t.Fatalf("long page collapsed despite accumulating inter-paragraph gaps: blocks=%d", len(got))
	}
	for _, paragraph := range got {
		if len(strings.Fields(paragraph.Text)) > 40 || len([]rune(paragraph.Text)) > 2048 {
			t.Fatalf("paragraph exceeded conservative size bounds: lines=%d chars=%d", len(strings.Fields(paragraph.Text)), len([]rune(paragraph.Text)))
		}
	}
}

func TestOfficialManualPage14HasMultipleParagraphBlocks(t *testing.T) {
	root := projectRoot(t)
	d, err := NewDocs(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/luadraw-doc-en.pdf#page=14"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "blocks 1-") || strings.Contains(text, "blocks 1-1/") {
		t.Fatalf("official PDF page 14 should expose multiple stable paragraph blocks: %s", text)
	}
}

func TestNavigationSearchOutputHonorsSnippetAndTotalBudget(t *testing.T) {
	d := navigationDocs(t)
	text := "function alpha()\n  " + strings.Repeat("context ", 260) + "needle\nend\n"
	path := filepath.Join(d.root, "doc", "src", "body-en", "alpha.tex")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	short, _, err := d.Execute(context.Background(), map[string]any{"mode": "search", "query": "needle", "snippet_chars": 64, "max_chars": 256}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	long, _, err := d.Execute(context.Background(), map[string]any{"mode": "search", "query": "needle", "snippet_chars": 1200, "max_chars": 1600}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(short)) >= len([]rune(long)) || len([]rune(short)) > 700 || len([]rune(long)) > 2200 {
		t.Fatalf("snippet/total budgets not honored: short=%d long=%d", len([]rune(short)), len([]rune(long)))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
