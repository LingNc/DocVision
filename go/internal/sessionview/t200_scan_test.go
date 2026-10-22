package sessionview

import (
	"path/filepath"
	"strings"
	"testing"
)

// T200：预览页把两类"不是会话"的 jsonl 也当成会话列出来——P55 的工作区快照
// 日志（`work/rounds/<章>/journal-try<N>.jsonl`，侧栏因此多出十几条没有对话、
// 没有轨迹的空会话）与清理脚本的整目录备份（`<项目名>_记录_<日期>_<时刻>/`，
// 整个副本被读成一个挂着几十条会话的项目组）。两者都必须在扫描时整棵跳过。
func TestScanSkipsRoundsJournalsAndCleanBackups(t *testing.T) {
	root := t.TempDir()
	// 真正的会话：必须照旧出现。
	real := jsonl(root, "latex_project", "书", "work", "sessions", "convert_chapter_001.jsonl")
	writeFile(t, real, transcript(
		`{"t":"msg","role":"user","text":"转换第一章"}`,
		`{"t":"msg","role":"assistant","text":"提交了"}`,
	))
	// P55 快照日志 + 裸仓库：跳过。
	writeFile(t, jsonl(root, "latex_project", "书", "work", "rounds", "chapter_001", "journal-try1.jsonl"),
		`{"line":1,"commit":"abc1234"}`+"\n")
	writeFile(t, filepath.Join(root, "latex_project", "书", "work", "rounds", "chapter_001", "repo.git", "config"),
		"[core]\n\tbare = true\n")
	// 清理脚本的整目录备份：跳过（哪怕是完整的工程副本）。
	writeFile(t, jsonl(root, "latex_project", "书_记录_20261021_2308", "work", "sessions", "convert_chapter_001.jsonl"),
		transcript(`{"t":"msg","role":"user","text":"备份里的旧会话"}`))

	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(sessions) != 1 {
		var ids []string
		for _, s := range sessions {
			ids = append(ids, s.ID)
		}
		t.Fatalf("扫描到 %d 个会话（%s），want 1（只有真正的转录）", len(sessions), strings.Join(ids, ", "))
	}
	if sessions[0].ID != "latex_project/书/work/sessions/convert_chapter_001.jsonl" {
		t.Fatalf("留下的会话 ID = %q", sessions[0].ID)
	}
}

// 规则本身也钉一遍：`rounds` 只在 `work/` 下面才跳过（书名恰叫 rounds 的
// 项目不能被误吞），备份目录名按 `<名>_记录_<8 位日期>_<4 位时刻>` 认。
func TestSkipScanDir(t *testing.T) {
	cases := []struct {
		path, name string
		want       bool
	}{
		{"/r/p/work/media", "media", true},
		{"/r/p/work/rounds", "rounds", true},
		{"/r/p/rounds", "rounds", false},
		{"/r/p/work/rounds/chapter_001/repo.git", "repo.git", true},
		{"/r/p/书_记录_20261021_2308", "书_记录_20261021_2308", true},
		{"/r/p/书_记录_2026", "书_记录_2026", false},
		{"/r/p/work/sessions", "sessions", false},
	}
	for _, c := range cases {
		if got := skipScanDir(c.path, c.name); got != c.want {
			t.Errorf("skipScanDir(%q, %q) = %v, want %v", c.path, c.name, got, c.want)
		}
	}
}
