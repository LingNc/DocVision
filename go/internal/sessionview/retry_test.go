package sessionview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	v, proj := retryTestServer(t, true)
	cases := []struct {
		name string
		body string
		want int
	}{
		{"项目穿越", `{"project":"../etc","chapter":"chapter_002"}`, http.StatusNotFound},
		{"项目不存在", `{"project":"书B","chapter":"chapter_002"}`, http.StatusNotFound},
		{"章节名非法", `{"project":"书A","chapter":"../../x"}`, http.StatusBadRequest},
		{"章节不存在", `{"project":"书A","chapter":"chapter_099"}`, http.StatusNotFound},
		// P56⑤：子章节给按钮，但父章节整合完（提交产物已在）就必须改跑父章节。
		{"子章节父已整合", `{"project":"书A","chapter":"chapter_003_child_01"}`, http.StatusBadRequest},
		// 父节点既没整合、也没有正在跑它的临时树：多半是名字打错了。
		{"子章节找不到输入", `{"project":"书A","chapter":"chapter_009_child_01"}`, http.StatusNotFound},
		{"请求体不是 JSON", `not json`, http.StatusBadRequest},
		{"角色非法", `{"project":"书A","chapter":"chapter_002","role":"style"}`, http.StatusBadRequest},
		// 继续跑的前提是这一章已经有转录。
		{"没有转录", `{"project":"书A","chapter":"chapter_002","role":"continue"}`, http.StatusBadRequest},
		// 重新核对的前提是有已提交产物。
		{"还没有产物", `{"project":"书A","chapter":"chapter_002","role":"checker"}`, http.StatusBadRequest},
	}
	// "父章节已整合"这条要成立：父章节的提交产物必须在。
	if err := os.MkdirAll(filepath.Join(proj, "work", "chapters"), 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(proj, "work", "chapters", "chapter_003.tex"), []byte("% x\n"), 0o644)
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

