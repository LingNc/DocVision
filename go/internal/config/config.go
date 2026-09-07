// Package config loads, validates, and provides defaults for the
// DocVision configuration file (config.yaml). This is the reference
// implementation; see legacy/python/ for the archived Python version.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration loaded from config.yaml.
type Config struct {
	Mineru  MinerUConfig  `yaml:"mineru"`
	AI      AIConfig      `yaml:"ai"`
	Options OptionsConfig `yaml:"options"`
	Paths   PathsConfig   `yaml:"paths"`
	// Models is the named model registry. Every specialised AI used by
	// the latex / verify pipelines (classifier, drawing, style, chapter,
	// convert, verifier) references an entry here by name, so each model
	// can have its own base_url / api_key / request_body. Fields left
	// empty fall back to the top-level ai block.
	Models map[string]ModelConfig `yaml:"models"`
	Latex  LatexConfig            `yaml:"latex"`
	Verify VerifyConfig           `yaml:"verify"`
}

// ModelConfig is one entry of the named model registry. Empty fields
// inherit from the top-level ai block, so a minimal setup only needs
// model: "<name>" per entry.
type ModelConfig struct {
	BaseURL     string                 `yaml:"base_url"`
	APIKey      string                 `yaml:"api_key"`
	Model       string                 `yaml:"model"`
	RequestBody map[string]interface{} `yaml:"request_body"`
	// MaxTokens / Temperature override options.max_tokens /
	// options.temperature for this model when non-zero.
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}

// SessionTuning tunes one AI session type (context window, tool budget).
type SessionTuning struct {
	// ContextLimit is the session context window in tokens. When the
	// estimated conversation size approaches this limit the session
	// auto-compacts (AI summarisation). Default 131072 (128K).
	ContextLimit int `yaml:"context_limit"`
	// MaxToolRounds caps tool-calling rounds inside one session turn.
	MaxToolRounds int `yaml:"max_tool_rounds"`
	// MaxTokens is the per-request completion budget.
	MaxTokens int `yaml:"max_tokens"`
	// Temperature for this session; 0 falls back to the global value.
	Temperature float64 `yaml:"temperature"`
	// CompactionAt is the fraction of ContextLimit that triggers
	// auto-compaction (0.0-1.0). Default 0.85.
	CompactionAt float64 `yaml:"compaction_at"`
}

// LatexCompileConfig controls the LaTeX toolchain used to compile and
// rasterise AI-generated TikZ figures and the final book.
type LatexCompileConfig struct {
	// Engine: pdflatex / xelatex / lualatex. Default xelatex.
	Engine string `yaml:"engine"`
	// Timeout for one compile run, seconds. Default 120.
	Timeout int `yaml:"timeout"`
	// RasterCommand renders PDF pages to PNG. Default pdftoppm.
	RasterCommand string `yaml:"raster_command"`
	// RasterDPI for the PNG previews fed back to the model. Default 110.
	RasterDPI int `yaml:"raster_dpi"`
	// MaxFixRounds bounds the whole compile→review→fix cycle per figure
	// (safety cap on top of the session tool budget). Default 8.
	MaxFixRounds int `yaml:"max_fix_rounds"`
}

// LatexConfig configures the LaTeX output feature (two levels).
// level 2: per-image vectorisation (classify → text / tikz / raster).
// level 1: full-book LaTeX conversion built on top of level 2 image
// handling plus style analysis, chapter splitting and assembly.
type LatexConfig struct {
	// Level selects the output tier: 1 (full book .tex) or 2
	// (markdown with vector figures). Default 2.
	Level int `yaml:"level"`
	// Model registry keys used by each specialised session. Empty
	// values fall back to the top-level ai block.
	ClassifierModel string `yaml:"classifier_model"`
	DrawingModel    string `yaml:"drawing_model"`
	StyleModel      string `yaml:"style_model"`
	ChapterModel    string `yaml:"chapter_model"`
	ConvertModel    string `yaml:"convert_model"`
	// InsertImageDescription: when true, images kept as raster (no
	// vector structure) embed a readable AI explanation as
	// "[Image]( content )" instead of the bare image link. The same
	// flag controls whether confirmed TikZ figures embed the code
	// block instead of the compiled PDF link. Default false.
	InsertImageDescription bool `yaml:"insert_image_description"`
	// OutputDir receives the level-2 markdown output. Default ./finally_latex
	OutputDir string `yaml:"output_dir"`
	// ProjectDir is the working root for level-1 book builds.
	// Default ./latex_project
	ProjectDir string `yaml:"project_dir"`
	// Concurrency for per-image and per-chapter workers. Default 3
	// (sessions are long-lived and much heavier than plain img2text calls).
	Concurrency int                `yaml:"concurrency"`
	Compile     LatexCompileConfig `yaml:"compile"`
	// Sessions tunes each specialised AI session independently.
	Sessions struct {
		Drawing SessionTuning `yaml:"drawing"`
		Style   SessionTuning `yaml:"style"`
		Chapter SessionTuning `yaml:"chapter"`
		Convert SessionTuning `yaml:"convert"`
	} `yaml:"sessions"`
}

