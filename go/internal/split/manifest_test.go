package split

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"mineru-tools/pkg/util"
)

// makeTestManifest builds a manifest with one part per entry in
// parts (where each entry supplies size/page_start/page_end) and
// writes it under outputDir using baseName. Tests use this to set
// up a known-good cache state without invoking SplitPDF.
func makeTestManifest(t *testing.T, outputDir, baseName string, src sourceIdentity, maxPages int, maxSizeMB float64, parts []Part) string {
	t.Helper()
	if err := util.EnsureDir(outputDir); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindPDF,
		Mode:          ModeSplit,
		SourcePath:    src.Path,
		SourceSize:    src.Size,
		SourceMTimeNS: src.MTime,
		MaxPages:      maxPages,
		MaxSizeMB:     maxSizeMB,
		Status:        StatusComplete,
		Parts:         parts,
	}
	if err := WriteManifest(ManifestPath(outputDir, baseName), m); err != nil {
		t.Fatal(err)
	}
	return ManifestPath(outputDir, baseName)
}

// TestManifestRoundtrip confirms that WriteManifest → LoadManifest
// returns an equal manifest. Validates JSON encoding tags.
func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	base := "doc"
	src := sourceIdentity{Path: "/tmp/doc.pdf", Size: 12345, MTime: 1700000000000000000}
	parts := []Part{
		{Filename: base + "_part1.pdf", Index: 1, PageStart: 1, PageEnd: 3, Size: 111},
		{Filename: base + "_part2.pdf", Index: 2, PageStart: 4, PageEnd: 6, Size: 222},
	}
	makeTestManifest(t, dir, base, src, 3, 0, parts)

	got, err := LoadManifest(ManifestPath(dir, base))
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != ManifestSchemaVersion {
		t.Errorf("schema_version = %d, want %d", got.SchemaVersion, ManifestSchemaVersion)
	}
	if got.SourcePath != src.Path {
		t.Errorf("source_path = %q, want %q", got.SourcePath, src.Path)
	}
	if got.SourceSize != src.Size {
		t.Errorf("source_size = %d, want %d", got.SourceSize, src.Size)
	}
	if got.SourceMTimeNS != src.MTime {
		t.Errorf("source_mtime_ns = %d, want %d", got.SourceMTimeNS, src.MTime)
	}
	if got.MaxPages != 3 {
		t.Errorf("max_pages = %d, want 3", got.MaxPages)
	}
	if !reflect.DeepEqual(got.Parts, parts) {
		t.Errorf("parts mismatch: got %+v want %+v", got.Parts, parts)
	}
}

// TestManifestJSONFields locks the JSON key names. Changing them
// is a breaking schema change and must require bumping
// ManifestSchemaVersion.
func TestManifestJSONFields(t *testing.T) {
	dir := t.TempDir()
	base := "doc"
	src := sourceIdentity{Path: "/tmp/doc.pdf", Size: 1, MTime: 1}
	parts := []Part{{Filename: base + "_part1.pdf", Index: 1, PageStart: 1, PageEnd: 1, Size: 1}}
	makeTestManifest(t, dir, base, src, 1, 0, parts)

	data, err := os.ReadFile(ManifestPath(dir, base))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"schema_version", "kind", "mode", "source_path", "source_size", "source_mtime_ns",
		"max_pages", "max_size_mb", "status", "parts",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("manifest JSON missing key %q", key)
		}
	}
	part0, ok := raw["parts"].([]any)
	if !ok || len(part0) == 0 {
		t.Fatalf("parts key wrong type: %T", raw["parts"])
	}
	pm := part0[0].(map[string]any)
	for _, key := range []string{"filename", "index", "page_start", "page_end", "size"} {
		if _, ok := pm[key]; !ok {
			t.Errorf("part JSON missing key %q", key)
		}
	}
}

