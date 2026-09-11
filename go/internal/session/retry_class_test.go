package session

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// countingServer answers every request with the given status/body and
// counts calls.
func countingServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// TestClientDoesNotRetryClientErrors: 4xx 是"这次请求本身就错了"，重试多少次
// 都是同一个结果。现场一次 `thinking.type: disable` 让服务端 400 拒掉每个请求，
// 却被当成 transient 重试 5 次（2/4/8/16/30s）——8 张图光退避就烧掉十几分钟。
func TestClientDoesNotRetryClientErrors(t *testing.T) {
	srv, calls := countingServer(t, 400, `{"error":{"message":"unknown variant `+"`disable`"+`"}}`)
	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		APIMaxRetries: 5, RateLimitRetries: 5,
	})
	start := time.Now()
	_, sentinel, status := c.CallWithRetry(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if got := calls.Load(); got != 1 {
		t.Errorf("400 请求次数 = %d, want 1（不重试）", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("400 不该退避等待，耗时 %s", elapsed)
	}
	if !strings.HasPrefix(sentinel, "[SESSION_API_ERROR: HTTP 400 (不可重试)") {
		t.Errorf("sentinel = %q，应标明 HTTP 400 且不可重试", sentinel)
	}
	if status != "error" {
		t.Errorf("status = %q, want error", status)
	}

	// 401/403 同理：一次就收手。
	for _, code := range []int{401, 403} {
		srv, calls := countingServer(t, code, `{"error":{"message":"nope"}}`)
		c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", APIMaxRetries: 5})
		c.CallWithRetry(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
		if got := calls.Load(); got != 1 {
			t.Errorf("HTTP %d 请求次数 = %d, want 1", code, got)
		}
	}
	// 404/422 是"端点不支持流式"的提示，客户端会各降级重发一次
	// （stream → 非 stream，shouldFallbackToNonStream），所以是 2 次；
	// 关键是**没有退避重试**（否则会有 5+ 次）。
	for _, code := range []int{404, 422} {
		srv, calls := countingServer(t, code, `{"error":{"message":"nope"}}`)
		c := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", APIMaxRetries: 5})
		c.CallWithRetry(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
		if got := calls.Load(); got != 2 {
			t.Errorf("HTTP %d 请求次数 = %d, want 2（流式 + 一次降级）", code, got)
		}
	}
}

// TestClientRetriesServerErrors: 5xx 是 transient，仍按 api_max_retries 重试。
func TestClientRetriesServerErrors(t *testing.T) {
	srv, calls := countingServer(t, 500, `internal boom`)
	c := NewClient(config.ModelConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		APIMaxRetries: 2, RateLimitRetries: 5,
	})
	c.CallWithRetry(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	if got := calls.Load(); got != 3 { // 首次 + 2 次重试
		t.Errorf("500 请求次数 = %d, want 3", got)
	}
}

// TestClientBalanceErrorIsNeverRetried: 余额/配额与 HTTP 状态无关（网关可能用
// 400/402/429 任意一种报），一次都不该重试。
func TestClientBalanceErrorIsNeverRetried(t *testing.T) {
	for _, tc := range []struct {
		code int
		body string
	}{
		{429, `{"error":{"message":"余额不足或无可用资源包,请充值","code":1113}}`},
		{400, `{"error":{"message":"insufficient quota"}}`},
		{402, `{"error":{"message":"billing: please top up"}}`},
	} {
		srv, calls := countingServer(t, tc.code, tc.body)
		c := NewClient(config.ModelConfig{
			BaseURL: srv.URL, APIKey: "k", Model: "m",
			APIMaxRetries: 5, RateLimitRetries: 5,
		})
		_, sentinel, _ := c.CallWithRetry(&ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "x"}}})
		if sentinel != "[SESSION_INSUFFICIENT_BALANCE]" {
			t.Errorf("HTTP %d 余额错误 sentinel = %q", tc.code, sentinel)
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("HTTP %d 余额错误请求次数 = %d, want 1", tc.code, got)
		}
	}
}

// TestHTTPStatusCodeParsing 钉住 "HTTP 400: {...}" 的解析（流式路径也一样）。
func TestHTTPStatusCodeParsing(t *testing.T) {
	if code, ok := httpStatusCode("HTTP 400: {\"error\":1}"); !ok || code != 400 {
		t.Errorf("HTTP 400 解析失败: %d %v", code, ok)
	}
	if code, ok := httpStatusCode("api error: HTTP 503: upstream down"); !ok || code != 503 {
		t.Errorf("HTTP 503 解析失败: %d %v", code, ok)
	}
	if _, ok := httpStatusCode("connection reset by peer"); ok {
		t.Error("没有状态码时不该误判")
	}
}
