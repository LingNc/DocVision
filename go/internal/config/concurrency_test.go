package config

import "testing"

// T179：转换与核对各自解析并发上限（0 = 沿用 concurrency，都 0 时按老默认 3）。
func TestStageWorkers(t *testing.T) {
	cases := []struct {
		name string
		l    LatexConfig
		wC   int
		wK   int
		wO   int
	}{
		{"都用 concurrency", LatexConfig{Concurrency: 3}, 3, 3, 3},
		{"各自独立", LatexConfig{Concurrency: 3, ConvertConcurrency: 2, CheckerConcurrency: 8}, 2, 8, 3},
		{"只改核对", LatexConfig{Concurrency: 3, CheckerConcurrency: 6}, 3, 6, 3},
		{"全空按老默认 3", LatexConfig{}, 3, 3, 3},
	}
	for _, c := range cases {
		if got := c.l.ConvertWorkers(); got != c.wC {
			t.Errorf("%s: ConvertWorkers=%d want %d", c.name, got, c.wC)
		}
		if got := c.l.CheckerWorkers(); got != c.wK {
			t.Errorf("%s: CheckerWorkers=%d want %d", c.name, got, c.wK)
		}
		if got := c.l.OtherWorkers(); got != c.wO {
			t.Errorf("%s: OtherWorkers=%d want %d", c.name, got, c.wO)
		}
	}
}

// T179：模型级上限按**实际下发的模型名**归并——条目名不同、模型相同就共用一个
// 额度，并且取更保守（更小）的那个；0 与空模型名都不产生上限。
func TestModelConcurrencyLimits(t *testing.T) {
	cfg := &Config{Models: map[string]ModelConfig{
		"text":    {Model: "wire-a", MaxConcurrency: 4},
		"convert": {MaxConcurrency: 6},                  // model 继承 text → wire-a，与 text 共用额度
		"checker": {Model: "wire-b", MaxConcurrency: 2}, // 另一个模型，自己的额度
		"other":   {Model: "wire-c"},                    // 没写上限
		"blank":   {Model: "", MaxConcurrency: 9},       // 没模型名，跳过
	}}
	got := cfg.ModelConcurrencyLimits()
	if len(got) != 2 {
		t.Fatalf("limits=%v want 2 entries", got)
	}
	if got["wire-a"] != 4 {
		t.Errorf("wire-a=%d want 4（同模型取最小）", got["wire-a"])
	}
	if got["wire-b"] != 2 {
		t.Errorf("wire-b=%d want 2", got["wire-b"])
	}
}
