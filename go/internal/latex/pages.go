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
)

// partNumRe extracts the trailing part number from
// "<subject>_part12" style directory names.
var partNumRe = regexp.MustCompile(`_part(\d+)$`)

// findOriginPDFs locates the original scanned document PDFs that the
// MinerU API preserved for a subject (mineru_output/<subject>_partN/
// *_origin.pdf). These are the ORIGINAL PAGES — exactly what the style
// analysis needs to inspect real typography and layout. Returns the
// PDFs in part order (part1, part2, ...).
func findOriginPDFs(mineruDir, subject string) []string {
	entries, err := os.ReadDir(mineruDir)
	if err != nil {
		return nil
	}
	type part struct {
		num  int
		dir  string
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

// renderOriginPages rasterises every page of the origin PDFs to
// <outDir>/pNNN.png (global sequential numbering). Existing pages are
// kept (cache), so re-runs are cheap. Returns the number of pages
// available afterwards.
func (r *Runner) renderOriginPages(pdfs []string, outDir string) (int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	if err := r.comp.Available(); err != nil {
		return 0, err
	}
	seq := 0
	if pngs, _ := filepath.Glob(filepath.Join(outDir, "p*.png")); len(pngs) > 0 {
		seq = len(pngs)
	}
	for _, pdf := range pdfs {
		tag := sanitizeName(filepath.Base(filepath.Dir(pdf)))
		base := filepath.Join(outDir, "tmp_"+tag)
		marker := filepath.Join(outDir, ".done_"+tag)
		// Cache: skip parts that were fully rendered before.
		if fileExists(marker) {
			continue
		}
		rendered, err := r.comp.RasterizeAll(pdf, base)
		if err != nil {
			return seq, fmt.Errorf("渲染原始页面失败: %w", err)
		}
		sort.Strings(rendered)
		for _, src := range rendered {
			seq++
			dst := filepath.Join(outDir, fmt.Sprintf("p%03d.png", seq))
			if err := os.Rename(src, dst); err != nil {
				return seq, err
			}
		}
		if err := os.WriteFile(marker, []byte(strconv.Itoa(len(rendered))), 0o644); err != nil {
			return seq, err
		}
	}
	pngs, _ := filepath.Glob(filepath.Join(outDir, "p*.png"))
	return len(pngs), nil
}

// ListPagesTool tells the style analyst which original page renders
// exist (the naming is pNNN.png so view_page stays simple).
type ListPagesTool struct {
	PagesDir string
}

func (t *ListPagesTool) Name() string { return "list_pages" }

func (t *ListPagesTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "list_pages",
		"description": "List the ORIGINAL page renders (pNNN.png, in document order). These are the real scanned/layout pages of the book — the best source for typography, heading styles, headers/footers and overall design.",
		"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (t *ListPagesTool) Execute(_ string) (session.ToolResult, error) {
	pngs, _ := filepath.Glob(filepath.Join(t.PagesDir, "p*.png"))
	if len(pngs) == 0 {
		return session.ToolResult{Text: "(no original page renders available)"}, nil
	}
	first := filepath.Base(pngs[0])
	last := filepath.Base(pngs[len(pngs)-1])
	return session.ToolResult{Text: fmt.Sprintf("%d original pages available: %s ... %s (view with view_image, path = the page name, e.g. \"%s\")", len(pngs), first, last, first)}, nil
}
