package latex

// T231 现场（用户报的「明明核对通过了，页面还写着会话错误」）：核对的结论只写进
// 核对转录，转换转录还挂着之前那句 error；页面上转换行看的是**它自己**那条转录，
// 于是"一条子会话核对通过、父会话却显示会话错误"挂着不走。
//
// 还有一处更隐蔽：找这一章的转录目录用的是**前缀匹配**，`chapter_024_child_09`
// 会先命中孙节点的 `convert_chapter_024_child_09_child_01.jsonl`，于是结论被写到
// 别人的转录上（或者尝试号算错、锚点指向错误的那一次转换）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"t":"msg","role":"assistant","content":"ok"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lastChapterActivity 读转录里最后一条 chapter_activity 的状态（没有则空串）。
func lastChapterActivity(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读转录失败 %s: %v", path, err)
	}
	state := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			Kind  string `json:"kind"`
			State string `json:"state"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if row.Kind == "chapter_activity" {
			state = row.State
		}
	}
	return state
}

func TestChapterVerdictGoesToItsOwnConvertTranscript(t *testing.T) {
	proj := t.TempDir()
	nodeSessions := func(child string) string {
		return filepath.Join(proj, "work", "temp", "conv_chapter_024", "work", "_subdivision",
			child, "node", "sessions")
	}
	// 孙节点的转录名以本章 ID 为前缀——前缀匹配会把它当成"这一章的转录"。
	grandchild := filepath.Join(nodeSessions("child_01"), "convert_chapter_024_child_09_child_01.jsonl")
	own := filepath.Join(nodeSessions("child_09"), "convert_chapter_024_child_09.try3.jsonl")
	writeTranscript(t, grandchild)
	writeTranscript(t, own)

	if got := latestConvertAttempt(proj, "chapter_024_child_09"); got != 3 {
		t.Fatalf("这一章最近一次转换尝试 = %d，期望 3（不能被孙节点的转录带偏）", got)
	}
	if got := chapterConvertTranscript(proj, "chapter_024_child_09", 3); got != own {
		t.Fatalf("转换转录解析到 %s，期望 %s", got, own)
	}

	markChapterVerdict(proj, "chapter_024_child_09", 3, true, "")
	if got := lastChapterActivity(t, own); got != "pass" {
		t.Errorf("核对通过要写在这份产物对应的转换转录上，实际状态 %q", got)
	}
	if got := lastChapterActivity(t, grandchild); got != "" {
		t.Errorf("孙节点的转录不该被写：实际状态 %q", got)
	}
	// 没给结论时写 no_verdict，不写 error：转换这一轮是交付成功的，
	// "没有结论"是核对那边的事，页面上也不能读作会话错误。
	markChapterVerdict(proj, "chapter_024_child_09", 3, false, "checker 会话已退化，已排队从干净轮次继续核对")
	if got := lastChapterActivity(t, own); got != "no_verdict" {
		t.Errorf("核对没给结论时状态 = %q，期望 no_verdict", got)
	}
	// 有问题照旧写 issues。
	markChapterVerdict(proj, "chapter_024_child_09", 3, false, "1. 第 10 行缺 \\section")
	if got := lastChapterActivity(t, own); got != "issues" {
		t.Errorf("核对发现问题时状态 = %q，期望 issues", got)
	}
}

// 顶层章节的转录在 <proj>/work/sessions/，不该被递归子树里的同名章节带偏。
func TestChapterVerdictTopLevelTranscriptStaysTopLevel(t *testing.T) {
	proj := t.TempDir()
	top := filepath.Join(proj, "work", "sessions", "convert_chapter_002.jsonl")
	writeTranscript(t, top)
	nested := filepath.Join(proj, "work", "temp", "conv_chapter_002", "work", "_subdivision",
		"child_02", "node", "sessions", "convert_chapter_002_child_02.jsonl")
	writeTranscript(t, nested)

	if got := chapterConvertTranscript(proj, "chapter_002", 1); got != top {
		t.Fatalf("顶层章节的转换转录解析到 %s，期望 %s", got, top)
	}
	markChapterVerdict(proj, "chapter_002", 1, true, "")
	if got := lastChapterActivity(t, top); got != "pass" {
		t.Errorf("顶层转换转录应拿到 pass，实际 %q", got)
	}
	if got := lastChapterActivity(t, nested); got != "" {
		t.Errorf("别的章节的转录不该被写：实际状态 %q", got)
	}
}
