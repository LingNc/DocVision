package sessionview

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The viewer's page shell lives in v2.go (shared between the live server and
// the static export); the web/ build lands in assets/dist and both copies are
// //go:embed'ed from there.

// staticDataBlock JSON-encodes the snapshot the same way the old inliner did:
// SetEscapeHTML keeps a transcript containing "</script>" from ending the
// data block early and turning the page into injected markup.
func staticDataBlock(payload pageData) (string, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		return "", fmt.Errorf("会话预览: 序列化会话数据失败: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
