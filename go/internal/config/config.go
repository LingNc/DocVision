// Package config loads, validates, and provides defaults for the
// DocVision configuration file (config.yaml). This is the reference
// implementation; see legacy/python/ for the archived Python version.
package config

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	// Preview is the always-on variant of `docvision sessions --serve`:
	// when enabled, a latex run starts the read-only session viewer so the
	// transcripts can be watched in a browser while the book is built.
	Preview PreviewConfig `yaml:"preview"`
	// Estimate tunes the LOCAL token estimate (context threshold + the ≈
	// numbers the session preview shows). Provider numbers are never touched.
	Estimate EstimateConfig `yaml:"estimate"`
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
	// Price is this entry's billing rate. All-zero = unknown, and every
	// cost report then says "未配置价格" instead of inventing ¥0.
	Price PriceConfig `yaml:"price"`
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
	// ImageTokens overrides the top-level estimate: block for THIS model only
	// (same keys: method/tokens/px_per_token/min_tokens/max_tokens). Unset keys
	// inherit the estimate block, and anything unset there uses the code
	// default, so a model that needs its own rule only writes that one key.
	ImageTokens *EstimateConfig `yaml:"image_tokens"`
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
	// ToolRoundsWarnRatio starts the "N rounds left" reminder once the
	// session has used this fraction of MaxToolRounds. 0 = default 0.7.
	ToolRoundsWarnRatio float64 `yaml:"tool_rounds_warn_ratio"`
	// ToolRoundsGrace is how many EXTRA rounds may still call tools after
	// MaxToolRounds is reached (with a growing warning each round).
	// 0 = default 20, negative = no grace (tools stop exactly at the cap).
	ToolRoundsGrace int `yaml:"tool_rounds_grace"`
	// PruneToolChars locally shortens tool results longer than this many
	// characters (head+tail kept) before any AI compaction is paid for.
	// 0 = default 8192, negative = never prune.
	PruneToolChars int `yaml:"prune_tool_chars"`
	// KeepImages is how many of the most recent images stay attached
	// through a local prune; older ones become a text placeholder.
	// 0 = default 3, negative = keep every image.
	KeepImages int `yaml:"keep_images"`
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

// BashSandboxEnabled reports whether session bash runs inside the
// bubblewrap sandbox. tools.bash.sandbox wins; the deprecated
// latex.bash_sandbox is still honoured.
func (c *Config) BashSandboxEnabled() bool {
	if c.Tools.Bash.Sandbox != nil {
		return *c.Tools.Bash.Sandbox
	}
	return c.Latex.BashSandboxEnabled()
}

// BashMaxOutput is the character cap of the session bash tool:
// tools.bash.max_output wins, then the deprecated latex.bash_max_output,
// then the built-in default (5000).
func (c *Config) BashMaxOutput() int {
	if c.Tools.Bash.MaxOutput > 0 {
		return c.Tools.Bash.MaxOutput
	}
	if c.Latex.BashMaxOutput > 0 {
		return c.Latex.BashMaxOutput
	}
	return 5000
}

