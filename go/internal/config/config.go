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
	Mineru MinerUConfig `yaml:"mineru"`
	// AI is the resolved default model for the basic pipelines
	// (img2text etc.). It comes straight from the models: registry
	// entry "text" — there is no separate top-level ai: block anymore.
	ConfigVersion int           `yaml:"config_version"`
	Options       OptionsConfig `yaml:"options"`
	Tools         ToolsConfig   `yaml:"tools"`
	Paths         PathsConfig   `yaml:"paths"`
	// Models is the named model registry — the single source of truth
	// for every AI (img2text uses the "text" entry; the latex / verify
	// pipelines use classifier / drawing / style / chapter / convert /
	// verifier). Each entry may declare its own base_url / api_key /
	// model / request_body; empty fields inherit from the resolved
	// default (the "text" entry, or a legacy top-level ai: block).
	Models map[string]ModelConfig `yaml:"models"`
	Latex  LatexConfig            `yaml:"latex"`
	Verify VerifyConfig           `yaml:"verify"`
	// Img2Text overrides for the basic image-to-text pipeline. Model may
	// reference a models: registry name (default "text").
	Img2Text Img2TextConfig `yaml:"img2text"`
}

// ModelConfig is one entry of the named model registry. Empty fields
// inherit from the top-level ai block, so a minimal setup only needs
// model: "<name>" per entry.
type ModelConfig struct {
	BaseURL     string                 `yaml:"base_url"`
	APIKey      string                 `yaml:"api_key"`
	Model       string                 `yaml:"model"`
	RequestBody map[string]interface{} `yaml:"request_body"`
	// MaxTokens / Temperature are the per-model completion budget and
	// sampling temperature. They act as the fallback used when a
	// session or single-shot call does not set its own value
	// (latex.sessions.<session>.max_tokens always wins for sessions).
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	// Per-model API/request controls. Zero values inherit models.text
	// (via ResolveModel) and finally the built-in defaults
	// (400s read / 60s connect / 3 retries / 100 rate-limit cap).
	APITimeout        int `yaml:"api_timeout"`
	APIConnectTimeout int `yaml:"api_connect_timeout"`
	APIMaxRetries     int `yaml:"api_max_retries"`
	RateLimitRetries  int `yaml:"rate_limit_retries"`
	// Stream enables SSE streaming for this model's chat requests.
	// nil or true = streaming (default), false = single JSON response.
	// Streaming keeps long thinking/output requests alive with visible
	// progress instead of one silent wait.
	Stream *bool `yaml:"stream"`
	// APIStreamIdleTimeout bounds the gap between two stream chunks in
	// seconds (the stream is aborted after that much silence). 0
	// inherits api_timeout. It replaces the total request timeout while
	// streaming: a long but active stream is never killed.
	APIStreamIdleTimeout int `yaml:"api_stream_idle_timeout"`
	// Thinking is the vendor top-level "thinking" request field, e.g.
	// {type: enabled|disabled} (GLM-4.5+, DeepSeek). It is sent at the
	// TOP LEVEL of the request body, not inside request_body.extra_body
	// (extra_body is a Python-SDK concept and is ignored on the wire).
	// GLM 还支持 clear_thinking: false（保留式思考——历史 assistant 轮的
	// 思维链完整回传，提升缓存命中）；需要时直接写进这个 map。
	Thinking map[string]any `yaml:"thinking"`
	// ToolStream (GLM) requests streamed tool-call arguments alongside
	// content chunks (stream must be true). Our SSE assembler already
	// accumulates delta.tool_calls incrementally, so this is wire
	// compatible without parser changes.
	ToolStream *bool `yaml:"tool_stream"`
	// ReasoningEffort is the vendor top-level reasoning_effort field
	// (GLM-5.2+: max|xhigh|high|medium|low|minimal|none; only effective
	// while thinking is enabled).
	ReasoningEffort string `yaml:"reasoning_effort"`
}

// Streaming reports whether chat requests for this model use SSE
// streaming. Unset (nil) means streaming: it is the default.
func (m ModelConfig) Streaming() bool {
	if m.Stream == nil {
		return true
	}
	return *m.Stream
}

// validReasoningEfforts is the accepted reasoning_effort vocabulary
// (GLM-5.2 and above; other vendors may accept a subset).
var validReasoningEfforts = map[string]bool{
	"max": true, "xhigh": true, "high": true, "medium": true,
	"low": true, "minimal": true, "none": true,
}

