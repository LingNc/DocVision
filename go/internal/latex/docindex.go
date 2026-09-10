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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// DocEntry is one indexed original-document block. Global is the
// 1-based page number across the whole subject (what view_pdf/source takes).
//
// Marker/Label/Content come from the DOCVISION notes the images phase
// writes into the processed markdown (see parseMdMarkers): MinerU gives an
// image block no text at all when it has no caption, so without them the
// index only knows a file name for that picture. They are omitempty, so an
// index written by an older version still loads (fields stay empty).
type DocEntry struct {
	Seq    int        `json:"seq"`
	Part   string     `json:"part"`
	Page   int        `json:"page"`
	Global int        `json:"global"`
	Type   string     `json:"type"`
	BBox   [4]float64 `json:"bbox"`
	Text   string     `json:"text"`
	Img    string     `json:"img,omitempty"`
	// Marker is the DOCVISION note kind: styled-text / vector / image.
	Marker string `json:"marker,omitempty"`
	// Label is the note's description (style note, figure label, …).
	Label string `json:"label,omitempty"`
	// Content is the note body: STYLED-TEXT's printed text (CONTENT) or
	// RASTER's explanation (DESCRIBE).
	Content string `json:"content,omitempty"`
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

// ------------------------------------------------------------------
// DOCVISION notes in the processed markdown
// ------------------------------------------------------------------
//
// The images phase (level 1) rewrites every picture of the source
// markdown into a machine note carrying what the OCR content_list does
// NOT have for a picture without caption:
//
//	<!-- DOCVISION-STYLED-TEXT: 知识导图 -->
//	CONTENT: <the text printed in the picture>
//	LINK: [styled-text](images/<主题>/<sha>.jpg)
//
//	<!-- DOCVISION-VECTOR: mind-map diagram -->
//	LINK: [vector](images/<主题>/<sha>.jpg)
//	```latex …```
//
//	<!-- DOCVISION-IMAGE: <label> -->
//	DESCRIBE: <explanation>
//	LINK: [image](images/<主题>/<sha>.jpg)
//
// buildDocIndex therefore reads them back and joins them onto the image
// blocks by FILE NAME (the index stores "images/<sha>.jpg", the note
// "images/<主题>/<sha>.jpg").

// mdMarker is what one DOCVISION note says about one image file.
type mdMarker struct {
	Marker  string // styled-text / vector / image
	Label   string // 描述文本（样式说明 / 图标签）
	Content string // STYLED-TEXT 的 CONTENT / RASTER 的 DESCRIBE 原文
}

// marker kind names stored in DocEntry.Marker.
const (
	markerStyledText = "styled-text"
	markerVector     = "vector"
	markerImage      = "image"
)

// markerNoteWindow bounds how far a field line (CONTENT/DESCRIBE) or the
// LINK may sit from its note header. Notes are written as one compact
// block, so a distant LINK belongs to something else and must not be
// attached to a stale header.
const markerNoteWindow = 20

// markerType maps a DOCVISION-<TYPE> comment name to the short marker.
func markerType(name string) string {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "STYLED-TEXT":
		return markerStyledText
	case "VECTOR":
		return markerVector
	case "IMAGE":
		return markerImage
	}
	return ""
}

// markerLinkClass maps a [class](path) link class to the marker it
// implies; only note-carrying classes are recognised (a plain image link
// without a note adds nothing to the index).
func markerLinkClass(class string) string {
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "styled-text":
		return markerStyledText
	case "vector":
		return markerVector
	case "image":
		return markerImage
	}
	return ""
}

// markerKey is the join key between a note and a DocEntry: the image FILE
// name (the index keeps "images/<sha>.jpg", the note
// "images/<主题>/<sha>.jpg").
func markerKey(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "<>\"'")
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return filepath.Base(p)
}

