package latex

import (
	"encoding/json"
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
// rendered ON DEMAND by view_page (and then cached), never up front.
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

// pageCachePath returns the cache file for a global page number.
func pageCachePath(pagesDir string, page int) string {
	return filepath.Join(pagesDir, fmt.Sprintf("p%03d.png", page))
}

// ensurePageRendered renders page (global number) into the cache when
// not present yet, and returns the cache path. Rendering happens ONLY
// when the AI actually asks for that page.
func (r *Runner) ensurePageRendered(idx *pageIndex, pagesDir string, page int) (string, error) {
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return "", err
	}
	if err := r.comp.Available(); err != nil {
		return "", err
	}
	cache := pageCachePath(pagesDir, page)
	if util.FileExists(cache) {
		return cache, nil
	}
	pdf, local, err := idx.locate(page)
	if err != nil {
		return "", err
	}
	// pdftoppm -singlefile still appends .png to the output base.
	tmpBase := filepath.Join(pagesDir, fmt.Sprintf("tmp_%d", page))
	if err := r.comp.RasterizePages(pdf, local, local, tmpBase); err != nil {
		return "", err
	}
	if err := os.Rename(tmpBase+".png", cache); err != nil {
		return "", err
	}
	return cache, nil
}

// ListPagesTool tells the style analyst how many original pages exist
// (they are rendered on demand via view_page).
type ListPagesTool struct {
	Idx *pageIndex
}

func (t *ListPagesTool) Name() string { return "list_pages" }

func (t *ListPagesTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "list_pages",
		"description": "Report the number of ORIGINAL document pages (real scanned/layout pages) available for viewing via view_page. The best source for typography, heading styles, headers/footers and overall design.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *ListPagesTool) Execute(_ string) (session.ToolResult, error) {
	if t.Idx == nil || t.Idx.total == 0 {
		return session.ToolResult{Text: "(no original page renders available)"}, nil
	}
	return session.ToolResult{Text: fmt.Sprintf("%d original pages available (view_page page=1..%d). Pages render on demand and are cached.", t.Idx.total, t.Idx.total)}, nil
}

// ViewPageTool renders ONE original page on demand (with cache) and
// returns it as an image. Supports percent crop + zoom like view_image.
type ViewPageTool struct {
	Idx      *pageIndex
	PagesDir string
	Runner   *Runner
}

func (t *ViewPageTool) Name() string { return "view_page" }

func (t *ViewPageTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "view_page",
		"description": "View one ORIGINAL document page (real typography/layout). Renders on demand and caches. Optional percent crop (0-100, relative to full page) and zoom_width let you inspect details like heading styles, headers/footers and font shapes.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page":       map[string]any{"type": "integer", "description": "global page number, 1-based"},
				"left":       map[string]any{"type": "number", "description": "crop left percent (0-100)"},
				"top":        map[string]any{"type": "number", "description": "crop top percent"},
				"right":      map[string]any{"type": "number", "description": "crop right percent (100 = full width)"},
				"bottom":     map[string]any{"type": "number", "description": "crop bottom percent (100 = full height)"},
				"zoom_width": map[string]any{"type": "integer", "description": "rescale cropped image to this pixel width for detail inspection"},
			},
			"required": []string{"page"},
		},
	}}
}

func (t *ViewPageTool) Execute(argsJSON string) (session.ToolResult, error) {
	var args struct {
		Page      int     `json:"page"`
		Left      float64 `json:"left"`
		Top       float64 `json:"top"`
		Right     float64 `json:"right"`
		Bottom    float64 `json:"bottom"`
		ZoomWidth int     `json:"zoom_width"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return session.ToolResult{}, fmt.Errorf("view_page 参数错误: %w", err)
	}
	if t.Idx == nil || t.Idx.total == 0 {
		return session.ToolResult{}, fmt.Errorf("原始页面不可用（未找到 MinerU 保留的 origin PDF）")
	}
	if args.Page < 1 || args.Page > t.Idx.total {
		return session.ToolResult{}, fmt.Errorf("page 必须在 1..%d", t.Idx.total)
	}
	path, err := t.Runner.ensurePageRendered(t.Idx, t.PagesDir, args.Page)
	if err != nil {
		return session.ToolResult{}, err
	}
	if args.Left > 0 || args.Top > 0 || args.Right > 0 || args.Bottom > 0 || args.ZoomWidth > 0 {
		img, err := decodeImage(path)
		if err != nil {
			return session.ToolResult{}, err
		}
		cropped := cropPercent(flattenToOpaque(img), args.Left, args.Top, args.Right, args.Bottom)
		if args.ZoomWidth > 0 {
			cropped = scaleToWidth(cropped, args.ZoomWidth)
		}
		b64, err := encodeJPEGBase64(cropped)
		if err != nil {
			return session.ToolResult{}, err
		}
		return session.ToolResult{ImageBase64: b64, ImageMIME: "image/jpeg"}, nil
	}
	b64, err := ReadImageFile(path)
	if err != nil {
		return session.ToolResult{}, err
	}
	return session.ToolResult{ImageBase64: b64, ImageMIME: "image/png"}, nil
}
