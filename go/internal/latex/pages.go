package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mineru-tools/internal/session"
	"mineru-tools/pkg/util"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// partNumRe extracts the trailing part number from
// "<subject>_part12" style directory names.
var partNumRe = regexp.MustCompile(`_part(\d+)$`)

// pageSrc maps a run of global page numbers onto one origin PDF.
type pageSrc struct {
	pdf   string
	first int // global first page (1-based)
	count int
}

// pageIndex is the global page table over all origin PDFs. Pages are
// rendered through view_pdf on the "source" mount (no cache; pdftoppm per page).
type pageIndex struct {
	srcs  []pageSrc
	total int
}

// buildPageIndex locates the MinerU-preserved origin PDFs for a
// subject (mineru_output/<subject>_part*/*_origin.pdf, in part order)
// and counts their pages to build the global page table.
func buildPageIndex(mineruDir, subject string) (*pageIndex, error) {
	pdfs := findOriginPDFs(mineruDir, subject)
	if len(pdfs) == 0 {
		return nil, fmt.Errorf("no origin pdfs for %s", subject)
	}
	idx := &pageIndex{}
	for _, pdf := range pdfs {
		n, err := api.PageCountFile(pdf)
		if err != nil {
			return nil, fmt.Errorf("count pages of %s: %w", pdf, err)
		}
		idx.srcs = append(idx.srcs, pageSrc{pdf: pdf, first: idx.total + 1, count: n})
		idx.total += n
	}
	return idx, nil
}

// locate resolves a global page number to (pdf, local page).
func (idx *pageIndex) locate(page int) (string, int, error) {
	if page < 1 || page > idx.total {
		return "", 0, fmt.Errorf("page %d out of range 1..%d", page, idx.total)
	}
	for _, src := range idx.srcs {
		if page < src.first+src.count {
			return src.pdf, page - src.first + 1, nil
		}
	}
	return "", 0, fmt.Errorf("page %d not found", page)
}

// findOriginPDFs returns the origin PDFs of a subject in part order
// (partless dir first, then part1, part2, ... part10 natural sort).
func findOriginPDFs(mineruDir, subject string) []string {
	entries, err := os.ReadDir(mineruDir)
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
		if name == subject {
			parts = append(parts, part{num: 0, dir: filepath.Join(mineruDir, name)})
			continue
		}
		if !strings.HasPrefix(name, subject+"_") {
			continue
		}
		m := partNumRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		parts = append(parts, part{num: n, dir: filepath.Join(mineruDir, name)})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].num < parts[j].num })

	var pdfs []string
	for _, p := range parts {
		matches, _ := filepath.Glob(filepath.Join(p.dir, "*_origin.pdf"))
		sort.Strings(matches)
		pdfs = append(pdfs, matches...)
	}
	return pdfs
}

// renderSourcePage renders ONE global page of the original book into
// the cache dir (used by the watermark sampler; AI sessions view source
// pages through view_pdf on the "source" mount instead).
func renderSourcePage(comp *Compiler, idx *pageIndex, pagesDir string, page int) (string, error) {
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return "", err
	}
	if err := comp.Available(); err != nil {
		return "", err
	}
	cache := filepath.Join(pagesDir, fmt.Sprintf("p%03d.png", page))
	if util.FileExists(cache) {
		return cache, nil
	}
	pdf, local, err := idx.locate(page)
	if err != nil {
		return "", err
	}
	tmpBase := filepath.Join(pagesDir, fmt.Sprintf("tmp_%d", page))
	if err := comp.RasterizePages(pdf, local, local, tmpBase); err != nil {
		return "", err
	}
	if err := os.Rename(tmpBase+".png", cache); err != nil {
		return "", err
	}
	return cache, nil
}

// ListSourcePagesTool reports the ORIGINAL book pages and how to view
// them. Only the mount-relative CLEAN names are exposed ("book_part1.pdf"),
// never the mineru_output tree: the same tree is mounted at /source for
// the bash tool, so the two path spaces line up.
type ListSourcePagesTool struct {
	// View is the minimal set of original PDFs of this book.
	View *pdfView
	// Mount is the mount name the view is reachable under ("" = not
	// mounted, only the ranges are reported).
	Mount string
	// Index supplies per-page text/image detail and section starts.
	Index *DocIndex
}

func (t *ListSourcePagesTool) Name() string { return "list_source_pages" }

