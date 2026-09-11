package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeThinkingCfg(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestThinkingTypeTypoIsRepairedAndWarns: 现场写的是 `disable`，服务端报
// `unknown variant \`disable\`, expected one of \`adaptive\`, \`enabled\`, \`disabled\“
// 并拒掉每一个请求——8/8 张图全部"保留原图"。这种一眼能认出的笔误区动
// 纠正成 disabled，并在 stderr 告警（配置照旧能跑）。
func TestThinkingTypeTypoIsRepairedAndWarns(t *testing.T) {
	p := writeThinkingCfg(t, `
config_version: 7
models:
  drawing: { model: m, base_url: http://x/v1, thinking: { type: disable } }
  convert: { model: m, base_url: http://x/v1, thinking: { type: ENABLE } }
`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Models["drawing"].Thinking["type"]; got != "disabled" {
		t.Errorf("drawing thinking.type = %v, want disabled", got)
	}
	if got := cfg.Models["convert"].Thinking["type"]; got != "enabled" {
		t.Errorf("convert thinking.type = %v, want enabled（大小写不敏感）", got)
	}
}

// TestThinkingTypeUnknownIsFatal: 认不出的取值直接报错——宁可启动失败，也
// 不要跑一整轮全是 fallback。
func TestThinkingTypeUnknownIsFatal(t *testing.T) {
	p := writeThinkingCfg(t, `
config_version: 7
models:
  drawing: { model: m, base_url: http://x/v1, thinking: { type: disableddd } }
`)
	_, err := LoadConfig(p)
	if err == nil {
		t.Fatal("未知 thinking.type 应当报错")
	}
	for _, want := range []string{"models.drawing.thinking.type", "disableddd", "enabled / disabled / adaptive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息里应含 %q，实际: %v", want, err)
		}
	}
}

// TestThinkingTypeValidValuesKeepWorking: adaptive / enabled / disabled 原样通过。
func TestThinkingTypeValidValuesKeepWorking(t *testing.T) {
	p := writeThinkingCfg(t, `
config_version: 7
models:
  text: { model: m, base_url: http://x/v1, thinking: { type: adaptive } }
  style: { model: m, base_url: http://x/v1, thinking: { enabled: true } }
`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Models["text"].Thinking["type"]; got != "adaptive" {
		t.Errorf("adaptive 应原样保留, got %v", got)
	}
	// 没有 type 键的（GLM 的 enabled:true 写法）不动它。
	if got := cfg.Models["style"].Thinking["enabled"]; got != true {
		t.Errorf("没有 type 键时不该改动 thinking, got %v", got)
	}
}
