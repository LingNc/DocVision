package latex

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"mineru-tools/internal/session"
)

// mdImageRef is one image reference found in a document's markdown.
type mdImageRef struct {
	path string
	line int // 0-based line index
}

// scanImageRefs returns every image ref of a markdown document in order.
func scanImageRefs(content string) ([]mdImageRef, []string) {
	lines := strings.Split(content, "\n")
	var refs []mdImageRef
	for i, line := range lines {
		for _, m := range imageRefRe.FindAllStringSubmatch(line, -1) {
			refs = append(refs, mdImageRef{path: m[1], line: i})
		}
	}
	return refs, lines
}

// matchImageRef finds the index of target among the markdown refs. It
// accepts the exact ref, a path with/without the images/ prefix, or a
// bare file name (a unique match is required — an ambiguous name yields
// -1 instead of guessing).
func matchImageRef(refs []mdImageRef, target string) int {
	target = strings.TrimSpace(filepath.ToSlash(target))
	if target == "" {
		return -1
	}
	for i, r := range refs {
		if r.path == target {
			return i
		}
	}
	norm := strings.TrimPrefix(strings.TrimPrefix(target, "./"), "images/")
	if strings.Contains(norm, "/") {
		// Partial path (subject/fig.jpg): match by suffix.
		for i, r := range refs {
			rel := strings.TrimPrefix(filepath.ToSlash(r.path), "images/")
			if rel == norm || strings.HasSuffix(rel, "/"+norm) {
				return i
			}
		}
	}
	// Bare file name: require a unique basename match.
	base := path.Base(norm)
	hits := 0
	hit := -1
	for i, r := range refs {
		if path.Base(filepath.ToSlash(r.path)) == base {
			hits++
			hit = i
		}
	}
	if hits == 1 {
		return hit
	}
	return -1
}

// imageSubject returns the per-document subfolder of a markdown image
// ref ("images/测试-概率论/x.jpg" -> "测试-概率论"), or "" when the ref
// has no folder. Used so tools can resolve bare file names.
func imageSubject(ref string) string {
	p := strings.TrimPrefix(filepath.ToSlash(ref), "images/")
	dir := path.Dir(p)
	if dir == "." || dir == "/" || dir == "" {
		return ""
	}
	return dir
}

// ImageContextTool lets a figure session inspect the document context
// around ANY image reference of the source markdown and discover the
// previous/next image refs — the same expandable up/down semantics as
// img2text's get_more_context, but text-window based. It never returns
// pixels: view_image does that. This is the backbone of cross-page
// figure merging: pagination shows a split table/diagram as several
// consecutive image refs; the session inspects neighbours to decide
// whether they are continuations.
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
		"description": "TEXT context (no pixels): show the markdown lines around an image ref (expandable up/down window, like get_more_context) plus the previous/next image refs with their line numbers. Use it to detect cross-page continuations; use view_image to actually LOOK at an image.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "Image ref as in the markdown (default: the image this session is drawing). A bare file name is also accepted (resolved inside this document's image folder).",
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

	refs, lines := scanImageRefs(t.Content)
	idx := matchImageRef(refs, target)
	if idx < 0 {
		return session.ToolResult{}, fmt.Errorf("markdown 中找不到图片 %s（可直接用文件名，程序会在本文档图片目录内匹配）", target)
	}

	deltaTo := func(i int) int {
		if i < 0 || i >= len(refs) {
			return 0
		}
		return refs[i].line - refs[idx].line
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
		if d := deltaTo(i); d == 0 {
			fmt.Fprintf(&b, "%s (line %d)\n", r.path, r.line+1)
		} else {
			fmt.Fprintf(&b, "%s (line %d, %+d lines from this image)\n", r.path, r.line+1, d)
		}
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
	fmt.Fprintf(&b, "image %d of %d in this document: %s\n", idx+1, len(refs), refs[idx].path)
	fmt.Fprintf(&b, "\n## PREVIOUS image ref:\n%s", build(idx-1, up, down))
	fmt.Fprintf(&b, "\n\n## THIS image context (up %d / down %d lines; request again with bigger up/down to expand):\n%s", up, down, build(idx, up, down))
	fmt.Fprintf(&b, "\n\n## NEXT image ref:\n%s", build(idx+1, up, down))
	return session.ToolResult{Text: b.String()}, nil
}
