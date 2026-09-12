package session

import "testing"

func TestTextTokens(t *testing.T) {
	// CJK-heavy text costs roughly one token per rune.
	cjk := textTokens("函数图像的坐标轴")
	if cjk < 6 {
		t.Fatalf("CJK tokens too low: %d", cjk)
	}
	// ASCII costs ~1 token per 4 chars.
	ascii := textTokens("abcdefgh")
	if ascii < 2 || ascii > 12 {
		t.Fatalf("ASCII tokens out of range: %d", ascii)
	}
}

func TestMessageTokensMultimodal(t *testing.T) {
	m := ChatMessage{Role: "user", Content: []map[string]interface{}{
		{"type": "text", "text": "hello world this is a test"},
		{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64,xxx"}},
	}}
	// 没有尺寸信息 → 按配置的兜底常量计。
	if got, want := messageTokens(m), ImageTokens(0, 0); got < want {
		t.Fatalf("image cost missing: %d < %d", got, want)
	}
	// 带上尺寸估算时按那一张算，不再退回常量。
	m2 := ChatMessage{Role: "user", Content: m.Content, ImageTokens: []int{3000}}
	if got := messageTokens(m2) - messageTokens(ChatMessage{Role: "user", Content: []map[string]interface{}{
		{"type": "text", "text": "hello world this is a test"},
	}}); got != 3000 {
		t.Fatalf("per-image estimate ignored: delta %d", got)
	}
}

func TestContentString(t *testing.T) {
	if got := ContentString(ChatMessage{Content: "hi"}); got != "hi" {
		t.Fatalf("got %q", got)
	}
	if got := ContentString(ChatMessage{Content: nil}); got != "" {
		t.Fatalf("nil -> %q", got)
	}
}
