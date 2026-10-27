package latex

import (
	"fmt"
)

// 章节规模的提示与硬规则（T169）。
//
// 起因：真实项目里划分阶段给出的章节从一百多行到两万行不等，几千行的章节丢给
// 一个转换会话，既慢又容易漏内容（它读不完、写不完、也核对不完）。递归拆分机制
// 本来就有，但触发条件是模型自评的"我觉得太大了"——和"有疑问就翻原书"一样靠不
// 住。所以把规模变成**可测量**的东西：首条消息直接告诉它会话要处理多少行，超过
// 上限且还没拆分的节点在 submit 被打回，必须先 request_chapter_split。
//
// 上限来自 config 的 latex.max_chapter_lines（未写默认 3000，0 = 关闭）。

// chapterSizeNote 是首条消息里的规模提示。返回空串表示没有可说的。
func chapterSizeNote(lines, limit, depth int) string {
	if lines <= 0 {
		return ""
	}
	note := fmt.Sprintf("This chapter is %d lines of markdown.", lines)
	if limit <= 0 || lines <= limit {
		return note
	}
	if depth >= maxChapterSplitDepth {
		return note + " It is big, but this is the deepest subdivision level: convert it here as faithfully as you can and report what you could not reproduce."
	}
	return note + fmt.Sprintf(" That is more than one session converts well (about %d lines): call request_chapter_split FIRST, into 2-%d semantic parts of roughly %d-%d lines each, then read the children summary, integrate, compile and verify the parent.",
		limit, maxChapterChildren, limit/3, limit/2)
}

// chapterSizeReject 是"没拆分就提交"的打回文案。返回空串表示放行。
//
// splitRequested 为真说明这个节点已经拆过（现在是在整合子章节），不能再要求
// 它拆；深度到顶时同样放行——拆不动了，只能在这里尽力而为。
func chapterSizeReject(lines, limit, depth int, splitRequested bool) string {
	if limit <= 0 || lines <= limit || splitRequested || depth >= maxChapterSplitDepth {
		return ""
	}
	return fmt.Sprintf("REJECTED: 这一章有 %d 行 markdown，超过单会话转换的上限（%d 行）——一个会话硬啃会又慢又容易漏内容。"+
		"先调用 request_chapter_split，按语义把它拆成 2-%d 段（每段大约 %d-%d 行），"+
		"让子会话各自转换、各自过 checker；子章节都过了以后你再整合、编译、对照原书，然后提交本节点。",
		lines, limit, maxChapterChildren, limit/3, limit/2)
}
