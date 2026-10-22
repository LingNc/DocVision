package session

// T205：模型把工具调用写成正文（"skills://" 伪调用 + "工作区工具不可用"）时，
// 会话层**只记警告、不改行为**。T201 曾直接判失败，但那等于按正文特征替用户
// 做决定——各家网关的异常形态各异，正常回复里提到这类字符串也会被一起打死；
// 真正要看的是"这一轮有没有产出"，有没有产出用户一眼就能看出来、重试一次就好。

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

const toolProseLeakText = `{"paths":["skills://","skills://plugins"]}bash -lc pwd; ls -la
The chapter workspace tools are not available in this session, so I cannot convert it.`

func TestToolProseLeakWarnsButDoesNotFailRun(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		sse(w, toolProseLeakText, "stop", 60)
	}))
	defer srv.Close()

	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	client := NewClient(config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	client.SetLogger(log)
	// 有工具却一次都没调用：以前（T201）这里会直接判失败。
	sess := NewSession(client, config.ModelConfig{Model: "m"}, config.SessionTuning{MaxTokens: 100}, "sys",
		[]Tool{noopTool{}}, log, 1, "test")
	got, runErr := sess.Run(RunOptions{UserText: "convert this chapter"})
	if runErr != nil {
		t.Fatalf("退化回复不该判失败（T205）：%v", runErr)
	}
	if got != toolProseLeakText {
		t.Fatalf("正文应原样返回，得到 %q", got)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1（不重发、不额外提醒）", n)
	}
	if !sess.toolProseWarned {
		t.Fatal("应记一次警告标记，便于后续排查")
	}
}

// 警告每个会话只记一次：这种回复会连着来，刷屏没有意义。
func TestToolProseLeakWarnsOnce(t *testing.T) {
	sess := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "sys", nil, nil, 1, "test")
	if LooksLikeToolProseLeak("all good, nothing to report") {
		t.Fatal("普通正文不该命中")
	}
	if !LooksLikeToolProseLeak(toolProseLeakText) {
		t.Fatal("退化回复应命中")
	}
	if sess.toolProseWarned {
		t.Fatal("初始不应置位")
	}
}
