package sessionview

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/retryq"
)

// T287：作图转录叫 vector_<项目名>__<哈希>__<描述>.jsonl，项目名与描述常带中文。
// 以前的 ASCII 白名单正则把这类名字判成"不是转录文件"，中文项目里
// 「重画这张图」和「删除记录」一律失败。
func TestValidTranscriptNameAllowsUnicode(t *testing.T) {
	for _, name := range []string{
		"convert_chapter_002.jsonl",
		"a.jsonl",
		"vector_书A__00693c1e__3D几何图.jsonl",
		"vector_高等数学释疑解难-齐植兰__ab12__coordinate_plot.jsonl",
	} {
		if !validTranscriptName(name) {
			t.Fatalf("应当接受 %q", name)
		}
	}
	for _, name := range []string{
		"", ".jsonl", ".hidden.jsonl", "a/b.jsonl", `a\b.jsonl`,
		"../escape.jsonl", "notes.txt", "noext",
	} {
		if validTranscriptName(name) {
			t.Fatalf("应当拒绝 %q", name)
		}
	}
}

// 端到端：中文项目名 + 中文/带哈希的转录名，点「重画这张图」应当排进队列。
func TestPostRetryVectorRoleAcceptsChineseNames(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "source", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "source", "progress_items"), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "vector_书A__00693c1e4171cfd8__3D几何图.jsonl"
	if err := os.WriteFile(filepath.Join(sess, name), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"project":"书A","role":"vector","path":"` + name + `"}`
	if rec := postRetry(t, v, body); rec.Code != 200 {
		t.Fatalf("中文转录名应当能排队, status = %d body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Role != "vector" || pending[0].Request.Path != name {
		t.Fatalf("队列 = %+v", pending)
	}
}