// SessionTuning tunes one AI session type (context window, tool budget).
type SessionTuning struct {
	// ContextLimit is the session context window in tokens. When the
	// estimated conversation size approaches this limit the session
	// auto-compacts (AI summarisation). Default 131072 (128K).
	ContextLimit int `yaml:"context_limit"`
	// MaxToolRounds caps tool-calling rounds inside one session turn.
	MaxToolRounds int `yaml:"max_tool_rounds"`
	// MaxTokens is the per-request completion budget (max_tokens). It is
	// the FINAL answer budget as the vendor defines it; where a vendor
	// bills thinking separately (e.g. DeepSeek's 32K CoT budget) it does
	// not include it.
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
	// FinalReview runs the final book-doctor session after a successful
	// full-book build: it reads the finished PDF and does the final
	// consolidation (front matter, TOC, order, layout), then recompiles.
	// Default true.
	FinalReview *bool `yaml:"final_review"`
}

// FinalReviewEnabled reports whether the final review session runs
// (nil → default true).
// KeepTempDirsEnabled reports whether temporary work directories are
// kept after use (default false).
func (c LatexConfig) KeepTempDirsEnabled() bool {
	return c.KeepTempDirs != nil && *c.KeepTempDirs
}

// KeepSessionRecordsEnabled reports whether session transcripts are kept
// after a successful session (default false).
func (c LatexConfig) KeepSessionRecordsEnabled() bool {
	return c.KeepSessionRecords != nil && *c.KeepSessionRecords
}

// BashSandboxEnabled reports whether session bash runs inside the
// bubblewrap sandbox (default true).
func (c LatexConfig) BashSandboxEnabled() bool {
	return c.BashSandbox == nil || *c.BashSandbox
}

func (c LatexCompileConfig) FinalReviewEnabled() bool {
	return c.FinalReview == nil || *c.FinalReview
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
	// CheckerModel reviews converted chapters (text model, e.g. a
	// small/fast model from the registry). Empty falls back to
	// convert_model.
	CheckerModel string `yaml:"checker_model"`
	// InsertImageDescription: when true, images kept as raster (no
	// vector structure) embed a readable AI explanation as
	// "[Image]( content )" instead of the bare image link. The same
	// flag controls whether confirmed TikZ figures embed the code
	// block instead of the compiled PDF link. Default false.
	InsertImageDescription bool `yaml:"insert_image_description"`
	// ChapterGranularity controls the level-1 chapter split: "small"
	// (default) splits at section level into coherent self-contained
	// units; "large" keeps each whole top-level chapter as one file.
	ChapterGranularity string `yaml:"chapter_granularity"`

	// RemoveWatermark: when true, LaTeX sessions are instructed to detect
	// and EXCLUDE watermark artifacts (repeated decorative overlay text /
	// logos) instead of reproducing them. Default false (keep as-is).
	RemoveWatermark bool `yaml:"remove_watermark"`
	// Concurrency for per-image and per-chapter workers. Default 3
	// (sessions are long-lived and much heavier than plain img2text calls).
	Concurrency int                `yaml:"concurrency"`
	Compile     LatexCompileConfig `yaml:"compile"`
	// KeepTempDirs keeps the temporary work directories (chapter
	// splitting sandbox, per-chapter compile scratch, vector figure
	// workspace) after use instead of deleting them, so a run can be
	// inspected afterwards. Debug logging implies keeping them.
	KeepTempDirs *bool `yaml:"keep_temp_dirs"`
	// KeepSessionRecords keeps the session transcripts (JSONL) after a
	// session succeeds. Independent from KeepTempDirs: some users want
	// every conversation on disk. Debug logging implies keeping them.
	KeepSessionRecords *bool `yaml:"keep_session_records"`
	// BashSandbox wraps session bash commands in bubblewrap: inside the
	// sandbox only the session's mounts exist (writable mounts writable,
	// read-only mounts read-only). nil/true = enabled; also falls back to
	// a plain shell with a warning when bubblewrap is unavailable.
	BashSandbox *bool `yaml:"bash_sandbox"`
	// BashMaxOutput caps how many characters a session's bash tool
	// returns to the model. Default 5000.
	BashMaxOutput int `yaml:"bash_max_output"`
	// Sessions tunes each specialised AI session independently.
	Sessions struct {
		Drawing SessionTuning `yaml:"drawing"`
		Style   SessionTuning `yaml:"style"`
		Chapter SessionTuning `yaml:"chapter"`
		Convert SessionTuning `yaml:"convert"`
		// Checker is the small text model that reviews each converted
		// chapter before assembly. Defaults to the convert tuning.
		Checker SessionTuning `yaml:"checker"`
	} `yaml:"sessions"`
}