// UsesDeprecatedBashKeys reports whether the config still uses the old
// latex.bash_* location (the loader prints a migration hint).
func (c *Config) UsesDeprecatedBashKeys() bool {
	return c.Latex.BashSandbox != nil || c.Latex.BashMaxOutput > 0
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
	// FigureCheck re-verifies every redrawn figure against the original
	// bitmap with a vision model before it is accepted. Default OFF: it
	// costs one extra vision call per figure and most books do not need
	// it. When it fails, the drawing session that submitted the figure
	// gets the discrepancy list back and fixes its own work.
	FigureCheck FigureCheckConfig `yaml:"figure_check"`
	// KeepTempDirs keeps the temporary work directories (chapter
	// splitting sandbox, per-chapter compile scratch, vector figure
	// workspace) after use instead of deleting them, so a run can be
	// inspected afterwards. Debug logging implies keeping them.
	KeepTempDirs *bool `yaml:"keep_temp_dirs"`
	// KeepSessionRecords keeps the session transcripts (JSONL) after a
	// session succeeds. Independent from KeepTempDirs: some users want
	// every conversation on disk. Debug logging implies keeping them.
	KeepSessionRecords *bool `yaml:"keep_session_records"`
	// BashSandbox is the deprecated location of tools.bash.sandbox; kept so
	// existing configs keep working (resolved by Config.BashSandboxEnabled).
	BashSandbox *bool `yaml:"bash_sandbox"`
	// BashMaxOutput is the deprecated location of tools.bash.max_output
	// (resolved by Config.BashMaxOutput).
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
// PriceConfig is one model's billing rate, in `currency` per MILLION tokens
// (the unit every Chinese provider quotes).
//
// Input  = 未命中前缀缓存的输入；Cached = 命中缓存的输入（留空/0 时按 Input
// 计，宁可高估不要凭空打折）；Output = 输出（含思考 tokens，厂商就是这么计
// 的）。价格为 0 = 未知，成本报告里显示"未配置价格"，绝不当成免费。
type PriceConfig struct {
	Input    float64 `yaml:"input"`
	Cached   float64 `yaml:"cached"`
	Output   float64 `yaml:"output"`
	Currency string  `yaml:"currency"`
}

// Configured reports whether any rate is set.
func (p PriceConfig) Configured() bool {
	return p.Input != 0 || p.Cached != 0 || p.Output != 0
}

// CachedRate is the cache-hit input rate: the configured value, or the plain
// input rate when unset (an unknown discount must not silently lower a bill).
func (p PriceConfig) CachedRate() float64 {
	if p.Cached != 0 {
		return p.Cached
	}
	return p.Input
}

// CostOf returns the money for one request's counters (tokens, not millions).
// inputTokens is the FULL prompt; cachedTokens is the cached part of it.
func (p PriceConfig) CostOf(inputTokens, cachedTokens, outputTokens int) float64 {
	if !p.Configured() {
		return 0
	}
	fresh := inputTokens - cachedTokens
	if fresh < 0 {
		fresh = 0
	}
	return (float64(fresh)*p.Input + float64(cachedTokens)*p.CachedRate() +
		float64(outputTokens)*p.Output) / 1e6
}

// ModelPrices maps WIRE model names (what the provider reports in usage lines)
// to their rates, so a transcript can be priced without knowing which registry
// entry produced it. Entries without a configured price are skipped.
func (c *Config) ModelPrices() map[string]PriceConfig {
	out := map[string]PriceConfig{}
	if c == nil {
		return out
	}
	def := c.Models["text"].Model
	for _, mc := range c.Models {
		if !mc.Price.Configured() {
			continue
		}
		name := mc.Model
		if name == "" {
			name = def
		}
		if name == "" {
			continue
		}
		out[name] = mc.Price
	}
	if len(out) == 0 {
		return map[string]PriceConfig{}
	}
	return out
}

// FigureCheckConfig configures the per-figure verification loop (level 2
// vector figures). The check compares the ORIGINAL bitmap with the REDRAWN
// figure — the drawing session only ever sees its own render, so a figure that
// compiles cleanly can still be visibly wrong (missing labels, rearranged
// layout, content running off the canvas).
type FigureCheckConfig struct {
	// Enabled is off by default (see LatexConfig.FigureCheck).
	Enabled bool `yaml:"enabled"`
	// Model is the models: registry key of a VISION-capable model
	// (default "verifier"). Empty falls back to that same key.
	Model string `yaml:"model"`
	// MaxRounds caps the fix loop: round 1 verifies, every failed round is
	// sent back to the same drawing session, and after MaxRounds the figure
	// is delivered with a warning instead of looping forever.
	MaxRounds int `yaml:"max_rounds"`
}

// PreviewConfig controls the automatic session preview server.
type PreviewConfig struct {
	// Enabled starts the viewer when a latex run begins. Default false:
	// a service nobody asked for should not appear on a port.
	Enabled bool `yaml:"enabled"`
	// Host is the bind address (default 127.0.0.1 = this machine only;
	// "0.0.0.0" exposes the transcripts to the local network — the
	// viewer is read-only but the transcripts contain the whole book).
	Host string `yaml:"host"`
	// Port default 8848 (sessionview.DefaultAddr). A config cannot say "let the
	// kernel pick" with 0: setDefaults turns 0 into 8848, because a plain int
	// cannot tell "key absent" from "written as 0". The kernel-pick path is the
	// command line's --port 0; the banner prints whatever net.Listen bound.
	Port int `yaml:"port"`
}

// Addr renders host:port for net.Listen. Host "" = loopback. Port 0 means
// "let the kernel pick" — reachable from the command line (--port 0) and from a
// hand-built struct; after setDefaults a loaded config's 0 has become 8848. The
// banner reports the address net.Listen actually bound, port 0 included.
func (p PreviewConfig) Addr() string {
	host := p.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(p.Port))
}