func (t *ListSourcePagesTool) Definition() map[string]any {
	desc := "Index of the ORIGINAL book pages (the real typeset pages): the source PDFs with their page ranges, " +
		"the section starts detected from the OCR layout, and — with page=N — the text snippets and extracted image " +
		"file names of that page (" +
		"a picture without an OCR caption shows its DOCVISION note instead: kind [styled-text]/[vector]/[image], " +
		"label and the text or description read from the picture). " +
		"View any page with view_pdf {path:\"<mount>:<file>\", page:<local page>} " +
		"(crop/zoom behave like every other PDF)."
	if t.Mount != "" {
		desc += " The source PDFs are mounted read-only as \"" + t.Mount + "\" (bash: /" + t.Mount + ")."
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "list_source_pages",
		"description": desc,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page": map[string]any{"type": "integer", "description": "optional GLOBAL page number (1-based): print this page's text snippets and image names"},
			},
		},
	}}
}

func (t *ListSourcePagesTool) Execute(argsJSON string) (session.ToolResult, error) {
	if t.View == nil || t.View.total == 0 {
		return session.ToolResult{Text: "(no original pages available: MinerU kept no *_origin.pdf)"}, nil
	}
	page := 0
	if argsJSON != "" {
		if args, err := parseJSONObject(argsJSON); err == nil {
			page = intArg(args, "page", 0)
		}
	}
	if page > 0 {
		return t.pageDetail(page)
	}
	var b strings.Builder
	if t.Mount != "" {
		fmt.Fprintf(&b, "Source PDFs (read-only mount %q; bash path /%s):\n", t.Mount, t.Mount)
	} else {
		b.WriteString("Source PDFs (not mounted; page ranges only):\n")
	}
	for _, f := range t.View.Files {
		path := f.Name
		if t.Mount != "" {
			path = t.Mount + ":" + f.Name
		}
		fmt.Fprintf(&b, "  %s  pages 1..%d  (global %d..%d)\n", path, f.Count, f.First, f.First+f.Count-1)
	}
	fmt.Fprintf(&b, "TOTAL %d original pages.\n", t.View.total)
	if t.Mount != "" {
		fmt.Fprintf(&b, "View a page: view_pdf {path:\"%s:<file>\", page:<local page>} with optional left/top/right/bottom/zoom.\n", t.Mount)
	}
	if secs := derivedSections(t.Index, 120); len(secs) > 0 {
		b.WriteString("Section starts detected in the OCR layout (global page — heading):\n")
		for _, s := range secs {
			fmt.Fprintf(&b, "  g%d  %s\n", s.Global, s.Title)
		}
	} else {
		b.WriteString("No heading-like lines detected; use doc_search with a phrase from the text to find its page instead.\n")
	}
	b.WriteString("Page detail (text snippets + extracted image names): list_source_pages {page:<GLOBAL>}.")
	return session.ToolResult{Text: b.String()}, nil
}

