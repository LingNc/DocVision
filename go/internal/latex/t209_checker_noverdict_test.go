package latex

import "testing"

// T209：只有"核对会话自己没给结论"的失败才跳过反馈轮——真正的核对结论
// （哪怕是 issues 文本）照旧喂回去修。
func TestCheckerNoVerdict(t *testing.T) {
	noVerdict := []string{
		"checker 多次未能提交结论（会话未调用 submit）",
		"checker 多次未能提交结论",
		"checker API failure: 502 bad gateway",
	}
	for _, s := range noVerdict {
		if !checkerNoVerdict(s) {
			t.Errorf("checkerNoVerdict(%q) = false, want true", s)
		}
	}
	real := []string{
		"",
		"1. Missing Content: Examples 8.29, 8.30 are absent.",
		"2. The table on p58 is split across two pages without a continued caption.",
	}
	for _, s := range real {
		if checkerNoVerdict(s) {
			t.Errorf("checkerNoVerdict(%q) = true, want false（这是真正的核对结论）", s)
		}
	}
}
