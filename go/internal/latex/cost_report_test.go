package latex

import "testing"

// T40：费用表「阶段」列按显示宽度对齐（CJK 按 2 列），不能按字节/字符数。
func TestPadStage(t *testing.T) {
	cases := []struct {
		s    string
		want int // 期望显示宽度
	}{
		{"vector", 6},
		{"全书修复", 8},
		{"终", 2},
		{"style-fix", 9},
	}
	for _, c := range cases {
		padded := padStage(c.s, 11)
		w := 0
		for _, r := range padded {
			if r >= 0x2E80 {
				w += 2
			} else {
				w++
			}
		}
		if w != 11 {
			t.Errorf("padStage(%q, 11) display width = %d, want 11", c.s, w)
		}
		if len(padded) != len(c.s)+11-c.want {
			t.Errorf("padStage(%q, 11) padded %d spaces, want %d", c.s, len(padded)-len(c.s), 11-c.want)
		}
	}
	// 超长不截断。
	if got := padStage("verylongstagename", 11); got != "verylongstagename" {
		t.Errorf("over-long stage name should be returned as-is, got %q", got)
	}
}
