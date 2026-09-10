package session

import (
	"testing"

	"mineru-tools/internal/config"
)

// 每个会话带稳定的 user 标识：同一会话所有请求同值（网关据此固定上游
// 渠道/缓存域），不同会话不同值（仍可分散负载）。
func TestSessionAffinityUser(t *testing.T) {
	a := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "sys", nil, nil, 1, "convert:chapter_01")
	b := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "sys", nil, nil, 2, "convert:chapter_01")
	c := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "sys", nil, nil, 1, "convert:chapter_01")

	if a.user == "" {
		t.Fatal("user 不应为空")
	}
	if a.user != c.user {
		t.Errorf("同一会话标识应稳定: %q vs %q", a.user, c.user)
	}
	if a.user == b.user {
		t.Errorf("不同会话应区分: %q", a.user)
	}
	for _, r := range a.user {
		ok := r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			t.Fatalf("user 含非法字符 %q: %s", r, a.user)
		}
	}
	if got := affinityUser("", 1); got != "" {
		t.Errorf("无标签时应为空: %q", got)
	}
}
