package session

import (
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// T273：网关/中转站把另一套 agent 的工具名递给模型时（Claude Code 的
// Read/Bash/Grep、别家 harness 的 read_session…），会话不该卡在
// "unknown tool" 上——大小写/分隔符自动归一，真正换了名字的用 tool_aliases
// 按模型映射，找不到时把本会话可用的工具名如实回来。
func TestResolveToolNames(t *testing.T) {
	tools := []Tool{
		&fakeTool{name: "read_file"},
		&fakeTool{name: "write_file"},
		&fakeTool{name: "bash"},
		&fakeTool{name: "grep"},
		&fakeTool{name: "submit"},
	}
	newSession := func(aliases map[string]string) *Session {
		return &Session{
			tools:    tools,
			modelCfg: config.ModelConfig{ToolAliases: aliases},
		}
	}
	cases := []struct {
		name    string
		aliases map[string]string
		call    string
		want    string // "" = 找不到
	}{
		{"精确名", nil, "read_file", "read_file"},
		{"大小写不同", nil, "Bash", "bash"},
		{"全大写", nil, "SUBMIT", "submit"},
		{"分隔符不同", nil, "read-file", "read_file"},
		{"空格分隔", nil, "read file", "read_file"},
		{"命名空间前缀", nil, "mcp__docvision__grep", "grep"},
		{"别名映射", map[string]string{"Read": "read_file"}, "Read", "read_file"},
		{"别名键大小写不敏感", map[string]string{"Read": "read_file"}, "read", "read_file"},
		{"别名值写错大小写", map[string]string{"Edit": "WRITE_FILE"}, "Edit", "write_file"},
		{"别名指向不存在的工具", map[string]string{"Edit": "patch_file"}, "Edit", ""},
		{"无关工具名", nil, "ToolSearch", ""},
	}
	for _, c := range cases {
		s := newSession(c.aliases)
		var got string
		if tool := s.resolveTool(c.call); tool != nil {
			got = tool.Name()
		}
		if got != c.want {
			t.Fatalf("%s：%q 解析为 %q，want %q", c.name, c.call, got, c.want)
		}
	}
}

// 归一化后撞上两个工具就不猜。
func TestResolveToolAmbiguousDoesNotGuess(t *testing.T) {
	s := &Session{tools: []Tool{&fakeTool{name: "read_file"}, &fakeTool{name: "readfile2"}}}
	if tool := s.resolveTool("read_file"); tool == nil || tool.Name() != "read_file" {
		t.Fatalf("精确名仍应命中")
	}
	if tool := s.resolveTool("ReadFile2"); tool == nil || tool.Name() != "readfile2" {
		t.Fatalf("大小写归一化应命中 readfile2")
	}
}

// 找不到工具时的回执要能自愈：列出可用工具名 + 给一个相似建议。
func TestUnknownToolTextListsTools(t *testing.T) {
	s := &Session{tools: []Tool{
		&fakeTool{name: "read_file"}, &fakeTool{name: "bash"}, &fakeTool{name: "submit"},
	}}
	text := s.unknownToolText("Read")
	if !strings.Contains(text, "unknown tool Read") {
		t.Fatalf("应说明是哪个名字：%s", text)
	}
	if !strings.Contains(text, "read_file") || !strings.Contains(text, "bash") || !strings.Contains(text, "submit") {
		t.Fatalf("应列出本会话可用的工具：%s", text)
	}
	if !strings.Contains(text, "想找的可能是 read_file") {
		t.Fatalf("Read 应建议 read_file：%s", text)
	}
	if got := s.unknownToolText("ToolSearch"); strings.Contains(got, "想找的可能是") {
		t.Fatalf("没有相似工具时不该瞎建议：%s", got)
	}
}

// executeTool 走解析后的工具：大小写不同也照样执行，日志里记的是规范名。
func TestExecuteToolCaseInsensitive(t *testing.T) {
	s := &Session{tools: []Tool{&fakeTool{name: "bash", res: ToolResult{Text: "ok"}}}}
	res := s.executeTool(ToolCall{Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "BASH", Arguments: "{}"}})
	if res.Text != "ok" {
		t.Fatalf("大写工具名应照常执行，got %q", res.Text)
	}
	if s.ToolInvoked != 1 {
		t.Fatalf("ToolInvoked = %d", s.ToolInvoked)
	}
}
