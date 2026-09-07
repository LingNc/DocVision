package latex

// Read-only original-document tools for conversion sessions:
// doc_search locates a markdown fragment in the MinerU block index and
// returns the GLOBAL page number; view_page (existing pages.go tool,
// shared render cache) then renders that original PDF page. list_pages
// is included so sessions can sanity-check the page range.

import (
	"fmt"
	"strings"

	"mineru-tools/internal/session"
)

// DocSearchTool searches the compiled original-document index.
type DocSearchTool struct {
	Index *DocIndex
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
		b.WriteString(FormatEntry(e) + "\n")
	}
	b.WriteString("view_page {page: pN} renders that original PDF page (bbox is in PDF points, top-left origin; page sizes are in the index). Adjacency in the index does NOT imply relation.")
	return session.ToolResult{Text: b.String()}, nil
}
