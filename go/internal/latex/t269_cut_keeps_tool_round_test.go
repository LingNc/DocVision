package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T269：复盘切口不许把"最后一个完整工具轮"切开。
//
// 现场：细分父节点申请子会话（request_chapter_split）的那一条正好是复盘判定的
// 切口，T241 的口径把这一条连同回执一起裁掉——平衡保住了，可续跑回放的历史里
// "我拆过章"这个事实凭空消失，页面也不再显示它申请过子章节。这里钉住：调用与
// 回执都在紧跟的行里时就后移到回执之后；凑不齐回执则一个字不动。
func TestExtendPastToolRound(t *testing.T) {
	lines := []string{
		`{"t":"msg","role":"user","text":"go"}`,
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"s1"}]}`,
		`{"t":"meta","kind":"chapter_activity"}`,
		`{"t":"msg","role":"tool","tool_call_id":"s1","text":"SUBDIVISION REQUESTED"}`,
		`{"t":"msg","role":"assistant","text":"I cannot access the workspace tools."}`,
	}
	if got := extendPastToolRound(lines, 1); got != 4 {
		t.Fatalf("切口在工具调用上应后移到回执之后（4），got %d", got)
	}
	// 回执还没到：原样不动，交给平衡检查处理。
	if got := extendPastToolRound(lines[:2], 1); got != 1 {
		t.Fatalf("没有回执时不该后移，got %d", got)
	}
	// 切口本身不是调用（user / 回执 / 普通文本）都不动。
	for _, keep := range []int{0, 3, 4} {
		if got := extendPastToolRound(lines, keep); got != keep {
			t.Fatalf("keep=%d 不该后移，got %d", keep, got)
		}
	}
	// 一轮两个调用、只回了一条：不动。
	part := []string{
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"a"},{"id":"b"}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"a","text":"ok"}`,
	}
	if got := extendPastToolRound(part, 0); got != 0 {
		t.Fatalf("回执不齐时不该后移，got %d", got)
	}
}

// T269：原地截断在"切口正落在工具调用上"时保留那一轮，丢掉的是后面的退化段。
func TestTrimKeepsTrailingToolRound(t *testing.T) {
	proj := t.TempDir()
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_007.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"t":"msg","role":"user","text":"convert"}`,
		`{"t":"msg","role":"assistant","text":"splitting","tool_calls":[{"id":"s1"}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"s1","text":"SUBDIVISION REQUESTED: 3 child ranges persisted."}`,
		`{"t":"msg","role":"assistant","text":"I cannot access the chapter workspace tools."}`,
		`{"t":"msg","role":"user","text":"Your tools ARE available in this session."}`,
		`{"t":"msg","role":"assistant","text":"This session exposes only skills__list."}`,
	}
	if err := os.WriteFile(trPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	// 复盘给的切口 = 2（正好停在申请子会话那一条之前）。
	kept, dropped, err := r.trimTranscriptInPlace(proj, "chapter_007", trPath, 2, "复盘判为退化")
	if err != nil {
		t.Fatal(err)
	}
	if kept != 3 || dropped != 3 {
		t.Fatalf("切口应后移到回执之后（保留 3 条、丢掉 3 条），got %d/%d", kept, dropped)
	}
	all, err := os.ReadFile(trPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimRight(string(all), "\n"), "\n")
	if !strings.Contains(rows[1], "request_chapter_split") && !strings.Contains(rows[1], `"tool_calls"`) {
		t.Fatalf("申请子会话那一条必须留在转录里：%s", rows[1])
	}
	if !strings.Contains(rows[2], "SUBDIVISION REQUESTED") {
		t.Fatalf("回执也必须留着：%s", rows[2])
	}
	if !strings.Contains(rows[3], `"kind":"cut"`) || !strings.Contains(rows[3], `"dropped":3`) {
		t.Fatalf("末尾应是 cut 记录且 dropped=3：%s", rows[3])
	}
}
