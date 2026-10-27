package config

import "testing"

// T204：核对会话的工具轮次按递归层级加宽——父节点要通读整棵子树，最里层只管
// 自己那一片。默认 50 + 20×（离最深一层还有几层），也可以改成按层翻倍。
func TestToolRoundsAtLevels(t *testing.T) {
	base := SessionTuning{MaxToolRounds: 50, MaxToolRoundsPerLevel: 20, MaxToolRoundsFactor: 1}
	for levels, want := range map[int]int{0: 50, 1: 70, 2: 90, 3: 110} {
		if got := base.ToolRoundsAtLevels(levels); got != want {
			t.Errorf("线性 %d 层 = %d, want %d", levels, got, want)
		}
	}
	doubling := SessionTuning{MaxToolRounds: 50, MaxToolRoundsPerLevel: 0, MaxToolRoundsFactor: 2}
	if got := doubling.ToolRoundsAtLevels(3); got != 400 {
		t.Errorf("翻倍 3 层 = %d, want 400", got)
	}
	// 只有倍数、没有加数：1~3 层分别 100/200/400。
	for levels, want := range map[int]int{1: 100, 2: 200, 3: 400} {
		if got := doubling.ToolRoundsAtLevels(levels); got != want {
			t.Errorf("翻倍 %d 层 = %d, want %d", levels, got, want)
		}
	}
	// 负的 per_level = 只在倍数上走；负/零的 base 保持原样（0=不限、负数=无上限）。
	flat := SessionTuning{MaxToolRounds: 50, MaxToolRoundsPerLevel: -1, MaxToolRoundsFactor: 1}
	if got := flat.ToolRoundsAtLevels(3); got != 50 {
		t.Errorf("不加宽 = %d, want 50", got)
	}
	if got := (SessionTuning{MaxToolRounds: 0}).ToolRoundsAtLevels(3); got != 0 {
		t.Errorf("0=不限 应保持 0，得到 %d", got)
	}
	if got := (SessionTuning{MaxToolRounds: -5}).ToolRoundsAtLevels(3); got != -5 {
		t.Errorf("负数=无上限 应保持 -5，得到 %d", got)
	}
	// 离谱的倍数有上限兜底，不至于把一次核对变成一天的工具调用。
	huge := SessionTuning{MaxToolRounds: 50, MaxToolRoundsPerLevel: 0, MaxToolRoundsFactor: 10}
	if got := huge.ToolRoundsAtLevels(3); got != checkerToolRoundsCeil {
		t.Errorf("超大倍数 = %d, want %d", got, checkerToolRoundsCeil)
	}
}

// 未配置时 checker 块拿到默认：50 轮 + 每层 20、纯线性；convert 块不受影响。
func TestCheckerSessionDefaultsPerLevel(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	c := cfg
	s := c.LatexSession("checker")
	if s.MaxToolRounds != checkerDefaultToolRounds {
		t.Fatalf("checker 基础轮次 = %d", s.MaxToolRounds)
	}
	if s.MaxToolRoundsPerLevel != checkerDefaultRoundsPerLevel || s.MaxToolRoundsFactor != 1 {
		t.Fatalf("checker 加宽默认 = %d / %v", s.MaxToolRoundsPerLevel, s.MaxToolRoundsFactor)
	}
	if got := s.ToolRoundsAtLevels(3); got != 110 {
		t.Fatalf("顶层核对轮次 = %d, want 110", got)
	}
	conv := c.LatexSession("convert")
	if conv.MaxToolRoundsPerLevel != 0 || conv.ToolRoundsAtLevels(3) != conv.MaxToolRounds {
		t.Fatalf("convert 不该被加宽: %+v", conv)
	}
}
