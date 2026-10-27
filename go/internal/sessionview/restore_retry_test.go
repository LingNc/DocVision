package sessionview

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/retryq"
)

// 切回第 N 次尝试（P63）：页面只排一条 role=restore 的请求，带上要切回的号。
func TestPostRetryRestoreQueuesAttempt(t *testing.T) {
	v, proj := retryTestServer(t, true)
	arch := filepath.Join(proj, "work", "attempts", "chapter_002", "try1", "work")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(arch, "x.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"restore","attempt":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 {
		t.Fatalf("队列 = %+v", pending)
	}
	if pending[0].Role != "restore" || pending[0].Attempt != 1 {
		t.Fatalf("请求应为 restore/第 1 次，实际 %+v", pending[0])
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["ok"] != true {
		t.Fatalf("响应 = %s (%v)", rec.Body.String(), err)
	}
	note, _ := resp["note"].(string)
	if !strings.Contains(note, "不会被打断") {
		t.Fatalf("回执要说清「正在跑的会话不受影响」：%s", note)
	}
}

func TestPostRetryRestoreRejections(t *testing.T) {
	v, proj := retryTestServer(t, true)
	// 只有第 1 次有留档。
	arch := filepath.Join(proj, "work", "attempts", "chapter_002", "try1", "work")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body string
		code int
		want string
	}{
		{"没写第几次", `{"project":"书A","chapter":"chapter_002","role":"restore"}`, http.StatusBadRequest, "第几次尝试"},
		{"负数", `{"project":"书A","chapter":"chapter_002","role":"restore","attempt":-1}`, http.StatusBadRequest, "第几次尝试"},
		{"没有这份留档", `{"project":"书A","chapter":"chapter_002","role":"restore","attempt":9}`, http.StatusNotFound, "无法切回"},
		{"子章节归父章节", `{"project":"书A","chapter":"chapter_002_child_01","role":"restore","attempt":1}`, http.StatusBadRequest, "请对父章节切回"},
	}
	for _, c := range cases {
		rec := postRetry(t, v, c.body)
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.want) {
			t.Fatalf("%s: status = %d, body = %s（期望 %d 含 %q）",
				c.name, rec.Code, strings.TrimSpace(rec.Body.String()), c.code, c.want)
		}
	}
	for _, f := range retryq.Pending(proj) {
		t.Fatalf("被拒的请求不该进队列: %+v", f)
	}
	// 上面每条都 4xx，队列里不该有东西。
	if got := fmt.Sprint(len(retryq.Pending(proj))); got != "0" {
		t.Fatalf("队列条数 = %s", got)
	}
}
