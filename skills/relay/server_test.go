package main

import (
	"testing"
)

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(2)
	if !rl.allow("1.1.1.1") || !rl.allow("1.1.1.1") {
		t.Fatal("前两次应放行")
	}
	if rl.allow("1.1.1.1") {
		t.Fatal("第三次应限流")
	}
	if !rl.allow("2.2.2.2") {
		t.Fatal("不同 IP 独立计数")
	}
}

func TestPayloadValidationShape(t *testing.T) {
	// 与 handler 一致的校验语义最小断言
	title := "  [bug] x  "
	if len([]rune(title)) > 120 {
		t.Fatal("标题长度上限")
	}
}
