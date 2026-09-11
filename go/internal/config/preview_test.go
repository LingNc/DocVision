package config

import (
	"strings"
	"testing"
)

// preview 是"跑 latex 时自动起会话预览服务"的开关（用户要的开关/接口/端口
// 三项）。默认必须**关闭**：没人要求的服务不该自己占端口；地址默认只本机。
func TestPreviewDefaultsAreOffAndLoopback(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if cfg.Preview.Enabled {
		t.Fatal("preview.enabled 默认必须为 false")
	}
	if cfg.Preview.Host != "127.0.0.1" {
		t.Fatalf("默认只监听本机，got %q", cfg.Preview.Host)
	}
	if cfg.Preview.Port != 8848 {
		t.Fatalf("默认端口 8848，got %d", cfg.Preview.Port)
	}
	if got := cfg.Preview.Addr(); got != "127.0.0.1:8848" {
		t.Fatalf("Addr = %q", got)
	}
	// 显式配置要原样生效。
	cfg2 := &Config{Preview: PreviewConfig{Enabled: true, Host: "0.0.0.0", Port: 9001}}
	setDefaults(cfg2)
	if got := cfg2.Preview.Addr(); got != "0.0.0.0:9001" {
		t.Fatalf("显式配置的 Addr = %q", got)
	}
	// 端口 0 = 内核挑：Addr 必须保留 0，不能悄悄换成 8848。
	cfg3 := &Config{Preview: PreviewConfig{Host: "127.0.0.1", Port: 0}}
	if got := cfg3.Preview.Addr(); got != "127.0.0.1:0" {
		t.Fatalf("端口 0 必须原样透传（由内核挑），got %q", got)
	}
}

// 模板里必须有这一块，否则 `docvision init` 生成的 config 里看不到它。
func TestPreviewBlockIsInTemplate(t *testing.T) {
	for _, want := range []string{"preview:", "  enabled: false", `host: "127.0.0.1"`, "port: 8848"} {
		if !strings.Contains(ConfigTemplate, want) {
			t.Errorf("配置模板缺少 %q", want)
		}
	}
}

// 逐图校验默认**关闭**（多一次视觉调用，多数书不需要），但一旦打开就必须有
// 可用的模型键与至少一轮校验；轮次上限不能是 0（那等于开了又什么都不做）。
func TestFigureCheckDefaultsAreOffButSane(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if cfg.Latex.FigureCheck.Enabled {
		t.Fatal("figure_check.enabled 默认必须为 false")
	}
	if cfg.Latex.FigureCheck.Model != "verifier" {
		t.Fatalf("默认校验模型应为 verifier，got %q", cfg.Latex.FigureCheck.Model)
	}
	if cfg.Latex.FigureCheck.MaxRounds != 2 {
		t.Fatalf("默认轮次上限应为 2，got %d", cfg.Latex.FigureCheck.MaxRounds)
	}
	// 显式配置不被默认值覆盖。
	cfg2 := &Config{Latex: LatexConfig{FigureCheck: FigureCheckConfig{Enabled: true, Model: "checker", MaxRounds: 5}}}
	setDefaults(cfg2)
	if !cfg2.Latex.FigureCheck.Enabled || cfg2.Latex.FigureCheck.Model != "checker" || cfg2.Latex.FigureCheck.MaxRounds != 5 {
		t.Fatalf("显式配置被默认值改写了: %+v", cfg2.Latex.FigureCheck)
	}
}
