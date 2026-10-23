package sessionview

/*
 * P58 补（A）：老命名的复盘转录也要进章节树，不再自成一个阶段。
 *
 * 第 1 批（P58）的复盘转录名是 `rescue_<章>_<时间戳>.jsonl`，剩余部分整段不是
 * 章节 ID，所以它拿不到章节身份、落在「错误处理」阶段——用户看到的是"复盘会话
 * 在树里找不到，只在最后那一组里堆着"。P61 换成 `rescue_<章>[.tryN].jsonl` 之后
 * 新转录有了身份，老转录仍是老样子。这里补的是一条**读时**推断：文件名给不出
 * 章节就按转录自己的 meta 补（`t=meta kind=rescue_target` 的 chapter，退一步是
 * 系统快照里的 `session_label: rescue:<章>`），补到了就与带编号的转录完全同组；
 * 两条都没有的仍留在「错误处理」（不丢）。
 *
 * D：老名字不在磁盘上改，编号由界面按 mtime 读时推断。
 */

import (
	"encoding/json"
	"strings"
	"testing"
)

// 老命名 + 目标备注：拿到章节身份，挂进章节转换组当复盘行，标题点名「复盘」。
func TestScanLegacyNamedRescueUsesTargetNoteForChapter(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "rescue_chapter_004_20261022-101010.jsonl")
	writeFile(t, p, transcript(
		`{"t":"meta","kind":"system","session_label":"rescue:chapter_004","model":"fake"}`,
		`{"t":"meta","kind":"rescue_target","chapter":"chapter_004","reviewed_role":"convert","reviewed_transcript":"convert_chapter_004.try2","reviewed_attempt":2}`,
		`{"t":"msg","role":"user","text":"review that session"}`,
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
	if got.ChapterRole != "rescue" || got.ChapterID != "chapter_004" ||
		got.ChapterParent != "" || got.ChapterDepth != 0 {
		t.Fatalf("老命名复盘应当按目标备注补出章节身份：%+v", got)
	}
	if got.Stage != "convert" || got.StageTitle != "章节转换" {
		t.Errorf("阶段 = %q/%q，应与它复盘的那一章同组（convert/章节转换）", got.Stage, got.StageTitle)
	}
	if got.Label != "rescue:chapter_004" || got.Title != "复盘 · chapter_004" {
		t.Errorf("label/title = %q/%q，期望 rescue:chapter_004 / 复盘 · chapter_004", got.Label, got.Title)
	}
	if got.ReviewedRole != "convert" || got.ReviewedTranscript != "convert_chapter_004.try2" {
		t.Fatalf("被复盘的会话 = %q/%q，期望 convert / convert_chapter_004.try2",
			got.ReviewedRole, got.ReviewedTranscript)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"chapterRole":"rescue"`,
		`"chapterId":"chapter_004"`,
		`"reviewedRole":"convert"`,
		`"reviewedTranscript":"convert_chapter_004.try2"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("JSON 缺 %s: %s", want, string(raw))
		}
	}
}

// 老命名 + 只有会话标签（P58/P61 那批没有目标备注）：会话标签就够推断章节。
func TestScanLegacyNamedRescueFallsBackToSessionLabel(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "rescue_chapter_007_child_02_20261022-101010.jsonl")
	writeFile(t, p, transcript(
		`{"t":"meta","kind":"system","session_label":"rescue:chapter_007_child_02","model":"fake"}`,
		`{"t":"msg","role":"user","text":"review"}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.ChapterID != "chapter_007_child_02" || got.ChapterParent != "chapter_007" || got.ChapterDepth != 1 {
		t.Fatalf("会话标签应当补出章节身份：%+v", got)
	}
	if got.Stage != "convert" {
		t.Errorf("stage = %q，want convert", got.Stage)
	}
	// 没有目标备注：被复盘的会话未知（历史页签不说"复盘了 …"），但章节照旧。
	if got.ReviewedRole != "" || got.ReviewedTranscript != "" {
		t.Errorf("没有目标备注时不该编一个被复盘的会话：%q/%q", got.ReviewedRole, got.ReviewedTranscript)
	}
}

// 老命名 + 什么线索都没有：留在「错误处理」组——不能因为要"隐藏独立阶段"
// 就把它弄丢。
func TestScanLegacyNamedRescueWithoutTargetStaysInErrorStage(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "work", "sessions", "rescue_chapter_004_20261022-101010.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"review"}`,
	))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1（没有目标的复盘转录不能消失）", len(sessions))
	}
	got := sessions[0]
	if got.ChapterID != "" {
		t.Fatalf("没有可用目标时不该编章节身份：%+v", got)
	}
	if got.Stage != "rescue" || got.StageTitle != "错误处理" {
		t.Errorf("阶段 = %q/%q，want rescue/错误处理", got.Stage, got.StageTitle)
	}
	if got.Title != "复盘 · chapter_004_20261022-101010" {
		t.Errorf("title = %q（无身份时仍是原样点名，不丢）", got.Title)
	}
}
