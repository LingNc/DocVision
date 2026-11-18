// Package texcheck 是 T3 验收的重型判据：编译执行与内容覆盖率。
// 算法逐字移植自旧线 go/internal/latex/chaptercoverage.go（T168）与编译门禁，
// 阈值依据旧线实测（正常章节 0.89–0.98，砍 1/4 掉到 0.63–0.76）。
package texcheck

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// MinChapterCoverage 覆盖率阈值（低于则第一次打回）。
const MinChapterCoverage = 0.85

// MinChapterCoverageGrams md 里 CJK 三字组太少（纯公式/纯图章节）时指标无意义，跳过判定。
const MinChapterCoverageGrams = 200

// CoverageDecision 判定（T168 修订）：不达标第一次打回，第二次提交放行（放行≠忽略：
// 缺口连同数字记进 notes 随报告向上带）。reject=是否打回。
func CoverageDecision(cov float64, attempts int) (reject bool) {
	if cov >= MinChapterCoverage {
		return false
	}
	return attempts == 0
}

// CoverageGapNote 把"放行的缺口"写成一条说明文本。
func CoverageGapNote(cov float64, missing []string, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		note = "未说明原因"
	}
	miss := "（未能定位具体段落）"
	if len(missing) > 0 {
		shown := missing
		if len(shown) > 3 {
			shown = shown[:3]
		}
		miss = strings.Join(shown, " ／ ")
	}
	return fmt.Sprintf("内容覆盖率 %.0f%%（低于 %.0f%% 阈值，第二次提交放行）：说明：%s；缺得最多的是：%s",
		cov*100, MinChapterCoverage*100, note, miss)
}

// CjkStream 只保留汉字，用于三字组比对（移植旧线 cjkStream）。
func CjkStream(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CjkGrams CJK 三字组集合（移植旧线 cjkGrams；吃 CjkStream 输出，两侧同法抽取比值才有意义）。
func CjkGrams(s string) map[string]struct{} {
	const n = 3
	out := make(map[string]struct{}, len(s)/2)
	if len(s) < n {
		return out
	}
	for i := 0; i+n <= len(s); i++ {
		out[s[i:i+n]] = struct{}{}
	}
	return out
}

// MdPlainText 把章节 md 变成只含正文文字的文本：去代码块/数学/HTML 注释/图片，
// 保留链接文字与表格文字（移植旧线 mdPlainText，逐字）。
func MdPlainText(md string) string {
	var out strings.Builder
	fence, mathBlock := false, false
	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if strings.Contains(line, "$$") {
			parts := strings.Split(line, "$$")
			if len(parts)%2 == 0 {
				mathBlock = !mathBlock
			}
			keep := make([]string, 0, (len(parts)+1)/2)
			for i, part := range parts {
				if i%2 == 0 {
					keep = append(keep, part)
				}
			}
			line = strings.Join(keep, " ")
			if mathBlock {
				continue
			}
		}
		if mathBlock {
			continue
		}
		for {
			i := strings.Index(line, `\[`)
			if i < 0 {
				break
			}
			j := strings.Index(line[i:], `\]`)
			if j < 0 {
				line = line[:i]
				break
			}
			line = line[:i] + " " + line[i+j+2:]
		}
		for {
			i := strings.Index(line, "$")
			if i < 0 {
				break
			}
			j := strings.Index(line[i+1:], "$")
			if j < 0 {
				line = line[:i]
				break
			}
			line = line[:i] + " " + line[i+1+j+1:]
		}
		line = stripHTMLComments(line)
		line = stripMarkdownImages(line)
		line = linkTextOnly(line)
		line = strings.NewReplacer("|", " ", ">", " ", "`", " ", "*", " ", "_", " ", "#", " ").Replace(line)
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

func stripHTMLComments(s string) string {
	for {
		i := strings.Index(s, "<!--")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "-->")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + " " + s[i+j+3:]
	}
}

func stripMarkdownImages(s string) string {
	for {
		i := strings.Index(s, "![")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "](")
		if j < 0 {
			return s
		}
		k := strings.Index(s[i+j:], ")")
		if k < 0 {
			return s
		}
		s = s[:i] + " " + s[i+j+k+1:]
	}
}

func linkTextOnly(s string) string {
	for {
		i := strings.Index(s, "[")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "](")
		if j < 0 {
			return s
		}
		k := strings.Index(s[i+j:], ")")
		if k < 0 {
			return s
		}
		text := s[i+1 : i+j]
		s = s[:i] + text + " " + s[i+j+k+1:]
	}
}

// CoverageOf md 正文三字组在 PDF 文本里出现的比例 + 缺得最多的几段（移植旧线 coverageOf）。
// md CJK 太少时不判：返回 1。
func CoverageOf(mdText, pdfText string) (float64, []string) {
	mdCJK := CjkStream(mdText)
	ga := CjkGrams(mdCJK)
	if len(ga) < MinChapterCoverageGrams {
		return 1, nil
	}
	gb := CjkGrams(CjkStream(pdfText))
	hit := 0
	for g := range ga {
		if _, ok := gb[g]; ok {
			hit++
		}
	}
	cov := float64(hit) / float64(len(ga))

	var missing []string
	for _, line := range strings.Split(mdText, "\n") {
		line = strings.TrimSpace(line)
		lineCJK := CjkStream(line)
		if len(lineCJK) < 10 {
			continue
		}
		lg := CjkGrams(lineCJK)
		if len(lg) == 0 {
			continue
		}
		lhit := 0
		for g := range lg {
			if _, ok := gb[g]; ok {
				lhit++
			}
		}
		if float64(lhit)/float64(len(lg)) < 0.5 {
			missing = append(missing, clipRunes(line, 70))
			if len(missing) >= 5 {
				break
			}
		}
	}
	return cov, missing
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ChapterCoverage 从磁盘取 md 与 PDF 文本再比对。
// pdftotext 不可用时返回错误——调用方据此跳过判定（不能因缺工具卡死整本书）。
func ChapterCoverage(mdPath, pdfPath string) (float64, []string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return 0, nil, fmt.Errorf("pdftotext 不可用（poppler-utils）: %w", err)
	}
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return 0, nil, err
	}
	out, err := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", pdfPath, "-").Output()
	if err != nil {
		return 0, nil, fmt.Errorf("pdftotext %s: %w", pdfPath, err)
	}
	cov, missing := CoverageOf(MdPlainText(string(data)), string(out))
	return cov, missing, nil
}
