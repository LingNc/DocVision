// Package split — DOCX splitting support.
//
// A DOCX file is a ZIP archive; the document body lives in
// word/document.xml as a sequence of top-level <w:p> (paragraph) and
// <w:tbl> (table) elements inside <w:body>. Because DOCX has no fixed
// page metadata, page count is estimated as paragraphCount/20.
//
// Splitting strategy mirrors SplitPDF:
//   - Output files are named "{base}_part{N}.docx" in outputDir.
//   - A binary search over element indices respects both maxPages and
//     maxSizeMB. Each probe writes (and deletes) a temp DOCX.
//   - Existing parts whose total paragraph count matches the source are
//     skipped unless force=true.
package split

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"mineru-tools/pkg/util"
)

// paragraphsPerPage is the heuristic used to convert a paragraph count
// into an estimated page count. Typical Word documents have ~10-20
// paragraphs per page; we use 10 to be conservative and stay under
// MinerU's 200-page limit.
const paragraphsPerPage = 10

// convertDocxToPDF converts a DOCX file to PDF using LibreOffice.
// The PDF is written to outDir with the same base name. Returns the
// path to the created PDF file.
func convertDocxToPDF(docxPath, outDir string) (string, error) {
	absDocx, err := filepath.Abs(docxPath)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve output dir: %w", err)
	}

	cmd := exec.Command("libreoffice",
		"--headless",
		"--convert-to", "pdf",
		"--outdir", absOut,
		absDocx,
	)
	// Suppress LibreOffice's stderr (Java warnings etc.) — the
	// conversion result is verified by checking the output file.
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("libreoffice 转换失败: %w", err)
	}

	baseName := util.BaseNameNoExt(docxPath)
	pdfPath := filepath.Join(outDir, baseName+".pdf")
	if !util.FileExists(pdfPath) {
		return "", fmt.Errorf("转换后的 PDF 文件未生成: %s", pdfPath)
	}
	return pdfPath, nil
}

// docxBodyElement represents one top-level element inside <w:body>.
// isParagraph is true for <w:p>; isSectPr is true for the trailing
// <w:sectPr> (which carries page-layout settings and should only
// appear in the final part); data is the original raw XML bytes for
// that element, captured directly from the source so namespaces and
// formatting are preserved exactly.
type docxBodyElement struct {
	isParagraph bool
	isSectPr    bool
	data        []byte
}

// readDOCXPageCount reads docProps/app.xml from the DOCX ZIP and returns
// the <Pages> value. Returns 0 if the file or element is missing.
func readDOCXPageCount(path string) int {
	r, err := zip.OpenReader(path)
	if err != nil {
		return 0
	}
	defer r.Close()

	rc, err := openZipEntry(r, "docProps/app.xml")
	if err != nil {
		return 0
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return 0
	}

	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "Pages" {
			var pages int
			if dec.DecodeElement(&pages, &start) == nil {
				return pages
			}
		}
	}
}

// readDOCXBody opens the DOCX at path and extracts the top-level body
// elements from word/document.xml. It returns:
//   - prefix:  raw bytes from the start of document.xml up to and
//     including the <w:body> start tag (preserved verbatim).
//   - elements: one entry per top-level child of <w:body>, in order.
//   - suffix: raw bytes from </w:body> to the end of document.xml,
//     preserved verbatim.
//
// We walk the XML token stream once to locate the boundaries of each
// top-level body element, then slice the original byte ranges. This
// preserves namespaces, attribute order, and whitespace exactly as the
// source has them — round-tripping through xml.Encoder would reformat
// things and (worse) can mangle xmlns declarations.
func readDOCXBody(path string) (prefix []byte, elements []docxBodyElement, suffix []byte, err error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open docx zip: %w", err)
	}
	defer r.Close()

	rc, err := openZipEntry(r, "word/document.xml")
	if err != nil {
		return nil, nil, nil, err
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read document.xml: %w", err)
	}

	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false

	inBody := false

	for {
		startOff := dec.InputOffset()

		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decode token: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if !inBody {
				if t.Name.Local == "body" {
					// Prefix = everything up to and including the
					// closing '>' of <w:body>.
					prefix = append(prefix, raw[:dec.InputOffset()]...)
					inBody = true
				}
				continue
			}
			// New top-level body element. Capture the byte range
			// from startOff (the '<' of this element) up to the
			// matching end tag's closing '>'.
			elEnd, err := findElementEnd(dec, raw)
			if err != nil {
				return nil, nil, nil, err
			}
			el := docxBodyElement{
				isParagraph: t.Name.Local == "p",
				isSectPr:    t.Name.Local == "sectPr",
				data:        append([]byte(nil), raw[startOff:elEnd]...),
			}
			elements = append(elements, el)

		case xml.EndElement:
			if !inBody {
				continue
			}
			if t.Name.Local == "body" {
				// Suffix = everything from this end tag's '<' to EOF.
				suffix = append(suffix, raw[startOff:]...)
			}
		}
	}

	if !inBody {
		return nil, nil, nil, fmt.Errorf("word/document.xml has no <w:body> element")
	}
	return prefix, elements, suffix, nil
}

