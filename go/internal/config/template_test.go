package config

import "testing"

// The embedded template must always pass the strict validation used by
// `docvision setup` (unknown keys, placeholders, semantics).
func TestConfigTemplateValidates(t *testing.T) {
	problems := ValidateData([]byte(ConfigTemplate))
	for _, p := range problems {
		// Placeholder credentials are expected in the template and are
		// reported; everything else must be clean.
		if contains(p, "占位符") || contains(p, "不能为空") {
			continue
		}
		t.Errorf("template problem: %s", p)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
