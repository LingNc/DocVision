package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEstimateDefaults 钉住 estimate 块的代码默认值：两种方法共用一套参数，
// 只写一个键时其余按默认补齐。
func TestEstimateDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	e := cfg.Estimate
	if e.Method != EstimateMethodPixels || e.Tokens != 1100 || e.PxPerToken != 750 || e.MinTokens != 85 || e.MaxTokens != 4096 {
		t.Fatalf("estimate 默认值 = %+v", e)
	}
}

// TestResolveImageEstimatePerModel 钉住"同一个模型可以单独配一套，未写的键继承
// 全局、没配的模型完全跟全局走"这条解析顺序。
func TestResolveImageEstimatePerModel(t *testing.T) {
	cfg := &Config{
		Estimate: EstimateConfig{Method: EstimateMethodPixels, Tokens: 1100, PxPerToken: 750, MinTokens: 85, MaxTokens: 4096},
		Models: map[string]ModelConfig{
			"text": {Model: "deepseek-v4.1-flash"},
			// 这个端点按张固定计费，只写自己要改的键。
			"drawing": {Model: "glm-5.3-flash", ImageTokens: &EstimateConfig{Method: EstimateMethodFixed, Tokens: 1050}},
			// 这个端点像素上限更高，其余继承全局。
			"verifier": {Model: "glm-5.3-flash-official", ImageTokens: &EstimateConfig{MaxTokens: 8192}},
		},
	}

	// 没配的条目：完全跟全局走。
	if got := cfg.ResolveImageEstimate("text"); got.Method != EstimateMethodPixels || got.PxPerToken != 750 || got.MaxTokens != 4096 {
		t.Fatalf("未配置条目的规则 = %+v，期望继承全局", got)
	}
	// 条目名不是配置里的键（查看器按 wire id 问）时也跟全局走。
	if got := cfg.ResolveImageEstimate("some-other-wire-model"); got.PxPerToken != 750 {
		t.Fatalf("未知模型的规则 = %+v，期望继承全局", got)
	}
	// 单独配了方法的条目：方法/参数用自己那份，没写的键仍是全局的。
	own := cfg.ResolveImageEstimate("drawing")
	if own.Method != EstimateMethodFixed || own.Tokens != 1050 {
		t.Fatalf("drawing 规则 = %+v，期望 fixed 1050", own)
	}
	if own.PxPerToken != 750 || own.MinTokens != 85 || own.MaxTokens != 4096 {
		t.Fatalf("drawing 未写的键应继承全局：%+v", own)
	}
	// 只改一个键：其余全是全局的。
	partial := cfg.ResolveImageEstimate("verifier")
	if partial.Method != EstimateMethodPixels || partial.MaxTokens != 8192 || partial.MinTokens != 85 {
		t.Fatalf("verifier 规则 = %+v，期望只覆盖 max_tokens", partial)
	}
	// 改全局不影响单独配置的条目。
	cfg.Estimate.PxPerToken = 500
	if got := cfg.ResolveImageEstimate("drawing"); got.PxPerToken != 500 {
		t.Fatalf("继承值没跟着全局走：%+v", got)
	}
	if got := cfg.ResolveImageEstimate("drawing"); got.Method != EstimateMethodFixed {
		t.Fatalf("单独配置的方法被全局覆盖了：%+v", got)
	}
}

// TestHasRetiredEstimateKeys 钉住"旧键不再生效时必须出声"：LoadConfig 对未知键
// 本身不严格，若不说一声，用户改了 image_px_per_token 却看到估算按默认值跑，无从下手。
func TestHasRetiredEstimateKeys(t *testing.T) {
	if !hasRetiredEstimateKeys([]byte("estimate:\n  image_px_per_token: 900\n")) {
		t.Error("旧键 image_px_per_token 没被认出来")
	}
	if !hasRetiredEstimateKeys([]byte("models:\n  drawing:\n    image_tokens_max: 512\n")) {
		t.Error("旧键 image_tokens_max 没被认出来")
	}
	if hasRetiredEstimateKeys([]byte("estimate:\n  method: pixels\n  px_per_token: 750\n")) {
		t.Error("新键被误判为旧键")
	}
}

// TestEstimateConfigFile 钉住配置文件的读入：全局 estimate 与
// models.<名>.image_tokens 都能写，0/空表示未设。
func TestEstimateConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
mineru:
  token: "abc"
estimate:
  method: fixed
  tokens: 900
models:
  text:
    base_url: "https://example.com/v1"
    api_key: "sk-test"
    model: "wire-default"
  drawing:
    model: "glm-5.3-flash"
    image_tokens:
      method: pixels
      px_per_token: 780
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Estimate.Method != EstimateMethodFixed || cfg.Estimate.Tokens != 900 {
		t.Fatalf("全局 estimate = %+v", cfg.Estimate)
	}
	if cfg.Estimate.PxPerToken != 750 || cfg.Estimate.MaxTokens != 4096 {
		t.Fatalf("全局未写的键应补默认：%+v", cfg.Estimate)
	}
	got := cfg.ResolveImageEstimate("drawing")
	if got.Method != EstimateMethodPixels || got.PxPerToken != 780 {
		t.Fatalf("drawing = %+v，期望 pixels 780", got)
	}
	// 全局是 fixed 900，drawing 没写 tokens ⇒ 继承 900（fixed 模式下的"取不到
	// 尺寸"也用这个值）。
	if got.Tokens != 900 {
		t.Fatalf("drawing.tokens = %d，期望继承全局 900", got.Tokens)
	}
	// 没配 image_tokens 的条目仍按全局的 fixed 900 算。
	if got := cfg.ResolveImageEstimate("text"); got.Method != EstimateMethodFixed || got.Tokens != 900 {
		t.Fatalf("text = %+v，期望继承全局 fixed 900", got)
	}
}

// TestValidateDataEstimateMethod 钉住非法 method 是配置错误（全局与按模型都要
// 检查），并且上下限写反也要报——不能静默按默认值跑。
func TestValidateDataEstimateMethod(t *testing.T) {
	base := "config_version: " + itoa(CurrentConfigVersion) + "\nmineru:\n  token: \"abc\"\n"
	cases := []struct {
		name string
		body string
		want string
	}{
		{"全局 method 非法", "estimate:\n  method: pixel\n", "estimate.method 必须为 fixed/pixels/none"},
		{"按模型 method 非法", "models:\n  drawing:\n    image_tokens:\n      method: per-image\n", "models.drawing.image_tokens.method 必须为 fixed/pixels/none"},
		{"上下限写反", "estimate:\n  min_tokens: 400\n  max_tokens: 100\n", "estimate.max_tokens 不能小于 min_tokens"},
		{"按模型上下限写反", "models:\n  drawing:\n    image_tokens:\n      min_tokens: 900\n      max_tokens: 100\n", "models.drawing.image_tokens.max_tokens 不能小于 min_tokens"},
		{"参数为负", "estimate:\n  px_per_token: -1\n", "estimate 的 tokens/px_per_token/min_tokens/max_tokens 不能为负"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := strings.Join(ValidateData([]byte(base+c.body)), "\n")
			if !strings.Contains(problems, c.want) {
				t.Fatalf("problems = %q，期望包含 %q", problems, c.want)
			}
		})
	}
	// 三个合法取值都不该报问题。
	for _, m := range []string{EstimateMethodFixed, EstimateMethodPixels, EstimateMethodNone} {
		problems := strings.Join(ValidateData([]byte(base+"estimate:\n  method: "+m+"\n")), "\n")
		if strings.Contains(problems, "estimate") {
			t.Fatalf("合法 method %q 被判错：%q", m, problems)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
