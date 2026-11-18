package prep

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// organize.go：把 MinerU 的 content_list 拼成章节 md（PREP 收尾）。
// 产物契约（docs/latex.md + docindex 的回填约定）：
//   - text/interline_equation/footnote/ 等文本块 → 原文落 md；
//   - image/table/chart 块 → `![caption](images/<sha>.<ext>)`（相对 part 根）；
//   - 多 part 的整本 md = 各 part md 按序拼接（part 间空行）。
// 纯文本拼装，无判据；幂等（覆盖重写，md 是 PREP 的可再生产物）。

// blockLine 渲染单个 content_list 块为一行/几行 md。
func blockLine(b map[string]any) string {
	typ, _ := b["type"].(string)
	img, _ := b["img_path"].(string)
	caption := joinAnyList(b["image_caption"])
	if caption == "" {
		caption = joinAnyList(b["table_caption"])
	}
	switch typ {
	case "image", "table", "chart":
		if img == "" {
			return ""
		}
		alt := caption
		if alt == "" {
			alt = typ
		}
		return fmt.Sprintf("![%s](%s)", alt, img)
	case "text", "footnote", "header", "list", "index", "code":
		if s, ok := b["text"].(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	case "equation", "interline_equation":
		if s, ok := b["text"].(string); ok && s != "" {
			return "$$" + strings.TrimSpace(s) + "$$"
		}
		if img != "" {
			return fmt.Sprintf("![equation](%s)", img)
		}
		return ""
	default:
		if s, ok := b["text"].(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	}
}

func joinAnyList(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// OrganizePart 把一个 part 的 content_list 拼成 md 文本（不写盘，便于测试与组合）。
func OrganizePart(autoDir string) (string, error) {
	cl := contentListIn(autoDir)
	if cl == "" {
		return "", fmt.Errorf("无 content_list（auto 目录：%s）", autoDir)
	}
	raw, err := os.ReadFile(cl)
	if err != nil {
		return "", err
	}
	items, err := parseContentList(raw)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, b := range items {
		if ln := blockLine(b); ln != "" {
			lines = append(lines, ln)
		}
	}
	return strings.Join(lines, "\n\n") + "\n", nil
}

// Organize 把整本（一个或多个 part）拼成 <stem>.md 写到 outDir。
// parts 按给定顺序拼接（调用方按 part 号排好序）。返回写出的 md 路径列表。
func Organize(autoDirs []string, outDir string) ([]string, error) {
	if len(autoDirs) == 0 {
		return nil, fmt.Errorf("无 part 可组织")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var outs []string
	var all []string
	for _, auto := range autoDirs {
		md, err := OrganizePart(auto)
		if err != nil {
			return outs, err
		}
		base := filepath.Base(filepath.Dir(auto)) // <part名>/auto → <part名>
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		outPath := filepath.Join(outDir, stem+".md")
		if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
			return outs, err
		}
		outs = append(outs, outPath)
		all = append(all, strings.TrimSpace(md))
	}
	// 整本 md：各 part 顺序拼接
	bookStem := strings.TrimSuffix(filepath.Base(filepath.Dir(autoDirs[0])), filepath.Ext(filepath.Base(filepath.Dir(autoDirs[0]))))
	bookStem = strings.TrimSuffix(bookStem, "_part1")
	bookPath := filepath.Join(outDir, bookStem+".md")
	if err := os.WriteFile(bookPath, []byte(strings.Join(all, "\n\n")+"\n"), 0o644); err != nil {
		return outs, err
	}
	outs = append(outs, bookPath)
	return outs, nil
}

// contentListIn 在 auto 目录里找 content_list（优先非 v2）。
func contentListIn(autoDir string) string {
	matches, _ := filepath.Glob(filepath.Join(autoDir, "*_content_list.json"))
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), "_v2") {
			return m
		}
	}
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}
