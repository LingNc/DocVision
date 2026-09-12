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
	// 没有尺寸信息 → 按规则里的常量计（全局默认规则下是 1100）。
	if got, want := messageTokens(m, ""), ImageTokensForModel("", 0, 0); got < want {
		t.Fatalf("image cost missing: %d < %d", got, want)
	}
	// 带上尺寸估算时按那一张算，不再退回常量。
	m2 := ChatMessage{Role: "user", Content: m.Content, ImageTokens: []int{3000}}
	if got := messageTokens(m2, "") - messageTokens(ChatMessage{Role: "user", Content: []map[string]interface{}{
		{"type": "text", "text": "hello world this is a test"},
	}}, ""); got != 3000 {
		t.Fatalf("per-image estimate ignored: delta %d", got)
	}
	// 同一个模型名下的消息在带尺寸时按**该模型**的规则算。
	defer ResetEstimates()
	SetModelEstimate("wire-flat", ImageEstimate{Method: ImageMethodFixed, Tokens: 850})
	if got := messageTokens(ChatMessage{Role: "user", Content: m.Content}, "wire-flat"); got !=
		messageTokens(ChatMessage{Role: "user", Content: []map[string]interface{}{
			{"type": "text", "text": "hello world this is a test"},
		}}, "wire-flat")+850 {
		t.Fatalf("per-model rule ignored: %d", got)
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
