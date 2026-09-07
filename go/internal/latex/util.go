package latex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// b64Encode encodes bytes as standard base64 (no data: prefix).
func b64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// imageRefRe matches markdown/HTML image references exactly like the
// img2text package does, so both pipelines agree on what an image is.
var imageRefRe = regexp.MustCompile(`(?i)(?:!\[.*?\]\(|<img[^>]*?src=["'])(images/.+?\.(?:jpg|jpeg|png|gif|webp))(?:\)|["'][^>]*>)`)

// sanitizeName turns an arbitrary string into a filesystem-safe name.
func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case unicodeIsSafeLetter(r):
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x4E00 && r <= 0x9FFF: // CJK
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		out = "img"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// unicodeIsSafeLetter reports ASCII letters (CJK handled separately so
// generated figure names stay readable for Chinese documents).
func unicodeIsSafeLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// fileExists reports whether path exists.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// parseJSONObject extracts the first balanced JSON object from a model
// reply (models love wrapping JSON in prose or fences).
func parseJSONObject(s string) (map[string]interface{}, error) {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil, fmt.Errorf("回复中未找到 JSON 对象")
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		switch c {
		case '\\':
			if inStr {
				esc = true
			}
		case '"':
			inStr = !inStr
		case '{':
			if !inStr {
				depth++
			}
		case '}':
			if !inStr {
				depth--
				if depth == 0 {
					raw := s[start : i+1]
					var out map[string]interface{}
					if err := json.Unmarshal([]byte(raw), &out); err != nil {
						return nil, fmt.Errorf("解析 JSON 失败: %w", err)
					}
					return out, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("JSON 对象未闭合")
}
