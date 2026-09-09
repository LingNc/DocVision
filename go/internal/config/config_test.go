package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Minimal YAML that omits every defaultable field.
	minimal := `
mineru:
  token: "abc"
ai:
  base_url: "https://example.com/v1"
  api_key: "sk-test"
  model: "test-model"
`
	if err := os.WriteFile(path, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Spot-check a few defaults from each section.
	if cfg.Mineru.APIBaseURL != "https://mineru.net/api/v4" {
		t.Errorf("APIBaseURL default = %q", cfg.Mineru.APIBaseURL)
	}
	if cfg.Mineru.ModelVersion != "vlm" {
		t.Errorf("ModelVersion default = %q", cfg.Mineru.ModelVersion)
	}
	if !cfg.Mineru.IsOCR {
		t.Error("IsOCR default should be true")
	}
	if cfg.Mineru.MaxConcurrent != 5 {
		t.Errorf("MaxConcurrent default = %d", cfg.Mineru.MaxConcurrent)
	}
	if cfg.Options.MaxContextLinesUp != 10 {
		t.Errorf("MaxContextLinesUp default = %d", cfg.Options.MaxContextLinesUp)
	}
	if cfg.Options.Temperature != 0.10 {
		t.Errorf("Temperature default = %v", cfg.Options.Temperature)
	}
	if cfg.Options.MaxTokens != 65536 {
		t.Errorf("MaxTokens default = %d", cfg.Options.MaxTokens)
	}
	if cfg.Options.MermaidValidation != "auto" || cfg.Options.MermaidCommand != "mmdc" ||
		cfg.Options.MermaidFixAttempts == nil || *cfg.Options.MermaidFixAttempts != 3 || cfg.Options.MermaidTimeout != 30 {
		var attempts int
		if cfg.Options.MermaidFixAttempts != nil {
			attempts = *cfg.Options.MermaidFixAttempts
		}
		t.Errorf("Mermaid defaults = mode=%q command=%q attempts=%d timeout=%d",
			cfg.Options.MermaidValidation, cfg.Options.MermaidCommand,
			attempts, cfg.Options.MermaidTimeout)
	}
	if cfg.Paths.InputDir != "./files" {
		t.Errorf("InputDir default = %q", cfg.Paths.InputDir)
	}
	if cfg.Paths.FinallyDir != "./finally" {
		t.Errorf("FinallyDir default = %q", cfg.Paths.FinallyDir)
	}
	if cfg.Paths.LogsDir != "./logs" {
		t.Errorf("LogsDir default = %q", cfg.Paths.LogsDir)
	}
}

func TestLoadConfig_PreservesExplicitValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mineru:
  api_base_url: "https://custom.api/v1"
  token: "abc"
  model_version: "pipeline"
  is_ocr: false
  enable_formula: false
  enable_table: false
  language: "en"
  poll_interval: 7
  log_poll_interval: 9
  poll_timeout: 0
  progress_threshold: 50
  max_pages_per_part: 150
  max_size_mb: 100
  max_concurrent: 3
  upload_timeout: 600
ai:
  base_url: "https://custom.api/v1"
  api_key: "sk-test"
  model: "m"
  request_body:
    enable_thinking: true
options:
  concurrency: 20
  temperature: 0.5
  output_language: "English"
  format_fix_attempts: 2
  max_tokens: 32768
tools:
  context:
    initial_up: 20
    initial_down: 8
    max_up: 100
    max_down: 100
    max_calls: 7
  mermaid:
    validation: "strict"
    command: "custom-mmdc"
    fix_attempts: 4
    timeout: 45
paths:
  input_dir: "./in"
  split_dir: "./split"
  mineru_output: "./m_out"
  output_dir: "./out"
  images_dir: "./out/img"
  finally_dir: "./fin"
  logs_dir: "./logz"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Mineru.APIBaseURL != "https://custom.api/v1" {
		t.Errorf("APIBaseURL overridden = %q", cfg.Mineru.APIBaseURL)
	}
	if cfg.Options.Temperature != 0.5 {
		t.Errorf("Temperature overridden = %v", cfg.Options.Temperature)
	}
	if cfg.Options.MermaidValidation != "strict" || cfg.Options.MermaidCommand != "custom-mmdc" ||
		cfg.Options.MermaidFixAttempts == nil || *cfg.Options.MermaidFixAttempts != 4 || cfg.Options.MermaidTimeout != 45 {
		var attempts int
		if cfg.Options.MermaidFixAttempts != nil {
			attempts = *cfg.Options.MermaidFixAttempts
		}
		t.Errorf("Mermaid overrides = mode=%q command=%q attempts=%d timeout=%d",
			cfg.Options.MermaidValidation, cfg.Options.MermaidCommand,
			attempts, cfg.Options.MermaidTimeout)
	}
	if cfg.Paths.FinallyDir != "./fin" {
		t.Errorf("FinallyDir overridden = %q", cfg.Paths.FinallyDir)
	}
	if cfg.Paths.LogsDir != "./logz" {
		t.Errorf("LogsDir overridden = %q", cfg.Paths.LogsDir)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig("/nonexistent/config.yaml"); err == nil {
		t.Error("expected error for missing file")
	}
}

