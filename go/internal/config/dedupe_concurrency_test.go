package config

import "testing"

// TestDedupeConcurrencyDefault 钉住"0 是有效取值"这条：只有 YAML 里没有这个键
// 时才补默认 2，显式写 0 必须保持 0（调用方按串行处理）。
func TestDedupeConcurrencyDefault(t *testing.T) {
	base := "config_version: " + itoa(CurrentConfigVersion) + "\nmineru:\n  token: \"abc\"\n"
	cases := []struct {
		name string
		body string
		want int
	}{
		{"未写 = 默认 2", "", 2},
		{"显式 0 = 串行", "latex:\n  dedupe_concurrency: 0\n", 0},
		{"显式 3", "latex:\n  dedupe_concurrency: 3\n", 3},
		{"显式 1 = 串行", "latex:\n  dedupe_concurrency: 1\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := LoadConfigData([]byte(base+c.body), "test.yaml")
			if err != nil {
				t.Fatalf("LoadConfigData: %v", err)
			}
			if got := cfg.Latex.DedupeConcurrency; got != c.want {
				t.Fatalf("dedupe_concurrency = %d，期望 %d", got, c.want)
			}
		})
	}
}