// 继续跑：有转录就排进队列，角色透传。
func TestPostRetryContinueRole(t *testing.T) {
	v, proj := retryTestServer(t, true)
	if err := os.MkdirAll(filepath.Join(proj, "work", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "sessions", "convert_chapter_002.try2.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","reason":"我改了点东西"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Role != "continue" || pending[0].Reason != "我改了点东西" {
		t.Fatalf("队列 = %+v", pending)
	}
}

func postDrop(t *testing.T, v *viewerServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/drop", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	v.ServeHTTP(rec, req)
	return rec
}

// 删一条"转错的记录"：转录没了，同一次尝试的作废归档也一起删。
func TestPostDropDeletesTranscriptAndArchive(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	tr := filepath.Join(sess, "convert_chapter_002.try3.jsonl")
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(tr, old, old); err != nil {
		t.Fatal(err)
	}
	arch := filepath.Join(proj, "work", "attempts", "chapter_002", "try3")
	if err := os.MkdirAll(arch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(arch, "chapter_002.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postDrop(t, v, `{"project":"书A","path":"`+tr+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(tr); !os.IsNotExist(err) {
		t.Fatalf("转录应已删除: %v", err)
	}
	if _, err := os.Stat(arch); !os.IsNotExist(err) {
		t.Fatalf("作废归档应已删除: %v", err)
	}
}

// 还在写的会话不给删（90 秒内有更新），--no-retry 的实例整条入口关掉。
func TestPostDropRejections(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(sess, "convert_chapter_002.jsonl")
	if err := os.WriteFile(live, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := postDrop(t, v, `{"project":"书A","path":"`+live+`"}`); rec.Code != http.StatusConflict {
		t.Fatalf("正在写的转录应拒绝, status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec := postDrop(t, v, `{"project":"书A","path":"/etc/passwd"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非转录文件应拒绝, status = %d", rec.Code)
	}
	if rec := postDrop(t, v, `{"project":"书A","path":"`+filepath.Join(proj, "chapters", "chapter_002.md")+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("sessions 之外的文件应拒绝, status = %d", rec.Code)
	}
	off, _ := retryTestServer(t, false)
	if rec := postDrop(t, off, `{"project":"书A","path":"x.jsonl"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("关掉写入口应 403, status = %d", rec.Code)
	}
}

// 「从第几条记录之后继续」：范围与角色都要校验，合法时把 from/path 带进队列。
func TestPostRetryFromRecord(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n"
	if err := os.WriteFile(filepath.Join(sess, "convert_chapter_002.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// 只有「继续跑」能带 from。
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"convert","from":2,"path":"convert_chapter_002.jsonl"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("convert 带 from 应 400, status = %d", rec.Code)
	}
	// 超出范围 / 不是转录 / 源文件不存在。
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":9,"path":"convert_chapter_002.jsonl"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("超出范围应 400, status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":2,"path":"chapter_002.md"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非转录应 400, status = %d", rec.Code)
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":2,"path":"convert_chapter_009.jsonl"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("源不存在应 404, status = %d", rec.Code)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":2,"path":"convert_chapter_002.jsonl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	// File.Path 是队列文件自己的路径，请求里的 Path 在 Request 上（名字撞了）。
	if len(pending) != 1 || pending[0].From != 2 || pending[0].Request.Path != "convert_chapter_002.jsonl" {
		t.Fatalf("队列 = %+v", pending)
	}
}

// 矢量图重画（P53 矢量图侧）：只认 source/sessions 下的 vector_*.jsonl，
// 排进队列时不带章节（章节角标不该被它污染）。
func TestPostRetryVectorRole(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "source", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "source", "progress_items"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sess, "vector_book__pic001__label.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"project":"书A","role":"vector","path":"`
	if rec := postRetry(t, v, body+`convert_chapter_002.jsonl"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非作图转录应 400, status = %d", rec.Code)
	}
	if rec := postRetry(t, v, body+`vector_missing.jsonl"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("找不到转录应 404, status = %d", rec.Code)
	}
	rec := postRetry(t, v, body+`vector_book__pic001__label.jsonl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Role != "vector" || pending[0].Chapter != "" {
		t.Fatalf("队列 = %+v", pending)
	}
	// 从某条记录之后继续：行号越界服务端就拒（别排进队列白排）。
	body3 := `{"project":"书A","role":"vector","from":%d,"path":"vector_book__pic001__label.jsonl"}`
	if rec := postRetry(t, v, fmt.Sprintf(body3, 99)); rec.Code != http.StatusBadRequest {
		t.Fatalf("越界应 400, status = %d", rec.Code)
	}
	// 没有章节的请求不该挂到章节角标上。
	if got := v.pendingRetries([]SessionInfo{{Project: "书A"}}); len(got) != 0 {
		t.Fatalf("章节角标被污染: %+v", got)
	}
}

// T184：核对的转录也能"从第几条记录之后继续"——队列里角色仍是 checker（产物
// 不动、只换核对会话），源转录必须是这一章的**核对**转录，别的角色/章节不给。
func TestPostRetryCheckerFromRecord(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n"
	if err := os.WriteFile(filepath.Join(sess, "checker_chapter_002.try2.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// 核对要有已提交产物（这是既有前置条件，和 from 无关）。
	if err := os.MkdirAll(filepath.Join(proj, "work", "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "chapters", "chapter_002.tex"), []byte("%% x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 拿转换的转录来切核对 → 400（角色错位）。
	if err := os.WriteFile(filepath.Join(sess, "convert_chapter_002.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"checker","from":2,"path":"convert_chapter_002.jsonl"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("核对拿转换转录应 400, status = %d", rec.Code)
	}
	// 超出范围 → 400。
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"checker","from":9,"path":"checker_chapter_002.try2.jsonl"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("超出范围应 400, status = %d", rec.Code)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"checker","from":2,"path":"checker_chapter_002.try2.jsonl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Role != "checker" || pending[0].From != 2 ||
		pending[0].Request.Path != "checker_chapter_002.try2.jsonl" {
		t.Fatalf("队列 = %+v", pending)
	}
}

// P55 的兼容提示：老版本产生的会话没有工作区快照，排队回执里必须说清"只会
// 回退对话"，别让人以为文件也跟着回去了；有快照时不带这句。
func TestPostRetryFromRecordSnapshotNote(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sess := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n"
	if err := os.WriteFile(filepath.Join(sess, "convert_chapter_002.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":2,"path":"convert_chapter_002.jsonl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "没有工作区快照") {
		t.Fatalf("老会话的回执该提示只回退对话: %s", rec.Body.String())
	}
	// 补一条覆盖第 2 行的快照 journal → 不再提示。
	rounds := filepath.Join(proj, "work", "rounds", "chapter_002")
	if err := os.MkdirAll(rounds, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rounds, "journal-try1.jsonl"), []byte("{\"line\":2,\"sha\":\"abc123\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue","from":2,"path":"convert_chapter_002.jsonl"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "没有工作区快照") {
		t.Fatalf("有快照时不该提示: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "工作区会一起退回那一刻") {
		t.Fatalf("回执该说明工作区一起退: %s", rec.Body.String())
	}
}

// P56：误触的出口——排队请求还没被 latex 消费时，页面必须能撤回来（否则只能
// 眼睁睁看它开一个新尝试）。撤只认还没消费的待办；同章另一种角色、已消费的
// `.done.json` 都不许动。
func TestPostRetryCancel(t *testing.T) {
	v, proj := retryTestServer(t, true)
	// 继续跑要求这一章已经有转录，先放一条。
	if err := os.MkdirAll(filepath.Join(proj, "work", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "sessions", "convert_chapter_002.jsonl"), []byte("{\"t\":\"msg\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"convert"}`); rec.Code != 200 {
		t.Fatalf("排队失败: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_002","role":"continue"}`); rec.Code != 200 {
		t.Fatalf("排队继续跑失败: %d %s", rec.Code, rec.Body.String())
	}
	dir := filepath.Join(proj, "work", "retry")
	if n := len(retryqPending(t, dir)); n != 2 {
		t.Fatalf("应有 2 条待办，实际 %d", n)
	}
	cancel := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/retry/cancel", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		v.ServeHTTP(rec, req)
		return rec
	}
	// 只撤 convert，continue 那条留着。
	rec := cancel(`{"project":"书A","chapter":"chapter_002","role":"convert"}`)
	if rec.Code != 200 {
		t.Fatalf("撤销失败: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Removed int      `json:"removed"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("回执不是 JSON: %v（%s）", err, rec.Body.String())
	}
	if out.Removed != 1 || len(out.Files) != 1 {
		t.Fatalf("应只撤 1 条，回执 %+v", out)
	}
	left := retryqPending(t, dir)
	if len(left) != 1 || left[0] != "continue" {
		t.Fatalf("剩下的应该是 continue 那条，实际 %v", left)
	}

	// 已消费的（改了名）不算待办，撤销不能把它删掉。
	done := filepath.Join(dir, "req-1-chapter_002.done.json")
	if err := os.WriteFile(done, []byte(`{"chapter":"chapter_002","role":"continue"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := cancel(`{"project":"书A","chapter":"chapter_002","role":"continue"}`); rec.Code != 200 {
		t.Fatalf("撤销失败: %d", rec.Code)
	}
	if _, err := os.Stat(done); err != nil {
		t.Fatalf("已消费的留档不该被删: %v", err)
	}
	if rec := cancel(`{"project":"书A","chapter":"chapter_002","role":"continue"}`); rec.Code != 200 {
		t.Fatalf("重复撤销应成功（幂等）: %d", rec.Code)
	}

	// 关掉写入口时同样不许撤（与 /api/retry 一致）。
	off, _ := retryTestServer(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/retry/cancel", bytes.NewBufferString(`{"project":"书A","chapter":"chapter_002"}`))
	rec = httptest.NewRecorder()
	off.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("--no-retry 时应拒绝撤销，实际 %d", rec.Code)
	}
}

// retryqPending 读队列里还没消费的请求，返回它们的角色（convert 缺省补上）。
func retryqPending(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, f := range retryq.Pending(filepath.Dir(filepath.Dir(dir))) {
		role := f.Role
		if role == "" {
			role = "convert"
		}
		out = append(out, role)
	}
	return out
}

// TestPostRetryCheckerChildProduct（T197）：递归子章节的产物不在
// work/chapters/<章>.tex，而在 <父>/children/<子>.tex。以前"有没有提交产物"只查
// 顶层路径，于是子章节的「重新核对」一律 400"还没有提交产物"——按钮看着有、
// 点了就报错（同一批里页面的二次确认文案还把转换口径套在核对会话上）。
func TestPostRetryCheckerChildProduct(t *testing.T) {
	v, proj := retryTestServer(t, true)
	base := "chapter_014_child_04"
	children := filepath.Join(proj, "work", "chapters", "chapter_014", "children")
	if err := os.MkdirAll(filepath.Join(children, base), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(children, base+".tex"), []byte("\\chapter{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 父节点还在等子会话（临时细分树里有它的 md），转换口径的闸门不挡它。
	sub := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work", "_subdivision", "child_04", "node")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, base+".md"), []byte("# 子章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postRetry(t, v, `{"project":"书A","chapter":"`+base+`","role":"checker"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("子章节有产物时应能重新核对，status = %d body = %s", rec.Code, rec.Body.String())
	}
	// 暂存目录 children.tmp 里的半成品不算"已提交"。
	v2, proj2 := retryTestServer(t, true)
	tmpDir := filepath.Join(proj2, "work", "chapters", "chapter_014", "children", "children.tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, base+".tex"), []byte("\\chapter{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub2 := filepath.Join(proj2, "work", "temp", "conv_chapter_014", "work", "_subdivision", "child_04", "node")
	if err := os.MkdirAll(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub2, base+".md"), []byte("# 子章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := postRetry(t, v2, `{"project":"书A","chapter":"`+base+`","role":"checker"}`); rec.Code == http.StatusOK {
		t.Fatal("children.tmp 里的暂存产物不该被当成已提交")
	}
}

// TestPostTruncateDeepTranscript（T196）：递归子章节的转录不在 work/sessions/ 下，
// 而在父工作区的细分树里（work/temp/conv_<父>/work/_subdivision/**/node/sessions/）。
// 页面发的是 SessionInfo.Path（绝对路径），截断必须按"落在 <项目>/work/ 之下"解析
// ——以前按 filepath.Base 去 work/sessions/ 找，子章节一律 404，用户就"找不到那个
// 删掉这条再继续的功能"。同时钉住越界与穿越都被拒。
func TestPostTruncateDeepTranscript(t *testing.T) {
	v, proj := retryTestServer(t, true)
	deep := filepath.Join(proj, "work", "temp", "conv_chapter_007", "work", "_subdivision",
		"child_01", "node", "work", "_subdivision", "child_02", "node", "sessions")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "checker_chapter_007_child_01_child_02.jsonl"
	body := "{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n"
	full := filepath.Join(deep, name)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/truncate", strings.NewReader(body))
		rec := httptest.NewRecorder()
		v.ServeHTTP(rec, req)
		return rec
	}
	rec := post(`{"project":"书A","path":"` + filepath.ToSlash(full) + `","line":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("子章节转录应能就地截断，status = %d: %s", rec.Code, rec.Body.String())
	}
	if got, _ := os.ReadFile(full); string(got) != "{\"t\":\"msg\"}\n{\"t\":\"msg\"}\n" {
		t.Fatalf("截断结果不对: %q", string(got))
	}
	// 备份仍然落在 work/sessions/.trash/ 下（与顶层同一处，便于捞回）。
	trash, _ := filepath.Glob(filepath.Join(proj, "work", "sessions", ".trash", name+".*.jsonl"))
	if len(trash) != 1 {
		t.Fatalf("备份缺失: %v", trash)
	}
	// 落在工作区之外、或不在 work/ 之下的绝对路径一律拒。
	outside := filepath.Join(proj, "chapters", "x.jsonl")
	if err := os.WriteFile(outside, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{"project":"书A","path":"` + filepath.ToSlash(outside) + `","line":2}`); rec.Code == http.StatusOK {
		t.Fatal("工作区之外的路径不该被接受")
	}
	if rec := post(`{"project":"书A","path":"../../etc/passwd","line":2}`); rec.Code == http.StatusOK {
		t.Fatal("路径穿越不该被接受")
	}
}

// TestPostTruncateInPlace：默认操作是**就地截断**（不新开尝试、不复制转录），
// 而且原文必须先备份、隔夜不动的旧转录才敢改。
func TestPostTruncateInPlace(t *testing.T) {
	v, proj := retryTestServer(t, true)
	sessions := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "convert_chapter_002.jsonl"
	body := "{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n"
	full := filepath.Join(sessions, name)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/truncate", strings.NewReader(body))
		rec := httptest.NewRecorder()
		v.ServeHTTP(rec, req)
		return rec
	}
	rec := post(`{"project":"书A","path":"` + name + `","line":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{\"n\":1}\n{\"n\":2}\n" {
		t.Fatalf("就地截断结果不对: %q", string(got))
	}
	// 备份必须存在且是完整原文。
	trash, err := filepath.Glob(filepath.Join(sessions, ".trash", name+".*.jsonl"))
	if err != nil || len(trash) != 1 {
		t.Fatalf("备份文件缺失: %v %v", trash, err)
	}
	if data, _ := os.ReadFile(trash[0]); string(data) != body {
		t.Fatalf("备份内容不对: %q", string(data))
	}
	// 还在写就拒（90 秒内更新）。
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := post(`{"project":"书A","path":"` + name + `","line":1}`).Code; got != http.StatusConflict {
		t.Fatalf("活跃会话 status = %d, want 409", got)
	}
	if got := post(`{"project":"书A","path":"` + name + `","line":0}`).Code; got != http.StatusBadRequest {
		t.Fatalf("line=0 status = %d, want 400", got)
	}
}

// TestPostRetryChildCheckerIgnoresParent：**重新核对**不动产物，所以不受"父章节
// 已整合"那条限制（P56⑤ 只管 convert/continue）；同一章用 convert 才被拦。
func TestPostRetryChildCheckerIgnoresParent(t *testing.T) {
	v, proj := retryTestServer(t, true)
	chapters := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(chapters, 0o755); err != nil {
		t.Fatal(err)
	}
	// 父章节整合完（父 .tex 在）+ 子章节产物也在。
	for _, n := range []string{"chapter_003.tex", "chapter_003_child_01.tex"} {
		if err := os.WriteFile(filepath.Join(chapters, n), []byte("% x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	body := `{"project":"书A","chapter":"chapter_003_child_01","role":"checker"}`
	if rec := postRetry(t, v, body); rec.Code != http.StatusOK {
		t.Fatalf("子章节重新核对 status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_003_child_01","role":"convert"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("子章节重转 status = %d, want 400（父已整合）", rec.Code)
	}
}

// TestPostRetryChildUsesTempTree：父节点正在跑这一支时，子章节的 md 在节点的临时
// 细分树深处（父节点还没整合，`work/chapters/<父>.tex` 不在）。这一条曾因为
// `filepath.Glob` 的 `**` 只匹配一层而被误判成"找不到输入"。
func TestPostRetryChildUsesTempTree(t *testing.T) {
	v, proj := retryTestServer(t, true)
	md := filepath.Join(proj, "work", "temp", "conv_chapter_004", "work", "_subdivision", "chapter_004_child_02", "source", "chapter_004_child_02.md")
	if err := os.MkdirAll(filepath.Dir(md), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(md, []byte("# 子章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := postRetry(t, v, `{"project":"书A","chapter":"chapter_004_child_02"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}
