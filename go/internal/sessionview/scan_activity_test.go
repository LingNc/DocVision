package sessionview

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T207：一条转录里可以跑好几轮 Run。"活动标记停在 running" 与 "已经写过
// run_end" 谁新谁旧必须按行号判断——旧实现只看"出现过 run_end"，于是正在跑
// 的会话被判成「已中断」、live 也被强制关掉，侧栏那一片看着全停了。
func TestActivityInterruptedOnlyWhenRunEndIsNewer(t *testing.T) {
	dir := t.TempDir()
	running := filepath.Join(dir, "convert_chapter_001.jsonl")
	// 上一轮 Run 的结局在前，后面又开了新的一轮（活动标记 + 新消息）。
	writeFile(t, running, transcript(
		`{"t":"msg","role":"user","text":"go"}`,
		`{"t":"msg","role":"assistant","text":"a"}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
		`{"t":"meta","kind":"chapter_activity","state":"queued"}`,
		`{"t":"meta","kind":"chapter_activity","state":"running"}`,
		`{"t":"msg","role":"assistant","text":"still working"}`,
	))
	stat, err := readStat(running)
	if err != nil {
		t.Fatal(err)
	}
	if got := activityAfterEnd(stat, stat.ChapterActivity); got != "running" {
		t.Fatalf("activity = %q, want running（run_end 是上一轮的）", got)
	}

	finished := filepath.Join(dir, "convert_chapter_002.jsonl")
	// 活动标记在前、run_end 在后 = 这次尝试真的结束了 → 已中断。
	writeFile(t, finished, transcript(
		`{"t":"meta","kind":"chapter_activity","state":"running"}`,
		`{"t":"msg","role":"assistant","text":"working"}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
	))
	stat2, err := readStat(finished)
	if err != nil {
		t.Fatal(err)
	}
	if got := activityAfterEnd(stat2, stat2.ChapterActivity); got != "interrupted" {
		t.Fatalf("activity = %q, want interrupted", got)
	}
}

// 上一轮 Run 的错误结局不能算到正在跑的这一轮头上。
func TestRunEndErrorClearedByLaterMessages(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "checker_chapter_001.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"check"}`,
		`{"t":"meta","kind":"run_end","ok":false,"error":"boom"}`,
		`{"t":"msg","role":"assistant","text":"retrying"}`,
	))
	stat, err := readStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if stat.RunEndError != "" {
		t.Fatalf("RunEndError = %q, want 空（那一轮已经过去了）", stat.RunEndError)
	}
}

// Scan 侧的完整性：正在跑的会话必须是 live + 运行中。
func TestScanLiveWhenRunningActivityIsNewest(t *testing.T) {
	root := t.TempDir()
	p := jsonl(root, "latex_project", "书", "work", "sessions", "convert_chapter_001.jsonl")
	writeFile(t, p, transcript(
		`{"t":"msg","role":"user","text":"go"}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
		`{"t":"meta","kind":"chapter_activity","state":"running"}`,
		`{"t":"msg","role":"assistant","text":"working"}`,
	))
	// 让 mtime 落进 LiveWindow。
	now := time.Now()
	if err := os.Chtimes(p, now, now); err != nil {
		t.Fatal(err)
	}
	items, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("scanned %d", len(items))
	}
	if !items[0].Live {
		t.Fatal("正在跑（活动标记比 run_end 新）的会话必须是 live")
	}
	if items[0].ChapterActivity != "running" {
		t.Fatalf("activity = %q", items[0].ChapterActivity)
	}
}