// pageDetail lists one global page: where it lives and what is on it.
//
// Locating prefers the part recorded in the page index (part + local page
// = exact, immune to page-count drift), and falls back to the cumulative
// global offset when the index has no block on that page.
func (t *ListSourcePagesTool) pageDetail(page int) (session.ToolResult, error) {
	f, local, err := t.View.Locate(page)
	viaPart := false
	if e := t.Index.EntryOnPage(page); e != nil {
		if pf, plocal, perr := t.View.LocatePart(e.Part, e.Page+1); perr == nil {
			f, local, err, viaPart = pf, plocal, nil, true
		}
	}
	if err != nil {
		return session.ToolResult{Text: "OUT OF RANGE: " + err.Error()}, nil
	}
	path := f.Name
	if t.Mount != "" {
		path = t.Mount + ":" + f.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Global page %d -> %s, local page %d.\n", page, path, local)
	if viaPart {
		fmt.Fprintf(&b, "(located through the OCR page index: part %s, page %d)\n", f.Part, local)
	}
	fmt.Fprintf(&b, "Look at it: view_pdf {path:\"%s\", page:%d}\n", path, local)
	if t.Index != nil {
		var texts, imgs []DocEntry
		for _, e := range t.Index.Entries {
			if e.Global != page {
				continue
			}
			if e.Type == "image" || e.Img != "" {
				imgs = append(imgs, e)
				continue
			}
			texts = append(texts, e)
		}
		if len(texts) > 0 {
			b.WriteString("Text on this page:\n")
			for i, e := range texts {
				if i >= 12 {
					fmt.Fprintf(&b, "  ...(%d more blocks)\n", len(texts)-i)
					break
				}
				fmt.Fprintf(&b, "  %s\n", snippet(e.Text, 160))
			}
		}
		if len(imgs) > 0 {
			b.WriteString("Extracted images on this page (view them with view_image). A picture with no OCR caption carries the DOCVISION note of the markdown instead — [styled-text] / [vector] / [image] + label + the text or description read from the picture:\n")
			for _, e := range imgs {
				name := filepath.Base(e.Img)
				if name == "." || name == "" {
					continue
				}
				kind := ""
				if e.Type == "table" {
					kind = "[table] "
				} else if e.Type == "equation" {
					kind = "[formula] "
				}
				if tag := markerTag(e); tag != "" {
					kind += tag + " "
				}
				note := snippet(e.Text, 80)
				if note == "" {
					note = snippet(e.Content, 80)
				}
				if note != "" {
					fmt.Fprintf(&b, "  %s   %s<%s>\n", name, kind, note)
				} else {
					fmt.Fprintf(&b, "  %s   %s\n", name, strings.TrimSpace(kind))
				}
			}
		}
		if len(texts) == 0 && len(imgs) == 0 {
			b.WriteString("(no OCR blocks indexed for this page)\n")
		}
	}
	return session.ToolResult{Text: strings.TrimRight(b.String(), "\n")}, nil
}

// sectionStart is one detected heading.
type sectionStart struct {
	Global int
	Title  string
}

var (
	namedSectionRe = regexp.MustCompile(`^(第[0-9一二三四五六七八九十百零〇]+[章节篇部讲]|Chapter\s+[0-9IVX]+|Part\s+[0-9IVX]+|Appendix\s+[A-Z0-9]?|附录[A-Z0-9]?|前言|序言|引言|绪论|目录|参考文献|参考书目|索引|致谢|后记|总结|结语|词汇表|基础篇|强化篇|综合篇|冲刺篇|真题篇|模拟篇)`)
	// 1.2 / 1.2.3 / 1、/ 1. section numbers (a bare integer is a page
	// number, never a heading).
	numberedSectionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,3}\s*\S|^[0-9]+[、.．]\s*\S`)
	// （一）… / 一、… Chinese outline markers.
	cjkOutlineRe  = regexp.MustCompile(`^[（(][一二三四五六七八九十百]+[）)]\s*\S|^[一二三四五六七八九十百]+[、.．]\s*\S`)
	numericOnlyRe = regexp.MustCompile(`^[0-9\s.、,，/\\-]+$`)
)

// derivedSections scans the OCR blocks for heading-like short lines and
// returns them in page order (deduplicated). This gives a usable table
// of contents even when the book has none. Repetition is the strongest
// signal against a candidate: body fragments ("于是", "所以") recur,
// headings do not.
func derivedSections(idx *DocIndex, max int) []sectionStart {
	if idx == nil {
		return nil
	}
	freq := map[string]int{}
	for _, e := range idx.Entries {
		if e.Img != "" || e.Type == "image" {
			continue
		}
		freq[strings.ToLower(strings.Join(strings.Fields(e.Text), " "))]++
	}
	var out []sectionStart
	seen := map[string]bool{}
	for _, e := range idx.Entries {
		if e.Img != "" || e.Type == "image" {
			continue
		}
		title := strings.Join(strings.Fields(e.Text), " ")
		if !headingLike(title) {
			continue
		}
		key := strings.ToLower(title)
		if seen[key] || freq[key] > 2 {
			continue
		}
		seen[key] = true
		out = append(out, sectionStart{Global: e.Global, Title: snippet(title, 70)})
		if len(out) >= max {
			break
		}
	}
	return out
}

// headingLike reports whether a line looks like a section heading:
// short, no sentence punctuation, and either a named/numbered section
// start or an all-short standalone line.
func headingLike(t string) bool {
	r := []rune(t)
	if len(r) == 0 || len(r) > 40 {
		return false
	}
	if strings.ContainsAny(t, "。；，,;.!?！？") {
		return false
	}
	// Front-matter / metadata lines ("责任编辑：高芳", "邮编/100070",
	// pure page numbers, URLs) are not section starts.
	if strings.ContainsAny(t, "：:【】@") || strings.Contains(t, "http") {
		return false
	}
	if numericOnlyRe.MatchString(t) {
		return false
	}
	return namedSectionRe.MatchString(t) || numberedSectionRe.MatchString(t) ||
		cjkOutlineRe.MatchString(t)
}
