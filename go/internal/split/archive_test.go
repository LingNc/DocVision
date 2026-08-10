package split

import (
	"os"
	"path/filepath"
	"testing"
)

// writePDFStub writes a non-empty file with .pdf extension.
// archiveSourceFile cares only about file existence and basename,
// so a stub is enough for tests that exercise archiveSourceFile
// in isolation (those never call SplitPDF).
func writePDFStub(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("stub-pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeDOCXStub(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("stub-docx"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// pdfStubWithManifest builds a synthetic PDF stub plus a matching
// manifest so SplitPDF's cache-hit path succeeds without invoking
// pdfcpu. We need this in archive tests because the SplitAll
// callers must succeed for the archive logic to run.
//
// `parts` lists the basenames of part files that already exist in
// outDir; the manifest's recorded sizes come from os.Stat.
func pdfStubWithManifest(t *testing.T, inputDir, outDir, base string, parts ...string) string {
	t.Helper()
	src := writePDFStub(t, inputDir, base+".pdf")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := statSource(src)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindPDF,
		Mode:          ModeSplit,
		SourcePath:    id.Path,
		SourceSize:    id.Size,
		SourceMTimeNS: id.MTime,
		MaxPages:      200,
		MaxSizeMB:     0,
	}
	for i, name := range parts {
		partFull := filepath.Join(outDir, name)
		st, err := os.Stat(partFull)
		if err != nil {
			t.Fatalf("stat part %s: %v", partFull, err)
		}
		m.Parts = append(m.Parts, Part{
			Filename:  name,
			Index:     i + 1,
			PageStart: i + 1,
			PageEnd:   i + 1,
			Size:      st.Size(),
		})
	}
	if err := WriteManifest(ManifestPath(outDir, base), m); err != nil {
		t.Fatal(err)
	}
	return src
}

// TestSplitAllArchivesPDF confirms SplitAll moves a successfully
// split PDF source into doneDir. The cache-hit path keeps pdfcpu
// out of the way so the stub bytes don't matter.
func TestSplitAllArchivesPDF(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")

	partPath := filepath.Join(outDir, "doc_part1.pdf")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partPath, []byte("part"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := pdfStubWithManifest(t, inputDir, outDir, "doc", "doc_part1.pdf")

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be archived, but still exists at %s (err=%v)", src, err)
	}
	if _, err := os.Stat(filepath.Join(doneDir, "doc.pdf")); err != nil {
		t.Fatalf("expected archived file in doneDir: %v", err)
	}
}

// TestSplitAllArchivesDOCX verifies that a DOCX whose manifest is
// mode=split is archived. We precreate the DOCX source, the PDF
// part, and the DOCX manifest so SplitDOCX takes the manifest hit
// path and returns nil without invoking LibreOffice.
func TestSplitAllArchivesDOCX(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")

	src := writeDOCXStub(t, inputDir, "doc.docx")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	partPath := filepath.Join(outDir, "doc_part1.pdf")
	if err := os.WriteFile(partPath, []byte("part"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(partPath)
	if err != nil {
		t.Fatal(err)
	}
	id, err := statSource(src)
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
			{Filename: "doc_part1.pdf", Index: 1, PageStart: 1, PageEnd: 1, Size: st.Size()},
		},
	}
	if err := WriteManifest(docxManifestPath(outDir, "doc"), m); err != nil {
		t.Fatal(err)
	}

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be archived, but still exists at %s (err=%v)", src, err)
	}
	if _, err := os.Stat(filepath.Join(doneDir, "doc.docx")); err != nil {
		t.Fatalf("expected archived file in doneDir: %v", err)
	}
}

// TestSplitAllLeavesPassthroughInPlace confirms a DOCX whose
// manifest is mode=passthrough is NOT archived — the source
// remains in inputDir for the next run to re-evaluate.
func TestSplitAllLeavesPassthroughInPlace(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")

	src := writeDOCXStub(t, inputDir, "doc.docx")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := statSource(src)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindDOCX,
		Mode:          ModePassthrough,
		SourcePath:    id.Path,
		SourceSize:    id.Size,
		SourceMTimeNS: id.MTime,
		MaxPages:      200,
		MaxSizeMB:     0,
		Parts:         nil,
	}
	if err := WriteManifest(docxManifestPath(outDir, "doc"), m); err != nil {
		t.Fatal(err)
	}

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("passthrough source should remain in inputDir, but stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(doneDir, "doc.docx")); !os.IsNotExist(err) {
		t.Fatalf("passthrough should not be archived, but file exists (err=%v)", err)
	}
}

