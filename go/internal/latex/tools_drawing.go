package latex

import (
	"fmt"
	"strings"

	"mineru-tools/internal/session"
)

// ImageContextTool lets a figure session inspect the document context
// around ANY image reference of the source markdown and discover the
// previous/next image refs — the same expandable up/down semantics as
// img2text's get_more_context, but text-window based. This is the
// backbone of cross-page figure merging: pagination shows a split
// table/diagram as several consecutive image refs; the session inspects
// neighbours to decide whether they are continuations.
type ImageContextTool struct {
	Content    string // full markdown of the current document
	CurrentImg string // the image this session was started for
	MaxUp      int    // hard cap for upward expansion (lines)
	MaxDown    int    // hard cap for downward expansion (lines)
}

const (
	ctxDefaultUp   = 10
	ctxDefaultDown = 10
)

func (t *ImageContextTool) Name() string { return "image_context" }

func (t *ImageContextTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "image_context",
		"description": "Show the markdown text lines around an image ref (expandable up/down window, like get_more_context) plus its prev/next image refs. Use it to detect cross-page continuations before drawing.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "Image path as it appears in the markdown (default: the image this session is drawing).",
				},
				"up": map[string]any{
					"type":        "integer",
					"description": "Context lines above (default 10).",
				},
				"down": map[string]any{
					"type":        "integer",
					"description": "Context lines below (default 10).",
				},
			},
		},
	}}
}

func (t *ImageContextTool) Execute(argsJSON string) (session.ToolResult, error) {
	target := t.CurrentImg
	up, down := ctxDefaultUp, ctxDefaultDown
	if argsJSON != "" {
		args, err := parseJSONObject(argsJSON)
		if err != nil {
			return session.ToolResult{}, err
		}
		if img, ok := args["image"].(string); ok && strings.TrimSpace(img) != "" {
			target = strings.TrimSpace(img)
		}
		if v, ok := args["up"].(float64); ok && v > 0 {
			up = int(v)
		}
		if v, ok := args["down"].(float64); ok && v > 0 {
			down = int(v)
		}
	}
	if max := t.MaxUp; max > 0 && up > max {
		up = max
	}
	if max := t.MaxDown; max > 0 && down > max {
		down = max
	}
	if target == "" {
		return session.ToolResult{}, fmt.Errorf("image 为空且会话未绑定当前图片")
	}

	lines := strings.Split(t.Content, "\n")
	type ref struct {
		path string
		line int
	}
	var refs []ref
	for i, line := range lines {
		for _, m := range imageRefRe.FindAllStringSubmatch(line, -1) {
			refs = append(refs, ref{path: m[1], line: i})
		}
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

	build := func(i, upN, downN int) string {
		if i < 0 || i >= len(refs) {
			return "(none)"
		}
		r := refs[i]
		lo := r.line - upN
		if lo < 0 {
			lo = 0
		}
		hi := r.line + downN + 1
		if hi > len(lines) {
			hi = len(lines)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s (line %d)\n", r.path, r.line+1)
		for ln := lo; ln < hi; ln++ {
			marker := "  "
			if ln == r.line {
				marker = "> "
			}
			fmt.Fprintf(&b, "%s%4d | %s\n", marker, ln+1, lines[ln])
		}
		return strings.TrimRight(b.String(), "\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "image %d of %d in this document: %s\n", idx+1, len(refs), target)
	fmt.Fprintf(&b, "\n## PREVIOUS image ref:\n%s", build(idx-1, up, down))
	fmt.Fprintf(&b, "\n\n## THIS image context (up %d / down %d lines; request again with bigger up/down to expand):\n%s", up, down, build(idx, up, down))
	fmt.Fprintf(&b, "\n\n## NEXT image ref:\n%s", build(idx+1, up, down))
	b.WriteString("\n\nDecide WITHOUT assuming: adjacency does NOT imply relation. If PREVIOUS/NEXT is the same table/figure continued across a page break, view_image it, then draw ONE combined figure and submit with \"merges\" listing the absorbed image paths. If they are unrelated, or this image is obviously complete on its own, just draw THIS image and merge nothing.")
	return session.ToolResult{Text: b.String()}, nil
}
