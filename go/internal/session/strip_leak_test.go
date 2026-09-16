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
			name: "残片后面跟着正文：正文保留（不被当标签值吃掉）",
			in:   "<parameter=path>\nx\n</parameter>\n</tool_call>\ndone",
			want: "done",
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

// T30：reasoning 通道关闭时，模型把 <thinking>…</thinking> 漏进正文——
// 整块移到 ReasoningContent（UI 折叠展示、GLM 保留式思考回传走原通道）。
func TestMoveLeakedThinking(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantText string
		wantRsn  string
	}{
		{
			name:     "整块移到思维链、正文保留",
			in:       "<thinking>Page 12: 解答题 format.</thinking>\nLet me look at the figure page closely.",
			wantText: "Let me look at the figure page closely.",
			wantRsn:  "Page 12: 解答题 format.",
		},
		{
			name:     "think 拼写同样认（Qwen 风格）",
			in:       "<think>hmm</think>done",
			wantText: "done",
			wantRsn:  "hmm",
		},
		{
			name:     "多块合并、已有思维链续在后面",
			in:       "<thinking>t1</thinking>mid<thinking>t2</thinking>",
			wantText: "mid",
			wantRsn:  "t1\n\nt2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := ChatMessage{Role: "assistant", Content: c.in}
			MoveLeakedThinking(&m)
			if got := ContentString(m); got != c.wantText {
				t.Fatalf("text = %q, want %q", got, c.wantText)
			}
			if m.ReasoningContent != c.wantRsn {
				t.Fatalf("reasoning = %q, want %q", m.ReasoningContent, c.wantRsn)
			}
		})
	}

	// user 消息不碰。
	u := ChatMessage{Role: "user", Content: "<thinking>x</thinking>"}
	MoveLeakedThinking(&u)
	if got := ContentString(u); got != "<thinking>x</thinking>" {
		t.Fatalf("user msg mutated: %q", got)
	}
	// 没有泄漏时不写 reasoning_content。
	plain := ChatMessage{Role: "assistant", Content: "ordinary reply"}
	MoveLeakedThinking(&plain)
	if plain.ReasoningContent != "" {
		t.Fatalf("reasoning set without leak: %q", plain.ReasoningContent)
	}
}
