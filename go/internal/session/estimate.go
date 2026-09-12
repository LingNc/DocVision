package session

import (
	"bytes"
	"encoding/base64"
	"image"
	"os"
	"strings"
	"sync"

	// 头解析用标准库的 image.DecodeConfig；webp 由该包自己注册。
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// EstimateConfig tunes the LOCAL token estimate. It never touches a provider
// number: t="usage" lines and the preview page's metrics copy the vendor's
// prompt_tokens/completion_tokens verbatim.
//
// 图片按尺寸折算的依据：2026-09-10 的 glm-5.3-flash-official 运行日志里，
// 单张图片给 prompt_tokens 带来的增量与 宽×高/750 基本吻合（292k 像素→440、
// 986k→1285、2.32M→2987），所以默认 750 像素/token；夹在 [85, 4096] 之间
// 避免极小图算成 0、超大图把估算推到天上。取不到尺寸（未知格式）时退回
// 每张一个固定常量。
type EstimateConfig struct {
	// ImagePxPerToken is how many pixels one prompt token covers. 0 = 750.
	ImagePxPerToken int
	// ImageTokensMin / ImageTokensMax clamp one image's estimate. 0 = 85/4096.
	ImageTokensMin int
	ImageTokensMax int
	// ImageFallback is the per-image estimate when the dimensions are unknown.
	// 0 = 1100.
	ImageFallback int
}

// DefaultEstimateConfig is the built-in estimate tuning.
func DefaultEstimateConfig() EstimateConfig {
	return EstimateConfig{ImagePxPerToken: 750, ImageTokensMin: 85, ImageTokensMax: 4096, ImageFallback: 1100}
}

// SetEstimateConfig replaces the process-wide estimate tuning; zero fields keep
// their defaults, so a config that only sets one key still gets sane bounds.
func SetEstimateConfig(c EstimateConfig) {
	d := DefaultEstimateConfig()
	if c.ImagePxPerToken <= 0 {
		c.ImagePxPerToken = d.ImagePxPerToken
	}
	if c.ImageTokensMin <= 0 {
		c.ImageTokensMin = d.ImageTokensMin
	}
	if c.ImageTokensMax <= 0 {
		c.ImageTokensMax = d.ImageTokensMax
	}
	if c.ImageTokensMax < c.ImageTokensMin {
		c.ImageTokensMax = c.ImageTokensMin
	}
	if c.ImageFallback <= 0 {
		c.ImageFallback = d.ImageFallback
	}
	estimateMu.Lock()
	estimateCfg = c
	estimateMu.Unlock()
}

// EstimateSettings returns the current estimate tuning.
func EstimateSettings() EstimateConfig {
	estimateMu.RLock()
	defer estimateMu.RUnlock()
	return estimateCfg
}

var (
	estimateMu  sync.RWMutex
	estimateCfg = DefaultEstimateConfig()
)

// ImageTokens estimates what one image costs in the prompt. width/height <= 0
// mean "unknown" and yield the configured fallback constant.
func ImageTokens(width, height int) int {
	c := EstimateSettings()
	if width <= 0 || height <= 0 {
		return c.ImageFallback
	}
	px := width * height
	tokens := px / c.ImagePxPerToken
	if tokens < c.ImageTokensMin {
		tokens = c.ImageTokensMin
	}
	if tokens > c.ImageTokensMax {
		tokens = c.ImageTokensMax
	}
	return tokens
}

// ImageTokensOfBytes estimates one image from its encoded bytes (PNG/JPEG/GIF/
// WebP). Bytes in an unknown format fall back to the constant.
func ImageTokensOfBytes(raw []byte) int {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return ImageTokens(0, 0)
	}
	return ImageTokens(cfg.Width, cfg.Height)
}

// ImageTokensOfBase64 is ImageTokensOfBytes for a base64 payload without the
// data: prefix (the shape tool images travel in).
func ImageTokensOfBase64(b64 string) int {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return ImageTokens(0, 0)
	}
	return ImageTokensOfBytes(raw)
}

// ImageTokensOfFile estimates one image from a file on disk (only the header is
// read). A missing/unreadable file falls back to the constant so an estimate
// never fails a viewer pass.
func ImageTokensOfFile(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return ImageTokens(0, 0)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return ImageTokens(0, 0)
	}
	return ImageTokens(cfg.Width, cfg.Height)
}

// TextTokens is the local text estimate the compaction threshold uses, exported
// for callers that display the same number (the session preview shows it with a
// ≈ prefix). An empty string is 0: the estimator's +8 is the per-message wrapper
// allowance, not a property of one text blob.
func TextTokens(s string) int {
	if s == "" {
		return 0
	}
	return textTokens(s)
}
