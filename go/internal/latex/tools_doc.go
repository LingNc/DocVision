package latex

// Read-only original-document tools for conversion sessions:
// doc_search locates a markdown fragment in the MinerU block index and
// returns the GLOBAL page number; list_source_pages + view_pdf (source mount,
// shared render cache) then renders that original PDF page. list_source_pages
// is included so sessions can sanity-check the page range.

import (
	"fmt"
	"strings"

	"mineru-tools/internal/session"
)

// DocSearchTool searches the compiled original-document index.
type DocSearchTool struct {
	Index *DocIndex
	// View (optional) lets every hit carry the exact source path and local
	// page (source:<file>.pdf page N), located through the MinerU part of
	// the block — no global-offset arithmetic needed by the model.
	View  *pdfView
	Mount string
}

// formatEntry renders one hit, appending the exact source page when the
// PDF view is available.
func (t *DocSearchTool) formatEntry(e DocEntry) string {
	s := FormatEntry(e)
	if t.View == nil {
		return s
	}
	if f, local, err := t.View.LocatePart(e.Part, e.Page+1); err == nil {
		path := f.Name
		if t.Mount != "" {
			path = t.Mount + ":" + f.Name
		}
		var size [2]float64
		if t.Index != nil {
			size = t.Index.Sizes[e.Part]
		}
		l, tp, r, b := CropHint(e.BBox, size)
		s += fmt.Sprintf("\n    source page: view_pdf {path:\"%s\", page:%d, left:%d, top:%d, right:%d, bottom:%d}", path, local, l, tp, r, b)
	}
	return s
}

func (t *DocSearchTool) Name() string { return "doc_search" }

func (t *DocSearchTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "doc_search",
		"description": "Search the original-document block index (read-only): text snippets, figure/table captions, equation LaTeX and image filenames from the MinerU parse. Maps a markdown fragment to its original PDF page (pN) and bbox. Query is space-separated keywords (ALL must match); a bare number also matches page numbers.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Keywords, an image filename (images/...), or a page number."},
				"max":   map[string]any{"type": "integer", "description": "Max results (default 12, cap 50)."},
			},
			"required": []string{"query"},
		},
	}}
}

func (t *DocSearchTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return session.ToolResult{}, fmt.Errorf("query 不能为空")
	}
	max := intArg(args, "max", 12)
	hits := t.Index.Search(query, max)
	if len(hits) == 0 {
		return session.ToolResult{Text: "无匹配。试试更短的关键词、图片文件名（images/...）或页码。"}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 个匹配（doc_search '%s'）：\n", len(hits), query)
	for _, e := range hits {
		b.WriteString(t.formatEntry(e) + "\n")
	}
	b.WriteString("Each hit shows its exact source page (view_pdf {path:\"<source>:<file>\", page:<local page>}); list_source_pages {page: pN} prints that page's text and images. The bbox is NOT in PDF points: it uses MinerU's layout coordinate space, which is about 2x the page's point size (a 493x720pt page reaches ~986x1440), so never divide it by the printed page size. When you need a crop, use the crop percentages printed with the hit. Adjacency in the index does NOT imply relation.")
	return session.ToolResult{Text: b.String()}, nil
}
