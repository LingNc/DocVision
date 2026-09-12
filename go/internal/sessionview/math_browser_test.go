package sessionview

import (
	"strings"
	"testing"
)

// LaTeX 数学渲染的行为测试（真实 chromium 渲染静态导出页）：
//
//	· $…$ 行内、$$…$$ 独立显示都变成 MathML（浏览器原生排版，页面不引任何库/外链）；
//	· 结构对得上：^ → msup、_ → msub、\frac → mfrac、\sqrt[n] → mroot、
//	  \vec → mover、大算符 \sum 的上下限 → munderover；
//	· 认不出来的宏**原样保留**在 mtext 里（不吞、不留空）。
func TestViewerMathRenderInChromium(t *testing.T) {
	root := t.TempDir()
	body := transcript(
		`{"t":"msg","role":"user","text":"勾股：$a^2+b^2=c^2$，还有行内分数 $\\frac{1}{2}$。"}`,
		`{"t":"msg","role":"user","text":"向量 $\\vec{v}$，三次根 $\\sqrt[3]{x}$，求和 $\\sum_{i=1}^{n} i$，未知宏 $\\foobar{x}$。"}`,
		`{"t":"msg","role":"user","text":"独立公式：$$\\int_0^1 x^2 \\, dx = \\frac{1}{3}$$"}`,
	)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_chapter_001.jsonl"), body)

	dom := renderViewerDOM(t, root, "convert_chapter_001", "")

	for _, w := range []string{
		"<msup>",                  // a^2
		"<mfrac>",                 // \frac{1}{2}
		"<mroot>",                 // \sqrt[3]{x}
		"<mover",                  // \vec{v}
		"<munderover>",            // \sum_{i=1}^{n}
		`display="block"`,         // $$…$$ 独立显示
		"<mtext>\\foobar</mtext>", // 未知宏原样保留
	} {
		if !strings.Contains(dom, w) {
			t.Errorf("渲染结果里缺少 %s", w)
		}
	}
}
