// Package split implements PDF splitting by page count and file size.
//
// Behaviour mirrors the archived Python reference
// (legacy/python/python/split_pdfs.py):
//   - Output files are named "{base}_part{N}.pdf" in outputDir.
//   - A binary search over page ranges respects both maxPages and maxSizeMB.
//   - Existing parts whose total page count matches the source are skipped
//     unless force=true.
//   - A sidecar split manifest (see manifest.go) is written after a
//     successful split. Subsequent invocations that find a matching,
//     fully-verifiable manifest can skip pdfcpu PageCountFile/TrimFile
//     entirely. --force always bypasses the cache.
//
// Note: pdfcpu does not expose a "split-by-page-count from an open context"
// API, so the helpers operate on file paths and use api.TrimFile to write
// page ranges. Each binary-search probe writes (and deletes) a temp file.
package split

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"mineru-tools/pkg/util"
)

// pdfConf is reused for all pdfcpu calls. nil = library defaults; we keep it
// explicit so tests can override later if needed.
var pdfConf = model.NewDefaultConfiguration()

// estimatePartSizeMB extracts pages [fromPage, toPage] (1-based, inclusive)
// from srcPath into a temporary PDF, returns its size in MB, then deletes it.
func estimatePartSizeMB(srcPath string, fromPage, toPage int) (float64, error) {
	tmp, err := os.CreateTemp("", "split-estimate-*.pdf")
	if err != nil {
		return 0, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	pageRange := fmt.Sprintf("%d-%d", fromPage, toPage)
	if err := api.TrimFile(srcPath, tmpPath, []string{pageRange}, pdfConf); err != nil {
		return 0, fmt.Errorf("trim %s: %w", pageRange, err)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("stat temp: %w", err)
	}
	return float64(info.Size()) / (1024 * 1024), nil
}

// splitBySizeAndPages finds the largest end page (1-based, inclusive) in the
// range [start+1, start+maxPages] such that pages [start+1, end] fit within
// maxSizeMB. start is 0-based (matches Python). Returns the chosen end page,
// 1-based and inclusive — i.e. the last page belonging to the current part.
//
// If maxSizeMB <= 0, size is not constrained and the page-count cap is used.
// If a single page already exceeds the limit, a warning is printed and that
// single page is returned (matching Python).
func splitBySizeAndPages(srcPath string, start, totalPages, maxPages int, maxSizeMB float64) (int, error) {
	// Page-count cap (1-based, inclusive end).
	end := start + maxPages
	if end > totalPages {
		end = totalPages
	}

	if maxSizeMB <= 0 {
		return end, nil
	}

	// Single-page edge: probe page (start+1) on its own.
	singleMB, err := estimatePartSizeMB(srcPath, start+1, start+1)
	if err != nil {
		return 0, err
	}
	if singleMB > maxSizeMB {
		fmt.Printf("  [警告] 第 %d 页单页大小 %.1fMB 已超过限制 %.0fMB，仍将单独输出\n",
			start+1, singleMB, maxSizeMB)
		return start + 1, nil
	}

	// If the whole page-count slice fits, take it.
	fullMB, err := estimatePartSizeMB(srcPath, start+1, end)
	if err != nil {
		return 0, err
	}
	if fullMB <= maxSizeMB {
		return end, nil
	}

	// Binary search for the largest end ∈ [start+1, end] that fits.
	lo, hi := start+1, end
	best := start + 1
	for lo <= hi {
		mid := (lo + hi) / 2
		sizeMB, err := estimatePartSizeMB(srcPath, start+1, mid)
		if err != nil {
			return 0, err
		}
		if sizeMB <= maxSizeMB {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best, nil
}

// isAlreadySplit reports whether existingParts taken together cover exactly
// totalPages. It opens each part to read its page count; any read error
// causes a false result (so the caller re-splits).
func isAlreadySplit(existingParts []string, totalPages int) bool {
	sum := 0
	for _, p := range existingParts {
		n, err := api.PageCountFile(p)
		if err != nil {
			return false
		}
		sum += n
	}
	return sum == totalPages
}

// sourceIdentity captures the stat info used for manifest matching.
// It is also used by the legacy skip path so both code paths agree
// on what "the source" means.
type sourceIdentity struct {
	Path  string
	Size  int64
	MTime int64
}

// statSource returns a sourceIdentity for pdfPath. Returns the error
// from os.Stat unchanged — both the manifest and the legacy skip
// path treat unreadable sources as a hard failure.
func statSource(pdfPath string) (sourceIdentity, error) {
	info, err := os.Stat(pdfPath)
	if err != nil {
		return sourceIdentity{}, err
	}
	return sourceIdentity{
		Path:  pdfPath,
		Size:  info.Size(),
		MTime: info.ModTime().UnixNano(),
	}, nil
}

// tryManifestHit checks whether a valid manifest exists for the
// given source and parameters, and whether every recorded part
// still matches on disk. On hit it logs a skip line and returns
// true. On miss it returns false (errors are not fatal here — any
// read/parse/mismatch failure silently degrades to a miss so the
// caller can re-split safely).
func tryManifestHit(outputDir, baseName string, src sourceIdentity, maxPages int, maxSizeMB float64) bool {
	manifestPath := ManifestPath(outputDir, baseName)
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return false
	}
	if !m.Matches(MatchParams{
		SourcePath:  normalizeSourcePath(src.Path),
		SourceSize:  src.Size,
		SourceMTime: src.MTime,
		MaxPages:    maxPages,
		MaxSizeMB:   maxSizeMB,
	}) {
		return false
	}
	ok, err := m.VerifyAgainstDisk(outputDir)
	if err != nil || !ok {
		return false
	}
	fmt.Printf("[跳过] %s: 命中 split manifest (%d 个部分)，跳过 pdfcpu 解析\n",
		filepath.Base(src.Path), len(m.Parts))
	return true
}

// recordedPart is one part as captured during the split loop, so
// that the manifest can be built without re-reading the part files.
type recordedPart struct {
	Index     int
	PageStart int
	PageEnd   int
	Size      int64
	Filename  string
}

// SplitPDF splits pdfPath into "{base}_partN.pdf" files in outputDir,
// honouring both maxPages and maxSizeMB (0 = no size limit).
//
// If a valid split manifest exists for pdfPath and every recorded
// part is still on disk with matching size, the split is skipped
// without invoking pdfcpu (unless force=true). Without a manifest
// the existing page-sum skip check still applies, so legacy output
// directories continue to work.
func SplitPDF(pdfPath string, maxPages int, maxSizeMB float64, outputDir string, force bool) error {
	if !util.FileExists(pdfPath) {
		return fmt.Errorf("文件不存在: %s", pdfPath)
	}
	if err := util.EnsureDir(outputDir); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	baseName := util.BaseNameNoExt(pdfPath)
	displayName := filepath.Base(pdfPath)

	src, err := statSource(pdfPath)
	if err != nil {
		return fmt.Errorf("读取源文件失败: %w", err)
	}

	// Manifest hit is checked before pdfcpu is touched. --force bypasses.
	//
	// statSource uses os.Stat (not pdfcpu) so a hit path is robust
	// to a source that pdfcpu can no longer parse — e.g. the file
	// was overwritten with garbage since the manifest was written.
	// That is exactly the contract called out by the test
	// TestSplitPDFCorruptSourceStillSkippedOnHit.
	if !force {
		if tryManifestHit(outputDir, baseName, src, maxPages, maxSizeMB) {
			return nil
		}
	}

	totalPages, err := api.PageCountFile(pdfPath)
	if err != nil {
		return fmt.Errorf("读取 PDF 失败: %w", err)
	}

	// Legacy skip path (no manifest): existing parts already cover
	// the source's full page count. Kept for backwards compatibility
	// with directories produced before the manifest format existed.
	if !force {
		existing, err := util.GlobSorted(filepath.Join(outputDir, baseName+"_part*.pdf"))
		if err != nil {
			return fmt.Errorf("查找已有分片失败: %w", err)
		}
		if len(existing) > 0 {
			if isAlreadySplit(existing, totalPages) {
				fmt.Printf("[跳过] %s: 已分割为 %d 个部分，跳过\n", displayName, len(existing))
				return nil
			}
			fmt.Printf("[信息] %s: 已有 %d 个部分但页数不匹配，重新分割\n", displayName, len(existing))
		}
	}

	fmt.Printf("[信息] %s: 共 %d 页\n", displayName, totalPages)

	var recorded []recordedPart

	part := 1
	start := 0 // 0-based start, matches Python.
	for start < totalPages {
		end, err := splitBySizeAndPages(pdfPath, start, totalPages, maxPages, maxSizeMB)
		if err != nil {
			return fmt.Errorf("规划第 %d 部分失败: %w", part, err)
		}
		// end is 1-based inclusive last page of this part.
		partName := fmt.Sprintf("%s_part%d.pdf", baseName, part)
		partPath := filepath.Join(outputDir, partName)
		pageRange := fmt.Sprintf("%d-%d", start+1, end)

		if err := api.TrimFile(pdfPath, partPath, []string{pageRange}, pdfConf); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", partName, err)
		}

		info, err := os.Stat(partPath)
		if err != nil {
			return fmt.Errorf("stat %s: %w", partName, err)
		}
		recorded = append(recorded, recordedPart{
			Index:     part,
			PageStart: start + 1,
			PageEnd:   end,
			Size:      info.Size(),
			Filename:  partName,
		})
		pageCount := end - start
		if maxSizeMB > 0 {
			fmt.Printf("  → %s  (页 %d–%d, 共 %d 页, %.1fMB)\n",
				partName, start+1, end, pageCount, float64(info.Size())/(1024*1024))
		} else {
			fmt.Printf("  → %s  (页 %d–%d, 共 %d 页)\n",
				partName, start+1, end, pageCount)
		}

		start = end
		part++
	}

	fmt.Printf("[完成] 共分割为 %d 个部分，输出到 %s/\n\n", part-1, outputDir)

	// Manifest write: only after every part has been successfully
	// written and stat'd. A failure here must not silently produce
	// a half-valid manifest — we log and continue, since the
	// on-disk parts are already correct and the next run will
	// re-validate via the legacy path.
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindPDF,
		Mode:          ModeSplit,
		SourcePath:    normalizeSourcePath(src.Path),
		SourceSize:    src.Size,
		SourceMTimeNS: src.MTime,
		MaxPages:      maxPages,
		MaxSizeMB:     maxSizeMB,
		Parts:         make([]Part, 0, len(recorded)),
	}
	for _, r := range recorded {
		m.Parts = append(m.Parts, Part{
			Filename:  r.Filename,
			Index:     r.Index,
			PageStart: r.PageStart,
			PageEnd:   r.PageEnd,
			Size:      r.Size,
		})
	}
	if err := WriteManifest(ManifestPath(outputDir, baseName), m); err != nil {
		fmt.Fprintf(os.Stderr, "[警告] 写入 split manifest 失败: %v\n", err)
	}

	// Best-effort cleanup of stale parts from a previous run that
	// are not present in the new manifest. Skips the manifest file
	// itself (different name pattern) and any non-pdf artefacts.
	cleanupStaleParts(outputDir, baseName, recorded)

	return nil
}

