package latex

import (
	"strings"
	"testing"
)

// T323：沙箱里没有 rg 之类的命令时，回执要直接给等价替代，而不是让模型反复
// 撞 127。
func TestMissingCommandNote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"exit error: exit status 127\n/usr/bin/bash: 行 1: rg: 未找到命令\n", "grep"},
		{"/usr/bin/bash: line 1: rg: command not found\n", "grep"},
		{"/usr/bin/bash: line 2: fd: command not found\n", "find"},
		{"/usr/bin/bash: line 3: jq: command not found\n", "python3"},
		{"/usr/bin/bash: line 4: nosuchtool: command not found\n", "coreutils"},
	}
	for _, c := range cases {
		got := missingCommandNote(c.in)
		if !strings.Contains(got, c.want) {
			t.Fatalf("输入 %q 期望提到 %q，实际 %q", c.in, c.want, got)
		}
	}
	// 正常输出不该被加提示。
	if got := missingCommandNote("index.tex\n"); got != "" {
		t.Fatalf("普通输出不该加提示：%q", got)
	}
}