// parseNoteHeader reads "<!-- DOCVISION-<TYPE>: <label> -->" from one
// already-trimmed line. A missing closing " -->" is tolerated: older
// output closed the comment on a later line.
func parseNoteHeader(line string) (marker, label string, ok bool) {
	if !strings.HasPrefix(line, "<!--") {
		return "", "", false
	}
	body := strings.TrimSpace(strings.TrimPrefix(line, "<!--"))
	body = strings.TrimSpace(strings.TrimSuffix(body, "-->"))
	if !strings.HasPrefix(body, "DOCVISION-") {
		return "", "", false
	}
	name, label, found := strings.Cut(strings.TrimPrefix(body, "DOCVISION-"), ":")
	if !found {
		return "", "", false
	}
	m := markerType(name)
	if m == "" {
		return "", "", false
	}
	return m, strings.TrimSpace(label), true
}

// parseLinkLine reads "LINK: [class](path)" from one trimmed line.
func parseLinkLine(line string) (class, path string, ok bool) {
	if !strings.HasPrefix(line, "LINK:") {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "LINK:"))
	open := strings.Index(rest, "[")
	if open < 0 {
		return "", "", false
	}
	rest = rest[open:]
	mid := strings.Index(rest, "](")
	if mid < 0 {
		return "", "", false
	}
	class = rest[1:mid]
	rest = rest[mid+2:]
	end := strings.Index(rest, ")")
	if end < 0 {
		return "", "", false
	}
	return strings.TrimSpace(class), strings.TrimSpace(rest[:end]), true
}

// parseMdMarkers scans the given markdown files (names relative to mdDir)
// for the DOCVISION notes above and returns them keyed by image file name.
// Files that do not exist are skipped; an empty mdNames returns an empty
// map, so a caller without a notes directory simply gets no backfill.
func parseMdMarkers(mdDir string, mdNames []string) map[string]mdMarker {
	out := map[string]mdMarker{}
	if mdDir == "" || len(mdNames) == 0 {
		return out
	}
	for _, name := range mdNames {
		base := filepath.Base(strings.TrimSpace(name))
		if base == "" || base == "." || base == string(filepath.Separator) {
			continue
		}
		f, err := os.Open(filepath.Join(mdDir, base))
		if err != nil {
			continue // missing/unreadable md: no backfill, never fatal
		}
		scanMdMarkers(f, out)
		_ = f.Close()
	}
	return out
}

// scanMdMarkers is the line scanner behind parseMdMarkers: one pass, no
// regex, so a huge or malformed file can neither backtrack nor allocate
// per match. Tolerant by construction — notes may be missing, field lines
// may be reordered or absent, CONTENT/DESCRIBE may span lines, files may
// be CRLF.
func scanMdMarkers(r io.Reader, out map[string]mdMarker) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // CONTENT lines can be long
	var (
		cur     *mdMarker // note header seen, waiting for its LINK
		curKey  string    // …already joined (LINK was written above the header)
		curLine int
		fields  []string // CONTENT:/DESCRIBE: lines of the current note
		pendKey string   // LINK seen with no note yet (field order may vary)
		pendLn  int
	)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" {
			continue
		}
		if m, label, ok := parseNoteHeader(line); ok {
			key := ""
			if pendKey != "" && lineNo-pendLn <= markerNoteWindow {
				key = pendKey
			}
			cur = &mdMarker{Marker: m, Label: label}
			curKey, curLine, fields, pendKey = key, lineNo, nil, ""
			if key != "" {
				out[key] = *cur // CONTENT/DESCRIBE below fills it in
			}
			continue
		}
		if class, path, ok := parseLinkLine(line); ok {
			key := markerKey(path)
			if cur != nil && key != "" && lineNo-curLine <= markerNoteWindow {
				storeNote(out, key, cur, fields)
			} else if cur == nil && key != "" && markerLinkClass(class) != "" {
				pendKey, pendLn = key, lineNo
			}
			cur, curKey, fields = nil, "", nil
			continue
		}
		if cur == nil {
			continue
		}
		if lineNo-curLine > markerNoteWindow || strings.HasPrefix(line, "```") {
			cur, curKey, fields = nil, "", nil
			continue
		}
		switch {
		case strings.HasPrefix(line, "CONTENT:"):
			fields = append(fields, strings.TrimSpace(strings.TrimPrefix(line, "CONTENT:")))
		case strings.HasPrefix(line, "DESCRIBE:"):
			fields = append(fields, strings.TrimSpace(strings.TrimPrefix(line, "DESCRIBE:")))
		case len(fields) > 0:
			fields = append(fields, line) // continuation of the field above
		default:
			continue
		}
		if curKey != "" {
			storeNote(out, curKey, cur, fields)
		}
	}
}

