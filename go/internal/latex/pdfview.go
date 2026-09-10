package latex

// pdfView is the MINIMAL, session-safe view of the original book PDFs.
//
// A session must never see the whole mineru_output tree (thousands of
// JSON blocks, other books): only the *_origin.pdf files of THIS book,
// exposed under clean, stable file names. buildPDFView creates
//
//	<proj>/work/pdfview/<part>.pdf -> <mineru_output>/<part>/<uuid>_origin.pdf
//
// so that the SAME tree can be used in three places with one mental
// model:
//
//	structured tools   bash (inside the sandbox)
//	source:<part>.pdf  /source/<part>.pdf
//
// The view is also the page table: global page numbers (1-based over
// all parts, in part order) are derived from it, aligned with the
// DocIndex numbering.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// pdfViewFile is one original PDF in the view.
type pdfViewFile struct {
	Name  string // clean name exposed to the model ("book_part1.pdf")
	PDF   string // real path of the origin PDF
	Part  string // MinerU part directory (the DocIndex page-index key)
	First int    // first global page number (1-based)
	Count int    // page count
}

// pdfView is the mounted, minimal set of original PDFs.
type pdfView struct {
	Dir   string        // <proj>/work/pdfview (mounted read-only)
	Files []pdfViewFile // in part order
	total int
}

// buildPDFView creates/refreshes the view for one subject ("book" for
// book.md). Existing links are replaced so a re-run with different
// parts cannot leave stale PDFs behind.
func buildPDFView(proj, mineruDir, subject string) (*pdfView, error) {
	pdfs := findOriginPDFs(mineruDir, subject)
	if len(pdfs) == 0 {
		return nil, fmt.Errorf("no origin pdfs for %s", subject)
	}
	dir := filepath.Join(proj, "work", "pdfview")
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	view := &pdfView{Dir: dir}
	used := map[string]bool{}
	for _, pdf := range pdfs {
		name := viewName(pdf, subject, used)
		if err := os.Symlink(pdf, filepath.Join(dir, name)); err != nil {
			return nil, fmt.Errorf("link %s: %w", name, err)
		}
		n, err := api.PageCountFile(pdf)
		if err != nil {
			return nil, fmt.Errorf("count pages of %s: %w", pdf, err)
		}
		view.Files = append(view.Files, pdfViewFile{
			Name: name, PDF: pdf, Part: filepath.Base(filepath.Dir(pdf)),
			First: view.total + 1, Count: n,
		})
		view.total += n
	}
	return view, nil
}

// viewName builds a clean, unique name for one origin PDF: the MinerU
// part directory name ("<subject>_part2" -> "book_part2.pdf"), or the
// subject name when the PDF is not the first part of its own subject.
func viewName(pdf, subject string, used map[string]bool) string {
	base := filepath.Base(filepath.Dir(pdf)) // the part directory
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = subject
	}
	base = strings.TrimSpace(sanitizeName(base))
	if base == "" {
		base = "source"
	}
	name := base + ".pdf"
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s_%d.pdf", base, i)
	}
	used[name] = true
	return name
}

// Locate resolves a GLOBAL page number to the view file and its local
// page (1-based inside that PDF).
func (v *pdfView) Locate(page int) (pdfViewFile, int, error) {
	if v == nil || v.total == 0 {
		return pdfViewFile{}, 0, fmt.Errorf("原始页面不可用（未找到 MinerU 保留的 *_origin.pdf）")
	}
	if page < 1 || page > v.total {
		return pdfViewFile{}, 0, fmt.Errorf("page %d out of range 1..%d", page, v.total)
	}
	for _, f := range v.Files {
		if page < f.First+f.Count {
			return f, page - f.First + 1, nil
		}
	}
	return pdfViewFile{}, 0, fmt.Errorf("page %d not found", page)
}

// FileByPart finds the view file that came from a MinerU part directory
// (the "part" recorded in the page index). This is the drift-free way to
// locate an indexed block: it does not depend on cumulative page offsets.
func (v *pdfView) FileByPart(part string) (pdfViewFile, bool) {
	if v == nil {
		return pdfViewFile{}, false
	}
	part = strings.TrimSpace(part)
	if part == "" {
		return pdfViewFile{}, false // no part info: never guess
	}
	for _, f := range v.Files {
		if f.Part == part {
			return f, true
		}
	}
	return pdfViewFile{}, false
}

// LocatePart maps a MinerU part directory plus a 1-based page inside that
// part to a view file. Because both sides key on the part itself, this
// stays correct even when a part's page count in the page index differs
// from the real PDF page count.
func (v *pdfView) LocatePart(part string, localPage int) (pdfViewFile, int, error) {
	f, ok := v.FileByPart(part)
	if !ok {
		return pdfViewFile{}, 0, fmt.Errorf("no source PDF for part %q", part)
	}
	if localPage < 1 || localPage > f.Count {
		return pdfViewFile{}, 0, fmt.Errorf("page %d out of range 1..%d of %s", localPage, f.Count, f.Name)
	}
	return f, localPage, nil
}

