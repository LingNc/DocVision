package latex

import (
	"testing"

	"mineru-tools/internal/config"
)

// TestCheckerNoVerdictRetriesDefault 钉住 T233 的默认值：核对没给结论时默认换
// 6 条全新核对会话（不是旧的那个"再开一次就收手"）。写 0 或负数同样取 6。
func TestCheckerNoVerdictRetriesDefault(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 6}, {-3, 6}, {1, 1}, {6, 6}, {10, 10},
	}
	for _, c := range cases {
		cfg := config.LatexConfig{CheckerNoVerdictRetryCount: c.in}
		if got := cfg.CheckerNoVerdictRetries(); got != c.want {
			t.Errorf("CheckerNoVerdictRetries(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestRescueOwnFailureDoesNotConsumeBudget 钉住 T233：复盘会话自己没给出结论时
// 不能吃掉目标的复盘次数——那一次要退回去，只单独记"复盘自己失败"。
func TestRescueOwnFailureDoesNotConsumeBudget(t *testing.T) {
	r := &Runner{cfg: &config.Config{}}
	r.cfg.Latex.Rescue.MaxPerAttempt = 1
	key := "t233/convert/rescue-own-failure.jsonl"

	if !r.rescueBudget(key) {
		t.Fatalf("第一次复盘应该还在预算内")
	}
	if r.rescueBudget(key) {
		t.Fatalf("预算 1 次用完后就该拒绝第二次")
	}
	// 复盘自己没给结论：退款 + 记一笔自己的失败，次数不能少。
	r.refundRescueBudget(key)
	if n := r.rescueOwnFailure(key); n != 1 {
		t.Fatalf("复盘自身失败应记 1 次，得到 %d", n)
	}
	if !r.rescueBudget(key) {
		t.Fatalf("退款之后应该还能再复盘一次")
	}
	// 连续自己失败攒到上限才收手。
	for i := 1; i < rescueMaxOwnFailures; i++ {
		r.rescueOwnFailure(key)
	}
	if n := r.rescueOwnFailure(key); n != rescueMaxOwnFailures+1 {
		t.Fatalf("累计次数不对: %d", n)
	}
}
