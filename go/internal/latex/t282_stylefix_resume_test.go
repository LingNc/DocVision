package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

func writeAssessTranscript(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := ""
	for _, ln := range lines {
		body += ln + "\n"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// T282/T283：所有会话都"能续就续"——磁盘上有转录就接着它，代码不替用户删。
func TestTranscriptResumeStateContinuesExisting(t *testing.T) {
	dir := t.TempDir()
	writeAssessTranscript(t, dir, "stylefix_assess_r1.jsonl", `{"t":"msg","role":"user","text":"整理问题清单"}`)

	msgs, path, resume, finished := transcriptResumeState(dir, "stylefix_assess_r1.jsonl", "submit_blocks")
	if !resume || finished {
		t.Fatalf("有转录就当续跑、未收尾: resume=%v finished=%v", resume, finished)
	}
	if filepath.Base(path) != "stylefix_assess_r1.jsonl" || len(msgs) != 1 {
		t.Fatalf("应当接着同一条转录: %s %d", path, len(msgs))
	}

	// 没有转录 → 全新一轮（路径就是那条即将写出的转录）。
	empty := t.TempDir()
	if msgs, path, resume, _ := transcriptResumeState(empty, "stylefix_integrate.jsonl", "submit_problems"); resume ||
		len(msgs) != 0 || filepath.Base(path) != "stylefix_integrate.jsonl" {
		t.Fatalf("没有转录时应当全新开始: %v %s %d", resume, path, len(msgs))
	}
}

// 收尾过的上一轮：仍接着同一条会话（不新开转录、不删东西），但标记 finished，
// 好让调用方提醒它旧产物还在。
func TestTranscriptResumeStateMarksFinishedTail(t *testing.T) {
	dir := t.TempDir()
	writeAssessTranscript(t, dir, "stylefix_blk_r1-1.jsonl",
		`{"t":"msg","role":"user","text":"修这一块"}`,
		`{"t":"msg","role":"assistant","text":"done","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit","arguments":"{}"}}]}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
	)
	if _, _, resume, finished := transcriptResumeState(dir, "stylefix_blk_r1-1.jsonl", "submit"); !resume || !finished {
		t.Fatalf("收尾过的一轮仍然续跑、且标记 finished: resume=%v finished=%v", resume, finished)
	}

	// run_end 之后又跑了（提醒续跑时被打断）→ 仍算未收尾。
	dir2 := t.TempDir()
	writeAssessTranscript(t, dir2, "stylefix_blk_r1-1.jsonl",
		`{"t":"msg","role":"user","text":"修这一块"}`,
		`{"t":"meta","kind":"run_end","ok":true}`,
		`{"t":"msg","role":"user","text":"你还没提交"}`,
	)
	if _, _, resume, finished := transcriptResumeState(dir2, "stylefix_blk_r1-1.jsonl", "submit"); !resume || finished {
		t.Fatalf("run_end 之后还有消息 → 未收尾: resume=%v finished=%v", resume, finished)
	}

	// 别的工具的终态不算这一轮交过（terminalTool 只认自己那一个）。
	dir3 := t.TempDir()
	writeAssessTranscript(t, dir3, "stylefix_integrate.jsonl",
		`{"t":"msg","role":"user","text":"整合"}`,
		`{"t":"msg","role":"assistant","text":"x","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit_style","arguments":"{}"}}]}`,
	)
	if _, _, _, finished := transcriptResumeState(dir3, "stylefix_integrate.jsonl", "submit_problems"); finished {
		t.Fatal("不是本会话的终态工具，不该算收尾")
	}
}

// T283：块会话被打断后，工作区里的章节副本（含已改过的地方）不能被重建清掉。
func TestTempDirKeepReusesInterruptedWorkspace(t *testing.T) {
	proj := t.TempDir()
	name := "stylefix-blk-r1-1"
	dir := filepath.Join(proj, "work", "temp", name)
	if err := os.MkdirAll(filepath.Join(dir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(dir, "chapters", "chapter_010.tex")
	if err := os.WriteFile(edited, []byte("上一轮改过的内容"), 0o644); err != nil {
		t.Fatal(err)
	}

	log, lerr := logger.NewLogger(filepath.Join(proj, "run.log"), filepath.Join(proj, "err.log"), 2)
	if lerr != nil {
		t.Fatal(lerr)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	got, cleanup, err := r.tempDirKeep(proj, name)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("应当复用原工作区: %s", got)
	}
	if data, rerr := os.ReadFile(edited); rerr != nil || string(data) != "上一轮改过的内容" {
		t.Fatalf("复用时不能清空工作区: %q %v", data, rerr)
	}
	cleanup()

	// 上一轮已经把目录清掉了 → 按原样新建一个空目录。
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	got2, cleanup2, err := r.tempDirKeep(proj, name)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	if entries, _ := os.ReadDir(got2); len(entries) != 0 {
		t.Fatalf("新建时应当是空目录: %v", entries)
	}
}