// cleanupStaleParts removes pdf files named "{base}_partN.pdf" in
// outputDir whose index does not appear in the new split. Hidden /
// non-pdf files are ignored. Manifests are never matched by the
// glob (different filename) so they are safe.
func cleanupStaleParts(outputDir, baseName string, kept []recordedPart) {
	keptNames := make(map[string]bool, len(kept))
	for _, r := range kept {
		keptNames[r.Filename] = true
	}
	pattern := filepath.Join(outputDir, baseName+"_part*.pdf")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, m := range matches {
		name := filepath.Base(m)
		if keptNames[name] {
			continue
		}
		_ = os.Remove(m)
	}
}

// SplitAll runs SplitPDF on every .pdf and SplitDOCX on every .docx
// file in inputDir (non-recursive, hidden files skipped). Each file
// is processed independently; an error on one file does not stop
// the others — the first error is returned at the end.
//
// If doneDir is non-empty, every source file whose split produced
// parts on disk is moved (os.Rename) into doneDir/basename so the
// input directory only contains files that still need processing.
// Sources whose split skipped work (DOCX passthrough, or a PDF
// manifest hit that produced nothing new) are NOT archived. The
// pass is best-effort: a rename failure is logged and never blocks
// the split of other files. When force=true, any previously-
// archived source with the same basename is restored to inputDir
// before splitting so the source path in the manifest matches.
func SplitAll(inputDir string, maxPages int, maxSizeMB float64, outputDir string, force bool, doneDir string) error {
	if !util.DirExists(inputDir) {
		return fmt.Errorf("输入目录不存在: %s", inputDir)
	}

	pdfs, err := util.ListFiles(inputDir, ".pdf")
	if err != nil {
		return fmt.Errorf("读取目录失败: %w", err)
	}
	docxs, err := util.ListFiles(inputDir, ".docx")
	if err != nil {
		return fmt.Errorf("读取目录失败: %w", err)
	}
	// ListFiles does not filter dotfiles — drop them to match Python.
	visible := pdfs[:0]
	for _, f := range pdfs {
		if !strings.HasPrefix(filepath.Base(f), ".") {
			visible = append(visible, f)
		}
	}
	pdfs = visible
	visible = docxs[:0]
	for _, f := range docxs {
		if !strings.HasPrefix(filepath.Base(f), ".") {
			visible = append(visible, f)
		}
	}
	docxs = visible

	// One-time housekeeping runs BEFORE the empty-input early
	// return: --force may need to restore previously-archived
	// sources even when inputDir currently looks empty, and the
	// *.done migration should happen on every run regardless of
	// whether anything is left to split.
	if doneDir != "" {
		if err := util.EnsureDir(doneDir); err != nil {
			fmt.Fprintf(os.Stderr, "[警告] 创建 done 目录失败: %v\n", err)
		}
		migrateDoneMarkers(inputDir, doneDir)
	}
	if force && doneDir != "" {
		restoreArchivedOnForce(inputDir, doneDir)
		// Refresh the listings after restoring — newly-restored
		// files are exactly the ones we want to split next.
		if pdfs, err = util.ListFiles(inputDir, ".pdf"); err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		if docxs, err = util.ListFiles(inputDir, ".docx"); err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		visible := pdfs[:0]
		for _, f := range pdfs {
			if !strings.HasPrefix(filepath.Base(f), ".") {
				visible = append(visible, f)
			}
		}
		pdfs = visible
		visible = docxs[:0]
		for _, f := range docxs {
			if !strings.HasPrefix(filepath.Base(f), ".") {
				visible = append(visible, f)
			}
		}
		docxs = visible
	}

	total := len(pdfs) + len(docxs)
	if total == 0 {
		fmt.Printf("[警告] 目录 %s 下没有找到 PDF 或 DOCX 文件\n", inputDir)
		return nil
	}

	fmt.Printf("找到 %d 个文件（%d 个 PDF, %d 个 DOCX）\n\n", total, len(pdfs), len(docxs))

	var firstErr error
	for _, f := range pdfs {
		if err := SplitPDF(f, maxPages, maxSizeMB, outputDir, force); err != nil {
			fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if doneDir != "" && pdfShouldArchive(f, outputDir) {
			_ = archiveSourceFile(f, doneDir)
		}
	}
	for _, f := range docxs {
		if err := SplitDOCX(f, maxPages, maxSizeMB, outputDir, force); err != nil {
			fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if doneDir != "" && docxShouldArchive(f, outputDir) {
			_ = archiveSourceFile(f, doneDir)
		}
	}
	return firstErr
}

// pdfShouldArchive reports whether the PDF source should be moved
// into doneDir after a successful split. We require at least one
// "{base}_part*.pdf" on disk — a manifest hit with a deleted part
// leaves the source needing re-split, so we do not archive it.
func pdfShouldArchive(srcPath, outputDir string) bool {
	base := util.BaseNameNoExt(srcPath)
	matches, err := util.GlobSorted(filepath.Join(outputDir, base+"_part*.pdf"))
	if err != nil {
		return false
	}
	return len(matches) > 0
}

// docxShouldArchive reports whether the DOCX source should be moved
// into doneDir. We only archive when the DOCX manifest records
// Mode=split (i.e. parts exist on disk). Passthrough sources stay
// in inputDir so the next run can re-evaluate them.
func docxShouldArchive(srcPath, outputDir string) bool {
	base := util.BaseNameNoExt(srcPath)
	m, err := LoadManifest(docxManifestPath(outputDir, base))
	if err != nil {
		return false
	}
	return m.Mode == ModeSplit && len(m.Parts) > 0
}

// archiveSourceFile moves srcPath into doneDir/basename. The
// destination must not already exist; we never overwrite an
// archived file. Rename failures (including cross-filesystem
// EXDEV) are logged and swallowed so the split of subsequent
// files proceeds.
func archiveSourceFile(srcPath, doneDir string) bool {
	if doneDir == "" {
		return false
	}
	if err := util.EnsureDir(doneDir); err != nil {
		fmt.Fprintf(os.Stderr, "[警告] 创建 done 目录失败 (%s): %v\n", doneDir, err)
		return false
	}
	dst := filepath.Join(doneDir, filepath.Base(srcPath))
	if _, err := os.Stat(dst); err == nil {
		fmt.Fprintf(os.Stderr, "[警告] done 目录已存在同名文件，跳过归档: %s\n", dst)
		return false
	}
	if err := os.Rename(srcPath, dst); err != nil {
		fmt.Fprintf(os.Stderr, "[警告] 归档源文件失败 (%s -> %s): %v\n", srcPath, dst, err)
		return false
	}
	fmt.Printf("[归档] %s -> %s/\n", filepath.Base(srcPath), filepath.Base(doneDir))
	return true
}

// migrateDoneMarkers moves any input_dir/*.pdf.done / *.docx.done
// files into doneDir, stripping the .done suffix on the way.
// Targets that already exist are skipped (warning logged). This is
// a one-shot migration for installations that previously archived
// files by renaming them in place.
func migrateDoneMarkers(inputDir, doneDir string) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".done") {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".pdf.done") &&
			!strings.HasSuffix(strings.ToLower(name), ".docx.done") {
			continue
		}
		src := filepath.Join(inputDir, name)
		dst := filepath.Join(doneDir, strings.TrimSuffix(name, ".done"))
		if _, err := os.Stat(dst); err == nil {
			fmt.Fprintf(os.Stderr, "[警告] done 目录已有同名文件，跳过迁移: %s\n", dst)
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "[警告] 迁移 .done 标记失败 (%s -> %s): %v\n", src, dst, err)
			continue
		}
		fmt.Printf("[迁移] %s -> %s\n", name, dst)
	}
}

// restoreArchivedOnForce moves any doneDir/{base}.pdf / {base}.docx
// whose basename is not currently in inputDir back into inputDir
// so --force re-splits from the original source. Same-filename
// conflicts are skipped (warning logged).
func restoreArchivedOnForce(inputDir, doneDir string) {
	if !util.DirExists(doneDir) {
		return
	}
	entries, err := os.ReadDir(doneDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if !strings.HasSuffix(name, ".pdf") && !strings.HasSuffix(name, ".docx") {
			continue
		}
		dst := filepath.Join(inputDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			fmt.Fprintf(os.Stderr, "[警告] input_dir 已有同名文件，跳过恢复: %s\n", dst)
			continue
		}
		src := filepath.Join(doneDir, e.Name())
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "[警告] 从 done 恢复源文件失败 (%s -> %s): %v\n", src, dst, err)
			continue
		}
		fmt.Printf("[恢复] %s -> %s/\n", e.Name(), filepath.Base(inputDir))
	}
}
