package session

import "testing"

// T32：模型随 tool_calls 把原始 XML 协议残片写进 content（真实调用已
// 走 tool_calls 通道），残片不得进入回放历史/转录正文。
func TestStripLeakedToolXML(t *testing.T) {
	call := ToolCall{ID: "c1", Type: "function"}
	call.Function.Name = "read_file"
	call.Function.Arguments = `{"path":"check:chapter_023.md"}`

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "纯残片（实测形态，含标签体内的值行）",
			in:   "<parameter=path>\ncheck:parts/\n</parameter>\n</function>\n</tool_call>",
			want: "",
		},
		{
			name: "完整 XML 块混在正文里",
			in:   "Let me check:\n<tool_call>\n<function=read_file>\n<parameter=path>\nwork/chapters/x.md\n</parameter>\n</function>\n</tool_call>\ndone",
			want: "Let me check:\n\ndone", // 块删除留下一个空行，Markdown 渲染无害
		},
		{
			name: "正常带工具调用的正文不受影响",
			in:   "Reading the chapter file first.",
			want: "Reading the chapter file first.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := ChatMessage{Role: "assistant", Content: c.in, ToolCalls: []ToolCall{call}}
			StripLeakedToolXML(&m)
			if got := ContentString(m); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	// 无工具调用的纯文本回复：即使讨论了协议格式也不许碰。
	plain := ChatMessage{Role: "assistant", Content: "the raw XML looked like <tool_call> but was ignored"}
	StripLeakedToolXML(&plain)
	if got := ContentString(plain); got != plain.Content {
		t.Fatalf("text-only reply mutated: %q", got)
	}
}
