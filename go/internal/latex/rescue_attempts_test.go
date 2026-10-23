package latex

/*
 * P58 补（B/C）：复盘转录按"这条目标被复盘的第几次"编号，不再覆盖上一份。
 *
 * 以前 `.tryN` 是**目标转录**的尝试号，同一目标第二次复盘会复用同一个文件名，
 * 于是开新会话前必须先把上一份退掉——那是一段真实发生过、已经收尾（有 run_end）
 * 的复盘记录，不该被后来的那次顶掉。现在每次复盘都拿磁盘上已有的最大号 + 1
 * （与 convert 的 lastAttemptNumber 同一口径），只有**没拿到结论**的那一次才退
 * 进 `work/sessions/.trash/`。
 *
 * 复盘自己那条转录在开跑之前写一行 `t=meta kind=rescue_target`（章节 / 被复盘
 * 的角色与转录名），预览页靠它补章节身份、历史页签靠它说清"复盘了 转换#1"。
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 同一目标被复盘两次：两段对话各写进自己的一号（`.jsonl` 与 `.try2.jsonl`），
// 两份都留着、都有 run_end；目标备注两份都在。
func TestRescueAttemptsAreNumberedAndKeepCompletedRecords(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		noSubmit:          true,
		verdicts:          []string{"retry", "retry"},
		keepRescueRecords: true,
	})
	if run.err == nil {
		t.Fatal("两轮提醒之后仍未提交，这一章应当按失败收尾")
	}
	if run.stats.rescue != 2 {
		t.Fatalf("默认预算下同一目标应复盘 2 次，实际 %d 次（请求 %d）", run.stats.rescue, run.stats.requests)
	}
	names := rescueTranscripts(t, run.proj)
	if len(names) != 2 {
		t.Fatalf("两次复盘应各留一条转录，实际 %v", names)
	}
	want := []string{"rescue_chapter_01.jsonl", "rescue_chapter_01.try2.jsonl"}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("第 %d 条复盘转录名 = %q，期望 %q（全部：%v）", i+1, names[i], name, names)
		}
		raw, err := os.ReadFile(filepath.Join(run.proj, "work", "sessions", name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)
		if !strings.Contains(body, `"kind":"run_end"`) {
			t.Errorf("%s 应当保留自己的 run_end（已完成的复盘不能被后来的那次覆盖）", name)
		}
		// C：复盘自己的目标备注——章节 + 被复盘的角色与转录名。
		for _, want := range []string{
			`"kind":"rescue_target"`,
			`"chapter":"chapter_01"`,
			`"reviewed_role":"convert"`,
			`"reviewed_transcript":"convert_chapter_01"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s 缺目标备注字段 %s", name, want)
			}
		}
	}
	// 有结论的复盘一条都不进 .trash。
	trash, _ := filepath.Glob(filepath.Join(run.proj, "work", "sessions", ".trash", "rescue_*.jsonl"))
	if len(trash) != 0 {
		t.Errorf("有结论的复盘转录不该被丢弃，实际 %v", trash)
	}
}

// 下一号只看磁盘上已有的（与 convert 的 lastAttemptNumber 同一口径），
// 第一次没有后缀、之后是 .try2/.try3。
func TestNextAttemptNumberFollowsDisk(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "rescue_chapter_002.jsonl")
	if got := nextAttemptNumber(base); got != 1 {
		t.Fatalf("还没有任何复盘转录时应当是第 1 次，实际 %d", got)
	}
	if err := os.WriteFile(base, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := nextAttemptNumber(base); got != 2 {
		t.Fatalf("已有一条时应当是第 2 次，实际 %d", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "rescue_chapter_002.try2.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := nextAttemptNumber(base); got != 3 {
		t.Fatalf("已有两条时应当是第 3 次，实际 %d", got)
	}
	if got := filepath.Base(attemptTranscriptPath(base, nextAttemptNumber(base))); got != "rescue_chapter_002.try3.jsonl" {
		t.Fatalf("落点名 = %q", got)
	}
}