// VerifyConfig configures the AI verification pass (核对输出的每张图与
// 对应内容). Disabled by default; when enabled it cross-checks each
// image against the embedded content and writes a report with fix
// suggestions without altering the output.
type VerifyConfig struct {
	// Enabled toggles the verification pass. Default false.
	Enabled bool `yaml:"enabled"`
	// VerifierModel is the registry key of the vision-capable
	// verification model. Empty falls back to the top-level ai block.
	VerifierModel string `yaml:"verifier_model"`
	// Concurrency of verification workers. Default 2.
	Concurrency int `yaml:"concurrency"`
	// ReportFile is written under the latex output dir. Default verify_report.md
	ReportFile string `yaml:"report_file"`
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
	MermaidValidation   string  `yaml:"mermaid_validation"`
	MermaidCommand      string  `yaml:"mermaid_command"`
	MermaidFixAttempts  *int    `yaml:"mermaid_fix_attempts"`
	MermaidTimeout      int     `yaml:"mermaid_timeout"`
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
	LogsDir      string `yaml:"logs_dir"`
	// DoneDir is where SplitAll moves source files after they have
	// been split successfully. Empty string disables archiving.
	// Default: filepath.Join(InputDir, "done").
	DoneDir string `yaml:"done_dir"`
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
	if err := validatePaths(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validatePaths enforces cross-field invariants that defaulting
// alone cannot express. The only rule currently is that DoneDir
// must not coincide with InputDir — archiving into the same
// directory would cause the next SplitAll to re-discover the
// files it just archived.
func validatePaths(cfg *Config) error {
	if cfg.Paths.InputDir == "" || cfg.Paths.DoneDir == "" {
		return nil
	}
	inAbs, err := filepath.Abs(cfg.Paths.InputDir)
	if err != nil {
		return fmt.Errorf("resolve input_dir: %w", err)
	}
	doneAbs, err := filepath.Abs(cfg.Paths.DoneDir)
	if err != nil {
		return fmt.Errorf("resolve done_dir: %w", err)
	}
	if inAbs == doneAbs {
		return fmt.Errorf("paths.done_dir (%s) must differ from paths.input_dir (%s)",
			cfg.Paths.DoneDir, cfg.Paths.InputDir)
	}
	return nil
}

// setDefaults fills in zero-valued fields with the same defaults that the
// archived Python implementation applies via dict.get(key, default).
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
	if cfg.Options.MermaidValidation == "" {
		cfg.Options.MermaidValidation = "auto"
	}
	if cfg.Options.MermaidCommand == "" {
		cfg.Options.MermaidCommand = "mmdc"
	}
	if cfg.Options.MermaidFixAttempts == nil {
		cfg.Options.MermaidFixAttempts = intPtr(3)
	}
	if cfg.Options.MermaidTimeout == 0 {
		cfg.Options.MermaidTimeout = 30
	}
	if cfg.Options.MaxTokens == 0 {
		cfg.Options.MaxTokens = 65536
	}

	// Latex defaults
	if cfg.Latex.Level == 0 {
		cfg.Latex.Level = 2
	}
	if cfg.Latex.OutputDir == "" {
		cfg.Latex.OutputDir = "./finally_latex"
	}
	if cfg.Latex.ProjectDir == "" {
		cfg.Latex.ProjectDir = "./latex_project"
	}
	if cfg.Latex.Concurrency == 0 {
		cfg.Latex.Concurrency = 3
	}
	if cfg.Latex.Compile.Engine == "" {
		cfg.Latex.Compile.Engine = "xelatex"
	}
	if cfg.Latex.Compile.Timeout == 0 {
		cfg.Latex.Compile.Timeout = 120
	}
	if cfg.Latex.Compile.RasterCommand == "" {
		cfg.Latex.Compile.RasterCommand = "pdftoppm"
	}
	if cfg.Latex.Compile.RasterDPI == 0 {
		cfg.Latex.Compile.RasterDPI = 110
	}
	if cfg.Latex.Compile.MaxFixRounds == 0 {
		cfg.Latex.Compile.MaxFixRounds = 8
	}
	defaultSessionTuning(&cfg.Latex.Sessions.Drawing)
	defaultSessionTuning(&cfg.Latex.Sessions.Style)
	defaultSessionTuning(&cfg.Latex.Sessions.Chapter)
	defaultSessionTuning(&cfg.Latex.Sessions.Convert)

	// Verify defaults (feature is OFF unless explicitly enabled).
	if cfg.Verify.Concurrency == 0 {
		cfg.Verify.Concurrency = 2
	}
	if cfg.Verify.ReportFile == "" {
		cfg.Verify.ReportFile = "verify_report.md"
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
	if cfg.Paths.LogsDir == "" {
		cfg.Paths.LogsDir = "./logs"
	}
	// DoneDir is anchored to the (already defaulted) InputDir so a
	// typical installation gets files/done out of the box.
	if cfg.Paths.DoneDir == "" {
		cfg.Paths.DoneDir = filepath.Join(cfg.Paths.InputDir, "done")
	}
}

// intPtr returns a pointer to the given int value. It exists so we can
// distinguish "field unset in YAML" (nil pointer, fill with default)
// from "field explicitly set to 0" (pointer to 0, preserve as-is) for
// settings where 0 carries a special meaning such as unlimited retries.
func intPtr(v int) *int {
	return &v
}

// defaultSessionTuning fills zero-valued SessionTuning fields.
func defaultSessionTuning(s *SessionTuning) {
	if s.ContextLimit == 0 {
		s.ContextLimit = 131072 // 128K
	}
	if s.MaxToolRounds == 0 {
		s.MaxToolRounds = 24
	}
	if s.MaxTokens == 0 {
		s.MaxTokens = 16384
	}
	if s.CompactionAt == 0 {
		s.CompactionAt = 0.85
	}
	if s.CompactionAt <= 0 || s.CompactionAt > 1 {
		s.CompactionAt = 0.85
	}
}

// ResolveModel merges the named registry entry with the top-level ai
// block: every empty field of the entry inherits the ai fallback. When
// name is empty or unknown the result is the ai block itself (plus the
// optional per-entry token/temperature overrides, which cannot be
// inherited and therefore keep their zero values).
func (c *Config) ResolveModel(name string) (ModelConfig, bool) {
	fallback := ModelConfig{
		BaseURL:     c.AI.BaseURL,
		APIKey:      c.AI.APIKey,
		Model:       c.AI.Model,
		RequestBody: c.AI.RequestBody,
		MaxTokens:   c.Options.MaxTokens,
	}
	if name == "" {
		return fallback, false
	}
	entry, ok := c.Models[name]
	if !ok {
		return fallback, false
	}
	if entry.BaseURL == "" {
		entry.BaseURL = fallback.BaseURL
	}
	if entry.APIKey == "" {
		entry.APIKey = fallback.APIKey
	}
	if entry.Model == "" {
		entry.Model = fallback.Model
	}
	if entry.RequestBody == nil {
		entry.RequestBody = fallback.RequestBody
	}
	if entry.MaxTokens == 0 {
		entry.MaxTokens = fallback.MaxTokens
	}
	return entry, true
}

// LatexSession returns the tuning block for a named latex session
// (drawing / style / chapter / convert), applying the shared defaults.
func (c *Config) LatexSession(name string) SessionTuning {
	var s SessionTuning
	switch name {
	case "drawing":
		s = c.Latex.Sessions.Drawing
	case "style":
		s = c.Latex.Sessions.Style
	case "chapter":
		s = c.Latex.Sessions.Chapter
	case "convert":
		s = c.Latex.Sessions.Convert
	default:
		s = SessionTuning{}
	}
	defaultSessionTuning(&s)
	return s
}
