package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T201：递归子章节的核对转录在父工作区的细分树里（`work/temp/conv_<父>/work/
// _subdivision/…/node/sessions/`）。「从某条记录之后继续核对」以前只认
// `work/sessions/`，子章节的核对会话必然报"源转录不存在"——用户口径："有的会话
// 截断消息可以，但是有的会话不行，比如核对会话"。
//
// 这里钉住：源可以在细分树里（相对/绝对路径都行），新尝试仍然写回
// `work/sessions/checker_<章>.try<N>.jsonl`（父节点的临时树会被清掉，新尝试不能
// 放在那儿；recheckChapter 也按同一位置读转录）。
func TestTruncateCheckerTranscriptFromSubtree(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_014_child_04"
	body := strings.Join([]string{
		`{"t":"msg","role":"user","content":"review it"}`,
		`{"t":"msg","role":"assistant","content":"step 1"}`,
		`{"t":"msg","role":"assistant","content":"step 2 (wrong)"}`,
	}, "\n") + "\n"
	sub := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work", "_subdivision", "child_04", "node", "sessions")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(sub, "checker_"+base+".jsonl")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "work", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}

	rel := "work/temp/conv_chapter_014/work/_subdivision/child_04/node/sessions/checker_" + base + ".jsonl"
	dst, n, err := r.truncateCheckerTranscript(proj, base, rel, 2)
	if err != nil {
		t.Fatalf("细分树里的核对转录应当可以切: %v", err)
	}
	if want := filepath.Join(proj, "work", "sessions", "checker_"+base+".try2.jsonl"); dst != want {
		t.Fatalf("新尝试写到 %q，期望 %q", dst, want)
	}
	if n != 2 {
		t.Fatalf("新尝试号 = %d，期望 2", n)
	}
	head, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(head), "\n") != 2 || strings.Contains(string(head), "wrong") {
		t.Fatalf("前缀不对: %q", string(head))
	}
	// 源转录一个字不动。
	orig, err := os.ReadFile(src)
	if err != nil || !strings.Contains(string(orig), "wrong") {
		t.Fatalf("源转录被改了: %v", err)
	}
	// 绝对路径同样接受（页面发的是 SessionInfo.Path）。
	r.retryCheckerAttempt = nil
	if _, _, err := r.truncateCheckerTranscript(proj, base, src, 1); err != nil {
		t.Fatalf("绝对路径应当也可以: %v", err)
	}
	// 工作区之外的路径一律拒（防止顺着请求改别的文件）。
	outside := filepath.Join(proj, "chapters", "checker_"+base+".jsonl")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.truncateCheckerTranscript(proj, base, outside, 1); err == nil {
		t.Fatal("work/ 之外的转录不该被接受")
	}
}
