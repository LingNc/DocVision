package session

import "testing"

// T201/T205：模型把工具调用写成正文、还声称"工作区工具不可用"的退化回复。
// 它是**诊断信号**（Run 里记一条警告），不是判失败的依据——见 t205 测试。
// 真实形态（多个章节的转换会话重复出现，逐字相同）：
//
//	{"paths":["skills://","skills://plugins"]}bash -lc ls -la /mnt/data | head
//	I can convert the chapter, but the chapter workspace tools are not available in this session. …
//
// 它没有 tool_calls，所以工具一次都不会执行；接连几轮只会收到同一份垃圾，
// 会话最后"正常结束"而章节没有产物——用户看到的就是"输出乱码，也不报错"。
func TestLooksLikeToolProseLeak(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			"skills:// 伪调用",
			`{"paths":["skills://","skills://plugins"]}I can convert the chapter, but the chapter workspace tools are not available in this session.`,
			true,
		},
		{
			"只说工具不可用",
			"I can't start the conversion yet because the chapter workspace tools are not available in this session.",
			true,
		},
		{
			"正常正文里提到工具",
			"我已经用 compile 编译通过，接下来核对原书页。",
			false,
		},
		{
			"正常 JSON 正文",
			`{"chapter":"chapter_002","pages":[12,13]}`,
			false,
		},
	}
	for _, c := range cases {
		if got := LooksLikeToolProseLeak(c.text); got != c.want {
			t.Errorf("%s: LooksLikeToolProseLeak = %v, want %v", c.name, got, c.want)
		}
	}
}
