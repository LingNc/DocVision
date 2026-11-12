package config

import "testing"

// T327：终审会话没配轮次时默认 256（用户指定），其余字段仍继承 convert；
// 显式写值/负数照旧生效。
func TestFinalReviewSessionRounds(t *testing.T) {
	var c Config
	c.Latex.Sessions.Convert.MaxToolRounds = 128
	c.Latex.Sessions.Convert.MaxTokens = 32768

	got := c.LatexSession("final-review")
	if got.MaxToolRounds != 256 {
		t.Fatalf("默认轮次应为 256，实际 %d", got.MaxToolRounds)
	}
	if got.MaxTokens != 32768 {
		t.Fatalf("其余字段应继承 convert 的 32768，实际 %d", got.MaxTokens)
	}

	// 显式配置优先。
	c.Latex.Sessions.FinalReview = SessionTuning{MaxToolRounds: 400}
	if got := c.LatexSession("final-review"); got.MaxToolRounds != 400 {
		t.Fatalf("显式 400 未生效，实际 %d", got.MaxToolRounds)
	}
	// 负数是"真正不限制"，不能被默认值覆盖。
	c.Latex.Sessions.FinalReview = SessionTuning{MaxToolRounds: -1}
	if got := c.LatexSession("final-review"); got.MaxToolRounds != -1 {
		t.Fatalf("显式 -1（不限制）被覆盖了：%d", got.MaxToolRounds)
	}
}
