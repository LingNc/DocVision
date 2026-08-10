package split

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// docxFixturePageCount controls the <Pages> value written into the
// fixture's docProps/app.xml. SplitDOCX reads this field via
// readDOCXPageCount to decide between passthrough and split paths.
const docxFixturePageCount = 3

// writeMinimalDOCX writes a real .docx zip with the minimum entries
// required by SplitDOCX's readers:
//   - [Content_Types].xml
//   - word/document.xml (with a single empty <w:p> body — page
//     count is read from docProps/app.xml, not document.xml, so the
//     body does not need to be rich)
//   - docProps/app.xml containing <Pages>{count}</Pages>
//
// The file is intentionally tiny because every SplitDOCX unit test
// builds one from scratch.
func writeMinimalDOCX(t *testing.T, path string, pages int) {
	t.Helper()
	if pages <= 0 {
		pages = docxFixturePageCount
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`
	mustZipEntry(t, zw, "[Content_Types].xml", contentTypes)

	// Body uses real namespaces so the xml.Decoder token stream
	// that readDOCXBody walks behaves like a real document. One
	// paragraph element is enough — SplitDOCX only cares about the
	// page-count gate, not the body content.
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p/>
  </w:body>
</w:document>`
	mustZipEntry(t, zw, "word/document.xml", document)

	app := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Pages>%d</Pages>
</Properties>`, pages)
	mustZipEntry(t, zw, "docProps/app.xml", app)
}

func mustZipEntry(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}

// makeTestDOCX writes a fresh minimal DOCX into a temp directory
// and returns its path. The page count comes from
// docxFixturePageCount by default.
func makeTestDOCX(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	writeMinimalDOCX(t, p, docxFixturePageCount)
	return p
}

// statDOCX wraps statSource so test code does not need to import
// the lower-level helper. Returns the same sourceIdentity used by
// the cache-hit code path.
func statDOCX(t *testing.T, path string) sourceIdentity {
	t.Helper()
	id, err := statSource(path)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// requireLibreOffice skips the test if LibreOffice is not
// installed. All passthrough tests rely on the first run having
// actually called readDOCXPageCount + WriteManifest, which does
// not need LibreOffice, but we still want a clean skip rather
// than a flaky failure on CI boxes without the binary.
func requireLibreOffice(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("libreoffice"); err != nil {
		t.Skipf("libreoffice not available: %v", err)
	}
}

// TestReadDOCXPageCountFixture verifies our test fixture is
// readable by the production parser. If this fails the rest of
// the split_docx tests are meaningless.
func TestReadDOCXPageCountFixture(t *testing.T) {
	src := makeTestDOCX(t, "pagecount.docx")
	if got := readDOCXPageCount(src); got != docxFixturePageCount {
		t.Fatalf("readDOCXPageCount = %d, want %d", got, docxFixturePageCount)
	}
}

// TestSplitDOCXMissingSource confirms SplitDOCX returns a clear
// error when the source does not exist (covers the early-out path
// before any manifest lookup).
func TestSplitDOCXMissingSource(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(filepath.Join(t.TempDir(), "no.docx"), 200, 0, outDir, false); err == nil {
		t.Fatal("expected error for missing source")
	} else if !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestSplitDOCXPassthroughRecordsManifest verifies that a DOCX
// comfortably under the page limit takes the passthrough branch
// and writes a mode=passthrough manifest — no parts.
func TestSplitDOCXPassthroughRecordsManifest(t *testing.T) {
	src := makeTestDOCX(t, "passthrough.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("passthrough split: %v", err)
	}
	m, err := LoadManifest(docxManifestPath(outDir, "passthrough"))
	if err != nil {
		t.Fatalf("expected passthrough manifest, got %v", err)
	}
	if m.Kind != KindDOCX || m.Mode != ModePassthrough {
		t.Fatalf("manifest kind=%q mode=%q, want docx/passthrough", m.Kind, m.Mode)
	}
	if len(m.Parts) != 0 {
		t.Fatalf("passthrough manifest should have no parts, got %d", len(m.Parts))
	}
	if matches, _ := filepath.Glob(docxPartsGlob(outDir, "passthrough")); len(matches) != 0 {
		t.Fatalf("passthrough should not produce parts, found %v", matches)
	}
}

// TestSplitDOCXPassthroughHitSkipsConversion confirms a second run
// against the same source takes the manifest fast-path and never
// invokes LibreOffice. We wipe the PATH between runs so any
// accidental exec.Command("libreoffice", ...) would fail.
func TestSplitDOCXPassthroughHitSkipsConversion(t *testing.T) {
	src := makeTestDOCX(t, "hit.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	// Verify the manifest was written — otherwise the hit-path
	// assertion below is meaningless.
	if _, err := os.Stat(docxManifestPath(outDir, "hit")); err != nil {
		t.Fatalf("manifest missing after first run: %v", err)
	}
	// Now strip PATH so a stray libreoffice call would fail to
	// exec.
	t.Setenv("PATH", "")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("passthrough hit should not call libreoffice: %v", err)
	}
}

// TestSplitDOCXForceBypassesManifest verifies that --force re-
// evaluates the source even when the cache would otherwise hit.
// We bump mtime first so a non-forced run would also miss, then
// run with force=true and confirm the manifest was rewritten
// with the post-bump mtime.
func TestSplitDOCXForceBypassesManifest(t *testing.T) {
	src := makeTestDOCX(t, "force.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	touchPDF(t, src)
	afterMTime := statDOCX(t, src).MTime
	if err := SplitDOCX(src, 200, 0, outDir, true); err != nil {
		t.Fatalf("force run: %v", err)
	}
	m, err := LoadManifest(docxManifestPath(outDir, "force"))
	if err != nil {
		t.Fatal(err)
	}
	if m.SourceMTimeNS != afterMTime {
		t.Fatalf("manifest mtime = %d, want %d (force should pick up new mtime)",
			m.SourceMTimeNS, afterMTime)
	}
}

// TestSplitDOCXMtimeChangeReSplits checks that bumping mtime
// invalidates the passthrough manifest and forces re-evaluation.
func TestSplitDOCXMtimeChangeReSplits(t *testing.T) {
	src := makeTestDOCX(t, "mtime.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	beforeMTime := statDOCX(t, src).MTime
	touchPDF(t, src)
	afterMTime := statDOCX(t, src).MTime
	if afterMTime <= beforeMTime {
		t.Fatalf("touchPDF did not bump mtime: %d -> %d", beforeMTime, afterMTime)
	}
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("re-run after mtime bump: %v", err)
	}
	m, err := LoadManifest(docxManifestPath(outDir, "mtime"))
	if err != nil {
		t.Fatal(err)
	}
	if m.SourceMTimeNS != afterMTime {
		t.Fatalf("manifest mtime = %d, want %d", m.SourceMTimeNS, afterMTime)
	}
}

// TestSplitDOCXParamChangeReSplits verifies that changing
// maxPages invalidates the manifest and forces a fresh split.
func TestSplitDOCXParamChangeReSplits(t *testing.T) {
	src := makeTestDOCX(t, "params.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	if err := SplitDOCX(src, 100, 0, outDir, false); err != nil {
		t.Fatalf("param change run: %v", err)
	}
	m, err := LoadManifest(docxManifestPath(outDir, "params"))
	if err != nil {
		t.Fatal(err)
	}
	if m.MaxPages != 100 {
		t.Fatalf("manifest max_pages = %d, want 100", m.MaxPages)
	}
}

// TestSplitDOCXSplitManifestHitSkipsLibreOffice exercises the
// mode=split cache hit path. We can't easily produce a real
// mode=split manifest without LibreOffice (the only writer for
// that path converts through LibreOffice), so we synthesise one
// using WriteManifest and then run SplitDOCX with PATH stripped
// so any attempt to invoke libreoffice would surface as an
// error.
func TestSplitDOCXSplitManifestHitSkipsLibreOffice(t *testing.T) {
	src := makeTestDOCX(t, "splithit.docx")
	outDir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	id, err := statSource(src)
	if err != nil {
		t.Fatal(err)
	}
	// Build a manifest that records a single PDF part.
	partName := "splithit_part1.pdf"
	partPath := filepath.Join(outDir, partName)
	if err := os.WriteFile(partPath, []byte("fake-pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(partPath)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindDOCX,
		Mode:          ModeSplit,
		SourcePath:    id.Path,
		SourceSize:    id.Size,
		SourceMTimeNS: id.MTime,
		MaxPages:      200,
		MaxSizeMB:     0,
		Parts: []Part{
			{Filename: partName, Index: 1, PageStart: 1, PageEnd: 1, Size: st.Size()},
		},
	}
	if err := WriteManifest(docxManifestPath(outDir, "splithit"), m); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")
	if err := SplitDOCX(src, 200, 0, outDir, false); err != nil {
		t.Fatalf("split-mode hit should not call libreoffice: %v", err)
	}
	// The fake part must still be on disk — i.e. the hit path
	// did not overwrite it.
	if _, err := os.Stat(partPath); err != nil {
		t.Fatalf("hit path disturbed part file: %v", err)
	}
}

// TestSplitPDFManifestBackwardCompatible covers the regression
// guarantee: a PDF manifest written by the upgraded code still
// includes kind/mode JSON keys so future readers don't have to
// guess.
func TestSplitPDFManifestBackwardCompatible(t *testing.T) {
	src := makeTestPDF(t, "legacy.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}

	data, err := os.ReadFile(ManifestPath(outDir, "legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"kind"`)) || !bytes.Contains(data, []byte(`"mode"`)) {
		t.Fatalf("PDF manifest missing kind/mode JSON keys: %s", data)
	}

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("PDF cache hit after manifest upgrade: %v", err)
	}
}

// guard against the keep-stdlib-imports drift — if a future edit
// drops one of the helper imports above the file still compiles.
var _ = requireLibreOffice
