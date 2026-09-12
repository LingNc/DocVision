package img2text

import (
	"fmt"
	"regexp"
	"strings"
)

// 本文件集中处理"失败时怎么把真话写进日志"的三件事：
//
//  1. 截断方向：只取校验器输出的**前** 400 字节时拿到的永远是 TeX 版本
//     横幅，真正的 `! Undefined control sequence.` / `l.NNN` 全在后面。
//     现在按"错误行"抽取，抽不到才退回**最后** N 行。
//  2. 原文：`[IMG_TYPE:]` 解析失败与"跳过无效响应"两处都带截断原文 +
//     总长度，看日志的人能知道模型到底回了什么。
//  3. 一致性：提示词只出 mermaid，模型若仍吐 ```latex/```tikz 绘图块，
//     该响应按无效处理（跳过、下轮重试），并把原文片段打进日志。

// snippetMaxChars 是"原文片段"的字符上限（按 rune 计，避免把多字节
// 汉字切成半个）。
const snippetMaxChars = 800

// errorLineMaxBytes 是抽取出来的错误行总长上限。
const errorLineMaxBytes = 1200

// tailLinesOnNoError 抽不到错误行时保留的输出最后几行。
const tailLinesOnNoError = 12

// rawOutputAheadOnNoError …当既没有错误行也没有尾部行时，退回开头多少
// 字节（正常不会发生，只是让函数永远有话说）。
const rawOutputAheadOnNoError = 400

var (
	// latexBannerRe 匹配 TeX 引擎的版本横幅行——抽不到错误行时这些行
	// 是最没用的，必须排除。
	latexBannerRe = regexp.MustCompile(`(?i)^(This is |entering extended mode|Document Class:|LaTeX2e|.*preloaded format=)`)
	// latexErrorLineRe 匹配 TeX 的报错行与上下文行。
	latexErrorLineRe = regexp.MustCompile(`^(!|l\.\d+|Package \S* *Error|Class \S* *Error|Emergency stop|Runaway argument|File \S+ Error)`)
	// latexBangHintRe 匹配 TeX 的 `!` 报错后面紧跟的那行解释（通常是
	// 出错命令的展开形式，如 `\foo ->...`）。
	latexBangHintRe = regexp.MustCompile(`^[\\/]`)
	// noiseLineRe 匹配各校验器输出里纯属"开场白"的行（mmdc 的
	// "Generating single mermaid chart" 之类），它们永远不是错误。
	noiseLineRe = regexp.MustCompile(`(?i)^(Generating |Rendering |Creating |Initiating )`)
)

// extractValidationError 从校验器（TeX 编译 / mmdc）的原始输出里抽出
// 真正有用的错误说明：
//
//   - 有 TeX 的 `!` 报错时，取所有报错行（`!` 开头、`l.<数字>`、
//     `Package … Error`、`Emergency stop`…）以及紧随其后的解释行；
//   - TeX 的版本横幅（`This is XeTeX, Version …` 等）一律排除；
//   - 没有 `!` 报错（mmdc 那类输出、编译中途被打断）时，退回**最后**
//     tailLinesOnNoError 行——不是"前 N 字节"，也不是中间那几行；
//   - 仍然为空（例如输出只有横幅）时退回开头一小段，保证日志不空。
func extractValidationError(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	picked := make([]string, 0, 8)
	if hasTexBangError(lines) {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || latexBannerRe.MatchString(trimmed) || noiseLineRe.MatchString(trimmed) {
				continue
			}
			if !latexErrorLineRe.MatchString(trimmed) {
				continue
			}
			picked = append(picked, trimmed)
			// 只有 `!` 报错才有"下一行是出错命令展开"这个约定。
			if strings.HasPrefix(trimmed, "!") {
				if j := i + 1; j < len(lines) {
					if next := strings.TrimSpace(lines[j]); next != "" && latexBangHintRe.MatchString(next) {
						picked = append(picked, next)
					}
				}
			}
		}
	}
	if len(picked) == 0 {
		picked = tailNonEmpty(lines, tailLinesOnNoError)
	}
	if len(picked) == 0 {
		return truncate(strings.TrimSpace(out), rawOutputAheadOnNoError)
	}
	return truncate(strings.Join(picked, " ⏎ "), errorLineMaxBytes)
}

// hasTexBangError reports whether the dump contains a TeX-style `!` error
// line. Only then is the error-line scan worth running: for other tools'
// output the tail is the informative part.
func hasTexBangError(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "!") {
			return true
		}
	}
	return false
}

// tailNonEmpty returns up to n trailing non-blank lines of lines.
func tailNonEmpty(lines []string, n int) []string {
	out := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		s := strings.TrimSpace(lines[i])
		if s == "" || noiseLineRe.MatchString(s) {
			continue
		}
		out = append(out, s)
	}
	// collected back-to-front: restore reading order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// snippet returns a truncated view of a model response for log lines,
// with the total length appended so a reader can tell how much was cut.
// rune-safe: Chinese text must never be sliced mid-character.
func snippet(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= snippetMaxChars {
		return fmt.Sprintf("%s (共 %d 字符)", s, len(runes))
	}
	return fmt.Sprintf("%s… (截断，共 %d 字符)", string(runes[:snippetMaxChars]), len(runes))
}

// expectedFormatHint is the one-line summary of the response shape the
// img2text pipeline can actually parse, appended to every "invalid
// response" log line so the deviation is obvious at a glance.
const expectedFormatHint = `期望格式：[IMG_TYPE: <类型>] 开头（类型如 mermaid / table / text / code / flowchart），` +
	"Mermaid 图用 ```" + "mermaid 围栏；不得输出 ```" + `tikz 或 LaTeX/TikZ 绘图代码块`

// leftoverDrawingBlockRe 匹配模型无视提示词仍然吐出的 LaTeX/TikZ 绘图
// 代码块。只认绘图用的 tikz / pgfplots / latex 围栏——`tex` 太宽，代码
// 截图里出现 LaTeX 源码是正常内容。
var leftoverDrawingBlockRe = regexp.MustCompile("(?is)```[ \t]*(?:tikz|pgfplots|latex)[ \t]*\\r?\\n")

// leftoverDrawingBlock reports whether the response carries a fenced
// LaTeX/TikZ drawing block, which img2text cannot validate or embed.
func leftoverDrawingBlock(text string) bool {
	return leftoverDrawingBlockRe.MatchString(text)
}
