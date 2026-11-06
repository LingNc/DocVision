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

// linkAbs creates a symlink at link whose stored target is an ABSOLUTE path.
//
// Why this exists: os.Symlink stores the target string verbatim and the
// kernel resolves it relative to the DIRECTORY CONTAINING THE LINK, not
// relative to the process working directory. Config paths are relative
// ("mineru_output/..."), so linking them directly produced DANGLING links
// under <proj>/work/pdfview/ and <proj>/work/views/... — the sessions then
// reported "文件不存在: source:<part>.pdf" for files that plainly existed,
// and the sandboxed bash saw empty /source and /project trees.
//
// An existing link is replaced when it points somewhere else, so repeated
// runs converge instead of keeping a stale target.
func linkAbs(src, link string) error {
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if target, rerr := os.Readlink(link); rerr == nil {
		if target == abs {
			return nil
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	} else if _, lerr := os.Lstat(link); lerr == nil {
		// A real file/directory sits there: never clobber it.
		return fmt.Errorf("%s 已存在且不是软链", link)
	}
	return os.Symlink(abs, link)
}

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
func buildPDFView(proj, mineruDir, splitDir, subject string) (*pdfView, error) {
	pdfs := findOriginPDFs(mineruDir, splitDir, subject)
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
	for _, o := range pdfs {
		name := viewName(o.Part, subject, used)
		if err := linkAbs(o.PDF, filepath.Join(dir, name)); err != nil {
			return nil, fmt.Errorf("link %s: %w", name, err)
		}
		n, err := api.PageCountFile(o.PDF)
		if err != nil {
			return nil, fmt.Errorf("count pages of %s: %w", o.PDF, err)
		}
		view.Files = append(view.Files, pdfViewFile{
			Name: name, PDF: o.PDF, Part: o.Part,
			First: view.total + 1, Count: n,
		})
		view.total += n
	}
	return view, nil
}

// viewName builds a clean, unique name for one origin PDF: the MinerU
// part directory name ("<subject>_part2" -> "book_part2.pdf"), or the
// subject name when the PDF is not the first part of its own subject.
func viewName(part, subject string, used map[string]bool) string {
	base := part // the MinerU part directory name
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
		// 兜底 PDF 的父目录是 split_files，不能拿它当 part 名（T292）。
		part := src.part
		if part == "" {
			part = filepath.Base(filepath.Dir(src.pdf))
		}
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
		link := filepath.Join(dir, e.Name)
		if e.Name == "source" {
			// T290：source 里除了书本身还堆着这一程的记账（作图会话转录、
			// dedupe 产物、逐图进度），对会话来说全是噪音，还会把"前面那些
			// 会话"直接摊到它眼前。这里只铺 md 与真正属于书的产物目录。
			ensureProjectSourceView(filepath.Join(proj, e.Target), link)
			continue
		}
		src := filepath.Join(proj, e.Target)
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			continue
		}
		if err := linkAbs(src, link); err != nil {
			// A real directory already sits there (or the link is
			// undeletable): keep it, the view still works.
			continue
		}
	}
	return dir
}

// projectSourceSubdirs 是 source 下真正属于"书"的产物目录：图片、矢量图成品、
// 单图 tikz 源。其余（sessions/dedupe/progress_items）都是流水账，会话不需要看。
var projectSourceSubdirs = []string{"images", "figures", "tikz"}

// ensureProjectSourceView 把 <proj>/source 铺成视图里的 source：md 文件与上面
// 那几个产物目录逐条软链，流水账一概不铺。视图里上一次运行留下的旧条目（含
// 旧版本整棵 source 的软链）会先清掉，避免它继续漏进会话。
func ensureProjectSourceView(src, dir string) {
	// 旧版本把整棵 source 软链在这里：先摘掉这条链，否则下面的"清理多余条目"
	// 会穿透到真正的 source 目录里去动手。
	if st, err := os.Lstat(dir); err == nil && st.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dir); err != nil {
			return
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	want := map[string]string{}
	if mds, err := filepath.Glob(filepath.Join(src, "*.md")); err == nil {
		for _, md := range mds {
			want[filepath.Base(md)] = md
		}
	}
	for _, sub := range projectSourceSubdirs {
		p := filepath.Join(src, sub)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			want[sub] = p
		}
	}
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if _, ok := want[e.Name()]; ok {
				continue
			}
			// 只删软链：视图里不该出现真文件，真出现了也留在原处不动。
			p := filepath.Join(dir, e.Name())
			if st, err := os.Lstat(p); err == nil && st.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(p)
			}
		}
	}
	for name, target := range want {
		_ = linkAbs(target, filepath.Join(dir, name))
	}
}
