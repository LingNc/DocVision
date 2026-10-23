package sessionview

import (
	"encoding/json"
	"strings"
	"testing"
)

// P60：修复会话在转录里记一条 `t=meta kind=fix_target`——它修的是**哪一条
// 转换产物**（与核对同一种锚点口径）、这一轮改了哪几条 issue。扫描器要把它读
// 成 `chapterRole: "fix"` + 锚点 + issue id 清单，同时按文件名给出章节身份
// （P60 第四步：fix_ 前缀与 convert_/checker_ 一样认，落进章节阶段组）。
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
	// P60 第四步：修复转录按文件名拿到章节身份（与 convert_/checker_ 同一套
	// 规则）——它是这一章那一支的会话，不再是没有身份的散「会话」。
	if got.ChapterID != "chapter_014_child_02" {
		t.Errorf("chapterId = %q, want chapter_014_child_02", got.ChapterID)
	}
	if got.ChapterParent != "chapter_014" {
		t.Errorf("chapterParent = %q, want chapter_014", got.ChapterParent)
	}
	if got.ChapterDepth != 1 {
		t.Errorf("chapterDepth = %d, want 1", got.ChapterDepth)
	}
	if got.ChapterAttempt != 1 {
		t.Errorf("chapterAttempt = %d, want 1（文件名没有 .tryN）", got.ChapterAttempt)
	}
	// 阶段：与转换/核对同一个章节阶段组（不是通用「会话」桶）。
	if got.Stage != "convert" || got.StageTitle != "章节转换" {
		t.Errorf("修复转录的阶段 = %q/%q，want convert/章节转换", got.Stage, got.StageTitle)
	}
	if got.Label != "fix:chapter_014_child_02" {
		t.Errorf("label = %q，want fix:chapter_014_child_02", got.Label)
	}
	if got.Title != "修复 · chapter_014_child_02" {
		t.Errorf("title = %q，want 修复 · chapter_014_child_02", got.Title)
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

// P60 第四步：修复转录的章节身份完全按文件名推断——父章、深度、`.tryN` 尝试号
// 与 convert_/checker_ 同一套规则；`LabelFor` 认 fix_ 前缀、阶段归组跟转换/核对
// 同一组，不再落进通用「会话」桶（用户口径：它是这一章的会话，换到别处找不到）。
func TestFixTranscriptChapterIdentity(t *testing.T) {
	rel := "work/temp/conv_chapter_004/work/_subdivision/child_02/node/sessions/fix_chapter_004_child_02.try2.jsonl"
	ident := chapterIdentity(rel)
	if ident.ID != "chapter_004_child_02" || ident.Parent != "chapter_004" ||
		ident.Role != "fix" || ident.Depth != 1 || ident.Attempt != 2 {
		t.Fatalf("章节身份 = %+v", ident)
	}
	if got := LabelFor(rel); got != "fix:chapter_004_child_02" {
		t.Errorf("LabelFor = %q，want fix:chapter_004_child_02", got)
	}
	if got := StageOf(SessionInfo{ID: rel, Label: LabelFor(rel)}); got != "fix" {
		// 角色仍是 fix（谁干的活），但**阶段组**是章节那一组——enrich 会把它
		// 归到 convert，与转换/核对同组（下面用 Scan 的结果钉住这一点）。
		t.Errorf("StageOf = %q，want fix（角色）", got)
	}
	// 书级修复会话（book_fix）没有章节身份，仍是「全书修复」。
	if got := TitleFor("work/sessions/book_fix.jsonl"); got != "全书修复" {
		t.Errorf("book_fix 的标题 = %q，want 全书修复", got)
	}
	if got := TitleFor(rel); got != "修复 · chapter_004_child_02" {
		t.Errorf("修复转录的标题 = %q，want 修复 · chapter_004_child_02", got)
	}
}

// P60 第四步：`node/sessions/fix_<章>.jsonl` 扫出来的行要落在章节阶段组里
// （与转换/核对同组），并且修复自己的角色/锚点不受影响。
func TestScanPutsFixIntoChapterStage(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "temp", "conv_chapter_004", "work", "_subdivision", "child_02", "node",
		"sessions", "fix_chapter_004_child_02.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"修复这几条"}`,
		`{"t":"meta","kind":"fix_target","convert":"convert_chapter_004_child_02.try2","convert_attempt":2,"issue_ids":["ISSUE-1"]}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterID != "chapter_004_child_02" || got.ChapterParent != "chapter_004" {
		t.Fatalf("chapterId/chapterParent = %q/%q", got.ChapterID, got.ChapterParent)
	}
	if got.ChapterRole != "fix" {
		t.Errorf("chapterRole = %q, want fix", got.ChapterRole)
	}
	// 阶段归组：与转换/核对同一个章节阶段组，不是通用「会话」。
	if got.Stage != "convert" {
		t.Errorf("stage = %q，want convert（章节阶段组）", got.Stage)
	}
	if got.ChapterAnchor != "convert_chapter_004_child_02.try2" || got.ChapterAnchorAttempt != 2 {
		t.Errorf("锚点 = %q/%d", got.ChapterAnchor, got.ChapterAnchorAttempt)
	}
	// 同组里核对仍是核对、修复仍是修复：角色字段没有被阶段覆盖。
	if got.ChapterAttempt != 1 {
		t.Errorf("chapterAttempt = %d，want 1", got.ChapterAttempt)
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
