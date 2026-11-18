// Package config 是项目级可调参数层（用户专用）。
//
// 纪律（用户要求，2026-11-18）：验收门槛全部程序强制、不由模型判断；阈值
// **只允许用户显式调整**——agent 无权自调，用户不提不改。因此本包只被
// dvagent 的 config-get/config-set 命令使用，**不进 MCP 工具面、不写进任何
// 任务说明书**。配置文件在项目根 skills_config.json，缺文件=默认值。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Figure 是作图数值判据的用户可调参数。
type Figure struct {
	// MinOverlap 墨迹重合度下限（默认 0.17，用户指定值）。
	MinOverlap float64 `json:"min_overlap" yaml:"min_overlap"`
	// MaxScaleDev 尺寸偏差上限（mm 口径，T312）。
	MaxScaleDev float64 `json:"max_scale_dev" yaml:"max_scale_dev"`
	// AcceptMinSubmits accept_as_is 放行所需最少提交次数（防偷懒）。
	AcceptMinSubmits int `json:"accept_min_submits" yaml:"accept_min_submits"`
}

// Split 是 PDF 拆分的用户可调参数。
type Split struct {
	PagesPerPart int `json:"pages_per_part" yaml:"pages_per_part"`
}

// Convert 是章节转换的用户可调参数。
type Convert struct {
	MaxLinesPerChapter int `json:"max_lines_per_chapter" yaml:"max_lines_per_chapter"`
}

// Style 是样式/字体的用户可调参数（doctor 的回退建议读这里）。
type Style struct {
	FontFamily   string `json:"font_family" yaml:"font_family"`     // 主字体（缺失时的回退由 FallbackFont 兜）
	FallbackFont string `json:"fallback_font" yaml:"fallback_font"`   // 特殊字体缺失时的回退（默认开源字体）
	PilotChapter string `json:"pilot_chapter" yaml:"pilot_chapter"`   // 试点章（style 先试转的章，gen-convert 报试点标记）
}

// MinerU 是 MinerU API 的用户可调参数（token 走环境变量 MINERU_TOKEN，不落盘）。
type MinerU struct {
	APIBaseURL    string `json:"api_base_url" yaml:"api_base_url"`
	ModelVersion  string `json:"model_version" yaml:"model_version"`
	IsOCR         bool   `json:"is_ocr" yaml:"is_ocr"`
	EnableFormula bool   `json:"enable_formula" yaml:"enable_formula"`
	EnableTable   bool   `json:"enable_table" yaml:"enable_table"`
	Language      string `json:"language" yaml:"language"`
	PollInterval  int    `json:"poll_interval" yaml:"poll_interval"`
	PollTimeout   int    `json:"poll_timeout" yaml:"poll_timeout"`
	UploadTimeout int    `json:"upload_timeout" yaml:"upload_timeout"`
}

// Upstream 上游与兜底通道的用户级配置（skills 自带 config.yaml 的 upstream 段；
// 默认值在 upgrade 包，这里只承载用户私有覆盖项，如自部署 relay 地址）。
type Upstream struct {
	// RelayURL 用户部署的 relay 匿名兜底服务器地址（troubleshoot relay-submit 用）。
	RelayURL string `json:"relay_url" yaml:"relay_url"`
}

// Config 是完整配置。
type Config struct {
	Figure   Figure   `json:"figure" yaml:"figure"`
	Split    Split    `json:"split" yaml:"split"`
	Convert  Convert  `json:"convert" yaml:"convert"`
	Style    Style    `json:"style" yaml:"style"`
	MinerU   MinerU   `json:"mineru" yaml:"mineru"`
	Upstream Upstream `json:"upstream" yaml:"upstream"`
}

// Defaults 是出厂默认（用户未写任何配置时的判据值）。
var Defaults = Config{
	Figure:  Figure{MinOverlap: 0.17, MaxScaleDev: 0.10, AcceptMinSubmits: 3},
	Split:   Split{PagesPerPart: 100},
	Convert: Convert{MaxLinesPerChapter: 300},
	Style:   Style{FontFamily: "", FallbackFont: "TeX Gyre Termes"},
}

// Path 项目配置文件路径。
func Path(projectRoot string) string {
	return filepath.Join(projectRoot, "skills_config.json")
}

// Load 读配置（缺文件/损坏 → 默认值 + 该错误可由调用方选择忽略）。
func Load(projectRoot string) (*Config, error) {
	cfg := Defaults
	raw, err := os.ReadFile(Path(projectRoot))
	if os.IsNotExist(err) {
		return &cfg, nil
	}
	if err != nil {
		return &cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return &Defaults, err // 损坏配置回默认，不让坏文件绕过门槛
	}
	return &cfg, nil
}

// Save 原子写配置。
func Save(projectRoot string, cfg *Config) error {
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path(projectRoot) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path(projectRoot))
}
