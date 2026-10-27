package latex

import (
	"strings"
	"testing"
)

// TestMdPlainTextDropsNonProse 钉住"只比正文"：公式、代码块、注释、图片、
// 表格竖线都不参与比对（公式抽成文本不可靠，会把正常章节误判成漏内容）。
func TestMdPlainTextDropsNonProse(t *testing.T) {
	md := strings.Join([]string{
		"# 第9章 极限",
		"## 定理 9.1",
		"设 $x_n$ 是严格递增数列，则有",
		"$$\\liminf_{n\\to\\infty} \\frac{y_n}{x_n} \\leqslant L$$",
		"```",
		"code 不该进入正文",
		"```",
		"<!-- DOCVISION-IMAGE: cover -->",
		"![图](images/book/a.png)",
		"| 列一 | 列二 |",
		"见 [原书第12页](http://x) 的说明。",
	}, "\n")
	got := mdPlainText(md)
	for _, bad := range []string{"liminf", "DOCVISION", "images/book", "```", "x_n", "列一 |"} {
		if strings.Contains(got, bad) {
			t.Errorf("mdPlainText 保留了不该比对的内容 %q:\n%s", bad, got)
		}
	}
	for _, want := range []string{"第9章 极限", "定理 9.1", "设", "是严格递增数列", "列一", "原书第12页"} {
		if !strings.Contains(got, want) {
			t.Errorf("mdPlainText 丢了正文 %q:\n%s", want, got)
		}
	}
}

// TestCoverageOfDetectsMissingContent 是这道门禁的核心用例：PDF 里缺了一整段，
// 覆盖率必须掉下去并点名缺的是哪一段。
func TestCoverageOfDetectsMissingContent(t *testing.T) {
	para := []string{
		"第一段讲的是数列极限的定义与基本性质，包含上下极限的等价刻画与常用不等式。",
		"第二段讨论 Stolz 定理的适用条件，并给出与 Toeplitz 定理的关系说明。",
		"第三段把渐近分析方法用到具体的级数上，说明误差项如何估计与控制。",
		"第四段是关于 Cauchy 乘积型极限的推导，重点在于一致收敛性的使用条件。",
		"第五段总结本章方法，并指出与前一章 Riemann 引理的衔接之处与差别。",
	}
	md := strings.Join(para, "\n")
	full := strings.Join(para, " ")
	cov, missing := coverageOf(mdPlainText(md), full)
	if cov < 0.999 {
		t.Fatalf("完整 PDF 的覆盖率应为 1，得到 %.3f（missing=%v）", cov, missing)
	}
	if len(missing) != 0 {
		t.Fatalf("完整 PDF 不应报缺段: %v", missing)
	}

	partial := strings.Join(para[:3], " ") // 后两段整体丢失
	cov, missing = coverageOf(mdPlainText(md), partial)
	if cov > minChapterCoverage {
		t.Fatalf("丢掉 40%% 内容时覆盖率应低于阈值 %.2f，得到 %.3f", minChapterCoverage, cov)
	}
	if len(missing) == 0 {
		t.Fatal("应点名缺失的段落")
	}
	if !strings.Contains(strings.Join(missing, "|"), "Cauchy") {
		t.Errorf("缺失清单里应包含丢失的段落: %v", missing)
	}
}

// TestCoverageOfSkipsFormulaOnlyChapters：CJK 太少的章节（纯公式/纯图）不判定，
// 否则正常章节会因为"没有正文可比"被误杀。
func TestCoverageOfSkipsFormulaOnlyChapters(t *testing.T) {
	md := "$$\\int_0^1 f(x)\\,dx = 1$$\n\n$\\alpha + \\beta$"
	if cov, missing := coverageOf(mdPlainText(md), ""); cov != 1 || missing != nil {
		t.Fatalf("纯公式章节应跳过判定，得到 cov=%.2f missing=%v", cov, missing)
	}
}

// TestNeedSourcePages：产物越长，要求对照的原书页越多，但夹在 2–4 页。
func TestNeedSourcePages(t *testing.T) {
	cases := map[int]int{1: 2, 3: 2, 6: 2, 9: 3, 12: 4, 60: 4}
	for pages, want := range cases {
		if got := needSourcePages(pages); got != want {
			t.Errorf("needSourcePages(%d) = %d, want %d", pages, got, want)
		}
	}
}

// TestPathWithin：区分"自己的产物"与"原书页"的唯一判据，必须稳。
func TestPathWithin(t *testing.T) {
	root := t.TempDir()
	if !pathWithin(root, root+"/a/b.pdf") {
		t.Fatal("工作区内的路径应判定为在内部")
	}
	if pathWithin(root, root+"-other/a.pdf") || pathWithin(root, "/etc/passwd") {
		t.Fatal("工作区外的路径不能被当成自己的产物")
	}
	if pathWithin("", "/tmp/x.pdf") {
		t.Fatal("空根不应命中")
	}
}

// TestCoverageDecisionSoftOnce 钉住用户定的口径（T168 修订）：覆盖率不达标
// **第一次打回**，同一次提交里仍不达标就**放行**（OCR 差异大的书可能永远到不了
// 阈值，硬卡会把整本书卡死），但缺口会由 coverageGapIssue 写进报告向上带。
func TestCoverageDecisionSoftOnce(t *testing.T) {
	// 达标：放行，且累计次数不变（不重置，历史缺口仍可在报告里看到）。
	if reject, checks := coverageDecision(0.99, 0.85, 0); reject || checks != 0 {
		t.Fatalf("达标应直接放行且不计数，得到 reject=%v checks=%d", reject, checks)
	}
	if reject, checks := coverageDecision(0.99, 0.85, 3); reject || checks != 3 {
		t.Fatalf("达标不应动已累计的次数，得到 reject=%v checks=%d", reject, checks)
	}
	// 第一次不达标：打回，计数到 1。
	if reject, checks := coverageDecision(0.70, 0.85, 0); !reject || checks != 1 {
		t.Fatalf("第一次不达标应打回并计数 1，得到 reject=%v checks=%d", reject, checks)
	}
	// 第二次不达标：放行，计数到 2。
	if reject, checks := coverageDecision(0.72, 0.85, 1); reject || checks != 2 {
		t.Fatalf("第二次不达标应放行并计数 2，得到 reject=%v checks=%d", reject, checks)
	}
}

// TestCoverageGapIssue：放行不等于忽略——缺口必须带上数字、原因与缺失段落。
func TestCoverageGapIssue(t *testing.T) {
	got := coverageGapIssue(0.72, []string{"第一段缺的原文", "第二段缺的原文"}, "")
	if !strings.Contains(got, "72%") || !strings.Contains(got, "85%") {
		t.Fatalf("缺口文案要带覆盖率与阈值: %s", got)
	}
	if !strings.Contains(got, "未说明原因") {
		t.Fatalf("没写 coverage_note 时要标出来: %s", got)
	}
	if !strings.Contains(got, "第一段缺的原文") {
		t.Fatalf("缺口文案要带缺失段落: %s", got)
	}
	got = coverageGapIssue(0.72, nil, "OCR 把整段排成了图片")
	if !strings.Contains(got, "OCR 把整段排成了图片") {
		t.Fatalf("要带上模型的解释: %s", got)
	}
}
