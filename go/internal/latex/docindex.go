package latex

// The MinerU intermediate output (mineru_output/<subject>[_partN]/)
// carries *_content_list.json: a flat list of every text / image /
// table / equation block with its local page number, bounding box and
// a text snippet or caption. buildDocIndex compiles those raw
// artifacts into a compact, read-only index so AI sessions can map a
// markdown fragment back to the ORIGINAL PDF page (doc_search) and
// then inspect it with view_pdf through the read-only "source" mount
// (global 1-based page numbers over the same origin PDFs).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// DocEntry is one indexed original-document block. Global is the
// 1-based page number across the whole subject (what view_pdf/source takes).
type DocEntry struct {
	Seq    int        `json:"seq"`
	Part   string     `json:"part"`
	Page   int        `json:"page"`
	Global int        `json:"global"`
	Type   string     `json:"type"`
	BBox   [4]float64 `json:"bbox"`
	Text   string     `json:"text"`
	Img    string     `json:"img,omitempty"`
}

// DocIndex is the searchable, read-only view of the original document.
type DocIndex struct {
	Parts   []string              `json:"parts"`
	Sizes   map[string][2]float64 `json:"page_sizes"`
	Entries []DocEntry            `json:"entries"`
	total   int
}

// docPartDirs returns the MinerU part directories that contributed to
// a source markdown ("X.md" -> X/ then X_part1, X_part2, ...), in the
// SAME order findOriginPDFs uses so global page numbers align.
func docPartDirs(mineruOutput, mdName string) []string {
	base := strings.TrimSuffix(mdName, filepath.Ext(mdName))
	entries, err := os.ReadDir(mineruOutput)
	if err != nil {
		return nil
	}
	type part struct {
		num int
		dir string
	}
	var parts []part
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == base {
			parts = append(parts, part{num: 0, dir: name})
			continue
		}
		if !strings.HasPrefix(name, base+"_") {
			continue
		}
		if m := partNumRe.FindStringSubmatch(name); m != nil {
			n, _ := strconv.Atoi(m[1])
			parts = append(parts, part{num: n, dir: name})
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].num < parts[j].num })
	dirs := make([]string, 0, len(parts))
	for _, p := range parts {
		dirs = append(dirs, p.dir)
	}
	return dirs
}

// snippet shortens s to n runes on one line.
func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// joinAny flattens a JSON string array (nil-safe).
func joinAny(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " / ")
}

// contentListPath picks the part's content list (skips the heavier v2).
func contentListPath(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_content_list.json"))
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), "_v2") {
			return m
		}
	}
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// buildDocIndexPart reads one part directory into entries + page count.
func buildDocIndexPart(dir string) (entries []DocEntry, pages int, size [2]float64, err error) {
	cl := contentListPath(dir)
	if cl == "" {
		return nil, 0, size, fmt.Errorf("无 content_list")
	}
	raw, err := os.ReadFile(cl)
	if err != nil {
		return nil, 0, size, err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, 0, size, fmt.Errorf("解析 %s: %w", filepath.Base(cl), err)
	}
	part := filepath.Base(dir)
	maxPage := 0
	for _, it := range items {
		typ, _ := it["type"].(string)
		page := 0
		if v, ok := it["page_idx"].(float64); ok {
			page = int(v)
		}
		if page+1 > maxPage {
			maxPage = page + 1
		}
		var bbox [4]float64
		if arr, ok := it["bbox"].([]any); ok && len(arr) == 4 {
			for i := 0; i < 4; i++ {
				if v, ok := arr[i].(float64); ok {
					bbox[i] = v
				}
			}
		}
		text, img := "", ""
		switch typ {
		case "image":
			img, _ = it["img_path"].(string)
			text = joinAny(it["image_caption"])
			if fn := joinAny(it["image_footnote"]); fn != "" {
				text += " | 脚注: " + fn
			}
		case "table", "chart":
			img, _ = it["img_path"].(string)
			text = joinAny(it["table_caption"])
			if tb, ok := it["table_body"].(string); ok && text == "" {
				text = snippet(tb, 120)
			}
		default:
			if t, ok := it["text"].(string); ok {
				text = t
			}
		}
		entries = append(entries, DocEntry{
			Part: part, Page: page, Type: typ, BBox: bbox,
			Text: snippet(text, 120), Img: img,
		})
	}
	// Exact page count from the origin PDF (last page may have no blocks).
	pdfs, _ := filepath.Glob(filepath.Join(dir, "*_origin.pdf"))
	if len(pdfs) > 0 {
		if n, err := api.PageCountFile(pdfs[0]); err == nil && n > maxPage {
			maxPage = n
		}
	}
	if lb, err := os.ReadFile(filepath.Join(dir, "layout.json")); err == nil {
		var layout struct {
			PDFInfo []struct {
				PageSize [2]float64 `json:"page_size"`
			} `json:"pdf_info"`
		}
		if json.Unmarshal(lb, &layout) == nil && len(layout.PDFInfo) > 0 {
			size = layout.PDFInfo[0].PageSize
		}
	}
	return entries, maxPage, size, nil
}

