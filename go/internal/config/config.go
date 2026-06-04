// Package config loads, validates, and provides defaults for the
// DocVision configuration file (config.yaml). Field names and defaults
// mirror the Python implementation exactly.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration loaded from config.yaml.
type Config struct {
	Mineru  MinerUConfig  `yaml:"mineru"`
	AI      AIConfig      `yaml:"ai"`
	Options OptionsConfig `yaml:"options"`
	Paths   PathsConfig   `yaml:"paths"`
}

// MinerUConfig holds MinerU API client settings.
type MinerUConfig struct {
	APIBaseURL        string `yaml:"api_base_url"`
	Token             string `yaml:"token"`
	ModelVersion      string `yaml:"model_version"`
	IsOCR             bool   `yaml:"is_ocr"`
	EnableFormula     bool   `yaml:"enable_formula"`
	EnableTable       bool   `yaml:"enable_table"`
	Language          string `yaml:"language"`
	PollInterval      int    `yaml:"poll_interval"`
	LogPollInterval   int    `yaml:"log_poll_interval"`
	PollTimeout       int    `yaml:"poll_timeout"`
	ProgressThreshold int    `yaml:"progress_threshold"`
	MaxPagesPerPart   int    `yaml:"max_pages_per_part"`
	MaxSizeMB         int    `yaml:"max_size_mb"`
	MaxConcurrent     int    `yaml:"max_concurrent"`
	UploadTimeout     int    `yaml:"upload_timeout"`
}

// AIConfig holds OpenAI-compatible image-to-text client settings.
type AIConfig struct {
	BaseURL     string                 `yaml:"base_url"`
	APIKey      string                 `yaml:"api_key"`
	Model       string                 `yaml:"model"`
	RequestBody map[string]interface{} `yaml:"request_body"`
}

// OptionsConfig holds tuning knobs for the image-to-text processing pipeline.
type OptionsConfig struct {
	MaxContextLinesUp   int     `yaml:"max_context_lines_up"`
	MaxContextLinesDown int     `yaml:"max_context_lines_down"`
	MaxWindowUp         int     `yaml:"max_window_up"`
	MaxWindowDown       int     `yaml:"max_window_down"`
	MaxRetries          int     `yaml:"max_retries"`
	APITimeout          int     `yaml:"api_timeout"`
	APIConnectTimeout   int     `yaml:"api_connect_timeout"`
	APIMaxRetries       int     `yaml:"api_max_retries"`
	RateLimitRetries    int     `yaml:"rate_limit_retries"`
	Concurrency         int     `yaml:"concurrency"`
	Temperature         float64 `yaml:"temperature"`
	OutputLanguage      string  `yaml:"output_language"`
	FormatFixAttempts   int     `yaml:"format_fix_attempts"`
	MaxTokens           int     `yaml:"max_tokens"`
}

// PathsConfig holds directory locations used by the workflow.
type PathsConfig struct {
	InputDir     string `yaml:"input_dir"`
	SplitDir     string `yaml:"split_dir"`
	MineruOutput string `yaml:"mineru_output"`
	OutputDir    string `yaml:"output_dir"`
	ImagesDir    string `yaml:"images_dir"`
	FinallyDir   string `yaml:"finally_dir"`
}

// LoadConfig reads the YAML file at path, applies defaults for any
// zero-valued fields, and returns the resulting Config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	setDefaults(cfg)
	return cfg, nil
}

// setDefaults fills in zero-valued fields with the defaults that the
// Python implementation applies via dict.get(key, default).
func setDefaults(cfg *Config) {
	// MinerU defaults
	if cfg.Mineru.APIBaseURL == "" {
		cfg.Mineru.APIBaseURL = "https://mineru.net/api/v4"
	}
	if cfg.Mineru.ModelVersion == "" {
		cfg.Mineru.ModelVersion = "vlm"
	}
	if !cfg.Mineru.IsOCR {
		cfg.Mineru.IsOCR = true
	}
	if !cfg.Mineru.EnableFormula {
		cfg.Mineru.EnableFormula = true
	}
	if !cfg.Mineru.EnableTable {
		cfg.Mineru.EnableTable = true
	}
	if cfg.Mineru.Language == "" {
		cfg.Mineru.Language = "ch"
	}
	if cfg.Mineru.PollInterval == 0 {
		cfg.Mineru.PollInterval = 3
	}
	if cfg.Mineru.LogPollInterval == 0 {
		cfg.Mineru.LogPollInterval = 3
	}
	if cfg.Mineru.ProgressThreshold == 0 {
		cfg.Mineru.ProgressThreshold = 80
	}
	if cfg.Mineru.MaxPagesPerPart == 0 {
		cfg.Mineru.MaxPagesPerPart = 200
	}
	if cfg.Mineru.MaxSizeMB == 0 {
		cfg.Mineru.MaxSizeMB = 200
	}
	if cfg.Mineru.MaxConcurrent == 0 {
		cfg.Mineru.MaxConcurrent = 5
	}
	if cfg.Mineru.UploadTimeout == 0 {
		cfg.Mineru.UploadTimeout = 300
	}

	// Options defaults
	if cfg.Options.MaxContextLinesUp == 0 {
		cfg.Options.MaxContextLinesUp = 10
	}
	if cfg.Options.MaxContextLinesDown == 0 {
		cfg.Options.MaxContextLinesDown = 5
	}
	if cfg.Options.MaxWindowUp == 0 {
		cfg.Options.MaxWindowUp = 50
	}
	if cfg.Options.MaxWindowDown == 0 {
		cfg.Options.MaxWindowDown = 50
	}
	if cfg.Options.MaxRetries == 0 {
		cfg.Options.MaxRetries = 5
	}
	if cfg.Options.APITimeout == 0 {
		cfg.Options.APITimeout = 400
	}
	if cfg.Options.APIConnectTimeout == 0 {
		cfg.Options.APIConnectTimeout = 60
	}
	if cfg.Options.APIMaxRetries == 0 {
		cfg.Options.APIMaxRetries = 3
	}
	if cfg.Options.Concurrency == 0 {
		cfg.Options.Concurrency = 10
	}
	if cfg.Options.Temperature == 0 {
		cfg.Options.Temperature = 0.10
	}
	if cfg.Options.OutputLanguage == "" {
		cfg.Options.OutputLanguage = "Chinese"
	}
	if cfg.Options.FormatFixAttempts == 0 {
		cfg.Options.FormatFixAttempts = 1
	}
	if cfg.Options.MaxTokens == 0 {
		cfg.Options.MaxTokens = 65536
	}

	// Paths defaults
	if cfg.Paths.InputDir == "" {
		cfg.Paths.InputDir = "./files"
	}
	if cfg.Paths.SplitDir == "" {
		cfg.Paths.SplitDir = "./split_files"
	}
	if cfg.Paths.MineruOutput == "" {
		cfg.Paths.MineruOutput = "./mineru_output"
	}
	if cfg.Paths.OutputDir == "" {
		cfg.Paths.OutputDir = "./output"
	}
	if cfg.Paths.ImagesDir == "" {
		cfg.Paths.ImagesDir = "./output/images"
	}
	if cfg.Paths.FinallyDir == "" {
		cfg.Paths.FinallyDir = "./finally"
	}
}
