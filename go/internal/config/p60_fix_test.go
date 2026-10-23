package config

import (
	"testing"
)

// P60：修复会话的开关默认关闭、轮数默认 2、模型默认为空（= 继承 convert）。
// 这一步只落骨架：默认配置下本批代码不产生任何运行期行为。
func TestFixConfigDefaultsAndAccessors(t *testing.T) {
	var l LatexConfig
	if l.FixEnabled() {
		t.Error("latex.fix.enabled 默认必须是关闭的（P60 第一步）")
	}
	if got := l.FixMaxRounds(); got != 2 {
		t.Errorf("latex.fix.max_rounds 默认 = %d, want 2", got)
	}
	if l.Fix.Model != "" {
		t.Errorf("latex.fix.model 默认应当为空（继承 convert）: %q", l.Fix.Model)
	}

	on := true
	l.Fix = FixConfig{Enabled: &on, MaxRounds: 5, Model: "fix-entry"}
	if !l.FixEnabled() {
		t.Error("显式 enabled: true 应当生效")
	}
	if got := l.FixMaxRounds(); got != 5 {
		t.Errorf("显式 max_rounds = %d, want 5", got)
	}
	// 0/负数不是"无限轮"：照 2 算，免得一份写错的配置把子章节拖进死循环。
	off := false
	l.Fix = FixConfig{Enabled: &off, MaxRounds: 0}
	if l.FixEnabled() {
		t.Error("显式 enabled: false 应当关闭")
	}
	if got := l.FixMaxRounds(); got != 2 {
		t.Errorf("max_rounds=0 应当按默认 2: %d", got)
	}
	l.Fix.MaxRounds = -3
	if got := l.FixMaxRounds(); got != 2 {
		t.Errorf("负数 max_rounds 应当按默认 2: %d", got)
	}
}

// P60：模板里那份 fix 块真能被读进来（键面由模板一致性测试守着，这里钉值）。
func TestFixConfigFromTemplates(t *testing.T) {
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		cfg, err := LoadConfig(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if cfg.Latex.FixEnabled() {
			t.Errorf("%s: 模板里 latex.fix.enabled 应当为 false", p)
		}
		if got := cfg.Latex.FixMaxRounds(); got != 2 {
			t.Errorf("%s: latex.fix.max_rounds = %d, want 2", p, got)
		}
		if cfg.Latex.Fix.Model != "" {
			t.Errorf("%s: latex.fix.model 应当为空: %q", p, cfg.Latex.Fix.Model)
		}
		if cfg.ConfigVersion != CurrentConfigVersion {
			t.Errorf("%s: config_version = %d, want %d（新增 latex.fix 块必须 bump）", p, cfg.ConfigVersion, CurrentConfigVersion)
		}
	}
	// 新增配置块必须 bump（P60 之前是 22）；这条同时挡住"加了键又忘了 bump"。
	if CurrentConfigVersion < 23 {
		t.Errorf("CurrentConfigVersion = %d，新增 latex.fix 块后至少应为 23", CurrentConfigVersion)
	}
}
