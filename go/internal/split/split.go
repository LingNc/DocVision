// Package split implements PDF splitting by page count and file size.
//
// Behaviour mirrors the Python reference (python/split_pdfs.py):
//   - Output files are named "{base}_part{N}.pdf" in outputDir.
//   - A binary search over page ranges respects both maxPages and maxSizeMB.
//   - Existing parts whose total page count matches the source are skipped
//     unless force=true.
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

// SplitPDF splits pdfPath into "{base}_partN.pdf" files in outputDir,
// honouring both maxPages and maxSizeMB (0 = no size limit).
//
// If existing "{base}_part*.pdf" files already account for the source's
// total page count, the split is skipped (unless force=true).
func SplitPDF(pdfPath string, maxPages int, maxSizeMB float64, outputDir string, force bool) error {
	if !util.FileExists(pdfPath) {
		return fmt.Errorf("文件不存在: %s", pdfPath)
	}
	if err := util.EnsureDir(outputDir); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	totalPages, err := api.PageCountFile(pdfPath)
	if err != nil {
		return fmt.Errorf("读取 PDF 失败: %w", err)
	}

	baseName := util.BaseNameNoExt(pdfPath)
	displayName := filepath.Base(pdfPath)

	// Skip detection.
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
	return nil
}

// SplitAll runs SplitPDF on every .pdf file in inputDir (non-recursive,
// hidden files skipped). Each file is processed independently; an error
// on one file does not stop the others — the first error is returned at
// the end.
func SplitAll(inputDir string, maxPages int, maxSizeMB float64, outputDir string, force bool) error {
	if !util.DirExists(inputDir) {
		return fmt.Errorf("输入目录不存在: %s", inputDir)
	}

	files, err := util.ListFiles(inputDir, ".pdf")
	if err != nil {
		return fmt.Errorf("读取目录失败: %w", err)
	}
	// ListFiles does not filter dotfiles — drop them to match Python.
	visible := files[:0]
	for _, f := range files {
		if !strings.HasPrefix(filepath.Base(f), ".") {
			visible = append(visible, f)
		}
	}
	files = visible

	if len(files) == 0 {
		fmt.Printf("[警告] 目录 %s 下没有找到 PDF 文件\n", inputDir)
		return nil
	}

	fmt.Printf("找到 %d 个 PDF 文件\n\n", len(files))

	var firstErr error
	for _, f := range files {
		if err := SplitPDF(f, maxPages, maxSizeMB, outputDir, force); err != nil {
			fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
