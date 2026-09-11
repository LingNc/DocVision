package config

import (
	"os"
	"testing"
)

// 价格表按"wire 模型名"建索引：转录里的 t="usage" 行记的是发给厂商的模型名
// （不是注册表键名），所以 models.<条目>.price 必须用该条目的 model 字段索引，
// 留空则继承 models.text 的模型名。
func TestModelPricesKeyedByWireModelName(t *testing.T) {
	src := []byte(`
config_version: 6
models:
  text:
    base_url: "https://x/v1"
    api_key: "sk-x"
    model: "wire-default"
    price:
      input: 1.0
      cached: 0.1
      output: 4.0
  big:
    model: "wire-big"
    price: {input: 2.0, cached: 0.2, output: 8.0, currency: "$"}
  noprice:
    model: "wire-free"
`)
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	prices := cfg.ModelPrices()
	if len(prices) != 2 {
		t.Fatalf("应只有两个配了价的条目，got %d: %+v", len(prices), prices)
	}
	p, ok := prices["wire-default"]
	if !ok || p.Input != 1.0 || p.Cached != 0.1 || p.Output != 4.0 {
		t.Fatalf("text 条目的价格没按 model 名索引: %+v", prices)
	}
	if p.Currency != "¥" {
		t.Fatalf("配了价格时货币应有默认值 ¥，got %q", p.Currency)
	}
	if p2, ok := prices["wire-big"]; !ok || p2.Currency != "$" {
		t.Fatalf("显式货币应保留: %+v", prices)
	}
	if _, ok := prices["wire-free"]; ok {
		t.Fatal("没配价格的条目不该出现在价格表里")
	}
}
