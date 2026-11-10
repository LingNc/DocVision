package sessionview

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/retryq"
)

// T309：作图会话的转录在 source/sessions/ 下，而"就地截断"和"删除这条记录"共用
// 的路径解析只认 work/——按钮看得见、点了必然 400「转录不在这个项目的工作区里」。
func vectorTranscriptFixture(t *testing.T) (*viewerServer, string, string) {
	t.Helper()
	v, proj := retryTestServer(t, true)
	dir := filepath.Join(proj, "source", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "vector_书A__aabbcc__fig.jsonl"
	body := strings.Join([]string{
		`{"t":"user","content":"画这张图"}`,
		`{"t":"assistant","content":"先编译"}`,
		`{"t":"tool","name":"compile"}`,
		`{"t":"assistant","content":"交一版"}`,
	}, "\n") + "\n"
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// 90 秒内的转录按"可能还在写"拒绝（真实防护），测试里往前挪十分钟。
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	return v, proj, full
}

func postJSONTo(t *testing.T, v *viewerServer, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBuffer(raw))
	rec := httptest.NewRecorder()
	v.ServeHTTP(rec, req)
	return rec
}

// 就地截断：按绝对路径截到第 2 行，原文备份进 work/sessions/.trash/。
func TestTruncateVectorTranscript(t *testing.T) {
	v, proj, full := vectorTranscriptFixture(t)
	rec := postJSONTo(t, v, "/api/truncate", map[string]any{"project": "书A", "path": full, "line": 2})
	if rec.Code != http.StatusOK {
		t.Fatalf("作图会话应当能就地截断，实际 %d：%s", rec.Code, rec.Body.String())
	}
	kept, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimRight(string(kept), "\n"), "\n") + 1; n != 2 {
		t.Fatalf("应当只剩 2 行，实际 %d：%s", n, kept)
	}
	trash, _ := filepath.Glob(filepath.Join(proj, "work", "sessions", ".trash", "*"))
	if len(trash) != 1 {
		t.Fatalf("原文要备份一份，实际 %d 份", len(trash))
	}
}

// 只给文件名（老调用）：work/sessions 里没有就退到 source/sessions。
func TestTruncateVectorTranscriptByBareName(t *testing.T) {
	v, _, full := vectorTranscriptFixture(t)
	rec := postJSONTo(t, v, "/api/truncate", map[string]any{"project": "书A", "path": filepath.Base(full), "line": 1})
	if rec.Code != http.StatusOK {
		t.Fatalf("裸文件名也要认得作图转录，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

// 删除这条记录：作图会话同样要能删（它的留档就在同目录，没有 work/attempts 归档）。
func TestDropVectorTranscript(t *testing.T) {
	v, _, full := vectorTranscriptFixture(t)
	rec := postJSONTo(t, v, "/api/drop", map[string]any{"project": "书A", "path": full})
	if rec.Code != http.StatusOK {
		t.Fatalf("作图会话应当能删记录，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Fatalf("转录应当被删掉：%v", err)
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if note, _ := res["note"].(string); strings.Contains(note, "work/attempts") {
		t.Fatalf("作图会话没有 work/attempts 归档，别这么说：%s", note)
	}
}

// 放宽到 source/sessions 不等于放开一切：项目里别的地方照样拒。
func TestTranscriptResolverRejectsOtherPaths(t *testing.T) {
	v, proj, _ := vectorTranscriptFixture(t)
	outside := filepath.Join(proj, "source", "images", "x.jsonl")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postJSONTo(t, v, "/api/truncate", map[string]any{"project": "书A", "path": outside, "line": 1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("会话目录之外的 jsonl 一律拒，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

// T310：撤销排队请求时拿队列文件名（req-….json）去跟转录名比，永远不等——
// 页面于是说"已经被 latex 消费，撤不回来了"，其实请求还在队里。
func TestCancelVectorRedrawRequest(t *testing.T) {
	v, proj, full := vectorTranscriptFixture(t)
	name := filepath.Base(full)
	if _, err := retryq.Queue(proj, retryq.Request{Project: "书A", Role: "vector", Path: name, Reason: "图不对，重画"}); err != nil {
		t.Fatal(err)
	}
	if got := len(retryq.Pending(proj)); got != 1 {
		t.Fatalf("排上之后应当有 1 条，实际 %d", got)
	}
	rec := postJSONTo(t, v, "/api/retry/cancel", map[string]any{"project": "书A", "role": "vector", "path": name})
	if rec.Code != http.StatusOK {
		t.Fatalf("撤销应当成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if n := int(res["removed"].(float64)); n != 1 {
		t.Fatalf("应当撤销 1 条，实际 %d：%s", n, rec.Body.String())
	}
	if got := len(retryq.Pending(proj)); got != 0 {
		t.Fatalf("撤销后队列应当空，实际剩 %d", got)
	}
}
