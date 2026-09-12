package config

import (
	"os"
	"strings"
	"testing"
)

// 价格表按"wire 模型名"建索引：转录里的 t="usage" 行记的是发给厂商的模型名
// （不是注册表键名），所以 models.<条目>.price 必须用该条目的 model 字段索引，
// 留空则继承 models.text 的模型名。
func TestModelPricesKeyedByWireModelName(t *testing.T) {
	src := []byte(`
config_version: 10
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
	if len(prices) != 3 {
		t.Fatalf("三个条目都该有价格（noprice 继承 text 的三项费率），got %d: %+v", len(prices), prices)
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
	// 自己没写 price 的条目继承 text 的费率（否则同一个网关下的每个模型都要
	// 重复三行数字），但货币仍是随附默认的 ¥。
	if p3, ok := prices["wire-free"]; !ok || p3.Input != 1.0 || p3.Output != 4.0 || p3.Currency != "¥" {
		t.Fatalf("没写 price 的条目应继承 text 的费率: %+v", prices)
	}
}

// TestPriceNotInheritedWhenTextHasNone 钉住另一半：models.text 自己没配价格时，
// 别的条目也不会凭空得到一份（全零 = 未知，费用报告静默跳过）。
func TestPriceNotInheritedWhenTextHasNone(t *testing.T) {
	cfg := &Config{Models: map[string]ModelConfig{
		defaultModelKey: {Model: "wire-default"},
		"drawing":       {Model: "wire-draw"},
	}}
	setDefaults(cfg)
	if cfg.Models["drawing"].Price.Configured() {
		t.Fatalf("text 没配价时 drawing 不该有价: %+v", cfg.Models["drawing"].Price)
	}
	if len(cfg.ModelPrices()) != 0 {
		t.Fatalf("没配价格时价格表必须为空: %+v", cfg.ModelPrices())
	}
}

// TestDuplicateWireModelNamePriceConflict：两个条目发往同一个 wire 模型却配了
// 不同单价时，价格表只能随机取一条（map 迭代顺序不定），费用报告会在两次运行
// 之间变来变去 —— 加载期直接报错，并指出该怎么改。
func TestDuplicateWireModelNamePriceConflict(t *testing.T) {
	base := `
mineru:
  token: "abc"
models:
  text:
    base_url: "https://x/v1"
    api_key: "sk-x"
    model: "wire-default"
`
	conflict := base + `
  a:
    model: "wire-shared"
    price: {input: 1, output: 4}
  b:
    model: "wire-shared"
    price: {input: 2, output: 8}
`
	problems := strings.Join(ValidateData([]byte("config_version: "+itoa(CurrentConfigVersion)+"\n"+conflict)), "\n")
	if !strings.Contains(problems, "冲突") || !strings.Contains(problems, "wire-shared") {
		t.Fatalf("同名不同价必须报错: %q", problems)
	}
	// 加载路径同样报错（运行期不接受"随机取一条"）。
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, []byte(conflict), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig 应报价格冲突")
	}
	// 同名同价（甚至都不配价）是正常写法：共用同一个 wire 模型的两个角色。
	same := base + `
  a:
    model: "wire-shared"
    price: {input: 1, output: 4}
  b:
    model: "wire-shared"
    price: {input: 1, output: 4}
`
	if problems := strings.Join(ValidateData([]byte("config_version: "+itoa(CurrentConfigVersion)+"\n"+same)), "\n"); strings.Contains(problems, "冲突") {
		t.Fatalf("同名同价不该报错: %q", problems)
	}
}
