// Package docindex 是 MinerU content_list 的只读检索索引（旧线 docindex.go 逐字移植）。
// 文档检索是转换会话唯一的"以书找页"通道——此包是 kernel 侧确定性资产，
// LLM 会话只读它。Session/渲染部分不进本包（新架构：agent 直接读 JSON 产物）。
package docindex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DocEntry is one searchable block (an OCR text block, or a figure/table block).
// A figure that was recut by the images phase carries our own marker as content:
// styled-text's printed text, a raster's explanation, or a vector's TikZ code.
type DocEntry struct {
	Seq     int       `json:"seq"`     // 1-based index in Entries (fast citation)
	Part    string    `json:"part"`    // MinerU part dir ("foo_part1")
	Page    int       `json:"page"`    // 0-based page inside the part
	Global  int       `json:"global"`  // 1-based page across the WHOLE split
	Type    string    `json:"type"`    // MinerU block type ("text"/"image"/"table"/"equation"/"interline_equation"/"chart"/"footnote"/...)
	Text    string    `json:"text"`    // text snippet (joined captions, or the marker content)
	Img     string    `json:"img"`     // image path relative to the part dir ("images/<sha>.jpg")
	BBox    [4]float64 `json:"bbox"`   // [x0,y0,x1,y1], MinerU coords
	Marker  string    `json:"marker"`  // our marker kind ("" / styled-text / vector / image)
	Label   string    `json:"label"`   // marker label (描述文本 / 图标签)
	Content string    `json:"content"` // marker CONTENT/DESCRIBE/矢量代码原文（无厂商图注时的兜底文本在 Text）
}

// DocIndex is the read-only compiled index.
type DocIndex struct {
	Parts   []string            `json:"parts"`
	Sizes   map[string][2]float64 `json:"sizes"` // part dir -> page size in pt (from layout.json)
	Entries []DocEntry          `json:"entries"`
	total   int
}

// Total reports the highest global page number (0 when empty).
func (idx *DocIndex) Total() int {
	if idx == nil || len(idx.Entries) == 0 {
		return 0
	}
	max := 0
	for _, e := range idx.Entries {
		if e.Global > max {
			max = e.Global
		}
	}
	return max
}

// PageSrc is one origin PDF's slice of the global page numbering.
type PageSrc struct {
	PDF   string `json:"pdf"`   // path to the origin PDF (list_source_pages renders pages from it)
	Part  string `json:"part"`  // MinerU part dir
	First int    `json:"first"` // 1-based first global page of this PDF
	Count int    `json:"count"` // page count of this PDF
}

// PageIndex is the global page table (built alongside the doc index).
type PageIndex struct {
	Srcs  []PageSrc `json:"srcs"`
	Total int       `json:"total"`
}

var partNumRe = regexp.MustCompile(`_part(\d+)$`)

// PageCounter 是精确页数的外部注入（旧线用 pdfcpu；注入 nil 时只用 content_list 的 maxPage）。
var PageCounter func(originPDF string) (int, error)

