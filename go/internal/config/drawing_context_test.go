package config

import "testing"

func TestDrawingContextDefaultAndExplicitLimits(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"absent", "{}", 256000},
		{"empty drawing", "latex:\n  sessions:\n    drawing: {}\n", 256000},
		{"explicit unlimited", "latex:\n  sessions:\n    drawing:\n      context_limit: 0\n", 0},
		{"explicit smaller", "latex:\n  sessions:\n    drawing:\n      context_limit: 64000\n", 64000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfigData([]byte(tc.body), "test.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.LatexSession("drawing").ContextLimit; got != tc.want {
				t.Fatalf("context=%d want=%d", got, tc.want)
			}
		})
	}
	cfg, err := LoadConfigData([]byte("{}"), "test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := (ModelConfig{ContextLimit: 128000}).ApplyModelLimits(cfg.LatexSession("drawing")).ContextLimit; got != 128000 {
		t.Fatalf("model limit not respected: %d", got)
	}
}
