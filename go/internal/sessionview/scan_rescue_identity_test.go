package sessionview

/*
 * P61：复盘会话的章节身份与状态可见性。
 *
 * 复盘转录名从「时间戳」改成与转换/核对同一种口径（`rescue_<章>[.tryN].jsonl`，
 * `.tryN` 是这条目标被复盘的第几次，P58 补起），于是它能拿到章节 ID/父章/深度，
 * 挂进章节树当子行。老命名（`rescue_<章>_<时间戳>`）在**文件名**层面整段不是
 * 章节 ID——`chapterIdentity` 只认名字，所以这里仍然为空；补身份的读时推断在
 * Scan 里（见 scan_rescue_legacy_meta_test.go）。
 *
 * `rescuing` 是复盘的中间状态：写在**目标**转录里，说明"这条会话正在被复盘"。
 * 它必须被认成活动状态，而且不能把目标会话算成 live（复盘在跑，目标没跑）。
 */

import (
	"encoding/json"
	"strings"
	"testing"
)

// 复盘转录带章节身份：ID/父章/深度/尝试号与 checker_ 同一套推导，角色是 rescue。
func TestRescueTranscriptChapterIdentity(t *testing.T) {
	top := chapterIdentity("work/sessions/rescue_chapter_004.try2.jsonl")
	if top.ID != "chapter_004" || top.Parent != "" || top.Role != "rescue" ||
		top.Depth != 0 || top.Attempt != 2 {
		t.Fatalf("顶层复盘身份 = %+v", top)
	}
	child := chapterIdentity("work/temp/conv_chapter_004/work/_subdivision/child_02/node/sessions/rescue_chapter_004_child_02.jsonl")
	if child.ID != "chapter_004_child_02" || child.Parent != "chapter_004" ||
		child.Role != "rescue" || child.Depth != 1 || child.Attempt != 1 {
		t.Fatalf("子章节复盘身份 = %+v", child)
	}
	// P58 的老命名整段不是章节 ID：chapterIdentity 只认文件名，所以这里没有身份
	// （Scan 会按转录自己的 meta 补，见 scan_rescue_legacy_meta_test.go）。
	old := chapterIdentity("work/sessions/rescue_chapter_004_20261022-101010.jsonl")
	if old.ID != "" || old.Role != "" {
		t.Fatalf("老命名的复盘转录不该拿到章节身份: %+v", old)
	}
	// 形近但不是章节 ID 的也不认（宁可没有身份，也不要挂错行）。
	for _, rel := range []string{
		"work/sessions/rescue_chapter_004_extra.jsonl",
		"work/sessions/rescue_book.jsonl",
	} {
		if got := chapterIdentity(rel); got.ID != "" {
			t.Errorf("%q 不该拿到章节身份: %+v", rel, got)
		}
	}
}

// 扫描后：复盘会话落进章节阶段组（这样网页的章节树才画得出它），标题点名「复盘」，
// 角色是 rescue——它永远不能被选成某一章的 convert/checker（选择在 web 侧）。
func TestScanRescueSessionJoinsChapterStage(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "rescue_chapter_014_child_02.try3.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"看一眼这条会话"}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterRole != "rescue" {
		t.Fatalf("chapterRole = %q, want rescue", got.ChapterRole)
	}
	if got.ChapterID != "chapter_014_child_02" || got.ChapterParent != "chapter_014" ||
		got.ChapterDepth != 1 || got.ChapterAttempt != 3 {
		t.Fatalf("章节身份 = %q/%q/%d/%d", got.ChapterID, got.ChapterParent, got.ChapterDepth, got.ChapterAttempt)
	}
	if got.Stage != "convert" || got.StageTitle != "章节转换" {
		t.Errorf("复盘会话的阶段 = %q/%q，应与它复盘的那一章同组（convert/章节转换）",
			got.Stage, got.StageTitle)
	}
	if got.Label != "rescue:chapter_014_child_02" {
		t.Errorf("label = %q", got.Label)
	}
	if got.Title != "复盘 · chapter_014_child_02" {
		t.Errorf("title = %q，want 复盘 · chapter_014_child_02", got.Title)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"chapterRole":"rescue"`,
		`"chapterId":"chapter_014_child_02"`,
		`"chapterAttempt":3`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON 缺 %s: %s", want, string(raw))
		}
	}
}

// `rescuing` 是活动状态：网页文案要能显示成「复盘处理中」，并且目标会话不算 live
// （复盘会话在跑，这条目标会话没有）。
func TestRescuingActivityIsNotLive(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "convert_chapter_014.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"转换这一章"}`,
		`{"t":"meta","kind":"chapter_activity","state":"running"}`,
		`{"t":"meta","kind":"run_end","ok":false,"error":"model rejected the request"}`,
		`{"t":"meta","kind":"chapter_activity","state":"rescuing"}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterActivity != "rescuing" {
		t.Fatalf("chapterActivity = %q, want rescuing", got.ChapterActivity)
	}
	if got.Live {
		t.Error("复盘处理中的目标会话不该算 live（在跑的是复盘会话，不是它）")
	}
}