// storeNote writes the current note (header + collected fields) under key.
func storeNote(out map[string]mdMarker, key string, cur *mdMarker, fields []string) {
	if key == "" || cur == nil {
		return
	}
	m := *cur
	m.Content = strings.TrimSpace(strings.Join(fields, "\n"))
	out[key] = m
}

// applyMarkers joins parsed notes onto the image blocks by file name. A
// block with no note (the common case) keeps the three fields empty, and
// a note that matches no block is simply dropped — never an error.
func (idx *DocIndex) applyMarkers(markers map[string]mdMarker) int {
	if len(markers) == 0 {
		return 0
	}
	hits := 0
	for i := range idx.Entries {
		e := &idx.Entries[i]
		if e.Img == "" {
			continue
		}
		m, ok := markers[markerKey(e.Img)]
		if !ok {
			continue
		}
		e.Marker, e.Label, e.Content = m.Marker, m.Label, m.Content
		hits++
	}
	return hits
}

// buildDocIndex compiles the MinerU artifacts for the given source
// markdown files into the read-only index AND the matching global page
// table (reported by list_source_pages). markerDir is the directory of
// the PROCESSED markdown (the images phase output) whose DOCVISION notes
// are joined onto the image blocks; "" disables the join.
func buildDocIndex(mineruOutput string, sourceMDs []string, markerDir, outPath string) (*DocIndex, *pageIndex, error) {
	idx := &DocIndex{Sizes: map[string][2]float64{}}
	pages := &pageIndex{}
	markers := parseMdMarkers(markerDir, sourceMDs)
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

// EntryOnPage returns the first indexed block on a global page (nil when
// the index has nothing there): used to map a global page to its MinerU
// part, which gives the drift-free (part, local page) location.
func (d *DocIndex) EntryOnPage(global int) *DocEntry {
	if d == nil {
		return nil
	}
	for i := range d.Entries {
		if d.Entries[i].Global == global {
			return &d.Entries[i]
		}
	}
	return nil
}

// layoutScale converts MinerU content_list bbox coordinates into PDF
// points. MinerU scores the page at 2x its point size for this pipeline
// (a 493x720pt page has content_list coordinates up to ~986x1440), so the
// bbox is NOT in points: dividing it by the index's page_size directly
// overflows past 100%. The crop hint below is derived with this factor and
// is meant as a starting point for view_pdf.
const layoutScale = 2.0

// CropHint converts a block bbox into view_pdf crop percentages
// (left/top/right/bottom), clamped to the page.
func CropHint(bbox [4]float64, pageSize [2]float64) (int, int, int, int) {
	if pageSize[0] <= 0 || pageSize[1] <= 0 {
		return 0, 0, 100, 100
	}
	pct := func(v, total float64) int {
		p := int(v / (total * layoutScale) * 100)
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		return p
	}
	return pct(bbox[0], pageSize[0]), pct(bbox[1], pageSize[1]), pct(bbox[2], pageSize[0]), pct(bbox[3], pageSize[1])
}

// FormatEntry renders one entry for tool output (pN = global page for
// view_pdf on the source mount / list_source_pages; the local page is
// 1-based, exactly what list_source_pages {page:N} and view_pdf print).
func FormatEntry(e DocEntry) string {
	s := fmt.Sprintf("#%d p%d (%s local p%d) %s bbox=[%.0f,%.0f,%.0f,%.0f]", e.Seq, e.Global, e.Part, e.Page+1, e.Type, e.BBox[0], e.BBox[1], e.BBox[2], e.BBox[3])
	if e.Img != "" {
		s += " img=" + e.Img
	}
	if e.Text != "" {
		s += "\n    " + e.Text
	}
	return s
}
