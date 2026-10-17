package config

import (
	"strings"
	"testing"
)

// 回归：pass 上限曾误用 len(extendsOf)（随 resolve 收缩），字母序排最前的
// 条目每轮都赶在父条目解析前被检查，最后一轮被砍掉 → 误报循环继承。
// 结构：bookfix 字母序在 convert 前，链深 2（bookfix → convert → heavy → text）。
func TestExtendsStarvedPassRegression(t *testing.T) {
	yamlDoc := cfgVer(`config_version: {{cfgver}}
mineru:
  api_base_url: "x"
  token: "t"
models:
  text:
    base_url: "https://x/v1"
    api_key: "k"
    model: "m"
  heavy:
    extends: "text"
  convert:
    extends: "heavy"
  bookfix:
    extends: "convert"
`)
	for _, prob := range ValidateData([]byte(yamlDoc)) {
		if strings.Contains(prob, "循环继承") {
			t.Fatalf("误报循环继承: %s", prob)
		}
	}
	// 真环仍要报出来
	bad := strings.Replace(yamlDoc, `  bookfix:
    extends: "convert"`, `  bookfix:
    extends: "bookfix"`, 1)
	found := false
	for _, prob := range ValidateData([]byte(bad)) {
		if strings.Contains(prob, "循环继承") {
			found = true
		}
	}
	if !found {
		t.Fatal("真循环未被检出")
	}
}
