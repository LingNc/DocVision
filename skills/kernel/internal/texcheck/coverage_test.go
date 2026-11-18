package texcheck

import (
	"fmt"
	"strings"
	"testing"
)

func TestCjkStreamAndGrams(t *testing.T) {
	if got := CjkStream("ab汉c字，d"); got != "汉字" {
		t.Fatalf("CjkStream: %q", got)
	}
	// 按旧线用法：CjkGrams 吃 CjkStream 输出（纯 CJK 流），两侧同法抽取比值才有意义。
	g := CjkGrams(CjkStream("汉字测试"))
	if len(g) == 0 {
		t.Fatal("三字组集合不应为空")
	}
	if len(CjkGrams("")) != 0 {
		t.Fatal("空串应为空集")
	}
}

func TestMdPlainText(t *testing.T) {
	md := "```go\ncode\n```\n$$x^2$$\n正文$e=mc^2$文字\n<!-- 注释 -->\n![图](a.png)\n[链接](http://x) 表格 |a|b|\n"
	got := MdPlainText(md)
	if strings.Contains(got, "code") {
		t.Fatalf("代码块未剥离: %q", got)
	}
	if !strings.Contains(got, "正文 文字") {
		t.Fatalf("行内公式未剥离: %q", got)
	}
	if strings.Contains(got, "注释") {
		t.Fatalf("HTML 注释未剥离: %q", got)
	}
	if !strings.Contains(got, "链接") || strings.Contains(got, "![图]") {
		t.Fatalf("图片/链接处理错: %q", got)
	}
}

func makeVariedMd() string {
	topics := []string{"监督学习", "无监督学习", "强化学习", "特征工程", "模型评估", "概率图模型",
		"支持向量机", "决策树集成", "神经网络反向传播", "梯度下降优化", "正则化方法", "贝叶斯推断"}
	var sb strings.Builder
	// 每句嵌入 34 个唯一字符（顺序 CJK 码点）——须占行内容半数以上，
	// 保证集合口径下删 1/4 句真的丢组、且被删行行级命中率 <50%（缺口定位判据才有对象）
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&sb, "第%d节讨论%s的原理与应用案例，编号", i+1, topics[i%len(topics)])
		for j := 0; j < 34; j++ {
			sb.WriteRune(rune(0x4e00 + i*40 + j))
		}
		sb.WriteString("。\n")
	}
	return sb.String()
}

func TestCoverageOf(t *testing.T) {
	md := makeVariedMd()
	mdText := MdPlainText(md)
	cov, _ := CoverageOf(mdText, md) // md 自身对照：应满覆盖
	if cov < 0.99 {
		t.Fatalf("自对照应满覆盖: %f", cov)
	}
	// 删掉 1/4 内容 → 低于阈值（对齐旧线实测：正常 0.89–0.98，砍 1/4 → 0.63–0.76）
	partial := md[: len(md)*3/4]
	cov2, missing := CoverageOf(mdText, partial)
	if cov2 >= MinChapterCoverage {
		t.Fatalf("缺 1/4 应低于阈值: %f", cov2)
	}
	if len(missing) == 0 {
		t.Fatal("应列出缺口段落")
	}
	// CJK 太少（纯公式章节）不判：返回 1
	cov3, _ := CoverageOf("$e=mc^2$", "nothing")
	if cov3 != 1 {
		t.Fatalf("CJK 太少应跳过判定: %f", cov3)
	}
}

func TestCoverageDecision(t *testing.T) {
	if CoverageDecision(0.9, 0) {
		t.Fatal("达标不应打回")
	}
	if !CoverageDecision(0.6, 0) {
		t.Fatal("第一次不达标应打回")
	}
	if CoverageDecision(0.6, 1) {
		t.Fatal("第二次不达标应放行（记缺口）")
	}
}

func TestCoverageGapNote(t *testing.T) {
	note := CoverageGapNote(0.6, []string{"缺段一"}, "")
	if !strings.Contains(note, "60%") || !strings.Contains(note, "缺段一") || !strings.Contains(note, "未说明原因") {
		t.Fatalf("缺口说明格式: %s", note)
	}
}