// TestSplitAllNoDoneDirNoArchive confirms that passing an empty
// doneDir disables archiving entirely. We need a real PDF
// fixture so SplitPDF succeeds.
func TestSplitAllNoDoneDirNoArchive(t *testing.T) {
	inputDir := t.TempDir()
	src := makeTestPDF(t, "doc.pdf")

	if err := SplitAll(inputDir, 200, 0, filepath.Join(t.TempDir(), "out"), false, ""); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain when doneDir is empty: %v", err)
	}
}

// TestSplitAllArchiveConflictSkips confirms that when the
// destination in doneDir already has the same basename, the
// source is left in place (no overwrite).
func TestSplitAllArchiveConflictSkips(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "doc_part1.pdf"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := pdfStubWithManifest(t, inputDir, outDir, "doc", "doc_part1.pdf")
	// Pre-existing done file with same basename — must not be
	// overwritten.
	existing := filepath.Join(doneDir, "doc.pdf")
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should remain when destination exists: %v", err)
	}
	// Existing destination must be untouched (still its original bytes).
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing destination was overwritten: %q", data)
	}
}

// TestSplitAllForceRestoresArchived verifies that with --force,
// any source file already in doneDir is moved back to inputDir
// before splitting runs. The post-restore split may fail because
// the stub bytes are not a valid PDF — we only assert that the
// restore step happened, not that the split succeeded.
func TestSplitAllForceRestoresArchived(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archived := filepath.Join(doneDir, "doc.pdf")
	if err := os.WriteFile(archived, []byte("archived"), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = SplitAll(inputDir, 200, 0, outDir, true, doneDir)
	if _, err := os.Stat(archived); !os.IsNotExist(err) {
		t.Fatalf("force should have moved file out of doneDir, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inputDir, "doc.pdf")); err != nil {
		t.Fatalf("force should have restored file to inputDir: %v", err)
	}
}

// TestSplitAllMigrateDoneMarkers confirms *.pdf.done / *.docx.done
// in inputDir are migrated to doneDir with the .done suffix
// stripped. Same-name destination conflicts are skipped (warning).
func TestSplitAllMigrateDoneMarkers(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// *.pdf.done should migrate to doneDir/foo.pdf.
	if err := os.WriteFile(filepath.Join(inputDir, "a.pdf.done"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// *.docx.done likewise.
	if err := os.WriteFile(filepath.Join(inputDir, "b.docx.done"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-existing doneDir/foo.pdf — should warn and not remove the marker.
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doneDir, "a.pdf"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Random .done-looking suffix should not be migrated.
	if err := os.WriteFile(filepath.Join(inputDir, "a.pdf.done.also"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	// b.docx.done → doneDir/b.docx, no conflict.
	if _, err := os.Stat(filepath.Join(doneDir, "b.docx")); err != nil {
		t.Fatalf("expected migrated b.docx: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inputDir, "b.docx.done")); !os.IsNotExist(err) {
		t.Fatalf("b.docx.done should have been migrated, err=%v", err)
	}
	// a.pdf.done conflict: marker must remain in inputDir, existing
	// doneDir/a.pdf must not be touched.
	if _, err := os.Stat(filepath.Join(inputDir, "a.pdf.done")); err != nil {
		t.Fatalf("conflicting marker should remain in inputDir: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(doneDir, "a.pdf")); string(data) != "existing" {
		t.Fatalf("existing doneDir/a.pdf was modified")
	}
	// Random .done-looking suffix should not be migrated.
	if _, err := os.Stat(filepath.Join(inputDir, "a.pdf.done.also")); err != nil {
		t.Fatalf("non-marker .done file should be left in place: %v", err)
	}
}

// TestArchiveSourceFileDoneDirMissing confirms archiveSourceFile
// creates the destination directory if it does not exist.
func TestArchiveSourceFileDoneDirMissing(t *testing.T) {
	inputDir := t.TempDir()
	doneDir := filepath.Join(t.TempDir(), "sub", "done") // not created yet
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := writePDFStub(t, inputDir, "doc.pdf")
	if !archiveSourceFile(src, doneDir) {
		t.Fatal("archiveSourceFile should succeed when doneDir is created on demand")
	}
	if _, err := os.Stat(filepath.Join(doneDir, "doc.pdf")); err != nil {
		t.Fatalf("expected archived file: %v", err)
	}
}

// TestSplitAllPDFPassthroughNotArchived — a PDF whose parts exist
// is archived; if no parts are written the source stays put. We
// verify by feeding a real PDF with no parts and no manifest.
func TestSplitAllPDFNoPartsNoArchive(t *testing.T) {
	inputDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	doneDir := filepath.Join(t.TempDir(), "done")
	src := makeTestPDF(t, "doc.pdf")

	if err := SplitAll(inputDir, 200, 0, outDir, false, doneDir); err != nil {
		t.Fatalf("SplitAll: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source with no parts should not be archived: %v", err)
	}
}
