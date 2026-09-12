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

// Image estimate methods. The method is a user choice because vision endpoints
// bill images differently: some charge a flat rate per image whatever it shows,
// others scale with the pixel grid.
const (
	// ImageMethodFixed charges one constant per image (default 1100 tokens:
	// measured ~1050 on deepseek-v4.1-flash, where large images saturate).
	ImageMethodFixed = "fixed"
	// ImageMethodPixels scales with size: clamp(width*height/PxPerToken,
	// MinTokens, MaxTokens) (default 750 px/token: measured 2.88M px -> 3697
	// tokens on glm-5.3-flash-official, i.e. ~780 px/token).
	ImageMethodPixels = "pixels"
	// ImageMethodNone counts no image tokens at all (the vendor number in the
	// usage line still contains them; this only switches the local estimate off).
	ImageMethodNone = "none"
)

// ImageEstimate is the LOCAL per-image token estimate: which method to use and
// its parameters. It never touches a provider number — t="usage" lines and the
// preview page's metrics copy the vendor's prompt_tokens verbatim.
//
// 每张图能算多少 token 取决于厂商（同一张图不同模型差 3~10 倍），所以这里是
// 两台模型可各自选一套规则：fixed（每张一个固定值）或 pixels（按尺寸折算）。
// 顶层 estimate: 是全局默认，models.<名>.image_tokens 只覆盖那一条。
type ImageEstimate struct {
	// Method is "fixed", "pixels" or "none". Empty = pixels.
	Method string
	// Tokens is the per-image value for method=fixed. 0 = 1100.
	Tokens int
	// PxPerToken is how many pixels one prompt token covers for
	// method=pixels. 0 = 750.
	PxPerToken int
	// MinTokens / MaxTokens clamp one image for method=pixels. 0 = 85/4096.
	MinTokens int
	MaxTokens int
}

// DefaultImageEstimate is the built-in rule: pixels, 750 px/token, clamped to
// [85, 4096] (an image with unknown dimensions then costs PxPerToken's
// companion constant, Tokens).
func DefaultImageEstimate() ImageEstimate {
	return ImageEstimate{
		Method:     ImageMethodPixels,
		Tokens:     1100,
		PxPerToken: 750,
		MinTokens:  85,
		MaxTokens:  4096,
	}
}

// Normalized fills every unset field from the default and keeps the bounds
// ordered, so a config that sets only one key still behaves sanely.
func (e ImageEstimate) Normalized() ImageEstimate {
	d := DefaultImageEstimate()
	switch strings.ToLower(strings.TrimSpace(e.Method)) {
	case ImageMethodFixed:
		e.Method = ImageMethodFixed
	case ImageMethodNone:
		e.Method = ImageMethodNone
	case ImageMethodPixels, "":
		e.Method = ImageMethodPixels
	default:
		e.Method = d.Method
	}
	if e.Tokens <= 0 {
		e.Tokens = d.Tokens
	}
	if e.PxPerToken <= 0 {
		e.PxPerToken = d.PxPerToken
	}
	if e.MinTokens <= 0 {
		e.MinTokens = d.MinTokens
	}
	if e.MaxTokens <= 0 {
		e.MaxTokens = d.MaxTokens
	}
	if e.MaxTokens < e.MinTokens {
		e.MaxTokens = e.MinTokens
	}
	return e
}

// Describe renders the rule in one line ("fixed 1100/张",
// "pixels 750px per token（85–4096）") — the preview page states the rule it
// actually used instead of keeping a copy that can go stale.
func (e ImageEstimate) Describe() string {
	e = e.Normalized()
	switch e.Method {
	case ImageMethodFixed:
		return "fixed " + itoa(e.Tokens) + "/张"
	case ImageMethodNone:
		return "不估算（本地按 0 计）"
	default:
		return "pixels " + itoa(e.PxPerToken) + "px per token（" + itoa(e.MinTokens) + "–" + itoa(e.MaxTokens) + "）"
	}
}

