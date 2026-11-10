package session

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// T306：非流式响应把 finish_reason 放在 choices[0] 里，decode 要把它提到
// 顶层——否则调用方（分类器）看不出"回复被 max_tokens 截断"，只能看到一句
// "回复中未找到 JSON 对象"。
func TestDecodeLiftsFinishReason(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"{\"confidence\":0.9"},"finish_reason":"length"}]}`
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
	out, err := (&Client{}).decode(resp, false, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.FinishReason != "length" {
		t.Fatalf("finish_reason 没提上来：%q", out.FinishReason)
	}
	if len(out.Choices) != 1 || ContentString(out.Choices[0].Message) == "" {
		t.Fatalf("choices 解析异常：%+v", out.Choices)
	}
}

// T306：base_url 少了 /v1 时网关回 200 + HTML，流式读取器收不到任何 data:
// 行——以前静默当成"空回复"，现场表现是每张图都"回复中未找到 JSON 对象"
// 却没有任何 HTTP 错误。现在这种"根本不是 SSE"的响应直接报错并指出 base_url。
func TestStreamOnHTMLBodyIsAnError(t *testing.T) {
	html := "<!doctype html><html><head><title>New API</title></head><body>app</body></html>"
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html)),
		Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}}
	_, err := (&Client{}).decode(resp, true, time.Now(), nil)
	if err == nil {
		t.Fatal("HTML 响应必须报错，不能静默当成空回复")
	}
	if !strings.Contains(err.Error(), "/v1") || !strings.Contains(err.Error(), "SSE") {
		t.Fatalf("错误要指出 base_url 与 SSE：%v", err)
	}
}

// T50 的退化空响应走的是真 SSE（text/event-stream）：不能被我这条判断误伤，
// 它要继续由 session 的"空响应重发一次"接手。
func TestStreamOnEmptySSEStaysEmptyReply(t *testing.T) {
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	out, err := (&Client{}).decode(resp, true, time.Now(), nil)
	if err != nil {
		t.Fatalf("SSE 空流不该报错（交给 T50 处理）：%v", err)
	}
	if len(out.Choices) != 1 || ContentString(out.Choices[0].Message) != "" {
		t.Fatalf("应当是空回复：%+v", out.Choices)
	}
}
