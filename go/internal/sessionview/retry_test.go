package sessionview

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/retryq"
)

func retryTestServer(t *testing.T, allowRetry bool) (*viewerServer, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "书A")
	for _, d := range []string{filepath.Join(proj, "chapters"), filepath.Join(proj, "work")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_002.md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := newViewerServer(root, nil)
	v.allowRetry = allowRetry
	return v, proj
}

func postRetry(t *testing.T, v *viewerServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/retry", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	v.ServeHTTP(rec, req)
	return rec
}

// 页面上的按钮走这条：只往 work/retry/ 排一个请求文件，不执行任何命令。
func TestPostRetryQueuesRequest(t *testing.T) {
	v, proj := retryTestServer(t, true)
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","reason":"看着不对"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Chapter != "chapter_002" || pending[0].Reason != "看着不对" {
		t.Fatalf("队列 = %+v", pending)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp["ok"] != true {
		t.Fatalf("响应 = %s (%v)", rec.Body.String(), err)
	}
}

func TestPostRetryRejections(t *testing.T) {
	v, _ := retryTestServer(t, true)
	cases := []struct {
		name string
		body string
		want int
	}{
		{"项目穿越", `{"project":"../etc","chapter":"chapter_002"}`, http.StatusNotFound},
		{"项目不存在", `{"project":"书B","chapter":"chapter_002"}`, http.StatusNotFound},
		{"章节名非法", `{"project":"书A","chapter":"../../x"}`, http.StatusBadRequest},
		{"章节不存在", `{"project":"书A","chapter":"chapter_099"}`, http.StatusNotFound},
		// 子章节由父章节的细分树产出，不在 chapters/ 下：拒绝并说明方向。
		{"子章节", `{"project":"书A","chapter":"chapter_003_child_01"}`, http.StatusBadRequest},
		{"请求体不是 JSON", `not json`, http.StatusBadRequest},
		{"角色非法", `{"project":"书A","chapter":"chapter_002","role":"style"}`, http.StatusBadRequest},
		// 重新核对的前提是有已提交产物。
		{"还没有产物", `{"project":"书A","chapter":"chapter_002","role":"checker"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := postRetry(t, v, tc.body).Code; got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPostRetryDisabled(t *testing.T) {
	v, proj := retryTestServer(t, false)
	if got := postRetry(t, v, `{"project":"书A","chapter":"chapter_002"}`).Code; got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
	if len(retryq.Pending(proj)) != 0 {
		t.Fatal("关掉入口后不该写队列")
	}
}

// 页面靠 /api/index 的 retries 给章节行挂「待重试」。
func TestPendingRetriesForIndex(t *testing.T) {
	v, proj := retryTestServer(t, true)
	if got := v.pendingRetries([]SessionInfo{{Project: "书A"}}); len(got) != 0 {
		t.Fatalf("空队列不该有 retries: %v", got)
	}
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: "chapter_002"}); err != nil {
		t.Fatal(err)
	}
	got := v.pendingRetries([]SessionInfo{{Project: "书A"}})
	if len(got["书A"]) != 1 || got["书A"][0] != "chapter_002" {
		t.Fatalf("retries = %v", got)
	}
	// img2text 板块的会话（Board 非空）不参与。
	if got := v.pendingRetries([]SessionInfo{{Project: "书A", Board: "img2text"}}); len(got) != 0 {
		t.Fatalf("img2text 不该出现在 retries: %v", got)
	}
}

// 角色透传：checker 请求在队列里是 checker，"重新核对"那一轮才认它。
func TestPostRetryCheckerRoleWithProduct(t *testing.T) {
	v, proj := retryTestServer(t, true)
	if err := os.MkdirAll(filepath.Join(proj, "work", "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "chapters", "chapter_002.tex"), []byte("\\chapter{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"checker","reason":"核对看错了"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Role != "checker" {
		t.Fatalf("队列 = %+v", pending)
	}
	roles := v.retryRoles([]SessionInfo{{Project: "书A"}})
	if roles["书A"]["chapter_002"] != "checker" {
		t.Fatalf("retryRoles = %v", roles)
	}
}