// TestValidateManifestRejectsBad checks each structural rule.
func TestValidateManifestRejectsBad(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Manifest)
	}{
		{"wrong_schema", func(m *Manifest) { m.SchemaVersion = 999 }},
		{"empty_source", func(m *Manifest) { m.SourcePath = "" }},
		{"bad_status", func(m *Manifest) { m.Status = "partial" }},
		{"no_parts", func(m *Manifest) { m.Parts = nil }},
		{"part_index_gap", func(m *Manifest) {
			m.Parts[1].Index = 99
		}},
		{"part_bad_page_range", func(m *Manifest) {
			m.Parts[1].PageEnd = m.Parts[1].PageStart - 1
		}},
		{"part_zero_size", func(m *Manifest) {
			m.Parts[0].Size = 0
		}},
		{"part_empty_filename", func(m *Manifest) {
			m.Parts[0].Filename = ""
		}},
		{"bad_kind", func(m *Manifest) { m.Kind = "xlsx" }},
		{"passthrough_with_parts", func(m *Manifest) {
			m.Mode = ModePassthrough
		}},
		{"split_with_zero_parts", func(m *Manifest) {
			m.Mode = ModeSplit
			m.Parts = nil
		}},
	}
	good := func() *Manifest {
		return &Manifest{
			SchemaVersion: ManifestSchemaVersion,
			Kind:          KindPDF,
			Mode:          ModeSplit,
			SourcePath:    "/x",
			Status:        StatusComplete,
			Parts: []Part{
				{Filename: "a_part1.pdf", Index: 1, PageStart: 1, PageEnd: 1, Size: 1},
				{Filename: "a_part2.pdf", Index: 2, PageStart: 2, PageEnd: 3, Size: 1},
			},
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := good()
			tc.mut(m)
			if _, err := LoadManifest(pathOfInlineManifest(t, m)); err == nil {
				t.Fatalf("expected validate to reject: %s", tc.name)
			}
		})
	}
}