// OriginPDFsFunc 是 part→origin PDF 映射的外部注入（旧线 findOriginPDFs；注入 nil 时跳过页表）。
var OriginPDFsFunc func(mineruOutput, splitDir, subject, part string) string

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
	if len(pdfs) > 0 && PageCounter != nil {
		if n, err := PageCounter(pdfs[0]); err == nil && n > maxPage {
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
		curLine int       // line of the header or of the last field line
		fields  []string  // CONTENT:/DESCRIBE: lines of the current note
		pendKey string    // LINK seen with no note yet (field order may vary)
		pendLn  int
		// 矢量图的"内容"就是它后面那个 ```latex 代码块：索引里必须带着
		// 它，否则 image 条目永远只有文件名与标签（doc_search 查不到图形
		// 本体，转换会话也无从参考）。vecKey 是正在等代码块的条目。
		vecKey  string
		inFence bool
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
			vecKey, inFence = "", false // 上一条矢量图没等到代码块就结束（或换了图）
			if key != "" {
				out[key] = *cur // CONTENT/DESCRIBE below fills it in
			}
			continue
		}
		if class, path, ok := parseLinkLine(line); ok {
			key := markerKey(path)
			if cur != nil && key != "" && lineNo-curLine <= markerNoteWindow {
				storeNote(out, key, cur, fields)
				if cur.Marker == markerVector {
					vecKey, inFence = key, false // 紧随其后的 ```latex 是这个图的本体
				}
			} else if cur == nil && key != "" && markerLinkClass(class) != "" {
				pendKey, pendLn = key, lineNo
			}
			cur, curKey, fields = nil, "", nil
			continue
		}
		// 矢量代码块：只吃紧跟 LINK 的那一段（中间空行/围栏之外的正文都
		// 算结束），避免把正文里的别的代码块粘到图上。
		if vecKey != "" {
			if !inFence {
				if strings.HasPrefix(line, "```latex") {
					inFence = true
					continue
				}
				vecKey = ""
			} else if strings.HasPrefix(line, "```") {
				vecKey, inFence = "", false
				continue
			} else {
				if m, ok := out[vecKey]; ok {
					if m.Content != "" {
						m.Content += "\n"
					}
					m.Content += line
					out[vecKey] = m
				}
				continue
			}
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
		// The window is measured from the LAST line that belonged to the
		// note, not from the header: a styled-text picture can carry a
		// 30-line CONTENT and its LINK must still be attached to it.
		curLine = lineNo
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
// a note that matches no block is simply dropped — never an error. It
// returns how many blocks were filled in (for the build log).
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
		// 厂商没给图注（image_caption/image_footnote 都空）时，text 用**我们
		// 自己**写在这张图上的内容兜底：styled-text 用图里的印刷原文、raster
		// 用我们生成的解释、vector 用图标签（不用 LaTeX 本体——整段代码塞进
		// "图注"位置只会把检索结果淹没）。有厂商图注时保持原样，我们的内容仍
		// 在 content 字段里，检索与输出两边都能看到，来源不会混。
		if strings.TrimSpace(e.Text) == "" {
			e.Text = markerFallbackText(m)
		}
		hits++
	}
	return hits
}

// markerFallbackText is the caption substitute for an image block whose own
// MinerU caption is empty: the text we read out of the picture (styled-text's
// printed text, a raster's generated explanation) or the figure's label.
func markerFallbackText(m mdMarker) string {
	first := m.Label
	if m.Marker != markerVector {
		// STYLED-TEXT / IMAGE：正文（印刷原文 / 我们的解释）比标签有用。
		if c := strings.TrimSpace(m.Content); c != "" {
			return snippet(c, 120)
		}
	}
	if c := strings.TrimSpace(m.Content); c != "" && strings.TrimSpace(first) == "" {
		first = c
	}
	return snippet(strings.TrimSpace(first), 120)
}

// Build compiles the MinerU artifacts for the given source
// markdown files into the read-only index AND the matching global page
// table. markerDir is the directory of the PROCESSED markdown (the images
// phase output) whose DOCVISION notes are joined onto the image blocks;
// "" disables the join. Returns (index, pageTable, error).
func Build(mineruOutput, splitDir string, sourceMDs []string, markerDir, outPath string) (*DocIndex, *PageIndex, error) {
	idx := &DocIndex{Sizes: map[string][2]float64{}}
	pages := &PageIndex{}
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
			// Align with the origin-PDF mapping: append the part's origin PDF to
			// the global page table at the current offset.
			pdf := ""
			if OriginPDFsFunc != nil {
				pdf = OriginPDFsFunc(mineruOutput, splitDir, subject, dir)
			}
			if pdf != "" && count > 0 {
				pages.Srcs = append(pages.Srcs, PageSrc{PDF: pdf, Part: dir, First: pages.Total + 1, Count: count})
				pages.Total += count
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
	// 回填 DOCVISION 注释（图片条目唯一的"内容"来源）：按图片文件名
	// 匹配，匹配不上就留空，绝不报错。
	idx.applyMarkers(markers)
	for i := range idx.Entries {
		idx.Entries[i].Seq = i + 1
	}
	if outPath != "" {
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
	}
	return idx, pages, nil
}

// Load reads a previously built index.
func Load(path string) (*DocIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx DocIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", path, err)
	}
	idx.total = idx.Total()
	return &idx, nil
}

// ------------------------------------------------------------------
// Lookup helpers
// ------------------------------------------------------------------

// EntryOnPage returns the entries on the given GLOBAL page (1-based).
// One entry per visual block: a composite block (an image plus its caption)
// is TWO entries (image + text), so the caller that wants "what's ON this
// page" dedupes by image file name.
func (idx *DocIndex) EntryOnPage(global int) []DocEntry {
	if global <= 0 {
		return nil
	}
	var out []DocEntry
	for _, e := range idx.Entries {
		if e.Global == global {
			out = append(out, e)
		}
	}
	return out
}

