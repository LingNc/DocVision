package latex

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// 章节内容的机械核对（T168）。
//
// 背景：实测有的章节会话图省事，用一段 python 把章节 md 机械包成 tex，然后既
// 不看自己编出来的 PDF，也不与**原书**逐段对照——文字漏段、表格丢行、公式被
// 降级成纯文本，checker 只对着 md 与 tex 看结构，发现不了"内容少了"。所以除了
// "编译 + 看自己的产物 + 看原书页"这三件事，再加一道**文本覆盖率**：把章节 md
// 的正文（去掉代码/公式/图片/注释）与编译出的 PDF 抽出的文本做 CJK 三字组比对，
// 少得太多就直接打回，并列出缺得最多的几段。
//
// 阈值依据（同一批真实产物上量的）：正常完成的章节 0.89–0.98；把 PDF 文本砍掉
// 1/4 后掉到 0.63–0.76，砍一半掉到 0.33–0.59。取 0.85 既能抓住"少转了十分之一
// 以上"，又不会被 OCR 噪声、重复页眉页脚误伤。只比 CJK：公式抽成文本不可靠，
// 公式对不对由编译、checker 与看图负责。
const minChapterCoverage = 0.85

// md 里 CJK 太少（纯公式/纯图的章节）时这个指标没有意义，直接跳过判定。
const minChapterCoverageGrams = 200

// coverageDecision 是这道门禁的判定（T168 修订）：覆盖率不达标**第一次打回**，
// 让模型自己把缺的内容补上；同一次提交里仍然不达标就**放行**——OCR 差异大的
// 书（整段换行、重复页眉、公式被排成图）确实可能永远到不了阈值，硬卡会把书
// 卡死。放行不是"忽略"：缺口连同覆盖率数字与缺失段落一起记进这一章的报告，
// 由既有的跨层合并机制一路带上去。
//
// 返回 reject=是否打回、checks=累计的不达标次数（第一次=1）。
func coverageDecision(cov, threshold float64, prevChecks int) (reject bool, checks int) {
	if cov >= threshold {
		return false, prevChecks
	}
	return prevChecks == 0, prevChecks + 1
}

// coverageGapIssue 把一次"放行的缺口"写成一条 issue 文本（会进报告并向上合并）。
func coverageGapIssue(cov float64, missing []string, note string) string {
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
		cov*100, minChapterCoverage*100, note, miss)
}

// cjkStream 只保留汉字，用于三字组比对（标点、空格、公式、英文都不参与）。
func cjkStream(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cjkGrams(s string) map[string]struct{} {
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

// mdPlainText 把章节 md 变成"只含正文文字"的文本：去掉代码块、数学（行内与
// 整块）、HTML 注释、图片，保留链接文字与表格里的文字——这些正是要出现在 PDF
// 里的东西。公式与图片本来就会被排版成别的东西，留在比对里只会制造噪声。
func mdPlainText(md string) string {
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
		// 整块公式 $$…$$ / \[…\]：可能跨行，也常与正文同行。
		if strings.Contains(line, "$$") {
			parts := strings.Split(line, "$$")
			if len(parts)%2 == 0 { // $$ 的个数是奇数 → 状态翻转
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
		// 行内公式 \[…\] 与 $…$：先整块替换再逐段删。
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

// coverageOf 返回 md 正文三字组在 PDF 文本里出现的比例，以及缺得最多的几段
// 原文（用于告诉模型到底少了什么）。md 的 CJK 太少时不判：返回 1。
func coverageOf(mdText, pdfText string) (float64, []string) {
	mdCJK := cjkStream(mdText)
	ga := cjkGrams(mdCJK)
	if len(ga) < minChapterCoverageGrams {
		return 1, nil
	}
	gb := cjkGrams(cjkStream(pdfText))
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
		lineCJK := cjkStream(line)
		if len(lineCJK) < 10 {
			continue
		}
		lg := cjkGrams(lineCJK)
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

// chapterCoverage 从磁盘上取 md 与 PDF 文本再比对。pdftotext 不可用时返回
// 错误——调用方据此跳过判定（不能因为缺工具就卡死整本书）。
func chapterCoverage(mdPath, pdfPath string) (float64, []string, error) {
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
	cov, missing := coverageOf(mdPlainText(string(data)), string(out))
	return cov, missing, nil
}
