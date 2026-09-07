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
	got := messageTokens(m)
	if got < imageTokens {
		t.Fatalf("image cost missing: %d", got)
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
