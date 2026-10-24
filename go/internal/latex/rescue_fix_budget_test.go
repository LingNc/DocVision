package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// P60 第四步：整章重开的预算收口。
//
// 老行为：整合完的父章节被核对打回，就归档这一版、换全新会话把整章连同子章节
// 一起从头再跑一遍。子章节的修复轮数已经用光之后再整章重来只是把同一段很贵的
// 对话重放一次——所以修复打开、且确有子章节把 `latex.fix.max_rounds` 用光时，
// 父章节不再整章重开：保留这一版整合产物继续往上走（未解决的问题照旧走 issues
// 库/报告），只记一行警告。下面三条测试分别钉住：预算用光→不重开；预算还在→
// 照旧重开；开关关着→照旧重开。

// p60WriteFixTranscript 造一条带 n 条 fix_target 锚点的修复转录（n 就是这一支
// 用掉的修复轮数）。
func p60WriteFixTranscript(t *testing.T, work, parentBase, child string, n int) {
	t.Helper()
	dir := filepath.Join(work, "_subdivision", child, "node", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(`{"t":"meta","kind":"fix_target","convert":"convert_` + parentBase + "_" + child + `","issue_ids":["ISSUE-1"]}` + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "fix_"+parentBase+"_"+child+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixBudgetExhausted 的判据本身：只有"修复打开 + 有子章节 + 某一支的轮数已满"
// 三条同时成立才收口；其余一律返回空（= 走今天的行为）。
func TestFixBudgetExhaustedHelper(t *testing.T) {
	children := []ChapterSubdivision{{Title: "First"}, {Title: "Second"}}
	cases := []struct {
		name    string
		enabled bool
		rounds  int
		max     int
		child   string // 把轮数写进哪个子章节（空 = 一条修复转录都不写）
		want    string
	}{
		{name: "开关关着", enabled: false, rounds: 2, max: 2, child: "child_01", want: ""},
		{name: "没有子章节进过修复", enabled: true, rounds: 0, max: 2, child: "", want: ""},
		{name: "预算还有", enabled: true, rounds: 1, max: 2, child: "child_01", want: ""},
		{name: "预算用光", enabled: true, rounds: 2, max: 2, child: "child_01", want: "child_01"},
		{name: "第二个子章节用光", enabled: true, rounds: 3, max: 3, child: "child_02", want: "child_02"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := keepTestRunner(t)
			flag := tc.enabled
			r.cfg.Latex.Fix = config.FixConfig{Enabled: &flag, MaxRounds: tc.max}
			work := t.TempDir()
			if tc.child != "" {
				p60WriteFixTranscript(t, work, "chapter_01", tc.child, tc.rounds)
			}
			got, used := r.fixBudgetExhausted(work, "chapter_01", children)
			if got != tc.want {
				t.Fatalf("fixBudgetExhausted = %q/%d，want %q", got, used, tc.want)
			}
			if tc.want == "" && used != 0 {
				t.Errorf("没有收口时不该报轮数: %d", used)
			}
		})
	}
}

// (a) 修复打开 + 预算用光（max_rounds=1，那一支已经修过一轮）+ 父章节核对不过：
// **不**整章重开——只跑过一次父章节转换会话与一次父章节核对，整合产物留下，
// 并记一行"预算用完"的警告。
func TestFixBudgetExhaustedSkipsChapterReattempt(t *testing.T) {
	stats, logText, proj := p60RunWith(t, p60Options{
		enabled: true, maxRounds: 1, parentIssues: true, checkerRounds: 1,
	})
	if stats.runErr != "" {
		t.Fatalf("预算用光时这一章应当带着未解决问题继续，不该失败: %s", stats.runErr)
	}
	// 修复真的跑过一轮（否则"预算用光"这个前提就不成立）。
	if got := stats.fixes["chapter_01_child_01"]; got == 0 {
		t.Fatalf("计划点名的 child_01 应当跑过修复会话: %v", stats.fixes)
	}
	if stats.parentSessions != 1 {
		t.Errorf("父章节转换会话 = %d，期望 1（不整章重开）", stats.parentSessions)
	}
	if stats.integrations != 1 {
		t.Errorf("父章节整合会话 = %d，期望 1", stats.integrations)
	}
	// 父章节核对 = 首次 1 次 + 反馈轮 3 次（`checker_rounds: 1` 会被下限抬到 3）。
	// 重点是"没有第二次尝试"：整章重开会再来一整套，这里只多出反馈轮。以前这条
	// 写的是 1——那时反馈轮都在重放上一轮的结论、一次都没真问过模型（T234）。
	if want := 1 + checkerAttemptBudget(1, 0); stats.parentCheckers != want {
		t.Errorf("父章节核对次数 = %d，期望 %d（首次 + 反馈轮，且没有第二次尝试）", stats.parentCheckers, want)
	}
	if dir := filepath.Join(proj, "work", "attempts", "chapter_01"); pathExists(dir) {
		t.Errorf("不该归档这一版尝试（说明整章重开了）: %s", dir)
	}
	// 整合产物照旧进书（不是"静悄悄丢掉这一章"）。
	if !fileExists(filepath.Join(proj, "work", "chapters", "chapter_01.tex")) {
		t.Error("预算收口后整合产物仍要落进 work/chapters/")
	}
	if !strings.Contains(logText, "不再整章重开") || !strings.Contains(logText, "修复轮数已用完") {
		t.Errorf("应当记一行「修复预算用完」的警告，日志里没有：\n%s", logText)
	}
}

// (b) 修复打开但预算还在（max_rounds=2，那一支只用掉一轮）：行为与今天一样——
// 核对不过就归档、整章重开，直到尝试次数用尽。
func TestFixBudgetAvailableKeepsChapterReattempt(t *testing.T) {
	stats, logText, proj := p60RunWith(t, p60Options{
		enabled: true, maxRounds: 2, parentIssues: true, checkerRounds: 1, expectError: true,
	})
	if stats.runErr == "" {
		t.Fatal("核对始终不过时这一章最终应当失败（尝试次数用尽）")
	}
	if stats.parentSessions < 2 {
		t.Errorf("预算还在时应当整章重开: 父章节转换会话 = %d，期望 >= 2", stats.parentSessions)
	}
	if dir := filepath.Join(proj, "work", "attempts", "chapter_01", "try1"); !pathExists(dir) {
		t.Errorf("整章重开应当先把上一版归档到 work/attempts/chapter_01/try1: %s", dir)
	}
	if strings.Contains(logText, "修复轮数已用完") {
		t.Error("预算还没用光，不该出现预算收口的警告")
	}
}

// (c) 开关关着：与今天一字不差——核对不过就归档、整章重开，没有任何修复相关
// 的会话或警告。
func TestFixDisabledKeepsChapterReattempt(t *testing.T) {
	stats, logText, proj := p60RunWith(t, p60Options{
		parentIssues: true, checkerRounds: 1, expectError: true,
	})
	if stats.dispatchOffered {
		t.Error("关闭时不该把 dispatch_fix 注册给任何会话")
	}
	if len(stats.fixes) != 0 {
		t.Errorf("关闭时不该有修复会话: %v", stats.fixes)
	}
	if stats.runErr == "" {
		t.Fatal("核对始终不过时这一章最终应当失败（尝试次数用尽）")
	}
	if stats.parentSessions < 2 {
		t.Errorf("关闭时应当整章重开: 父章节转换会话 = %d，期望 >= 2", stats.parentSessions)
	}
	if dir := filepath.Join(proj, "work", "attempts", "chapter_01", "try1"); !pathExists(dir) {
		t.Errorf("整章重开应当先把上一版归档: %s", dir)
	}
	if strings.Contains(logText, "修复轮数已用完") || strings.Contains(logText, "不再整章重开") {
		t.Error("关闭时不该出现修复预算相关的日志")
	}
}
