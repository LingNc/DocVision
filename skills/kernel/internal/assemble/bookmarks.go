package assemble

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------- T49：PDF 书签（逐字移植旧线） ----------

// EnsurePDFBookmarks 让成品 PDF 可导航：向 build/main.tex 幂等注入
// hyperref（若整个文档还没加载）与逐章 \pdfbookmark（放在每个
// \input{chapters/…} 前面，锚点 bk:<base>）。AI 终审可以自己布置书签——
// main.tex 里已有任何 \pdfbookmark/\bookmark 行就视为 AI 已接手，只补
// hyperref、不再注入基线锚点（避免与层级书签重复）。
func EnsurePDFBookmarks(proj, buildDir string) error {
	mainPath := filepath.Join(buildDir, "main.tex")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}
	main := string(data)
	hasHyperref := strings.Contains(main, "hyperref") || ClassLoadsHyperref(proj)
	aiOwnsBookmarks := strings.Contains(main, "\\pdfbookmark") || strings.Contains(main, "\\bookmark")

	var b strings.Builder
	changed := false
	for _, line := range strings.SplitAfter(main, "\n") {
		trimmed := strings.TrimSpace(line)
		if !hasHyperref && strings.HasPrefix(trimmed, "\\documentclass") {
			b.WriteString(line)
			b.WriteString("\\usepackage[bookmarksnumbered,bookmarksopen,hidelinks]{hyperref}\n")
			hasHyperref = true
			changed = true
			continue
		}
		if base := ChapterInputBase(trimmed); base != "" && !aiOwnsBookmarks {
			anchor := "bk:" + base
			if !strings.Contains(main, "{"+anchor+"}") {
				title, level := ChapterBookmarkTitle(proj, base)
				fmt.Fprintf(&b, "\\pdfbookmark[%d]{%s}{%s}\n", level, escapeBookmark(title), anchor)
				changed = true
			}
		}
		b.WriteString(line)
	}
	if !changed {
		return nil
	}
	return os.WriteFile(mainPath, []byte(b.String()), 0o644)
}

// ChapterInputBase 识别 "\input{chapters/<base>.tex}" 行，返回 base。
func ChapterInputBase(line string) string {
	const pre = "\\input{chapters/"
	if !strings.HasPrefix(line, pre) || !strings.HasSuffix(line, "}") {
		return ""
	}
	name := strings.TrimSuffix(strings.TrimPrefix(line, pre), "}")
	if strings.ContainsAny(name, "/\\{}") {
		return ""
	}
	return strings.TrimSuffix(name, ".tex")
}

// ChapterBookmarkTitle 取划分阶段的章节 md 首个标题行作书签文字与层级
// （转换单元按 markdown 标题边界拆分，首个标题的 # 数就是它在书里的真实
// 层级——#=0 层、##=1 层，clamp 到 0..3）。找不到时退回 base 名、0 层。
func ChapterBookmarkTitle(proj, base string) (string, int) {
	for _, dir := range []string{filepath.Join(proj, "chapters"), filepath.Join(proj, "work", "chapters")} {
		data, err := os.ReadFile(filepath.Join(dir, base+".md"))
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(data), "\n") {
			ln = strings.TrimSpace(ln)
			if strings.HasPrefix(ln, "#") {
				hashes := len(ln) - len(strings.TrimLeft(ln, "#"))
				level := hashes - 1
				if level < 0 {
					level = 0
				}
				if level > 3 {
					level = 3
				}
				return strings.TrimSpace(strings.TrimLeft(ln, "#")), level
			}
			if ln != "" {
				break // 首个非空行不是标题就不再找（避免误取正文）
			}
		}
	}
	return base, 0
}

// escapeBookmark 转义 pdfbookmark 参数里的 LaTeX 特殊字符。
func escapeBookmark(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	for _, c := range []string{"%", "&", "#", "_"} {
		s = strings.ReplaceAll(s, c, "\\"+c)
	}
	return s
}

// ClassLoadsHyperref 报告书的 cls 是否自行加载了 hyperref（加载过就别重复引入）。
func ClassLoadsHyperref(proj string) bool {
	entries, err := os.ReadDir(filepath.Join(proj, "style"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cls") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(proj, "style", e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "hyperref") {
			return true
		}
	}
	return false
}
