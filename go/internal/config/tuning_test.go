package config

import "testing"

// TestCheckerSessionInheritsConvert pins the documented checker
// semantics: unset fields fall back to the convert block instead of the
// generic built-in defaults.
func TestCheckerSessionInheritsConvert(t *testing.T) {
	cfg := &Config{}
	cfg.Latex.Sessions.Convert = SessionTuning{
		ContextLimit: 64000, MaxToolRounds: 33, MaxTokens: 4096,
		Temperature: 0.2, CompactionAt: 0.5,
	}
	cfg.Latex.Sessions.Checker = SessionTuning{MaxTokens: 512}
	setDefaults(cfg)

	got := cfg.LatexSession("checker")
	if got.MaxTokens != 512 {
		t.Errorf("MaxTokens = %d, want explicit 512", got.MaxTokens)
	}
	if got.ContextLimit != 64000 {
		t.Errorf("ContextLimit = %d, want inherited 64000", got.ContextLimit)
	}
	if got.MaxToolRounds != 33 {
		t.Errorf("MaxToolRounds = %d, want inherited 33", got.MaxToolRounds)
	}
	if got.CompactionAt != 0.5 {
		t.Errorf("CompactionAt = %v, want inherited 0.5", got.CompactionAt)
	}
	if got.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want inherited 0.2", got.Temperature)
	}
}

// TestStyleSessionBuiltinBudget keeps the larger style default in code
// so book.go no longer needs a hardcoded override.
func TestStyleSessionBuiltinBudget(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if got := cfg.LatexSession("style").MaxTokens; got != 32768 {
		t.Errorf("style MaxTokens = %d, want 32768", got)
	}
	if got := cfg.LatexSession("convert").MaxTokens; got != 16384 {
		t.Errorf("convert MaxTokens = %d, want 16384", got)
	}
}

func TestModelStreamDefaultsAndInheritance(t *testing.T) {
	cfg := &Config{}
	cfg.Models = map[string]ModelConfig{
		"text": {
			Model:           "base",
			Thinking:        map[string]any{"type": "disabled"},
			ReasoningEffort: "low",
		},
		"drawing": {Model: "draw"},
	}
	setDefaults(cfg)

	base, _ := cfg.ResolveModel("text")
	if !base.Streaming() {
		t.Error("stream must default to true")
	}
	draw, _ := cfg.ResolveModel("drawing")
	if !draw.Streaming() {
		t.Error("stream must be inherited as true")
	}
	if draw.Thinking["type"] != "disabled" || draw.ReasoningEffort != "low" {
		t.Errorf("thinking/effort not inherited: %#v %q", draw.Thinking, draw.ReasoningEffort)
	}

	off := false
	entry := cfg.Models["drawing"]
	entry.Stream = &off
	entry.Thinking = map[string]any{"type": "enabled"}
	cfg.Models["drawing"] = entry
	draw, _ = cfg.ResolveModel("drawing")
	if draw.Streaming() {
		t.Error("explicit stream: false must win")
	}
	if draw.Thinking["type"] != "enabled" {
		t.Errorf("explicit thinking must win: %#v", draw.Thinking)
	}
}

func TestSemanticChecksRejectBadThinkingAndEffort(t *testing.T) {
	cfg := &Config{
		Models: map[string]ModelConfig{
			"text": {
				BaseURL:         "http://x",
				APIKey:          "sk-real",
				Model:           "m",
				ReasoningEffort: "ultra",
				Thinking:        map[string]any{"type": "maybe"},
			},
		},
		Mineru:  MinerUConfig{APIBaseURL: "http://y", Token: "t", PollInterval: 1, MaxConcurrent: 1},
		Options: OptionsConfig{Concurrency: 1, MaxTokens: 1},
		Paths:   PathsConfig{InputDir: "in", OutputDir: "out", FinallyDir: "fin", LogsDir: "logs"},
	}
	problems := semanticChecks(cfg)
	joined := ""
	for _, p := range problems {
		joined += p + "\n"
	}
	for _, want := range []string{"reasoning_effort", "thinking.type"} {
		if !contains(joined, want) {
			t.Errorf("missing problem for %s in:\n%s", want, joined)
		}
	}
}
