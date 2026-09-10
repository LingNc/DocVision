package config

import "testing"

// 两个配置模板必须能被严格解析，且新键（tools.view.* 与 sessions 的软
// 限制/本地裁剪项）取到预期默认值。
func TestTemplatesLoadSoftBudgetKeys(t *testing.T) {
	for _, p := range []string{"../../../config.example.yaml", "../../../go/internal/config/default.yaml"} {
		cfg, err := LoadConfig(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if cfg.ViewImageMax() != 30 || cfg.ViewPDFMax() != 25 {
			t.Errorf("%s: view 预算 %d/%d", p, cfg.ViewImageMax(), cfg.ViewPDFMax())
		}
		if r := cfg.ViewWarnRatio(); r != 0.7 {
			t.Errorf("%s: warn_ratio %v", p, r)
		}
		tn := cfg.LatexSession("drawing")
		if tn.ToolRoundsGraceRounds() != 20 || tn.PruneToolCharsLimit() != 4096 || tn.KeepImagesCount() != 3 {
			t.Errorf("%s: tuning grace=%d prune=%d keep=%d", p, tn.ToolRoundsGraceRounds(), tn.PruneToolCharsLimit(), tn.KeepImagesCount())
		}
		if tn.ToolRoundsWarnRatio != 0.7 {
			t.Errorf("%s: round warn ratio %v", p, tn.ToolRoundsWarnRatio)
		}
	}
}
