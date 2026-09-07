package latex

import (
	"fmt"
	"strings"

	"mineru-tools/internal/session"
)

// ImageContextTool lets a figure session inspect the document context
// around ANY image reference of the source markdown and discover the
// previous/next image refs. This is the backbone of cross-page figure
// merging: a table/diagram split by pagination shows up as several
// consecutive image refs; the session must look at the neighbours to
// decide whether they are continuations.
type ImageContextTool struct {
	Content    string // full markdown of the current document
	CurrentImg string // the image this session was started for
	Radius     int    // context half-window in characters (default 600)
}

func (t *ImageContextTool) Name() string { return "image_context" }

func (t *ImageContextTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "image_context",
		"description": "Show the document text around an image reference and its neighbouring image refs (prev/next). Use it to detect cross-page continuations (split tables/diagrams) before drawing.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "Image path as it appears in the markdown (default: the image this session is drawing).",
				},
			},
		},
	}}
}

func (t *ImageContextTool) Execute(argsJSON string) (session.ToolResult, error) {
	radius := t.Radius
	if radius <= 0 {
		radius = 600
	}
	target := t.CurrentImg
	if argsJSON != "" {
		args, err := parseJSONObject(argsJSON)
		if err != nil {
			return session.ToolResult{}, err
		}
		if img, ok := args["image"].(string); ok && strings.TrimSpace(img) != "" {
			target = strings.TrimSpace(img)
		}
	}
	if target == "" {
		return session.ToolResult{}, fmt.Errorf("image 为空且会话未绑定当前图片")
	}

	type ref struct {
		path  string
		start int
	}
	var refs []ref
	for _, m := range imageRefRe.FindAllStringSubmatchIndex(t.Content, -1) {
		refs = append(refs, ref{path: t.Content[m[2]:m[3]], start: m[0]})
	}
	idx := -1
	for i, r := range refs {
		if r.path == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return session.ToolResult{}, fmt.Errorf("markdown 中找不到图片 %s", target)
	}

	build := func(i int) string {
		if i < 0 || i >= len(refs) {
			return "(none)"
		}
		r := refs[i]
		lo := r.start - radius
		if lo < 0 {
			lo = 0
		}
		hi := r.start + radius
		if hi > len(t.Content) {
			hi = len(t.Content)
		}
		return fmt.Sprintf("%s\n--- surrounding text ---\n%s", r.path,
			strings.TrimSpace(t.Content[lo:hi]))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "image %d of %d in this document: %s\n", idx+1, len(refs), target)
	fmt.Fprintf(&b, "\n## PREVIOUS image ref:\n%s", build(idx-1))
	fmt.Fprintf(&b, "\n\n## THIS image context:\n%s", build(idx))
	fmt.Fprintf(&b, "\n\n## NEXT image ref:\n%s", build(idx+1))
	fmt.Fprintf(&b, "\n\nIf PREVIOUS/NEXT looks like the SAME table or figure continued across a page break, view_image it and draw ONE combined figure; submit with the \"merges\" array listing every absorbed image path.")
	return session.ToolResult{Text: b.String()}, nil
}
