package latex

import (
	"regexp"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/prompts"
)

// renderPrompt 必须把 {OUTPUT_LANG} 与 {MAX_ROUNDS} 都填掉：历史上
// tikz 会话直接传了裸常量，占位符原样发给了模型。
func TestRenderPromptFillsPlaceholders(t *testing.T) {
	tuning := config.SessionTuning{MaxToolRounds: 42}
	got := renderPrompt(prompts.Must(prompts.FigureSystem), tuning, "Chinese")
	if strings.Contains(got, "{OUTPUT_LANG}") || strings.Contains(got, "{MAX_ROUNDS}") {
		t.Fatalf("占位符未被替换")
	}
	if !strings.Contains(got, "42") {
		t.Errorf("未写入轮次预算")
	}
	if !strings.Contains(got, "Chinese") {
		t.Errorf("未写入输出语言")
	}
	// 语言为空时回落到中文（与 img2text 的默认一致）
	if got := renderPrompt("x {OUTPUT_LANG}", tuning, ""); got != "x Chinese" {
		t.Errorf("空语言回落 = %q", got)
	}
}

// 所有会话系统提示词都不得残留未渲染占位符（按注册表声明的变量全量渲染）。
func TestAllSessionPromptsRenderable(t *testing.T) {
	ph := regexp.MustCompile(`\{[A-Z][A-Z_]{2,}\}`)
	for _, tpl := range prompts.Templates() {
		vars := map[string]string{}
		for _, v := range tpl.Vars {
			vars[v] = "X"
		}
		rendered := renderPrompt(prompts.Render(tpl.Name, vars), config.SessionTuning{MaxToolRounds: 5}, "Chinese")
		if left := ph.FindAllString(rendered, -1); len(left) > 0 {
			t.Errorf("模板 %s 渲染后残留 %v", tpl.Name, left)
		}
	}
}
