package latex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// T297：插图文件名是 64 位内容哈希，模型在 `\includegraphics` 里手打这种名字
// 十次里错一次（错在尾巴上，前缀一模一样）。光回一句 "File ... not found"
// 它只会再猜一次；所以编译失败时把"工作区里真有的、名字最接近的"直接列出来，
// 让它照抄。三处编译回执（work 会话、整章 convert、figure 预览）共用这一份。

var (
	missingGraphicRe1 = regexp.MustCompile("File `([^'`\n]+)' not found")
	missingGraphicRe2 = regexp.MustCompile("can't find file `([^'`\n]+)'")
)

// missingFileHints 返回一段"照抄这些名字"的提示；没有可对照的候选就返回空串。
func missingFileHints(errText, dir string) string {
	if dir == "" || errText == "" {
		return ""
	}
	var lines []string
	seen := map[string]bool{}
	for _, re := range []*regexp.Regexp{missingGraphicRe1, missingGraphicRe2} {
		for _, m := range re.FindAllStringSubmatch(errText, -1) {
			name := strings.TrimSpace(m[1])
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			if best := closestWorkspaceName(dir, name); best != "" {
				lines = append(lines, fmt.Sprintf("  %s\n    → 工作区里真实存在的是 %s（原样照抄这个）", name, best))
			}
			if len(lines) >= 3 {
				break
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "MISSING FILES（名字必须原样照抄，不要手打）：\n" + strings.Join(lines, "\n")
}

// closestWorkspaceName 在同一个目录里找与目标名公共前缀最长、且前缀足够长的
// 条目（哈希名只错尾部时前缀能到几十个字符；无关文件一般只有几个字符）。
func closestWorkspaceName(dir, name string) string {
	rel := filepath.FromSlash(name)
	abs := filepath.Join(dir, rel)
	entries, err := os.ReadDir(filepath.Dir(abs))
	if err != nil {
		return ""
	}
	base := filepath.Base(abs)
	best, bestN := "", 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n := commonPrefixLen(base, e.Name()); n > bestN {
			bestN, best = n, e.Name()
		}
	}
	if best == "" || bestN < 8 || best == base {
		return ""
	}
	return filepath.ToSlash(filepath.Join(filepath.Dir(rel), best))
}

// commonPrefixLen 是两个字符串的公共前缀字节数（文件名比较用，UTF-8 边界
// 不参与判断，只用来排序"谁最像"）。
func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