// PerImage is the estimate for one image under this rule. width/height <= 0
// mean "unknown": pixels mode then falls back to the fixed constant.
func (e ImageEstimate) PerImage(width, height int) int {
	e = e.Normalized()
	switch e.Method {
	case ImageMethodNone:
		return 0
	case ImageMethodFixed:
		return e.Tokens
	}
	if width <= 0 || height <= 0 {
		return e.Tokens
	}
	tokens := width * height / e.PxPerToken
	if tokens < e.MinTokens {
		tokens = e.MinTokens
	}
	if tokens > e.MaxTokens {
		tokens = e.MaxTokens
	}
	return tokens
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// The process-wide registry: one default rule plus per-model overrides. Keys
// are matched case-insensitively and may be either the config entry name
// (models.<name>) or the wire model id (models.<name>.model) — a session only
// knows the latter, the viewer's report the former.
var (
	estimateMu     sync.RWMutex
	estimateCfg    = DefaultImageEstimate()
	estimateModels = map[string]ImageEstimate{}
)

// SetEstimateConfig replaces the global (default) rule. Zero fields keep their
// defaults, so a config that only sets one key still gets sane bounds.
func SetEstimateConfig(c ImageEstimate) {
	estimateMu.Lock()
	estimateCfg = c.Normalized()
	estimateMu.Unlock()
}

// SetModelEstimate registers the rule for one model, overriding the global one
// for that model only. Same field semantics as SetEstimateConfig.
func SetModelEstimate(model string, c ImageEstimate) {
	key := estimateKey(model)
	if key == "" {
		return
	}
	estimateMu.Lock()
	if estimateModels == nil {
		estimateModels = map[string]ImageEstimate{}
	}
	estimateModels[key] = c.Normalized()
	estimateMu.Unlock()
}

func estimateKey(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// ResetEstimates drops every configured rule (global and per-model) and goes
// back to the built-in default. The registry is process-wide, so a caller that
// installed a config can undo it; tests use it to stay independent.
func ResetEstimates() {
	estimateMu.Lock()
	estimateCfg = DefaultImageEstimate()
	estimateModels = map[string]ImageEstimate{}
	estimateMu.Unlock()
}

// EstimateSettings returns the global rule.
func EstimateSettings() ImageEstimate {
	estimateMu.RLock()
	defer estimateMu.RUnlock()
	return estimateCfg
}

// EstimateForModel returns the rule that applies to one model: its own override
// when configured, otherwise the global default.
func EstimateForModel(model string) ImageEstimate {
	estimateMu.RLock()
	defer estimateMu.RUnlock()
	if e, ok := estimateModels[estimateKey(model)]; ok {
		return e
	}
	return estimateCfg
}

// ImageTokensForModel estimates one image of known dimensions for a model.
func ImageTokensForModel(model string, width, height int) int {
	return EstimateForModel(model).PerImage(width, height)
}

// ImageTokens estimates what one image costs in the prompt under the global
// rule (callers that know their model use ImageTokensForModel).
func ImageTokens(width, height int) int {
	return EstimateForModel("").PerImage(width, height)
}

// ImageTokensOfBytesForModel estimates one image from its encoded bytes (PNG/
// JPEG/GIF/WebP). Bytes in an unknown format cost the rule's fixed constant.
func ImageTokensOfBytesForModel(model string, raw []byte) int {
	e := EstimateForModel(model)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return e.PerImage(0, 0)
	}
	return e.PerImage(cfg.Width, cfg.Height)
}

// ImageTokensOfBytes is ImageTokensOfBytesForModel under the global rule.
func ImageTokensOfBytes(raw []byte) int { return ImageTokensOfBytesForModel("", raw) }

// ImageTokensOfBase64ForModel is ImageTokensOfBytesForModel for a base64
// payload without the data: prefix (the shape tool images travel in).
func ImageTokensOfBase64ForModel(model, b64 string) int {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return EstimateForModel(model).PerImage(0, 0)
	}
	return ImageTokensOfBytesForModel(model, raw)
}

// ImageTokensOfBase64 is ImageTokensOfBase64ForModel under the global rule.
func ImageTokensOfBase64(b64 string) int { return ImageTokensOfBase64ForModel("", b64) }

// ImageTokensOfFileForModel estimates one image from a file on disk (only the
// header is read). A missing/unreadable file costs the fixed constant so an
// estimate never fails a viewer pass.
func ImageTokensOfFileForModel(model, path string) int {
	e := EstimateForModel(model)
	f, err := os.Open(path)
	if err != nil {
		return e.PerImage(0, 0)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return e.PerImage(0, 0)
	}
	return e.PerImage(cfg.Width, cfg.Height)
}

// ImageTokensOfFile is ImageTokensOfFileForModel under the global rule.
func ImageTokensOfFile(path string) int { return ImageTokensOfFileForModel("", path) }

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