// EstimateConfig is the `estimate:` block: the LOCAL per-image token estimate
// behind the compaction threshold and the ≈ values in the session preview. It
// never changes a provider number — prompt_tokens from the API is copied
// verbatim; this only says how much one attached image is assumed to cost.
//
// 每张图算多少 token 取决于厂商（同一张图不同视觉模型能差 3~10 倍），所以有
// 两种方法可选：fixed（每张固定 Tokens）与 pixels（按尺寸折算
// width*height/PxPerToken，夹在 [MinTokens, MaxTokens] 之间）。这里写的是
// **全局默认**；models.<名>.image_tokens 只给那一条模型换方法或调参数。
type EstimateConfig struct {
	// Method is "fixed", "pixels" or "none". Empty = pixels.
	Method string `yaml:"method"`
	// Tokens is the per-image value for method=fixed, and also what pixels mode
	// charges when the dimensions cannot be read. 0 = 1100 (measured ≈1050 on
	// deepseek-v4.1-flash, where large images saturate).
	Tokens int `yaml:"tokens"`
	// PxPerToken is how many pixels one prompt token covers in pixels mode.
	// 0 = 750 (measured 2.88M px -> 3697 tokens on glm-5.3-flash-official,
	// i.e. ≈780 px/token).
	PxPerToken int `yaml:"px_per_token"`
	// MinTokens / MaxTokens clamp one image in pixels mode. 0 = 85/4096.
	MinTokens int `yaml:"min_tokens"`
	MaxTokens int `yaml:"max_tokens"`
}

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
// pipelines (img2text context expansion, latex image_context, the
// mermaid validation tool). Tools are no longer img2text-private.
type ToolsConfig struct {
	Context struct {
		InitialUp   int `yaml:"initial_up"`   // initial context lines above the image
		InitialDown int `yaml:"initial_down"` // initial context lines below the image
		MaxUp       int `yaml:"max_up"`       // expansion cap above (lines)
		MaxDown     int `yaml:"max_down"`     // expansion cap below (lines)
		MaxCalls    int `yaml:"max_calls"`    // max expansion requests per image
	} `yaml:"context"`
	// View bounds the session image-viewing tools. Both are SOFT budgets:
	// crossing them only adds a reminder to the tool result, it never
	// blocks the call.
	View struct {
		ImageMax  int     `yaml:"image_max"`  // view_image calls per session: 0=30, <0=unlimited
		PDFMax    int     `yaml:"pdf_max"`    // view_pdf calls per session: 0=25, <0=unlimited
		WarnRatio float64 `yaml:"warn_ratio"` // start reminding at this fraction (0=0.7)
	} `yaml:"view"`
	Mermaid struct {
		Validation  string `yaml:"validation"`   // off, auto, strict
		Command     string `yaml:"command"`      // Mermaid CLI command
		FixAttempts *int   `yaml:"fix_attempts"` // 0 = unlimited (in-code cap)
		Timeout     int    `yaml:"timeout"`      // seconds per validation
	} `yaml:"mermaid"`
	// Bash configures the session bash tool used by the latex AI
	// sessions: whether commands run inside the bubblewrap sandbox and
	// how much output is handed back to the model. It lives here (not
	// under latex:) because it describes the tool itself, like
	// tools.mermaid.
	Bash struct {
		Sandbox   *bool `yaml:"sandbox"`    // bubblewrap kernel sandbox (default true)
		MaxOutput int   `yaml:"max_output"` // characters of bash output fed to the model
	} `yaml:"bash"`
	// Python configures the Python environment session bash sees. The
	// sandbox runs with --unshare-net, so a session can never install a
	// package itself: the host provides the interpreter (system / venv /
	// conda), makes it visible read-only inside the sandbox, and installs
	// missing modules on demand (tools.python.auto_install). Lives here
	// for the same reason as Bash — it describes the tool environment.
	Python ToolsPythonConfig `yaml:"python"`
}

