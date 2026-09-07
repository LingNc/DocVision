package latex

import "mineru-tools/internal/img2text"

// img2textImageToBase64 loads and normalises an image via the img2text
// helper (RGB flatten + resize + JPEG base64).
func img2textImageToBase64(path string) (string, error) {
	return img2text.ImageToBase64(path, 1280)
}