// alignmentProblems cross-checks the page index against the real PDFs:
// if a part's indexed page count differs from the origin PDF's page
// count, the global page numbers derived from the index drift away from
// the cumulative offsets used here, and global-page lookups can land on
// the wrong part or page. Part-based lookups (FileByPart/LocatePart) stay
// correct in that case.
func (v *pdfView) alignmentProblems(pages *pageIndex) []string {
	if v == nil || pages == nil {
		return nil
	}
	var out []string
	byPart := map[string]pdfViewFile{}
	for _, f := range v.Files {
		byPart[f.Part] = f
	}
	for _, src := range pages.srcs {
		part := filepath.Base(filepath.Dir(src.pdf))
		f, ok := byPart[part]
		if !ok {
			out = append(out, fmt.Sprintf("part %s: no view file", part))
			continue
		}
		if f.Count != src.count {
			out = append(out, fmt.Sprintf("part %s: index says %d pages, the PDF has %d (global page numbers may drift; part-based lookups are exact)",
				part, src.count, f.Count))
		}
	}
	return out
}

// FileByName finds a view file by its clean name or its real path.
func (v *pdfView) FileByName(name string) (pdfViewFile, bool) {
	if v == nil {
		return pdfViewFile{}, false
	}
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "source:")
	for _, f := range v.Files {
		if f.Name == name || f.PDF == name || filepath.Base(f.PDF) == name {
			return f, true
		}
	}
	return pdfViewFile{}, false
}

// Mounts returns the read-only mount of this view for a session.
func (v *pdfView) Mounts() []Mount {
	if v == nil || v.Dir == "" {
		return nil
	}
	return []Mount{{Name: "source", Dir: v.Dir}}
}

// pageIndex converts the view into the legacy global page table (kept
// for the watermark sampler and index alignment).
func (v *pdfView) pageIndex() *pageIndex {
	if v == nil {
		return nil
	}
	idx := &pageIndex{total: v.total}
	for _, f := range v.Files {
		idx.srcs = append(idx.srcs, pageSrc{pdf: f.PDF, first: f.First, count: f.Count})
	}
	return idx
}

// names lists the exposed file names in part order (for prompts).
func (v *pdfView) names() []string {
	if v == nil {
		return nil
	}
	out := make([]string, 0, len(v.Files))
	for _, f := range v.Files {
		out = append(out, f.Name)
	}
	return out
}

// ------------------------------------------------------------------
// Project view
// ------------------------------------------------------------------

// projectViewEntries are the ONLY project trees a session may read.
// Everything else the pipeline produces (work/sessions transcripts,
// doc_index.json, pages/ renders, build/, out/) stays invisible: it is
// either internal bookkeeping or huge.
//
// Name is what the session sees (project:<Name>/...), Target is the real
// tree. "converted" and "reports" are the READ-ONLY channel to what other
// conversion sessions produced: their submitted .tex and their work
// reports. Session transcripts are deliberately NOT exposed.
var projectViewEntries = []struct{ Name, Target string }{
	{"source", "source"},
	{"style", "style"},
	{"chapters", "chapters"},
	{"converted", filepath.Join("work", "chapters")},
	{"reports", filepath.Join("work", "reports")},
}

// ensureProjectView materialises the narrow read-only project view
// (<proj>/work/views/project/{source,style,chapters} as symlinks) and
// returns its directory. It is idempotent and safe to call repeatedly
// (e.g. per session, since "style" only appears after the style phase).
func ensureProjectView(proj string) string {
	if proj == "" {
		return ""
	}
	dir := filepath.Join(proj, "work", "views", "project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	for _, e := range projectViewEntries {
		src := filepath.Join(proj, e.Target)
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			continue
		}
		link := filepath.Join(dir, e.Name)
		if target, err := os.Readlink(link); err == nil {
			if target == src {
				continue
			}
			_ = os.Remove(link)
		} else if _, err := os.Lstat(link); err == nil {
			continue // a real directory/file is already there
		}
		_ = os.Symlink(src, link)
	}
	return dir
}

// ------------------------------------------------------------------
// Per-chapter work view
// ------------------------------------------------------------------

// ensureChapterView materialises the private work view of ONE chapter
// conversion session: <proj>/work/views/chapter_<base>/chapters/ holds a
// symlink to that chapter's .tex and to its asset folder, so the session
// can read and write exactly its own files — a sibling chapter is not
// reachable here (it is read-only reference material through
// project:converted/). Writing through the .tex symlink creates the real
// file in <proj>/work/chapters/ when it does not exist yet.
func ensureChapterView(proj, base string) string {
	if proj == "" || base == "" {
		return ""
	}
	realChapDir := filepath.Join(proj, "work", "chapters")
	viewChapDir := filepath.Join(proj, "work", "views", "chapter_"+base, "chapters")
	if err := os.MkdirAll(realChapDir, 0o755); err != nil {
		return ""
	}
	if err := os.MkdirAll(viewChapDir, 0o755); err != nil {
		return ""
	}
	// own main file: always ensure the symlink exists (may dangle until written)
	texLink := filepath.Join(viewChapDir, base+".tex")
	if _, err := os.Lstat(texLink); err != nil {
		_ = os.Symlink(filepath.Join(realChapDir, base+".tex"), texLink)
	}
	// own asset folder: a real folder holding a symlink to the real one
	subLink := filepath.Join(viewChapDir, base)
	realSub := filepath.Join(realChapDir, base)
	if _, err := os.Lstat(subLink); err != nil {
		_ = os.MkdirAll(realSub, 0o755)
		_ = os.Symlink(realSub, subLink)
	}
	return filepath.Dir(viewChapDir)
}
