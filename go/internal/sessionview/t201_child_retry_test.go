package sessionview

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/retryq"
)

// T201：递归子章节的转录在父工作区的细分树里，不在 work/sessions/ 下。
// 以前「从某条记录之后继续核对」只认 work/sessions/，子章节的核对会话必然报
// "源转录不存在"（用户口径："有的会话截断消息可以，但是有的会话不行，比如核对会话"）。
func TestPostRetryCheckerChildFromSubtreeTranscript(t *testing.T) {
	v, proj := retryTestServer(t, true)
	base := "chapter_014_child_04"
	// 子章节已提交的产物（细分树的一层 children/）。
	children := filepath.Join(proj, "work", "chapters", "chapter_014", "children")
	if err := os.MkdirAll(filepath.Join(children, base), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(children, base+".tex"), []byte("\\chapter{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 子章节的核对转录：细分树里的 node/sessions/。
	trDir := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work", "_subdivision", "child_04", "node", "sessions")
	if err := os.MkdirAll(trDir, 0o755); err != nil {
		t.Fatal(err)
	}
	trPath := filepath.Join(trDir, "checker_"+base+".jsonl")
	body := ""
	for i := 0; i < 6; i++ {
		body += `{"t":"msg","role":"user","text":"第 ` + string(rune('1'+i)) + ` 条"}` + "\n"
	}
	if err := os.WriteFile(trPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// 页面发的是会话的绝对路径（SessionInfo.Path）。
	payload, _ := json.Marshal(map[string]any{
		"project": "书A", "chapter": base, "role": "checker",
		"reason": "轨迹回滚到 #4", "from": 4, "path": trPath,
	})
	rec := postRetry(t, v, string(payload))
	if rec.Code != http.StatusOK {
		t.Fatalf("子章节核对应能从细分树里的转录继续，status = %d body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 {
		t.Fatalf("队列里应有一条请求，实际 %d", len(pending))
	}
	got := pending[0].Request
	if got.Role != "checker" || got.From != 4 || got.Chapter != base {
		t.Fatalf("请求内容不对: %+v", got)
	}
	// 队列里记的是项目内相对路径：latex 侧按 <项目>/work/ 之下解析。
	if got.Path != "work/temp/conv_chapter_014/work/_subdivision/child_04/node/sessions/checker_chapter_014_child_04.jsonl" {
		t.Fatalf("队列里的源转录路径 = %q", got.Path)
	}
}

// 子章节的输入（细分树里的 md）已经不在了、父章节又没整合——以前是一条 404
// 死路。现在改成排队父章节的「继续跑」，点了就真的会跑起来。
func TestPostRetryChildConvertWithoutInputQueuesParent(t *testing.T) {
	v, proj := retryTestServer(t, true)
	parent, base := "chapter_009", "chapter_009_child_05"
	// 父章节有转换记录（可以继续跑），但细分树已经没了（没有子章节输入）。
	tr := filepath.Join(proj, "work", "sessions", "convert_"+parent+".jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte(`{"t":"msg","role":"user","text":"转换"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"`+base+`","reason":"乱码重跑"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("应当改为排队父章节，status = %d body = %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("回执不是 JSON: %s", rec.Body.String())
	}
	if res["parent"] != true || res["chapter"] != parent || res["role"] != "continue" {
		t.Fatalf("回执没说清改成了父章节：%v", res)
	}
	if note, _ := res["note"].(string); note == "" || !containsAll(note, parent, base) {
		t.Fatalf("回执说明不对: %q", note)
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Request.Chapter != parent || pending[0].Request.Role != "continue" {
		t.Fatalf("队列里应是父章节的 continue，实际 %+v", pending)
	}
}

// 父章节连转换记录都没有：那才真的没救，照实 404。
func TestPostRetryChildConvertWithoutParentTranscript(t *testing.T) {
	v, _ := retryTestServer(t, true)
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_009_child_05"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// T201：子章节的输入 md 在细分树的 `<id>/input/chapter.md`（不是 `<章>.md`），
// 且要按层级链整段比对——不同层级都可能有 `child_04`。
func TestChildMarkdownInTempTreeDepthExact(t *testing.T) {
	_, proj := retryTestServer(t, true)
	sub := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work", "_subdivision")
	write := func(rel string) {
		p := filepath.Join(sub, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("child_01/node/work/_subdivision/child_04/input/chapter.md")
	if childMarkdownInTempTree(proj, "chapter_014", "chapter_014_child_04") {
		t.Fatal("只存在更深层的同名 child_04 时，不该认成顶层这一支")
	}
	write("child_04/input/chapter.md")
	if !childMarkdownInTempTree(proj, "chapter_014", "chapter_014_child_04") {
		t.Fatal("顶层 child_04 的输入存在时应认出来")
	}
	if got := childInputRel("chapter_014_child_03_child_02"); got != "child_03/node/work/_subdivision/child_02/input/chapter.md" {
		t.Fatalf("嵌套层级路径 = %q", got)
	}
}
