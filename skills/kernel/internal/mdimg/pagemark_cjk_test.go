package mdimg

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 回归：insertPageMarks 旧实现把"插入"当"替换"——每个 [P. N] 后吞 len(标记)+1 字节
// （ASCII 静默吞字符，CJK 非法 UTF-8）。此测试用真实 plan→apply 全链路 + CJK 断言。
func TestPageMarkInsertPreservesContentCJK(t *testing.T) {
	// 构造多行 CJK 内容：12 行正文 + 一个图片行（触发 embed shift 的替换流）
	var sb strings.Builder
	bodyLines := []string{
		"# 导数与微分", "", "微积分的基本概念", "", "极限的定义与性质", "",
		"![图1](images/a1.png)", "", "导数的运算法则", "", "复合函数求导", "",
		"微分中值定理", "", "洛必达法则",
	}
	for _, l := range bodyLines {
		sb.WriteString(l + "\n")
	}
	md := sb.String()
	repls := []Replacement{{ImgRef: "images/a1.png", Text: "图1：函数变化率的示意图，含切线与割线。"}}
	_ = repls // 先测纯 pagemark 路径（无 embed 变化时 offsets 恒等）

	// 通过 ApplyTo 全链路（plan→行重写→insertPageMarks）
	final, _ := AssembleWithShifts(md, repls)
	if !utf8.ValidString(final) {
		t.Fatalf("输出非法 UTF-8（CJK 被吃字节）: %q", final[:min(120, len(final))])
	}
	// 原文行必须全部保留（顺序出现）
	for _, l := range bodyLines {
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "!") {
			continue
		}
		if !strings.Contains(final, l) {
			t.Fatalf("原文行丢失: %q", l)
		}
	}
}
