package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

func writeAttemptTranscript(t *testing.T, dir, name string, lines ...string) string {
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

const submitStyleCall = `{"t":"msg","role":"assistant","text":"done","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit_style","arguments":"{}"}}]}`

// T284：只有"还没交过终态工具"的那一次尝试才续。交过的（那一轮已经跑完了）
// 换下一条转录，旧记录原样留着——既不覆盖也不删。
func TestSessionAttemptPathResumesOnlyUnsubmitted(t *testing.T) {
	dir := t.TempDir()
	if _, path, resume := sessionAttemptPath(dir, "stylefix_style_r1", "submit_style"); resume ||
		filepath.Base(path) != "stylefix_style_r1.jsonl" {
		t.Fatalf("没有转录时应当是全新一次尝试: %v %s", resume, path)
	}

	writeAttemptTranscript(t, dir, "stylefix_style_r1.jsonl",
		`{"t":"msg","role":"user","text":"开始"}`,
		`{"t":"msg","role":"assistant","text":"先看看"}`)
	msgs, path, resume := sessionAttemptPath(dir, "stylefix_style_r1", "submit_style")
	if !resume || filepath.Base(path) != "stylefix_style_r1.jsonl" || len(msgs) != 2 {
		t.Fatalf("没交过就续同一条: resume=%v %s %d", resume, path, len(msgs))
	}
}

// 跑完的那一轮（交过 submit_style）不能被续上来——这正是"都结束了为什么又续上"。
func TestSessionAttemptPathSkipsFinishedAttempt(t *testing.T) {
	dir := t.TempDir()
	writeAttemptTranscript(t, dir, "stylefix_style_r1.jsonl",
		`{"t":"msg","role":"user","text":"开始"}`,
		submitStyleCall,
		`{"t":"meta","kind":"run_end","ok":true}`)

	msgs, path, resume := sessionAttemptPath(dir, "stylefix_style_r1", "submit_style")
	if resume || len(msgs) != 0 {
		t.Fatalf("已经交过的不该再续: resume=%v %d", resume, len(msgs))
	}
	if filepath.Base(path) != "stylefix_style_r1.2.jsonl" {
		t.Fatalf("应当换下一条尝试: %s", path)
	}

	// 下一条被打断 → 接着它；再往后才轮到 .3。
	writeAttemptTranscript(t, dir, "stylefix_style_r1.2.jsonl", `{"t":"msg","role":"user","text":"第二次"}`)
	if _, path, resume := sessionAttemptPath(dir, "stylefix_style_r1", "submit_style"); !resume ||
		filepath.Base(path) != "stylefix_style_r1.2.jsonl" {
		t.Fatalf("应当续 .2: %v %s", resume, path)
	}

	// 终态工具只认本会话那一个：别的 submit 不算这一轮交过。
	dir2 := t.TempDir()
	writeAttemptTranscript(t, dir2, "stylefix_assess_r1.jsonl",
		`{"t":"msg","role":"user","text":"分派"}`,
		`{"t":"msg","role":"assistant","text":"x","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit_style","arguments":"{}"}}]}`)
	if _, _, resume := sessionAttemptPath(dir2, "stylefix_assess_r1", "submit_blocks"); !resume {
		t.Fatal("不是本会话的终态工具，不该算已交")
	}
}

func TestTranscriptSubmittedReadsToolCalls(t *testing.T) {
	dir := t.TempDir()
	p := writeAttemptTranscript(t, dir, "a.jsonl", `{"t":"msg","role":"assistant","text":"x","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit","arguments":"{}"}}]}`)
	if !transcriptSubmitted(p, "submit") {
		t.Fatal("应当认出终态工具")
	}
	if transcriptSubmitted(p, "submit_style") {
		t.Fatal("不该把别的工具当成本会话终态")
	}
	if transcriptSubmitted(filepath.Join(dir, "nope.jsonl"), "submit") {
		t.Fatal("文件不存在时不该报已交")
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
