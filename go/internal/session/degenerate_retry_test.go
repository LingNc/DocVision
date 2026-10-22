package session

// T50：厂商在高压/异常时可能回一条 finish=length、0 completion token、
// 无内容/无思考/无工具调用的空消息。会话层应原样重发一次（上限 1 次），
// 仍退化才落 nudge/报错——而不是带着一条空 assistant 消息直接失败。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// sse replies with one assistant chunk plus a usage chunk in SSE format.
func sse(w http.ResponseWriter, content, finish string, completion int) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":%q}],\"usage\":{\"prompt_tokens\":1447,\"completion_tokens\":%d,\"total_tokens\":1447}}\n\n", content, finish, completion)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestDegenerateEmptyResponseRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// 退化响应：空内容、finish=length、completion_tokens=0。
			sse(w, "", "length", 0)
			return
		}
		sse(w, "recovered", "stop", 3)
	}))
	defer srv.Close()

	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	client.SetLogger(log)
	sess := NewSession(client, config.ModelConfig{Model: "m"}, config.SessionTuning{MaxTokens: 100}, "sys", nil, log, 1, "test")
	got, err := sess.Run(RunOptions{UserText: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "recovered" {
		t.Errorf("final = %q, want recovered", got)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("requests = %d, want 2 (degenerate retried once)", n)
	}
}

func TestDegenerateEmptyResponseGivesUpAfterOneRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// finish=stop 才是"退化空回复"（finish=length 由 T159 的预算翻倍路径
		// 接管，见 TestNudgeBoostsBudgetAfterLengthTruncation）。
		sse(w, "", "stop", 0)
	}))
	defer srv.Close()

	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	client.SetLogger(log)
	sess := NewSession(client, config.ModelConfig{Model: "m"}, config.SessionTuning{MaxTokens: 100}, "sys", nil, log, 1, "test")
	if _, err := sess.Run(RunOptions{UserText: "hi"}); err == nil {
		t.Fatal("Run should fail when the retry is degenerate too")
	}
	// 1 次原始请求 + 1 次重试 + 1 次 nudge = 3。
	if n := calls.Load(); n != 3 {
		t.Errorf("requests = %d, want 3 (retry + nudge)", n)
	}
}

func TestDegenerateEmptyResponseHelper(t *testing.T) {
	msg := func(content string) *ChatResponseChoice {
		m := ChatMessage{Role: "assistant", Content: content}
		return &ChatResponseChoice{Message: m}
	}
	deg := &ChatResponse{Usage: &Usage{CompletionTokens: 0}}
	if !degenerateEmptyResponse(deg, msg("")) {
		t.Error("empty content + 0 completion should be degenerate")
	}
	// 有真实输出 token 的空内容（finish=stop、思考被网关吞掉）同样是退化
	// 响应：真实案例 completion 1819 tokens、正文/思考/工具调用全空，会话
	// 静默停住。原样重发一次比走 nudge 更对。
	ok := &ChatResponse{Usage: &Usage{CompletionTokens: 12}}
	if !degenerateEmptyResponse(ok, msg("")) {
		t.Error("empty content with real completion tokens is still degenerate")
	}
	// 有内容不是退化响应。
	if degenerateEmptyResponse(deg, msg("text")) {
		t.Error("non-empty content is not degenerate")
	}
	// 无 usage 块不是退化响应（保守：不抢 nudge 路径）。
	if degenerateEmptyResponse(&ChatResponse{}, msg("")) {
		t.Error("missing usage block is not degenerate")
	}
}
