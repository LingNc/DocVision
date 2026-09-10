package latex

import (
	"regexp"
	"testing"

	"mineru-tools/internal/config"
)

// 内置提示词是纯字符串常量：占位符若没被替换就会原样发给模型（曾发生：
// 作图提示词里的 {OUTPUT_LANG} / {MAX_ROUNDS} 直接出现在 system prompt 里）。
// 这个测试守住所有常量。
func TestBuiltinPromptsAreRendered(t *testing.T) {
	prompts := map[string]string{
		"classifierSystemPrompt":  classifierSystemPrompt,
		"latexFigurePrompt":       latexFigurePrompt,
		"styleSystemPrompt":       styleSystemPrompt,
		"chapterSystemPrompt":     chapterSystemPrompt,
		"convertSystemPrompt":     convertSystemPrompt,
		"styleFixSystemPrompt":    styleFixSystemPrompt,
		"finalReviewSystemPrompt": finalReviewSystemPrompt,
		"fixSystemPrompt":         fixSystemPrompt,
	}
	tuning := config.SessionTuning{}
	out := renderPrompt(latexFigurePrompt, tuning, "Chinese")
	if !regexp.MustCompile(`Chinese`).MatchString(out) {
		t.Error("OUTPUT_LANG 未被替换")
	}
	ph := regexp.MustCompile(`\{[A-Z_]{3,}\}`)
	for name, body := range prompts {
		if left := ph.FindAllString(renderPrompt(body, tuning, "Chinese"), -1); len(left) > 0 {
			t.Errorf("%s 仍有未替换占位符: %v", name, left)
		}
	}
	if renderPrompt("", tuning, "Chinese") != "" {
		t.Error("空提示词应原样返回空串")
	}
	if got := renderPrompt(latexFigurePrompt, tuning, ""); ph.MatchString(got) {
		t.Error("未配置语言时应回退 Chinese 并完成替换")
	}
}
