package split

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/pkg/util"
)

// makeSourceUnreadableToPdfcpu removes read permissions so pdfcpu
// (which opens the file) fails, while os.Stat still succeeds.
// This is what proves the cache-hit path never invokes pdfcpu.
func makeSourceUnreadableToPdfcpu(t *testing.T, src string) {
	t.Helper()
	if err := os.Chmod(src, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o644) })
}

// TestSplitPDFCacheHitSkipsPdfcpu runs SplitPDF twice on the same
// source and verifies the second call:
//   - skips pdfcpu entirely (the source is chmod 0 between runs,
//     so any PageCountFile call would error with permission
//     denied)
//   - does not re-write existing parts (sizes are byte-identical)
func TestSplitPDFCacheHitSkipsPdfcpu(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}

	partsBefore := listParts(t, outDir, "doc")
	if len(partsBefore) == 0 {
		t.Fatal("first split wrote no parts")
	}
	sizesBefore := sizesByName(t, partsBefore)

	// Revoke read permission so pdfcpu (which opens the file) will
	// fail. os.Stat on the source still works, so manifest matching
	// proceeds and the hit path returns without touching pdfcpu.
	makeSourceUnreadableToPdfcpu(t, src)

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("cache hit should not call pdfcpu: %v", err)
	}

	// Parts unchanged.
	partsAfter := listParts(t, outDir, "doc")
	sizesAfter := sizesByName(t, partsAfter)
	if !sameSizes(sizesBefore, sizesAfter) {
		t.Fatalf("part sizes changed after cache hit: before=%v after=%v", sizesBefore, sizesAfter)
	}
}

// TestSplitPDFCacheMissOnMtimeChange verifies that bumping the
// source mtime invalidates the cache.
func TestSplitPDFCacheMissOnMtimeChange(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}

	// Bump source mtime.
	touchPDF(t, src)

	// Now revoke read permission so pdfcpu fails. The expected
	// outcome is: cache miss (mtime changed) → pdfcpu called →
	// permission denied error from open().
	makeSourceUnreadableToPdfcpu(t, src)

	err := SplitPDF(src, 3, 0, outDir, false)
	if err == nil {
		t.Fatal("expected pdfcpu error after mtime bump + unreadable source")
	}
	if !strings.Contains(err.Error(), "读取 PDF 失败") &&
		!strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error %v does not look like a pdfcpu read failure", err)
	}
}

// TestSplitPDFCacheMissOnParamChange verifies that changing
// maxPages invalidates the cache.
func TestSplitPDFCacheMissOnParamChange(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}

	// Different maxPages → cache miss. Source still valid → split
	// re-runs and writes a fresh manifest. We bypass the legacy
	// skip path by deleting the parts first; otherwise the legacy
	// page-sum check (3 parts × ≈2.7 pages = 8) would still match
	// and the call would short-circuit there instead of exercising
	// the manifest→re-split flow.
	for _, p := range listParts(t, outDir, "doc") {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}

	if err := SplitPDF(src, 2, 0, outDir, false); err != nil {
		t.Fatalf("re-split with new params: %v", err)
	}
	parts := listParts(t, outDir, "doc")
	// 8 pages / max 2 = 4 parts.
	if len(parts) != 4 {
		t.Fatalf("after param change got %d parts, want 4", len(parts))
	}
	// Manifest must now reflect maxPages=2.
	m, err := LoadManifest(ManifestPath(outDir, "doc"))
	if err != nil {
		t.Fatal(err)
	}
	if m.MaxPages != 2 {
		t.Fatalf("manifest max_pages = %d, want 2", m.MaxPages)
	}
}

// TestSplitPDFForceMiss confirms --force bypasses the manifest.
func TestSplitPDFForceMiss(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	sizesBefore := sizesByName(t, listParts(t, outDir, "doc"))

	// Revoke read permission so pdfcpu fails. Without --force the
	// cache would hit and pdfcpu would not be called, so the test
	// would pass for the wrong reason.
	makeSourceUnreadableToPdfcpu(t, src)

	if err := SplitPDF(src, 3, 0, outDir, true); err == nil {
		t.Fatal("expected error: --force must call pdfcpu and fail on unreadable source")
	}

	// Parts still on disk unchanged (no destructive cleanup
	// because the new split failed).
	sizesAfter := sizesByName(t, listParts(t, outDir, "doc"))
	if !sameSizes(sizesBefore, sizesAfter) {
		t.Fatalf("parts changed despite failed re-split: before=%v after=%v", sizesBefore, sizesAfter)
	}
}

