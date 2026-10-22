package session

// T205：模型把工具调用写成正文（"skills://" 伪调用 + "工作区工具不可用"）时，
// 会话层**不判失败**。T201 曾直接判失败，但那等于按正文特征替用户做决定——
// 各家网关的异常形态各异，正常回复里提到这类字符串也会被一起打死。
// P58（用户口径 2026-10-22）在这个基础上多走一步：既然"工具不可用"是幻觉、
// 工具一直在请求里，就把真实工具清单告诉它、要它现在就调（每会话一次），
// 仍然拒绝就照常收尾、交给提醒/复盘链路。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

const toolProseLeakText = `{"paths":["skills://","skills://plugins"]}bash -lc pwd; ls -la
The chapter workspace tools are not available in this session, so I cannot convert it.`

func TestToolProseLeakNudgesOnceButDoesNotFailRun(t *testing.T) {
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
	if n := calls.Load(); n != 2 {
		t.Fatalf("requests = %d, want 2（第一次回复 + 纠正后重试一次）", n)
	}
	// 纠正消息必须点名真实工具，并要求现在就调。
	var nudged bool
	for _, m := range sess.messages {
		text := ContentString(m)
		if m.Role == "user" && strings.Contains(text, "Your tools ARE available") {
			nudged = true
			if !strings.Contains(text, "noop") {
				t.Errorf("纠正消息里应列出真实工具名：%q", text)
			}
		}
	}
	if !nudged {
		t.Fatal("应追加一条「工具确实可用」的纠正消息")
	}
	if !sess.toolAccessNudged {
		t.Fatal("应记一次纠正标记，保证每会话只纠正一次")
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