// CropHint returns the bbox hint for the entry whose text matches
// `textRegex` on the given GLOBAL page (1-based). It returns the entry's
// own bbox by default; when the entry is a text block and the regex names
// a specific figure/table ("见图 3-2"), it returns that figure's bbox —
// exactly the region `view_pdf` should crop to. false = no hint.
func (idx *DocIndex) CropHint(global int, textRegex string) ([4]float64, bool) {
	var hint [4]float64
	re, err := regexp.Compile(textRegex)
	if err != nil || global <= 0 {
		return hint, false
	}
	on := idx.EntryOnPage(global)
	var target *DocEntry
	for i := range on {
		if on[i].Type == "text" && re.MatchString(on[i].Text) {
			target = &on[i]
			break
		}
	}
	if target == nil {
		return hint, false
	}
	hint = target.BBox
	var figures []DocEntry
	for _, e := range on {
		if e.Img == "" || e.BBox == ([4]float64{}) {
			continue
		}
		switch e.Type {
		case "image", "table", "chart":
			figures = append(figures, e)
		}
	}
	for _, f := range figures {
		if target.Label != "" && f.Label == target.Label {
			return f.BBox, true
		}
	}
	// 未命名图表（无 caption/label）：页内只有一张图时它就是 regex 所指
	// （"见图 3-2" 这类指称在扫描书里 99% 指向本页唯一图块）。
	if target.Label == "" && len(figures) == 1 {
		return figures[0].BBox, true
	}
	return hint, true
}

// Search returns the N best-matching entries for a query. A query is
// whitespace-split into terms; every term must match (AND), where one
// term may be "text" / "content" / "marker" / "label" (field match) or a
// plain substring / regex fragment (anywhere). Scores favour entries
// whose text STARTS WITH the query, then entries that CONTAIN all terms,
// then by query coverage (fraction of query bytes found). Ties keep
// doc order, so the first hit is the earliest occurrence.
func (idx *DocIndex) Search(query string, max int) []DocEntry {
	if idx == nil || strings.TrimSpace(query) == "" || max <= 0 {
		return nil
	}
	terms := strings.Fields(query)
	type hit struct {
		entry   DocEntry
		score   float64
		matched bool
	}
	var hits []hit
	var reQuery string
	var re *regexp.Regexp
	for _, t := range terms {
		if !strings.ContainsAny(t, `[.*+?()|`) {
			continue
		}
		if reQuery != "" {
			reQuery += "|"
		}
		reQuery += t
	}
	if reQuery != "" {
		re, _ = regexp.Compile(reQuery)
	}
	reMatch := func(re *regexp.Regexp, s string) bool {
		return re != nil && re.MatchString(s)
	}
	for _, e := range idx.Entries {
		var score float64
		matched := true
		for _, t := range terms {
			k, v, ok := strings.Cut(t, ":")
			if ok && (k == "text" || k == "content" || k == "marker" || k == "label") {
				var s string
				switch k {
				case "text":
					s = e.Text
				case "content":
					s = e.Content
				case "marker":
					s = e.Marker
				case "label":
					s = e.Label
				}
				if !strings.Contains(s, v) {
					matched = false
					break
				}
				continue
			}
			if strings.Contains(e.Text, t) || strings.Contains(e.Content, t) ||
				strings.Contains(e.Marker, t) || strings.Contains(e.Label, t) || reMatch(re, e.Text) {
				continue
			}
			matched = false
			break
		}
		if !matched {
			continue
		}
		lowerText := strings.ToLower(e.Text)
		full := strings.ToLower(query)
		if strings.HasPrefix(lowerText, full) {
			score = 2.0
		} else if strings.Contains(lowerText, full) {
			score = 1.5
		} else {
			cov := 0
			for _, t := range terms {
				if k, v, ok := strings.Cut(t, ":"); ok && (k == "text" || k == "content" || k == "marker" || k == "label") {
					if strings.Contains(strings.ToLower(e.Text), strings.ToLower(v)) {
						cov += len(v)
					}
					continue
				}
				if strings.Contains(lowerText, strings.ToLower(t)) {
					cov += len(t)
				}
			}
			score = float64(cov) / float64(len(full))
		}
		hits = append(hits, hit{entry: e, score: score, matched: true})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].entry.Seq < hits[j].entry.Seq
	})
	if len(hits) > max {
		hits = hits[:max]
	}
	out := make([]DocEntry, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.entry)
	}
	return out
}