// Img2TextConfig holds pipeline-specific overrides for the basic
// image-to-text workflow.
type Img2TextConfig struct {
	// Model is a models: registry name (default "text"). Empty falls
	// back to the resolved top-level AI block.
	Model       string         `yaml:"model"`
	MaxTokens   int            `yaml:"max_tokens"`
	Temperature float64        `yaml:"temperature"`
	RequestBody map[string]any `yaml:"request_body"`

	// Pipeline tuning (mirrors the legacy options: keys; a non-zero
	// value here overrides the options: counterpart).
	Concurrency       int    `yaml:"concurrency"`
	OutputLanguage    string `yaml:"output_language"`
	FormatFixAttempts int    `yaml:"format_fix_attempts"`
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

// ToolsConfig groups the per-tool tunables that are shared across
// pipelines (img2text context expansion, latex image_context, mermaid
// and tikz validation tools). Tools are no longer img2text-private.
type ToolsConfig struct {
	Context struct {
		InitialUp   int `yaml:"initial_up"`   // initial context lines above the image
		InitialDown int `yaml:"initial_down"` // initial context lines below the image
		MaxUp       int `yaml:"max_up"`       // expansion cap above (lines)
		MaxDown     int `yaml:"max_down"`     // expansion cap below (lines)
		MaxCalls    int `yaml:"max_calls"`    // max expansion requests per image
	} `yaml:"context"`
	Mermaid struct {
		Validation  string `yaml:"validation"`   // off, auto, strict
		Command     string `yaml:"command"`      // Mermaid CLI command
		FixAttempts *int   `yaml:"fix_attempts"` // 0 = unlimited (in-code cap)
		Timeout     int    `yaml:"timeout"`      // seconds per validation
	} `yaml:"mermaid"`
	Latex struct {
		Validation string `yaml:"validation"` // off, auto, strict (latex code block compile check)
		Engine     string `yaml:"engine"`     // xelatex / pdflatex / lualatex
	} `yaml:"latex"`
}

// OptionsConfig holds tuning knobs for the image-to-text processing pipeline.
type OptionsConfig struct {
	MaxContextLinesUp   int `yaml:"-"` // set from tools.context.initial_up
	MaxContextLinesDown int `yaml:"-"` // set from tools.context.initial_down
	MaxWindowUp         int `yaml:"-"` // set from tools.context.max_up
	MaxWindowDown       int `yaml:"-"` // set from tools.context.max_down
	MaxRetries          int `yaml:"-"` // set from tools.context.max_calls
	// ExtraInstruction is injected programmatically (not from yaml),
	// e.g. the latex watermark working memory; appended to the system prompt.
	ExtraInstruction   string  `yaml:"-"`
	Concurrency        int     `yaml:"concurrency"`
	Temperature        float64 `yaml:"temperature"`
	OutputLanguage     string  `yaml:"output_language"`
	FormatFixAttempts  int     `yaml:"-"` // img2text pipeline policy
	MermaidValidation  string  `yaml:"-"` // set from tools.mermaid.validation
	MermaidCommand     string  `yaml:"-"` // set from tools.mermaid.command
	MermaidFixAttempts *int    `yaml:"-"` // set from tools.mermaid.fix_attempts
	MermaidTimeout     int     `yaml:"-"` // set from tools.mermaid.timeout
	// TikZ compile-check (auto/strict/off). Engine defaults to xelatex
	// with automatic fallback to pdflatex/lualatex.
	LatexValidation string `yaml:"-"` // set from tools.latex.validation
	LatexEngine     string `yaml:"-"` // set from tools.latex.engine
	MaxTokens       int    `yaml:"max_tokens"`
	// LogLevel: info (default), debug or trace. Debug writes every AI
	// request/response summary, prompt and tool result into the log
	// file; trace additionally records per-chunk stream traffic and raw
	// dumps (console unaffected).
	LogLevel string `yaml:"log_level"`
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

	// LaTeX destinations (defaults: ./finally_latex, ./latex_project).
	LatexOutput  string `yaml:"latex_output"`
	LatexProject string `yaml:"latex_project"`
	// Fonts is the AI-managed font directory (defaults: ./fonts).
	Fonts string `yaml:"fonts"`
}

// LoadConfig reads the YAML file at path, applies defaults for any
// zero-valued fields, and returns the resulting Config.
// CurrentConfigVersion is the config schema version this binary expects.
// Bump it whenever yaml keys change; loaders warn when the file differs.
const CurrentConfigVersion = 6

// checkConfigVersion warns (non-fatally) when the loaded config was
// written for a different schema version.
func checkConfigVersion(cfg *Config) {
	if cfg.ConfigVersion != CurrentConfigVersion {
		fmt.Fprintf(os.Stderr, "⚠ 配置文件版本不匹配 (config_version: %d，当前程序期望 %d) —— 请参考 config.example.yaml 更新你的配置文件\n", cfg.ConfigVersion, CurrentConfigVersion)
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	checkConfigVersion(cfg)
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
	switch cfg.Latex.ChapterGranularity {
	case "", "small", "large":
	default:
		return fmt.Errorf("latex.chapter_granularity 必须为 small 或 large（当前 %q）", cfg.Latex.ChapterGranularity)
	}
	if cfg.Latex.BashMaxOutput < 0 {
		return fmt.Errorf("latex.bash_max_output 不能为负（当前 %d）", cfg.Latex.BashMaxOutput)
	}
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
	applyImg2TextOverrides(cfg)
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
	// tools.* defaults
	if cfg.Tools.Context.InitialUp == 0 {
		cfg.Tools.Context.InitialUp = 10
	}
	if cfg.Tools.Context.InitialDown == 0 {
		cfg.Tools.Context.InitialDown = 5
	}
	if cfg.Tools.Context.MaxUp == 0 {
		cfg.Tools.Context.MaxUp = 50
	}
	if cfg.Tools.Context.MaxDown == 0 {
		cfg.Tools.Context.MaxDown = 50
	}
	if cfg.Tools.Context.MaxCalls == 0 {
		cfg.Tools.Context.MaxCalls = 5
	}
	if cfg.Tools.Mermaid.Validation == "" {
		cfg.Tools.Mermaid.Validation = "auto"
	}
	if cfg.Tools.Mermaid.Command == "" {
		cfg.Tools.Mermaid.Command = "mmdc"
	}
	if cfg.Tools.Mermaid.Timeout == 0 {
		cfg.Tools.Mermaid.Timeout = 30
	}
	if cfg.Tools.Latex.Validation == "" {
		cfg.Tools.Latex.Validation = "auto"
	}
	if cfg.Tools.Latex.Engine == "" {
		cfg.Tools.Latex.Engine = "xelatex"
	}
	// Runtime carriers derived from tools:.
	cfg.Options.MaxContextLinesUp = cfg.Tools.Context.InitialUp
	cfg.Options.MaxContextLinesDown = cfg.Tools.Context.InitialDown
	cfg.Options.MaxWindowUp = cfg.Tools.Context.MaxUp
	cfg.Options.MaxWindowDown = cfg.Tools.Context.MaxDown
	cfg.Options.MaxRetries = cfg.Tools.Context.MaxCalls
	cfg.Options.MermaidValidation = cfg.Tools.Mermaid.Validation
	cfg.Options.MermaidCommand = cfg.Tools.Mermaid.Command
	cfg.Options.MermaidFixAttempts = cfg.Tools.Mermaid.FixAttempts
	cfg.Options.MermaidTimeout = cfg.Tools.Mermaid.Timeout
	cfg.Options.LatexValidation = cfg.Tools.Latex.Validation
	cfg.Options.LatexEngine = cfg.Tools.Latex.Engine
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

	if cfg.Options.LogLevel == "" {
		cfg.Options.LogLevel = "info"
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
	if cfg.Paths.LatexOutput == "" {
		cfg.Paths.LatexOutput = "./finally_latex"
	}
	if cfg.Paths.LatexProject == "" {
		cfg.Paths.LatexProject = "./latex_project"
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
	if cfg.Latex.KeepTempDirs == nil {
		off := false
		cfg.Latex.KeepTempDirs = &off
	}
	if cfg.Latex.KeepSessionRecords == nil {
		off := false
		cfg.Latex.KeepSessionRecords = &off
	}
	if cfg.Latex.BashSandbox == nil {
		enabled := true
		cfg.Latex.BashSandbox = &enabled
	}
	if cfg.Latex.BashMaxOutput == 0 {
		cfg.Latex.BashMaxOutput = 5000
	}
	defaultSessionTuning(&cfg.Latex.Sessions.Drawing)
	// The style analyst emits a full .cls + manual + example in one
	// reply, so its built-in budget is larger than the shared default.
	if cfg.Latex.Sessions.Style.MaxTokens == 0 {
		cfg.Latex.Sessions.Style.MaxTokens = 32768
	}
	defaultSessionTuning(&cfg.Latex.Sessions.Style)
	defaultSessionTuning(&cfg.Latex.Sessions.Chapter)
	defaultSessionTuning(&cfg.Latex.Sessions.Convert)
	if cfg.Latex.ChapterGranularity == "" {
		cfg.Latex.ChapterGranularity = "small"
	}

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
	if cfg.Paths.Fonts == "" {
		cfg.Paths.Fonts = "./fonts"
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
	// MaxToolRounds: <=0 means unlimited (no safety cap). The 128 default
	//	 lives in the config templates, not here — an explicit 0 in the user's
	//	 config must stay 0.
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
	// The registry entry "text" is the mandatory base: every other
	// entry (and unnamed requests) inherit its fields.
	fallback := c.Models[defaultModelKey]
	if name == "" || name == defaultModelKey {
		return fallback, name != ""
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
	if entry.Temperature == 0 {
		entry.Temperature = fallback.Temperature
	}
	if entry.APITimeout == 0 {
		entry.APITimeout = fallback.APITimeout
	}
	if entry.APIConnectTimeout == 0 {
		entry.APIConnectTimeout = fallback.APIConnectTimeout
	}
	if entry.APIMaxRetries == 0 {
		entry.APIMaxRetries = fallback.APIMaxRetries
	}
	if entry.RateLimitRetries == 0 {
		entry.RateLimitRetries = fallback.RateLimitRetries
	}
	if entry.Stream == nil {
		entry.Stream = fallback.Stream
	}
	if entry.APIStreamIdleTimeout == 0 {
		entry.APIStreamIdleTimeout = fallback.APIStreamIdleTimeout
	}
	if entry.Thinking == nil {
		entry.Thinking = fallback.Thinking
	}
	if entry.ToolStream == nil {
		entry.ToolStream = fallback.ToolStream
	}
	if entry.ReasoningEffort == "" {
		entry.ReasoningEffort = fallback.ReasoningEffort
	}
	return entry, true
}

// LatexSession returns the tuning block for a named latex session
// (drawing / style / chapter / convert / checker), applying the shared
// defaults. The checker block is the exception: any field it leaves at
// zero inherits the convert block (an explicit negative max_tool_rounds
// still means "unlimited").
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
	case "checker":
		s = c.Latex.Sessions.Checker
		inheritSessionTuning(&s, c.Latex.Sessions.Convert)
	default:
		s = SessionTuning{}
	}
	defaultSessionTuning(&s)
	return s
}

// inheritSessionTuning fills every zero field of dst from base. Zero
// means "not configured" for the inherited fields; a caller that wants
// unlimited tool rounds in an inheriting block must write a negative
// max_tool_rounds.
func inheritSessionTuning(dst *SessionTuning, base SessionTuning) {
	if dst.ContextLimit == 0 {
		dst.ContextLimit = base.ContextLimit
	}
	if dst.MaxToolRounds == 0 {
		dst.MaxToolRounds = base.MaxToolRounds
	}
	if dst.MaxTokens == 0 {
		dst.MaxTokens = base.MaxTokens
	}
	if dst.Temperature == 0 {
		dst.Temperature = base.Temperature
	}
	if dst.CompactionAt == 0 {
		dst.CompactionAt = base.CompactionAt
	}
}

// defaultModelKey is the registry entry that provides the default
// model for the basic pipelines (img2text etc.).
const defaultModelKey = "text"