// ToolsPythonConfig is tools.python: the Python environment handed to
// session bash.
type ToolsPythonConfig struct {
	// Enabled: provide python in session bash (default true; only an
	// explicit `enabled: false` turns it off).
	Enabled *bool `yaml:"enabled"`
	// Mode: system | venv | conda. venv/conda additionally bind EnvDir.
	Mode string `yaml:"mode"`
	// Interpreter: python executable used to build the environment and
	// to install packages ("" = python3 from PATH, or <EnvDir>/bin/python3).
	Interpreter string `yaml:"interpreter"`
	// EnvDir: venv directory (created when missing) or a conda prefix.
	EnvDir string `yaml:"env_dir"`
	// CondaEnv: conda environment NAME (mode=conda). Empty — or base/root —
	// means the conda BASE environment, so mode=conda with nothing else
	// configured just works (that is the common local setup).
	CondaEnv string `yaml:"conda_env"`
	// PipIndexURL: explicit PyPI mirror for the host-side auto-install
	// (`pip install -i <url>`). Empty = whatever the host pip config says
	// (~/.config/pip/pip.conf), which is invisible to DocVision and
	// silently breaks installs when the mirror dies.
	PipIndexURL string `yaml:"pip_index_url"`
	// Packages: modules ensured (import-checked, installed when missing)
	// before sessions start.
	Packages []string `yaml:"packages"`
	// AutoInstall: install a missing module on the HOST when a session's
	// bash hits ModuleNotFoundError, then tell the model to retry.
	AutoInstall *bool `yaml:"auto_install"`
	// InstallTimeout: seconds allowed for one pip install (default 300).
	InstallTimeout int `yaml:"install_timeout"`
}

// PythonEnabled reports whether session bash should provide Python.
func (c *Config) PythonEnabled() bool {
	return c.Tools.Python.Enabled == nil || *c.Tools.Python.Enabled
}

// PythonAutoInstall reports whether missing modules are installed on
// the host on demand (default true).
func (c *Config) PythonAutoInstall() bool {
	return c.Tools.Python.AutoInstall == nil || *c.Tools.Python.AutoInstall
}

