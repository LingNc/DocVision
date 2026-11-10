package config

import "testing"

// T307：重画图提交门槛的默认值——0 = 用默认（17% / 10%），负数是"关掉这条"。
// 用户配 0.25 这类显式值必须原样保留。
func TestFigureThresholdDefaults(t *testing.T) {
	cases := []struct {
		name       string
		in         float64
		wantOver   float64
		setDefault bool
	}{
		{"未写用默认", 0, 0, true},
		{"显式值保留", 0.25, 0.25, false},
		{"负数表示关闭", -1, -1, false},
	}
	for _, tc := range cases {
		cfg := &Config{}
		cfg.Latex.FigureCheck.MinOverlap = tc.in
		cfg.Latex.FigureCheck.MaxScaleDev = tc.in
		setDefaults(cfg)
		want := tc.wantOver
		if tc.setDefault {
			want = DefaultFigureMinOverlap
		}
		if cfg.Latex.FigureCheck.MinOverlap != want {
			t.Fatalf("%s：min_overlap 应为 %v，实际 %v", tc.name, want, cfg.Latex.FigureCheck.MinOverlap)
		}
		wantDev := tc.wantOver
		if tc.setDefault {
			wantDev = DefaultFigureMaxScaleDev
		}
		if cfg.Latex.FigureCheck.MaxScaleDev != wantDev {
			t.Fatalf("%s：max_scale_dev 应为 %v，实际 %v", tc.name, wantDev, cfg.Latex.FigureCheck.MaxScaleDev)
		}
	}
	if DefaultFigureMinOverlap != 0.17 || DefaultFigureMaxScaleDev != 0.10 {
		t.Fatalf("默认门槛必须是 17%% / 10%%：%v %v", DefaultFigureMinOverlap, DefaultFigureMaxScaleDev)
	}
}
