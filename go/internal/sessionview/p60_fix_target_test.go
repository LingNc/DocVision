package sessionview

import (
	"encoding/json"
	"strings"
	"testing"
)

// P60：修复会话在转录里记一条 `t=meta kind=fix_target`——它修的是**哪一条
// 转换产物**（与核对同一种锚点口径）、这一轮改了哪几条 issue。扫描器要把它读
// 成 `chapterRole: "fix"` + 锚点 + issue id 清单；这一步只读事实，分组与树
// 一行都不改（见本文件最后一条断言：章节身份仍是空的，P60 下一步才接树）。
func TestScanExposesFixTarget(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "temp", "conv_chapter_014", "work", "_subdivision", "child_02", "node",
		"sessions", "fix_chapter_014_child_02.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"修复这几条"}`,
		`{"t":"meta","kind":"fix_target","convert":"convert_chapter_014_child_02.try2","convert_attempt":2,"issue_ids":["ISSUE-1","figure-scale"]}`,
		`{"t":"msg","role":"tool","tool_call_id":"c1","text":"SUBMITTED. ok"}`,
	))

	stat, err := readStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !stat.SawFixTarget {
		t.Fatal("没有认出 fix_target")
	}
	if stat.FixAnchor != "convert_chapter_014_child_02.try2" || stat.FixAnchorAttempt != 2 {
		t.Fatalf("readStat 锚点 = %q/%d", stat.FixAnchor, stat.FixAnchorAttempt)
	}
	if len(stat.FixIssueIDs) != 2 || stat.FixIssueIDs[0] != "ISSUE-1" {
		t.Fatalf("issue id 清单 = %v", stat.FixIssueIDs)
	}
	// fix_target 不是 checker_target：两条锚点字段不能互相污染。
	if stat.ChapterAnchor != "" || stat.ChapterAnchorAttempt != 0 {
		t.Fatalf("fix_target 不该写进核对锚点: %q/%d", stat.ChapterAnchor, stat.ChapterAnchorAttempt)
	}

	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterRole != "fix" {
		t.Errorf("chapterRole = %q, want fix", got.ChapterRole)
	}
	// 锚点沿用 chapterAnchor 字段：网页按它把修复（与它之后的核对）挂到同一次
	// 转换下面。
	if got.ChapterAnchor != "convert_chapter_014_child_02.try2" || got.ChapterAnchorAttempt != 2 {
		t.Errorf("SessionInfo 锚点 = %q/%d", got.ChapterAnchor, got.ChapterAnchorAttempt)
	}
	if len(got.FixIssueIDs) != 2 {
		t.Errorf("FixIssueIDs = %v", got.FixIssueIDs)
	}
	// P60 第一步：不动既有分组——修复转录仍按老口径落到通用「会话」阶段，
	// 章节身份为空（章节树/历史下一步才接它）。
	if got.ChapterID != "" || got.ChapterParent != "" || got.ChapterDepth != 0 {
		t.Errorf("P60 第一步不该给修复转录造章节身份: %+v", got)
	}
	if got.Stage != "会话" {
		t.Errorf("修复转录的既有阶段归组不该变化: stage = %q", got.Stage)
	}

	// 网页只读 JSON：字段名必须就是 fixIssueIds。
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"chapterRole":"fix"`,
		`"fixIssueIds":["ISSUE-1","figure-scale"]`,
		`"chapterAnchor":"convert_chapter_014_child_02.try2"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON 缺 %s: %s", want, string(raw))
		}
	}
}

// P60：普通转换/核对转录没有 fix_target，角色与锚点一个都不能变。
func TestScanLeavesRoleWithoutFixTarget(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "checker_chapter_007.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"核对这一章"}`,
		`{"t":"meta","kind":"checker_target","convert":"convert_chapter_007","convert_attempt":1}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterRole != "checker" {
		t.Errorf("chapterRole = %q, want checker", got.ChapterRole)
	}
	if len(got.FixIssueIDs) != 0 {
		t.Errorf("不该有 fixIssueIds: %v", got.FixIssueIDs)
	}
}