// TestLoadConfig_DoneDirDefault confirms DoneDir defaults to a
// sub-directory of InputDir (anchored to the defaulted value).
func TestLoadConfig_DoneDirDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("mineru:\n  token: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := filepath.Join(cfg.Paths.InputDir, "done")
	if cfg.Paths.DoneDir != want {
		t.Fatalf("DoneDir default = %q, want %q", cfg.Paths.DoneDir, want)
	}
}

// TestLoadConfig_DoneDirEqualsInputDirRejected ensures the
// cross-field invariant: archiving into the same directory the
// sources live in is rejected so SplitAll can't loop forever.
func TestLoadConfig_DoneDirEqualsInputDirRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mineru:
  token: x
paths:
  input_dir: "./files"
  done_dir: "./files"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error when done_dir == input_dir")
	}
}

// TestLoadConfig_DoneDirOverride confirms an explicit
// paths.done_dir value is preserved.
func TestLoadConfig_DoneDirOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mineru:
  token: x
paths:
  done_dir: "/tmp/alt-done"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Paths.DoneDir != "/tmp/alt-done" {
		t.Fatalf("DoneDir = %q, want explicit override", cfg.Paths.DoneDir)
	}
}

// TestLoadConfig_MermaidFixAttemptsExplicitZero guards the *int semantics:
// when YAML explicitly sets mermaid_fix_attempts: 0 the loader must
// preserve the explicit zero (pointer to 0) and not silently rewrite it
// to the default. The downstream consumer (processor.go) interprets
// pointer-to-0 as "unlimited attempts, capped by safety limit".
func TestLoadConfig_MermaidFixAttemptsExplicitZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
mineru:
  token: x
tools:
  mermaid:
    fix_attempts: 0
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Options.MermaidFixAttempts == nil {
		t.Fatal("MermaidFixAttempts = nil, want non-nil pointer to 0")
	}
	if *cfg.Options.MermaidFixAttempts != 0 {
		t.Fatalf("*MermaidFixAttempts = %d, want 0", *cfg.Options.MermaidFixAttempts)
	}
}

// TestLoadConfig_MermaidFixAttemptsOmittedUsesDefault confirms the
// nil-pointer default path: when the YAML omits mermaid_fix_attempts
// entirely, the loader fills it in with a pointer to 3.
func TestLoadConfig_MermaidFixAttemptsOmittedUsesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("mineru:\n  token: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Options.MermaidFixAttempts == nil {
		t.Fatal("MermaidFixAttempts = nil, want non-nil pointer to 3")
	}
	if *cfg.Options.MermaidFixAttempts != 3 {
		t.Fatalf("*MermaidFixAttempts = %d, want 3", *cfg.Options.MermaidFixAttempts)
	}
}

// ai.model may reference a named registry entry; credentials then live
// New-style configs have no top-level ai: block; models.text is the
// single mandatory base and other entries inherit its fields.
func TestResolveModel_Inheritance(t *testing.T) {
	data := []byte(`
models:
  text:
    base_url: "https://registry.example/v1"
    api_key: "sk-registry"
    model: "glm-5.3-flash"
    temperature: 0.10
    api_timeout: 500
    rate_limit_retries: 50
  classifier:
    model: "Qwen/Qwen3.6-27B"
`)
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	setDefaults(cfg)
	mc, ok := cfg.ResolveModel("classifier")
	if !ok {
		t.Fatal("classifier should resolve")
	}
	if mc.BaseURL != "https://registry.example/v1" || mc.APIKey != "sk-registry" {
		t.Fatalf("classifier did not inherit default credentials: %+v", mc)
	}
	if mc.Model != "Qwen/Qwen3.6-27B" {
		t.Fatalf("own model must be kept: %+v", mc)
	}
	if mc.APITimeout != 500 || mc.RateLimitRetries != 50 {
		t.Fatalf("request controls must inherit from text: %+v", mc)
	}
	if mc.Temperature != 0.10 {
		t.Fatalf("temperature must inherit from text when left empty: %+v", mc)
	}
	// Unnamed requests fall back to text itself.
	mc2, _ := cfg.ResolveModel("")
	if mc2.Model != "glm-5.3-flash" {
		t.Fatalf("unnamed must resolve to text: %+v", mc2)
	}
}

// TestLatexCompileFinalReviewDefault: the final review session is on
// unless explicitly disabled.
func TestLatexCompileFinalReviewDefault(t *testing.T) {
	var c LatexCompileConfig
	if !c.FinalReviewEnabled() {
		t.Fatal("unset final_review must default to enabled")
	}
	off := false
	c.FinalReview = &off
	if c.FinalReviewEnabled() {
		t.Fatal("explicit final_review: false must disable the session")
	}
	on := true
	c.FinalReview = &on
	if !c.FinalReviewEnabled() {
		t.Fatal("explicit final_review: true must enable the session")
	}
}
