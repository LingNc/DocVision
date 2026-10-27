package session

import "testing"

// P58：模型声称"工具/工作区不可用"是幻觉，会话层要纠正一次。
func TestClaimsNoToolAccess(t *testing.T) {
	yes := []string{
		"I can convert chapter.md, but the chapter workspace tools are not available in this session.",
		"The workspace tools are unavailable, so I cannot read work:manual.md.",
		"I don't have access to the file system in this session.",
		"当前会话没有可用的工具，无法访问工作区文件。",
		"工具不可用，我无法读取章节文件。",
	}
	for _, s := range yes {
		if !claimsNoToolAccess(s) {
			t.Errorf("应识别为「声称工具不可用」：%q", s)
		}
	}
	no := []string{
		"",
		"I have read the manual and will now write the .tex file.",
		"The tools worked fine; the compile failed because of a missing package.",
		"Please submit your result with the submit tool.",
		"工作区文件已读完，开始写 .tex。",
	}
	for _, s := range no {
		if claimsNoToolAccess(s) {
			t.Errorf("不该识别为「声称工具不可用」：%q", s)
		}
	}
}

func TestToolNameList(t *testing.T) {
	s := &Session{tools: []Tool{&nameOnlyTool{name: "read_file"}, &nameOnlyTool{name: "submit"}}}
	if got := s.toolNameList(12); got != "read_file, submit" {
		t.Fatalf("toolNameList = %q", got)
	}
	if got := (&Session{}).toolNameList(3); got != "（无）" {
		t.Fatalf("空工具清单应写「（无）」，got %q", got)
	}
}

type nameOnlyTool struct{ name string }

func (f *nameOnlyTool) Name() string {
	return f.name
}
func (f *nameOnlyTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": f.name}}
}
func (f *nameOnlyTool) Execute(string) (ToolResult, error) { return ToolResult{}, nil }
