package config

import "testing"

func TestParseAlignGrid(t *testing.T) {
	cases := []struct {
		spec   string
		canvas int
		want   int
		bad    bool
	}{
		{"", 390, 371, false},
		{"0.95S", 390, 371, false},
		{"0.95s", 100, 95, false},
		{"0.5S", 100, 50, false},
		{"32", 0, 32, false},
		{"128", 390, 128, false},
		{"1S", 0, 0, true},
		{"0.95S", 0, 0, true},
		{"abc", 390, 0, true},
		{"0.95", 390, 0, true},
		{"0S", 100, 0, true},
		{"3", 0, 8, false}, // 下限 8
	}
	for _, c := range cases {
		got, err := ParseAlignGrid(c.spec, c.canvas)
		if c.bad {
			if err == nil {
				t.Errorf("ParseAlignGrid(%q,%d) 期望报错，得到 %d", c.spec, c.canvas, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAlignGrid(%q,%d) 报错：%v", c.spec, c.canvas, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseAlignGrid(%q,%d)=%d，期望 %d", c.spec, c.canvas, got, c.want)
		}
	}
}
