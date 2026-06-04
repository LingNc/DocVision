// Package img2text converts document images to structured text by calling
// an OpenAI-compatible vision API. The package mirrors the Python reference
// (python/img2text.py) and is split across several small files:
//
//   - image.go    : image loading, RGBA->RGB conversion, resize, JPEG encode
//   - context.go  : context-window line selection with delta support
//   - tool.go     : OpenAI tool definition for incremental context expansion
//   - client.go   : HTTP client for the chat completions API
//   - processor.go: multi-round tool calling, retry/backoff, format-fix retry
//   - progress.go : per-item JSON progress persistence
//   - runner.go   : top-level Run entry point with producer/consumer dispatch
package img2text

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	// WebP support via the extended image package.
	_ "golang.org/x/image/webp"

	xdraw "golang.org/x/image/draw"
)

// defaultJPEGQuality is the encoding quality used when re-encoding images
// before sending them to the AI. Matches the Python reference (quality=85).
const defaultJPEGQuality = 85

// ImageToBase64 opens imagePath, normalises it to RGB (compositing RGBA
// over a white background), downscales it so neither side exceeds
// maxSize, encodes it as JPEG and returns the base64 string.
//
// The blank import of "golang.org/x/image/webp" above registers the WebP
// decoder so the standard library's image.Decode can handle .webp files.
func ImageToBase64(imagePath string, maxSize int) (string, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open image %s: %w", imagePath, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("decode image %s: %w", imagePath, err)
	}

	// RGBA / other alpha-bearing modes are composited over a solid white
	// background. This matches the Python reference which uses
	// `img.convert("RGB")` (PIL's behaviour for RGBA is to flatten on a
	// black background; we deliberately use white to match the visual
	// intent of the source pipeline).
	img = flattenAlpha(img)

	// Resize only if needed. We use CatmullRom which is a high-quality
	// bicubic-style filter (similar to PIL's LANCZOS) but with lower
	// setup cost.
	img = maybeResize(img, maxSize)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: defaultJPEGQuality}); err != nil {
		return "", fmt.Errorf("encode jpeg: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// flattenAlpha converts any alpha-bearing image to an opaque RGB image by
// drawing it on top of a white background. For fully opaque sources the
// composite is a no-op write of the same pixels onto a white canvas, so
// we apply it unconditionally to keep the type surface narrow.
func flattenAlpha(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	// Paint background white.
	draw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	// Composite source on top.
	draw.Draw(dst, b, src, image.Point{}, draw.Over)
	return dst
}

// maybeResize returns a scaled copy of src when either dimension exceeds
// maxSize, preserving the aspect ratio. The scaling uses a CatmullRom
// kernel which is the standard high-quality choice in the x/image/draw
// package.
func maybeResize(src image.Image, maxSize int) image.Image {
	if maxSize <= 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSize && h <= maxSize {
		return src
	}
	r := float64(maxSize) / float64(w)
	if rh := float64(maxSize) / float64(h); rh < r {
		r = rh
	}
	nw := int(float64(w) * r)
	nh := int(float64(h) * r)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}