// findElementEnd consumes tokens from dec until the end tag that
// matches the most-recently-consumed start tag (the one whose closing
// '>' we are positioned just past) has been fully consumed. It returns
// the byte offset in raw immediately after that end tag's '>'.
//
// On entry the caller has just called dec.Token() and received a
// xml.StartElement; dec.InputOffset() is just past that start tag's
// '>'. findElementEnd consumes all child tokens (incrementing and
// decrementing depth as nested elements appear) until depth returns to
// 0, then returns.
func findElementEnd(dec *xml.Decoder, raw []byte) (int64, error) {
	depth := 1
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return 0, fmt.Errorf("unexpected EOF inside element")
		}
		if err != nil {
			return 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			_ = t
		case xml.EndElement:
			depth--
			if depth == 0 {
				return dec.InputOffset(), nil
			}
		}
	}
}

// openZipEntry returns a reader for the named entry in r, or an error
// if it does not exist.
func openZipEntry(r *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("docx missing required entry %q", name)
}

// writeDOCXPart writes a new DOCX to outputPath by copying every entry
// from srcPath and replacing word/document.xml with prefix + the given
// elements + suffix.
func writeDOCXPart(outputPath, srcPath string, prefix []byte, elements []docxBodyElement, suffix []byte) error {
	src, err := zip.OpenReader(srcPath)
	if err != nil {
		return fmt.Errorf("open source docx: %w", err)
	}
	defer src.Close()

	if err := util.EnsureDir(filepath.Dir(outputPath)); err != nil {
		return err
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	for _, f := range src.File {
		hdr := &zip.FileHeader{
			Name:     f.Name,
			Method:   f.Method,
			Modified: f.Modified,
		}
		hdr.SetMode(f.Mode())

		writer, err := w.CreateHeader(hdr)
		if err != nil {
			return fmt.Errorf("write header %s: %w", f.Name, err)
		}

		if f.Name == "word/document.xml" {
			if _, err := writer.Write(prefix); err != nil {
				return err
			}
			for _, el := range elements {
				if _, err := writer.Write(el.data); err != nil {
					return err
				}
			}
			if _, err := writer.Write(suffix); err != nil {
				return err
			}
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open source entry %s: %w", f.Name, err)
		}
		if _, err := io.Copy(writer, rc); err != nil {
			rc.Close()
			return fmt.Errorf("copy entry %s: %w", f.Name, err)
		}
		rc.Close()
	}
	return nil
}

// estimateDOCXPartSizeMB writes a temporary DOCX containing prefix +
// selected elements + suffix, returns its size in MB, then deletes the
// temp file.
func estimateDOCXPartSizeMB(srcPath string, prefix []byte, elements []docxBodyElement, suffix []byte) (float64, error) {
	tmp, err := os.CreateTemp("", "split-estimate-*.docx")
	if err != nil {
		return 0, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := writeDOCXPart(tmpPath, srcPath, prefix, elements, suffix); err != nil {
		return 0, fmt.Errorf("write temp docx: %w", err)
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("stat temp: %w", err)
	}
	return float64(info.Size()) / (1024 * 1024), nil
}

// countParagraphs returns the number of <w:p> elements in elements.
// Section properties and tables are not paragraphs.
func countParagraphs(elements []docxBodyElement) int {
	n := 0
	for _, e := range elements {
		if e.isParagraph {
			n++
		}
	}
	return n
}

// estimatedPages converts a paragraph count into an estimated page
// count using paragraphsPerPage.
func estimatedPages(paragraphs int) int {
	pages := paragraphs / paragraphsPerPage
	if paragraphs%paragraphsPerPage != 0 {
		pages++
	}
	return pages
}

// splitBySizeAndPagesDOCX finds the largest end index such that the
// elements elements[start:end] satisfy:
//   - estimated pages <= maxPages
//   - estimated file size <= maxSizeMB
//
// elementsPerPage is the ratio used to convert element count to pages.
// It is either the constant paragraphsPerPage (fallback) or computed
// from real page metadata (totalElements / realPages).
func splitBySizeAndPagesDOCX(srcPath string, start, totalElements, maxPages int, maxSizeMB float64, prefix []byte, allElements []docxBodyElement, suffix []byte, elementsPerPage int) (int, error) {
	// Page-count cap, expressed as an element index.
	maxEl := start + maxPages*elementsPerPage
	if maxEl > totalElements {
		maxEl = totalElements
	}

	if maxSizeMB <= 0 {
		return maxEl, nil
	}

	// Single-element edge.
	singleMB, err := estimateDOCXPartSizeMB(srcPath, prefix, allElements[start:start+1], suffix)
	if err != nil {
		return 0, err
	}
	if singleMB > maxSizeMB {
		fmt.Printf("  [警告] 第 %d 个元素单独大小 %.1fMB 已超过限制 %.0fMB，仍将单独输出\n",
			start+1, singleMB, maxSizeMB)
		return start + 1, nil
	}

	// If the full element slice fits, take it.
	fullMB, err := estimateDOCXPartSizeMB(srcPath, prefix, allElements[start:maxEl], suffix)
	if err != nil {
		return 0, err
	}
	if fullMB <= maxSizeMB {
		return maxEl, nil
	}

	// Binary search for the largest end ∈ [start+1, maxEl] that fits.
	lo, hi := start+1, maxEl
	best := start + 1
	for lo <= hi {
		mid := (lo + hi) / 2
		sizeMB, err := estimateDOCXPartSizeMB(srcPath, prefix, allElements[start:mid], suffix)
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

// isAlreadySplitDOCX reports whether existingParts together cover the
// same total paragraph count as the source. It opens each part, reads
// its body, and counts <w:p> elements; any read error returns false.
func isAlreadySplitDOCX(existingParts []string, totalParagraphs int) bool {
	sum := 0
	for _, p := range existingParts {
		_, elems, _, err := readDOCXBody(p)
		if err != nil {
			return false
		}
		sum += countParagraphs(elems)
	}
	return sum == totalParagraphs
}

// SplitDOCX splits docxPath into "{base}_partN.docx" files in
// outputDir, honouring both maxPages and maxSizeMB (0 = no size limit).
//
// If existing "{base}_part*.docx" files already account for the source's
// total paragraph count, the split is skipped (unless force=true).
func SplitDOCX(docxPath string, maxPages int, maxSizeMB float64, outputDir string, force bool) error {
	if !util.FileExists(docxPath) {
		return fmt.Errorf("文件不存在: %s", docxPath)
	}
	if err := util.EnsureDir(outputDir); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// Read page count from Word metadata.
	realPages := readDOCXPageCount(docxPath)
	displayName := filepath.Base(docxPath)

	if realPages > 0 {
		fmt.Printf("[信息] %s: %d 页 (来自 Word 元数据)\n", displayName, realPages)
	} else {
		fmt.Printf("[信息] %s: 无法读取页数元数据\n", displayName)
	}

	// If the file is comfortably under the limit, no conversion needed.
	// MinerU's rendered page count can be ~1.6x higher than Word's
	// metadata (e.g. Word 137 → MinerU 221). Use 50% as safety margin.
	safeLimit := maxPages / 2
	if realPages > 0 && realPages <= safeLimit {
		return nil
	}

	// Convert DOCX to PDF for MinerU processing. This handles:
	// 1. Page count exceeds limit — PDF splitter will split it.
	// 2. Page count close to limit — might exceed after rendering.
	// 3. Page count unknown — convert to PDF to get accurate count.
	fmt.Printf("  [信息] 转为 PDF 后分割...\n")
	pdfPath, err := convertDocxToPDF(docxPath, outputDir)
	if err != nil {
		return fmt.Errorf("DOCX 转 PDF 失败: %w", err)
	}
	fmt.Printf("  [完成] 已转为 PDF: %s\n", filepath.Base(pdfPath))
	// Split the converted PDF using the existing PDF splitter.
	if err := SplitPDF(pdfPath, maxPages, maxSizeMB, outputDir, force); err != nil {
		return err
	}
	// Remove the intermediate PDF (only the _part*.pdf files are needed).
	os.Remove(pdfPath)
	return nil
}