// buildDocIndex compiles the MinerU artifacts for the given source
// markdown files into the read-only index AND the matching global page
// table (reported by list_source_pages).
func buildDocIndex(mineruOutput string, sourceMDs []string, outPath string) (*DocIndex, *pageIndex, error) {
	idx := &DocIndex{Sizes: map[string][2]float64{}}
	pages := &pageIndex{}
	seen := map[string]bool{}
	for _, md := range sourceMDs {
		subject := strings.TrimSuffix(filepath.Base(md), filepath.Ext(filepath.Base(md)))
		for _, dir := range docPartDirs(mineruOutput, filepath.Base(md)) {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			entries, count, size, err := buildDocIndexPart(filepath.Join(mineruOutput, dir))
			if err != nil {
				continue
			}
			// Align with findOriginPDFs: append the part's origin PDF to
			// the global page table at the current offset.
			pdfs := findOriginPDFs(mineruOutput, subject)
			var pdf string
			for _, p := range pdfs {
				if filepath.Base(filepath.Dir(p)) == dir {
					pdf = p
					break
				}
			}
			if pdf != "" && count > 0 {
				pages.srcs = append(pages.srcs, pageSrc{pdf: pdf, first: pages.total + 1, count: count})
				pages.total += count
			}
			offset := idx.total
			for i := range entries {
				entries[i].Global = offset + entries[i].Page + 1
			}
			idx.Parts = append(idx.Parts, dir)
			idx.Sizes[dir] = size
			idx.Entries = append(idx.Entries, entries...)
			idx.total += count
		}
	}
	if len(idx.Entries) == 0 {
		return nil, nil, fmt.Errorf("mineru_output 中未找到可索引的 content_list")
	}
	for i := range idx.Entries {
		idx.Entries[i].Seq = i + 1
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, nil, err
	}
	data, err := json.MarshalIndent(idx, "", " ")
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return nil, nil, err
	}
	return idx, pages, nil
}

// loadDocIndex reads a previously built index (page table is rebuilt
// from the same origin PDFs by the caller when needed).
func loadDocIndex(path string) (*DocIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	idx := &DocIndex{}
	if err := json.Unmarshal(data, idx); err != nil {
		return nil, err
	}
	if idx.Sizes == nil {
		idx.Sizes = map[string][2]float64{}
	}
	return idx, nil
}

// Search returns entries containing ALL whitespace-separated terms
// (case-insensitive) across text snippet, image path and type. A bare
// integer term also matches the global page number.
func (idx *DocIndex) Search(query string, max int) []DocEntry {
	max = clampInt(max, 1, 50)
	fields := strings.Fields(strings.ToLower(query))
	if len(fields) == 0 {
		return nil
	}
	var out []DocEntry
	for _, e := range idx.Entries {
		hay := strings.ToLower(e.Text + " " + e.Img + " " + e.Type + " p" + strconv.Itoa(e.Global))
		ok := true
		for _, f := range fields {
			if !strings.Contains(hay, f) && strconv.Itoa(e.Global) != f {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
			if len(out) >= max {
				break
			}
		}
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// FormatEntry renders one entry for tool output (pN = global page for
// view_pdf on the source mount / list_source_pages).
func FormatEntry(e DocEntry) string {
	s := fmt.Sprintf("#%d p%d (%s local p%d) %s bbox=[%.0f,%.0f,%.0f,%.0f]", e.Seq, e.Global, e.Part, e.Page, e.Type, e.BBox[0], e.BBox[1], e.BBox[2], e.BBox[3])
	if e.Img != "" {
		s += " img=" + e.Img
	}
	if e.Text != "" {
		s += "\n    " + e.Text
	}
	return s
}
