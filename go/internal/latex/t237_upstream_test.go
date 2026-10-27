package latex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
)

// TestIsSessionUpstreamError：上游接口不通（连接超时/网关错误/限流）要单独认
// 出来——这类错误跟会话质量无关，只能在同一会话上退避重试（T237）。明确标了
// "不可重试"的 4xx 和余额不足不算（重试多少次都一样）。
func TestIsSessionUpstreamError(t *testing.T) {
	upstream := []string{
		"api error: [SESSION_CONNECTION_TIMEOUT]",
		"[SESSION_API_ERROR: HTTP 502: bad gateway]",
		"[SESSION_RATE_LIMIT_EXCEEDED]",
		"[SESSION_TIMEOUT]",
		"read tcp: connection reset by peer",
	}
	for _, msg := range upstream {
		if !isSessionUpstreamError(msg) {
			t.Fatalf("%q 应当算上游接口错误", msg)
		}
	}
	not := []string{
		"[SESSION_API_ERROR: HTTP 400 (不可重试): model not found]",
		"[SESSION_INSUFFICIENT_BALANCE]",
		"会话没调用 submit 就结束了",
		"",
	}
	for _, msg := range not {
		if isSessionUpstreamError(msg) {
			t.Fatalf("%q 不该算上游接口错误", msg)
		}
	}
}

// TestAPIRetryDelayBacksOff：退避从 30 秒起翻倍、封顶 10 分钟。
func TestAPIRetryDelayBacksOff(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute,
		8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, w := range want {
		if got := apiRetryDelay(i + 1); got != w {
			t.Fatalf("第 %d 次重试应等 %s，实际 %s", i+1, w, got)
		}
	}
	if got := apiRetryDelay(0); got != 30*time.Second {
		t.Fatalf("0 当第 1 次处理，实际 %s", got)
	}
}

// TestQueueUpstreamRetryQueuesContinue：上游接口错误只排一条"过一会儿续跑同一条
// 会话"的请求（role=continue、带 NotBefore），不截断、不开新尝试；次数用满后
// 返回 false，调用方按"暂停"收尾（T237/T243）。
func TestQueueUpstreamRetryQueuesContinue(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	tr := filepath.Join(dir, "work", "sessions", "convert_chapter_777.jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("{\"t\":\"msg\",\"role\":\"user\",\"text\":\"hi\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{Latex: config.LatexConfig{APIErrorRetries: 2}}, log: log}
	target := rescueTarget{ProjDir: dir, Base: "chapter_777", Role: "convert", TrPath: tr}

	if !r.queueUpstreamRetry(0, "[convert]", "chapter_777", target, "api error: [SESSION_CONNECTION_TIMEOUT]") {
		t.Fatal("第一次上游错误应当排上重试")
	}
	if !r.rescueQueuedFor(tr) {
		t.Fatal("排上重试后应标记这条转录为已排队（失败终态要写 queued）")
	}
	files := retryq.Pending(dir)
	if len(files) != 1 {
		t.Fatalf("队列里应当正好一条请求，实际 %d", len(files))
	}
	req := files[0].Request
	if req.Role != "continue" || req.Chapter != "chapter_777" || req.From != 0 || req.CutAttempt != 0 {
		t.Fatalf("请求应当是同一会话接着跑（role=continue、不截断），实际 %+v", req)
	}
	at, err := time.Parse(time.RFC3339, req.NotBefore)
	if err != nil {
		t.Fatalf("NotBefore 应当是 RFC3339 时间：%q (%v)", req.NotBefore, err)
	}
	if wait := time.Until(at); wait <= 0 || wait > 31*time.Second {
		t.Fatalf("第一次重试应当等约 30 秒，实际 %s", wait)
	}

	if !r.queueUpstreamRetry(0, "[convert]", "chapter_777", target, "api error: [SESSION_CONNECTION_TIMEOUT]") {
		t.Fatal("第二次上游错误应当排上重试（上限 2）")
	}
	if r.queueUpstreamRetry(0, "[convert]", "chapter_777", target, "api error: [SESSION_CONNECTION_TIMEOUT]") {
		t.Fatal("上限用满后不该再排——再排一次还是同一个接口错误，应当暂停（T243）")
	}
}

// TestDispatchRetriesHonoursNotBefore：还没到点的请求安静留在队列里——不派发、
// 不消费、不占章节锁（T237）。
func TestDispatchRetriesHonoursNotBefore(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	r := &Runner{cfg: &config.Config{}, log: log}
	future := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	if _, err := retryq.Queue(dir, retryq.Request{
		Chapter: "chapter_777", Role: "continue", NotBefore: future, Source: "upstream",
	}); err != nil {
		t.Fatal(err)
	}
	if n := r.dispatchRetries(dir); n != 0 {
		t.Fatalf("没到点的请求不该派发，实际派发 %d 条", n)
	}
	if files := retryq.Pending(dir); len(files) != 1 {
		t.Fatalf("请求应当还在队列里，实际 %d 条", len(files))
	}
	// 到点后（这里把时间放到过去）就照常交给派发流程，不再被挡。
	if _, err := retryq.Queue(dir, retryq.Request{
		Chapter: "chapter_777", Role: "continue", NotBefore: time.Now().Add(-time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	// 章节 md 不存在 → 前置条件不满足，请求留着；这里只要求它没有被 NotBefore 挡。
	if why := r.retryBlockReason(dir, "continue", "chapter_777", retryq.Request{}); why == "" {
		t.Fatal("缺章节 md 时前置条件应当说明原因")
	}
}
