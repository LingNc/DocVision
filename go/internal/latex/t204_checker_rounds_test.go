package latex

import (
	"testing"

	"mineru-tools/internal/config"
)

// testConfigWithCheckerRounds 造一份只配了核对轮次的配置（其它字段走默认）。
// setDefaults 是 config 包内的，这里直接给 LatexSession 需要的键赋显式值。
func testConfigWithCheckerRounds(t *testing.T, base, perLevel int, factor float64) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	cfg.Latex.Sessions.Checker.MaxToolRounds = base
	cfg.Latex.Sessions.Checker.MaxToolRoundsPerLevel = perLevel
	cfg.Latex.Sessions.Checker.MaxToolRoundsFactor = factor
	return cfg
}

// T204：层级从章节 ID 数出来（`chapter_014_child_03_child_02` = 第 2 层），
// 核对会话的轮次预算按"离最深一层还有几层"加宽。
func TestChapterDepthOf(t *testing.T) {
	cases := map[string]int{
		"chapter_014":                            0,
		"chapter_014_child_04":                   1,
		"chapter_014_child_03_child_02":          2,
		"chapter_001_child_01_child_02_child_03": 3,
	}
	for base, want := range cases {
		if got := chapterDepthOf(base); got != want {
			t.Errorf("chapterDepthOf(%q) = %d, want %d", base, got, want)
		}
	}
	if maxChapterSplitDepth != 3 {
		t.Fatalf("最大递归层数变了（%d），核对轮次的层级公式要一起改", maxChapterSplitDepth)
	}
}

// 端到端口径：最里层 = 配置的基础值，每往上一层 +20（默认），顶层到 110。
func TestCheckerRoundsScaleWithDepth(t *testing.T) {
	cfg := testConfigWithCheckerRounds(t, 50, 20, 1)
	for base, want := range map[string]int{
		"chapter_014":                            110, // 第 0 层：离最深一层还有 3 层
		"chapter_014_child_04":                   90,
		"chapter_014_child_03_child_02":          70,
		"chapter_014_child_03_child_02_child_01": 50, // 最深一层：基础值
	} {
		tuning := cfg.LatexSession("checker")
		levels := maxChapterSplitDepth - chapterDepthOf(base)
		got := tuning.MaxToolRounds
		if levels > 0 {
			got = tuning.ToolRoundsAtLevels(levels)
		}
		if got != want {
			t.Errorf("%s: 核对轮次 = %d, want %d", base, got, want)
		}
	}
}

// 按层翻倍的写法：50 → 100 → 200 → 400。per_level 写负数表示"只翻倍、不另加"。
func TestCheckerRoundsDoubling(t *testing.T) {
	cfg := testConfigWithCheckerRounds(t, 50, -1, 2)
	tuning := cfg.LatexSession("checker")
	if got := tuning.ToolRoundsAtLevels(3); got != 400 {
		t.Fatalf("翻倍 3 层 = %d, want 400", got)
	}
}
