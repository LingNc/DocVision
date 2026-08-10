package config

import (
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
		cfg.Options.MermaidFixAttempts != 2 || cfg.Options.MermaidTimeout != 30 {
		t.Errorf("Mermaid defaults = mode=%q command=%q attempts=%d timeout=%d",
			cfg.Options.MermaidValidation, cfg.Options.MermaidCommand,
			cfg.Options.MermaidFixAttempts, cfg.Options.MermaidTimeout)
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
  max_context_lines_up: 20
  max_context_lines_down: 8
  max_window_up: 100
  max_window_down: 100
  max_retries: 7
  api_timeout: 500
  api_connect_timeout: 90
  api_max_retries: 5
  rate_limit_retries: 0
  concurrency: 20
  temperature: 0.5
  output_language: "English"
  format_fix_attempts: 2
  mermaid_validation: "strict"
  mermaid_command: "custom-mmdc"
  mermaid_fix_attempts: 4
  mermaid_timeout: 45
  max_tokens: 32768
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
		cfg.Options.MermaidFixAttempts != 4 || cfg.Options.MermaidTimeout != 45 {
		t.Errorf("Mermaid overrides = mode=%q command=%q attempts=%d timeout=%d",
			cfg.Options.MermaidValidation, cfg.Options.MermaidCommand,
			cfg.Options.MermaidFixAttempts, cfg.Options.MermaidTimeout)
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