// TestSplitPDFPartDeletedMiss covers the "user deleted a part
// file" case. The manifest still exists and looks valid in
// isolation, but VerifyAgainstDisk must report a miss.
func TestSplitPDFPartDeletedMiss(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}

	// Delete the last part file.
	parts := listParts(t, outDir, "doc")
	last := parts[len(parts)-1]
	if err := os.Remove(last); err != nil {
		t.Fatal(err)
	}

	// Re-running with the same valid source should re-split,
	// because the manifest now misses on disk verification.
	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("re-split after part deletion: %v", err)
	}
	partsAfter := listParts(t, outDir, "doc")
	if len(partsAfter) != len(parts) {
		t.Fatalf("got %d parts after re-split, want %d", len(partsAfter), len(parts))
	}
	for _, p := range partsAfter {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected part %s on disk after re-split: %v", p, err)
		}
	}
}

// TestSplitPDFLegacyNoManifest verifies that an output directory
// with parts but no manifest still gets the legacy page-sum skip
// (existing behaviour preserved for back-compat).
func TestSplitPDFLegacyNoManifest(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	// Drop the manifest so the second run hits the legacy path.
	if err := os.Remove(ManifestPath(outDir, "doc")); err != nil {
		t.Fatal(err)
	}
	sizesBefore := sizesByName(t, listParts(t, outDir, "doc"))

	// Revoke read permission on source. With no manifest, the
	// legacy skip path still has to read the source to know the
	// total page count — so the call IS expected to fail. We
	// just confirm it fails with the pdfcpu read error, not with
	// some manifest parsing error. This documents that legacy
	// back-compat requires a readable source.
	makeSourceUnreadableToPdfcpu(t, src)
	err := SplitPDF(src, 3, 0, outDir, false)
	if err == nil {
		t.Fatal("expected error: legacy path reads source PDF for page count")
	}
	if !strings.Contains(err.Error(), "读取 PDF 失败") {
		t.Fatalf("unexpected error from legacy path: %v", err)
	}
	// Restore readability and re-run; the legacy skip should
	// now apply cleanly.
	_ = os.Chmod(src, 0o644)
	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("legacy skip after restore: %v", err)
	}
	sizesAfter := sizesByName(t, listParts(t, outDir, "doc"))
	if !sameSizes(sizesBefore, sizesAfter) {
		t.Fatalf("legacy skip rewrote parts: before=%v after=%v", sizesBefore, sizesAfter)
	}
}

// TestSplitPDFCorruptSourceStillSkippedOnHit covers the explicit
// invariant from the task: a source that pdfcpu can no longer
// parse must still be skipped when the manifest hits. We use
// chmod 0 instead of overwriting to keep size+mtime stable.
func TestSplitPDFCorruptSourceStillSkippedOnHit(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	makeSourceUnreadableToPdfcpu(t, src)
	// PageCountFile would explode with permission denied; cache
	// hit must skip it entirely.
	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("cache hit must not invoke pdfcpu: %v", err)
	}
}

// TestSplitPDFCacheHitPagesSumMatches asserts the manifest's page
// ranges cover the source end-to-end (sum of (end-start+1) ==
// total source pages). This is the structural sanity check the
// legacy skip path used to provide.
func TestSplitPDFCacheHitPagesSumMatches(t *testing.T) {
	src := makeTestPDF(t, "doc.pdf")
	outDir := filepath.Join(t.TempDir(), "out")

	if err := SplitPDF(src, 3, 0, outDir, false); err != nil {
		t.Fatalf("first split: %v", err)
	}
	m, err := LoadManifest(ManifestPath(outDir, "doc"))
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, p := range m.Parts {
		total += p.PageEnd - p.PageStart + 1
	}
	if total != testPDFPages {
		t.Fatalf("manifest parts cover %d pages, want %d", total, testPDFPages)
	}
}

// --- helpers below ---

func listParts(t *testing.T, dir, base string) []string {
	t.Helper()
	g, err := util.GlobSorted(filepath.Join(dir, base+"_part*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func sizesByName(t *testing.T, paths []string) map[string]int64 {
	t.Helper()
	m := make(map[string]int64, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		m[filepath.Base(p)] = info.Size()
	}
	return m
}

func sameSizes(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Ensure util stays referenced even if a future test removes its
// only caller; keeps imports honest without affecting behaviour.
var _ = util.GlobSorted