// PythonConfig returns the resolved python environment config.
func (c *Config) PythonConfig() ToolsPythonConfig {
	cfg := c.Tools.Python
	if cfg.Mode == "" {
		cfg.Mode = "system"
	}
	if cfg.InstallTimeout <= 0 {
		cfg.InstallTimeout = 300
	}
	if cfg.Enabled == nil {
		on := true
		cfg.Enabled = &on
	}
	if cfg.AutoInstall == nil {
		on := true
		cfg.AutoInstall = &on
	}
	return cfg
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
	MaxTokens          int     `yaml:"max_tokens"`
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
const CurrentConfigVersion = 9

// checkConfigVersion warns (non-fatally) when the loaded config was
// written for a different schema version.
// retiredEstimateKeys are the per-image estimate keys that method/tokens/
// px_per_token/min_tokens/max_tokens replaced. Nothing reads them any more, so
// a config that still carries them would fall back to the defaults without a
// word — LoadConfig only warns about unknown keys here (strict checks live in
// docvision setup).
var retiredEstimateKeys = []string{
	"image_px_per_token", "image_tokens_min", "image_tokens_max", "image_tokens_fallback",
}

// hasRetiredEstimateKeys reports whether the raw config still carries a retired
// estimate key.
func hasRetiredEstimateKeys(data []byte) bool {
	for _, k := range retiredEstimateKeys {
		if bytes.Contains(data, []byte(k)) {
			return true
		}
	}
	return false
}

// warnDeprecatedKeys prints migration hints for configs written with an
// older key layout. Non-fatal: the old keys still work.
func warnDeprecatedKeys(cfg *Config) {
	if cfg.UsesDeprecatedBashKeys() {
		fmt.Fprintln(os.Stderr, "⚠ latex.bash_sandbox / latex.bash_max_output 已迁移到 tools.bash.sandbox / tools.bash.max_output（旧键仍生效，建议改用新位置）")
	}
}

// warnRetiredEstimateKeys says it out loud when a config still has the old
// per-image estimate keys: after the rename nothing reads them, and a silent
// fallback to the defaults is exactly the kind of thing a user cannot debug.
func warnRetiredEstimateKeys(data []byte) {
	if hasRetiredEstimateKeys(data) {
		fmt.Fprintln(os.Stderr, "⚠ estimate.image_px_per_token / image_tokens_min / image_tokens_max / image_tokens_fallback 已被 estimate.method + tokens / px_per_token / min_tokens / max_tokens 取代（旧键不再生效，建议按 config.example.yaml 更新）")
	}
}

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
	warnDeprecatedKeys(cfg)
	warnRetiredEstimateKeys(data)
	setDefaults(cfg)
	if err := validateThinkingTypes(cfg); err != nil {
		return nil, err
	}
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
// thinkingTypes 是 models.*.thinking.type 的合法取值（GLM/DeepSeek/Qwen 的
// OpenAI 兼容端点都只认这三个）。
var thinkingTypes = map[string]bool{"enabled": true, "disabled": true, "adaptive": true}

// thinkingTypeTypos 收录一眼能认出的同义写法（用户现场写的是 `disable`）。
var thinkingTypeTypos = map[string]string{
	"disable": "disabled", "off": "disabled", "false": "disabled", "no": "disabled", "none": "disabled",
	"enable": "enabled", "on": "enabled", "true": "enabled", "yes": "enabled",
}

// validateThinkingTypes 校验每个模型的 thinking.type。
//
// 为什么值得在启动时拦住：这个字段是**原样**发到请求体顶层的，取值写错时
// 服务端会用 HTTP 400 拒掉**整个请求**（现场报
// `unknown variant \`disable\`, expected one of \`adaptive\`, \`enabled\`, \`disabled\“），
// 于是一次运行里 8/8 张图全部"保留原图"，日志却只说"TikZ 未通过"，看起来
// 像是绘图提示词或编译器的问题。一眼能认出的笔误（disable/off/on…）自动
// 纠正并告警，其余未知取值直接报错，附上模型名与合法取值。
func validateThinkingTypes(cfg *Config) error {
	names := make([]string, 0, len(cfg.Models))
	for name := range cfg.Models {
		names = append(names, name)
	}
	sort.Strings(names) // 报错与告警顺序稳定，便于复现
	for _, name := range names {
		m := cfg.Models[name]
		if m.Thinking == nil {
			continue
		}
		raw, ok := m.Thinking["type"]
		if !ok || raw == nil {
			continue
		}
		str, isStr := raw.(string)
		if !isStr {
			continue // 非字符串（bool/自定义结构）交给服务端自己判
		}
		v := strings.ToLower(strings.TrimSpace(str))
		if fixed, isTypo := thinkingTypeTypos[v]; isTypo {
			m.Thinking["type"] = fixed
			fmt.Fprintf(os.Stderr, "\u26a0 models.%s.thinking.type: %q 不是合法取值，已按 %q 发送（合法值：enabled / disabled / adaptive）\n", name, str, fixed)
			continue
		}
		if !thinkingTypes[v] {
			return fmt.Errorf("models.%s.thinking.type: %q 不是合法取值（合法值：enabled / disabled / adaptive）——该字段原样进请求体，写错会让服务端拒掉每一个请求", name, str)
		}
		m.Thinking["type"] = v
	}
	return nil
}

func validatePaths(cfg *Config) error {
	switch cfg.Latex.ChapterGranularity {
	case "", "small", "large":
	default:
		return fmt.Errorf("latex.chapter_granularity 必须为 small 或 large（当前 %q）", cfg.Latex.ChapterGranularity)
	}
	if cfg.Tools.Bash.MaxOutput < 0 {
		return fmt.Errorf("tools.bash.max_output 不能为负（当前 %d）", cfg.Tools.Bash.MaxOutput)
	}
	if cfg.Latex.BashMaxOutput < 0 {
		return fmt.Errorf("latex.bash_max_output（已废弃，请改用 tools.bash.max_output）不能为负（当前 %d）", cfg.Latex.BashMaxOutput)
	}
	switch cfg.Tools.Python.Mode {
	case "", "system", "venv", "conda":
	default:
		return fmt.Errorf("tools.python.mode 必须为 system / venv / conda（当前 %q）", cfg.Tools.Python.Mode)
	}
	if cfg.Tools.Python.Mode == "venv" && cfg.Tools.Python.EnvDir == "" {
		return fmt.Errorf("tools.python.mode=venv 必须同时设置 tools.python.env_dir（虚拟环境目录，不存在时会自动创建）")
	}
	// mode=conda 全空是合法的：用 conda 的 base 环境（用户本地本来就有 base）。
	if cfg.Tools.Python.InstallTimeout < 0 {
		return fmt.Errorf("tools.python.install_timeout 不能为负（当前 %d）", cfg.Tools.Python.InstallTimeout)
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
	// Bash tool settings are resolved lazily by Config.BashSandboxEnabled /
	// Config.BashMaxOutput (tools.bash.* wins, latex.bash_* deprecated):
	// they are deliberately not filled in here, otherwise a default written
	// into the legacy fields would mask an explicit tools.bash.* value.
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

	// Figure check defaults (feature is OFF unless explicitly enabled).
	if cfg.Latex.FigureCheck.Model == "" {
		cfg.Latex.FigureCheck.Model = "verifier"
	}
	if cfg.Latex.FigureCheck.MaxRounds == 0 {
		cfg.Latex.FigureCheck.MaxRounds = 2
	}

	// Preview defaults (feature is OFF unless explicitly enabled). Host
	// stays empty = loopback-only, which Addr() renders.
	if cfg.Preview.Host == "" {
		cfg.Preview.Host = "127.0.0.1"
	}
	if cfg.Preview.Port == 0 {
		cfg.Preview.Port = 8848
	}

	// Local per-image token estimate: only used for the compaction threshold
	// and the ≈ values the session preview shows. Zero fields = code default,
	// and an absent key inside a model's image_tokens block inherits this one.
	if cfg.Estimate.Method == "" {
		cfg.Estimate.Method = EstimateMethodPixels
	}
	if cfg.Estimate.Tokens == 0 {
		cfg.Estimate.Tokens = 1100
	}
	if cfg.Estimate.PxPerToken == 0 {
		cfg.Estimate.PxPerToken = 750
	}
	if cfg.Estimate.MinTokens == 0 {
		cfg.Estimate.MinTokens = 85
	}
	if cfg.Estimate.MaxTokens == 0 {
		cfg.Estimate.MaxTokens = 4096
	}
	// 上下限写反不在这里悄悄纠正：它是配置错误，由 semanticChecks 报出来；
	// 运行期 ImageEstimate.Normalized 仍会把界限理成有序的，估算不会失控。

	// Model prices: currency only (a rate of 0 stays 0 = unknown).
	for name, mc := range cfg.Models {
		if mc.Price.Configured() && mc.Price.Currency == "" {
			mc.Price.Currency = "¥"
			cfg.Models[name] = mc
		}
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
	if s.ToolRoundsWarnRatio <= 0 || s.ToolRoundsWarnRatio > 1 {
		s.ToolRoundsWarnRatio = 0.7
	}
}

// ToolRoundsGraceRounds resolves sessions.tool_rounds_grace (default 20).
func (s SessionTuning) ToolRoundsGraceRounds() int {
	if s.ToolRoundsGrace == 0 {
		return 20
	}
	if s.ToolRoundsGrace < 0 {
		return 0
	}
	return s.ToolRoundsGrace
}

// PruneToolCharsLimit resolves sessions.prune_tool_chars (default 4096,
// negative disables local pruning).
//
// Measured on real transcripts: 325 tool results, longest 6183 chars
// (doc_search), p99 5046, and not one above 8192 — an 8k threshold never
// fired. The tools that CAN return a lot are read_file (whole file, hard cap
// 64 KB) and bash (tools.bash.max_output, default 5000), so 4096 catches
// those and long doc_search dumps while leaving ordinary results alone.
// Pruned results keep the first half plus the last quarter of this budget.
func (s SessionTuning) PruneToolCharsLimit() int {
	if s.PruneToolChars == 0 {
		return 4096
	}
	if s.PruneToolChars < 0 {
		return 0
	}
	return s.PruneToolChars
}

// KeepImagesCount resolves sessions.keep_images (default 3, negative keeps
// every image attached).
func (s SessionTuning) KeepImagesCount() int {
	if s.KeepImages == 0 {
		return 3
	}
	if s.KeepImages < 0 {
		return -1
	}
	return s.KeepImages
}

// ViewWarnRatio resolves tools.view.warn_ratio (default 0.7).
func (c *Config) ViewWarnRatio() float64 {
	r := c.Tools.View.WarnRatio
	if r <= 0 || r > 1 {
		return 0.7
	}
	return r
}

// ViewImageMax resolves tools.view.image_max (default 30, negative = no
// budget). Soft: only used for reminders.
func (c *Config) ViewImageMax() int {
	if c.Tools.View.ImageMax == 0 {
		return 30
	}
	if c.Tools.View.ImageMax < 0 {
		return 0
	}
	return c.Tools.View.ImageMax
}

// ViewPDFMax resolves tools.view.pdf_max (default 25, negative = no budget).
func (c *Config) ViewPDFMax() int {
	if c.Tools.View.PDFMax == 0 {
		return 25
	}
	if c.Tools.View.PDFMax < 0 {
		return 0
	}
	return c.Tools.View.PDFMax
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

// Estimate methods accepted by estimate.method / models.<名>.image_tokens.method.
const (
	// EstimateMethodFixed charges one constant per image (estimate.tokens).
	EstimateMethodFixed = "fixed"
	// EstimateMethodPixels scales with size: width*height/px_per_token clamped
	// to [min_tokens, max_tokens].
	EstimateMethodPixels = "pixels"
	// EstimateMethodNone charges nothing for an image locally.
	EstimateMethodNone = "none"
)

// ResolveImageEstimate returns the per-image token estimate that applies to one
// models: entry: the entry's own image_tokens block wins key by key, every key
// it leaves unset inherits the top-level estimate: block, and anything still
// unset keeps the zero value (the session side then applies the code default).
// Unknown entry names get the global block — the viewer asks by model id, which
// need not be a configured entry.
func (c *Config) ResolveImageEstimate(name string) EstimateConfig {
	out := c.Estimate
	if entry, ok := c.Models[name]; ok && entry.ImageTokens != nil {
		o := *entry.ImageTokens
		if o.Method != "" {
			out.Method = o.Method
		}
		if o.Tokens != 0 {
			out.Tokens = o.Tokens
		}
		if o.PxPerToken != 0 {
			out.PxPerToken = o.PxPerToken
		}
		if o.MinTokens != 0 {
			out.MinTokens = o.MinTokens
		}
		if o.MaxTokens != 0 {
			out.MaxTokens = o.MaxTokens
		}
	}
	return out
}

// checkerDefaultToolRounds caps the per-chapter checker session when the
// config does not say otherwise (it does NOT inherit convert's rounds).
const checkerDefaultToolRounds = 50

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
		explicitRounds := s.MaxToolRounds
		inheritSessionTuning(&s, c.Latex.Sessions.Convert)
		// 核对会话是"读两个文件 + 搜索 + 交结论"的简单会话，不需要
		// convert 的上百轮：没显式配置就用 50（用户指定）。显式写负数
		// = 真正不限制，照旧生效。
		if explicitRounds == 0 {
			s.MaxToolRounds = checkerDefaultToolRounds
		}
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
