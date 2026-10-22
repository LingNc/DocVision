package sessionview

import (
	"encoding/json"
	"strings"
	"testing"
)

// P59：核对会话在转录里记一条 `t=meta kind=checker_target`，说明它核对的是
// **哪一条转换转录**（文件名 + 尝试号）。核对自己的 `.tryN` 号与转换的号没有
// 对应关系（手动重新核对只让核对的号加一），所以网页必须按这个锚点配对。
func TestScanExposesCheckerAnchor(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "checker_chapter_007.try2.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"核对这一章"}`,
		`{"t":"meta","kind":"checker_target","convert":"convert_chapter_007.try2","convert_attempt":2}`,
		`{"t":"msg","role":"tool","tool_call_id":"c1","text":"SUBMITTED. pass"}`,
	))

	stat, err := readStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if stat.ChapterAnchor != "convert_chapter_007.try2" || stat.ChapterAnchorAttempt != 2 {
		t.Fatalf("readStat anchor = %q/%d, want convert_chapter_007.try2/2",
			stat.ChapterAnchor, stat.ChapterAnchorAttempt)
	}

	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterAnchor != "convert_chapter_007.try2" || got.ChapterAnchorAttempt != 2 {
		t.Fatalf("SessionInfo anchor = %q/%d, want convert_chapter_007.try2/2",
			got.ChapterAnchor, got.ChapterAnchorAttempt)
	}
	// 网页只读 JSON：字段名必须就是 chapterAnchor / chapterAnchorAttempt。
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"chapterAnchor":"convert_chapter_007.try2"`, `"chapterAnchorAttempt":2`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON 缺 %s: %s", want, string(raw))
		}
	}
}

// 旧转录（没有 checker_target）保持空锚点：网页据此退回按尝试号配对的老规则。
func TestScanLeavesAnchorEmptyWithoutNote(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "checker_chapter_007.jsonl")
	writeFile(t, p, transcript(`{"t":"msg","role":"user","text":"核对这一章"}`))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].ChapterAnchor != "" || sessions[0].ChapterAnchorAttempt != 0 {
		t.Fatalf("旧转录不该有锚点: %q/%d",
			sessions[0].ChapterAnchor, sessions[0].ChapterAnchorAttempt)
	}
}
