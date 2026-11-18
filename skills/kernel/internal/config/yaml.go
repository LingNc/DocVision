package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// LoadLayered 分层加载：技能自带 config.yaml（默认） ← 项目 skills_config.json（覆盖）。
// 密钥类不在此层（走 profile.env/环境变量）。config.yaml 缺失=用内置 Defaults。
func LoadLayered(skillRoot, projectRoot string) (*Config, error) {
	cfg := Defaults
	if ypath := filepath.Join(skillRoot, "config.yaml"); ypath != "" {
		if raw, err := os.ReadFile(ypath); err == nil {
			var yc Config
			if err := yaml.Unmarshal(raw, &yc); err != nil {
				return &cfg, fmt.Errorf("config.yaml 解析失败（沿用内置默认）: %w", err)
			}
			cfg = mergeDefaults(cfg, yc)
		}
	}
	// 项目级覆盖（JSON）
	if raw, err := os.ReadFile(Path(projectRoot)); err == nil {
		var pc Config
		if jerr := yaml.Unmarshal(raw, &pc); jerr == nil {
			cfg = mergeOverride(cfg, pc)
		}
	}
	return &cfg, nil
}

// mergeDefaults：yaml 里非零值覆盖内置默认（零值=未写，保留默认）。
func mergeDefaults(base, over Config) Config {
	out := base
	if over.MinerU.APIBaseURL != "" {
		out.MinerU.APIBaseURL = over.MinerU.APIBaseURL
	}
	if over.MinerU.ModelVersion != "" {
		out.MinerU.ModelVersion = over.MinerU.ModelVersion
	}
	out.MinerU.IsOCR = over.MinerU.IsOCR
	out.MinerU.EnableFormula = over.MinerU.EnableFormula
	out.MinerU.EnableTable = over.MinerU.EnableTable
	if over.MinerU.Language != "" {
		out.MinerU.Language = over.MinerU.Language
	}
	if over.MinerU.PollInterval > 0 {
		out.MinerU.PollInterval = over.MinerU.PollInterval
	}
	if over.MinerU.PollTimeout >= 0 && (over.MinerU.PollTimeout > 0 || hasKeyPollTimeoutZero) {
		out.MinerU.PollTimeout = over.MinerU.PollTimeout
	}
	if over.MinerU.UploadTimeout > 0 {
		out.MinerU.UploadTimeout = over.MinerU.UploadTimeout
	}
	if over.Figure.MinOverlap > 0 {
		out.Figure.MinOverlap = over.Figure.MinOverlap
	}
	if over.Figure.MaxScaleDev > 0 {
		out.Figure.MaxScaleDev = over.Figure.MaxScaleDev
	}
	if over.Figure.AcceptMinSubmits > 0 {
		out.Figure.AcceptMinSubmits = over.Figure.AcceptMinSubmits
	}
	if over.Split.PagesPerPart > 0 {
		out.Split.PagesPerPart = over.Split.PagesPerPart
	}
	if over.Convert.MaxLinesPerChapter > 0 {
		out.Convert.MaxLinesPerChapter = over.Convert.MaxLinesPerChapter
	}
	if over.Style.FontFamily != "" {
		out.Style.FontFamily = over.Style.FontFamily
	}
	if over.Style.FallbackFont != "" {
		out.Style.FallbackFont = over.Style.FallbackFont
	}
	if over.Style.PilotChapter != "" {
		out.Style.PilotChapter = over.Style.PilotChapter
	}
	return out
}

// hasKeyPollTimeoutZero：yaml 显式写 0 时尊重（yaml.Unmarshal 后无法区分未写/写 0，
// 用 node 级解析太重；约定：项目 skills_config.json 的 0 生效（Load 原本语义），yaml 的 0 也生效）。
var hasKeyPollTimeoutZero = true

// mergeOverride：项目 JSON 非零值覆盖（与 mergeDefaults 同语义；JSON 经 yaml 包解析）。
func mergeOverride(base, over Config) Config { return mergeDefaults(base, over) }