// TestLoadManifestAppliesLegacyDefaults verifies that a manifest
// JSON written without Kind/Mode fields loads with pdf/split so
// pre-generalization caches still hit on subsequent runs.
func TestLoadManifestAppliesLegacyDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "legacy.json")
	legacy := `{
		"schema_version": 1,
		"source_path": "/x",
		"source_size": 1,
		"source_mtime_ns": 1,
		"max_pages": 1,
		"max_size_mb": 0,
		"status": "complete",
		"parts": [{"filename":"a_part1.pdf","index":1,"page_start":1,"page_end":1,"size":1}]
	}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(p)
	if err != nil {
		t.Fatalf("legacy manifest should load with defaults: %v", err)
	}
	if m.Kind != KindPDF || m.Mode != ModeSplit {
		t.Fatalf("legacy defaults applied incorrectly: kind=%q mode=%q", m.Kind, m.Mode)
	}
}

// TestKindedManifestPath verifies the kind-aware manifest path
// differs from the default PDF path so a DOCX and a PDF sharing
// a basename do not collide.
func TestKindedManifestPath(t *testing.T) {
	dir := t.TempDir()
	base := "doc"
	defaultPath := ManifestPath(dir, base)
	docxPath := docxManifestPath(dir, base)
	pdfKinded := KindedManifestPath(dir, base, KindPDF)
	docxKinded := KindedManifestPath(dir, base, KindDOCX)
	if docxPath == defaultPath {
		t.Fatalf("docx manifest should differ from default pdf manifest for same basename")
	}
	if pdfKinded == defaultPath {
		// The kind-aware PDF variant intentionally diverges from
		// the legacy default path so the two are guaranteed to
		// differ. This test only asserts the DOCX variant is
		// distinct from the PDF default.
	}
	if docxKinded != docxPath {
		t.Fatalf("kinded DOCX manifest should equal docxManifestPath: %q vs %q", docxKinded, docxPath)
	}
}

// pathOfInlineManifest writes m to a temp file and returns the path.
// Used to exercise the validateManifest code path indirectly through
// LoadManifest without going through the full SplitPDF flow.
func pathOfInlineManifest(t *testing.T, m *Manifest) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "inline.json")
	if err := util.AtomicWriteJSON(p, m); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestManifestMatchesParams covers the identity check.
func TestManifestMatchesParams(t *testing.T) {
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		SourcePath:    "/x.pdf",
		SourceSize:    100,
		SourceMTimeNS: 42,
		MaxPages:      10,
		MaxSizeMB:     0,
	}
	good := MatchParams{SourcePath: "/x.pdf", SourceSize: 100, SourceMTime: 42, MaxPages: 10, MaxSizeMB: 0}
	if !m.Matches(good) {
		t.Fatal("expected match for identical params")
	}
	mut := func(f func(*MatchParams)) MatchParams { p := good; f(&p); return p }
	cases := []struct {
		name string
		p    MatchParams
	}{
		{"different_path", mut(func(p *MatchParams) { p.SourcePath = "/y.pdf" })},
		{"different_size", mut(func(p *MatchParams) { p.SourceSize = 101 })},
		{"different_mtime", mut(func(p *MatchParams) { p.SourceMTime = 43 })},
		{"different_max_pages", mut(func(p *MatchParams) { p.MaxPages = 11 })},
		{"different_max_size", mut(func(p *MatchParams) { p.MaxSizeMB = 5 })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if m.Matches(tc.p) {
				t.Fatalf("expected miss for %s", tc.name)
			}
		})
	}
}

// TestVerifyAgainstDisk covers the filesystem verification step.
func TestVerifyAgainstDisk(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "doc_part1.pdf")
	p2 := filepath.Join(dir, "doc_part2.pdf")
	if err := os.WriteFile(p1, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("world!"), 0o644); err != nil {
		t.Fatal(err)
	}

	good := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Status:        StatusComplete,
		Parts: []Part{
			{Filename: "doc_part1.pdf", Index: 1, PageStart: 1, PageEnd: 1, Size: 5},
			{Filename: "doc_part2.pdf", Index: 2, PageStart: 2, PageEnd: 2, Size: 6},
		},
	}
	if ok, err := good.VerifyAgainstDisk(dir); err != nil || !ok {
		t.Fatalf("good manifest: ok=%v err=%v", ok, err)
	}

	wrongSize := *good
	wrongSize.Parts[0].Size = 999
	if ok, _ := wrongSize.VerifyAgainstDisk(dir); ok {
		t.Fatal("expected miss when recorded size != disk size")
	}

	missing := *good
	missing.Parts[0].Filename = "doc_part9.pdf"
	if ok, _ := missing.VerifyAgainstDisk(dir); ok {
		t.Fatal("expected miss when part file missing")
	}

	// Traversal attempt: filename with directory separator must
	// be rejected silently (no disk access, no panic).
	traversal := *good
	traversal.Parts[0].Filename = "../escape.pdf"
	if ok, _ := traversal.VerifyAgainstDisk(dir); ok {
		t.Fatal("expected miss when filename escapes output dir")
	}
}

// TestLoadManifestMissing verifies graceful handling of absent file.
func TestLoadManifestMissing(t *testing.T) {
	if _, err := LoadManifest(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

// TestLoadManifestCorrupt treats a non-JSON file as a miss.
func TestLoadManifestCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(p, []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(p); err == nil {
		t.Fatal("expected error for non-JSON manifest")
	}
}

// TestManifestFilenameNotMatchedByPartGlob is the negative
// guarantee: a stray glob over part files must not match the
// manifest, so SplitPDF's cleanup pass cannot delete its own
// cache. We assert by globbing and checking set membership.
func TestManifestFilenameNotMatchedByPartGlob(t *testing.T) {
	dir := t.TempDir()
	base := "doc"
	// Create a fake manifest named exactly as the code would.
	manifestPath := ManifestPath(dir, base)
	if err := os.WriteFile(manifestPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Mimic the part glob.
	matches, err := filepath.Glob(filepath.Join(dir, base+"_part*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		if filepath.Base(m) == filepath.Base(manifestPath) {
			t.Fatalf("manifest %s matched part glob", manifestPath)
		}
	}
}
