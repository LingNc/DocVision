package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// 轮次软限制：达到 warn_ratio 起在会话里插入（并就地更新）剩余轮次提醒，
// 到达硬上限后仍有 grace 轮可用工具，之后才真正关掉工具。
func TestToolRoundSoftBudget(t *testing.T) {
	tuning := config.SessionTuning{
		MaxToolRounds:       10,
		ToolRoundsWarnRatio: 0.7, // 提醒从第 7 轮起
		ToolRoundsGrace:     2,
	}
	if got := tuning.ToolRoundsGraceRounds(); got != 2 {
		t.Fatalf("grace = %d", got)
	}
	if got := (config.SessionTuning{}).ToolRoundsGraceRounds(); got != 20 {
		t.Fatalf("默认 grace = %d, 期望 20", got)
	}
	if got := (config.SessionTuning{ToolRoundsGrace: -1}).ToolRoundsGraceRounds(); got != 0 {
		t.Fatalf("grace<0 应关闭: %d", got)
	}
}

// 本地裁剪：超长工具结果只留头尾；老图片替换成文字占位（保留"看过什么"
// 的记录），最近 keep_images 张图仍在上下文里。
func TestPruneHistoryLocally(t *testing.T) {
	tuning := config.SessionTuning{PruneToolChars: 100, KeepImages: 1}
	s := NewSession(nil, config.ModelConfig{Model: "m"}, tuning, "SYS", nil, nil, 1, "test")
	s.SetMessages([]ChatMessage{
		{Role: "user", Content: []map[string]interface{}{
			{"type": "text", "text": "first image"},
			{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,AAA"}},
		}},
		{Role: "assistant", Content: ""},
		{Role: "tool", Content: strings.Repeat("x", 500)},
		{Role: "user", Content: []map[string]interface{}{
			{"type": "text", "text": "latest image"},
			{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,BBB"}},
		}},
	})
	msg := s.Messages()
	if n := s.pruneHistory(); n == 0 {
		t.Fatal("应发生本地裁剪")
	}
	msgs := s.Messages()
	// 老图 → 文字占位
	if hasImage(msgs[1]) {
		t.Errorf("最早那张图应被替换：%+v", msgs[1].Content)
	}
	if txt, _ := msgs[1].Content.(string); !strings.Contains(txt, "no longer carried") {
		t.Errorf("占位说明缺失: %v", msgs[1].Content)
	}
	// 最近一张图仍保留
	if !hasImage(msgs[4]) {
		t.Errorf("最近的图应保留: %+v", msgs[4].Content)
	}
	// 超长工具结果被截断
	if txt, _ := msgs[3].Content.(string); len(txt) > 300 || !strings.Contains(txt, "pruned") {
		t.Errorf("工具结果未裁剪: %d 字符", len(txt))
	}
	_ = msg
}

// 模型级图片上限（models.<名>.max_images）与 keep_images 是两件事：前者是平台
// 硬上限，在每次请求前把最旧的带图消息换成文字提示；后者只是本地历史偏好。
func TestPruneModelImagesEnforcesProviderCap(t *testing.T) {
	s := NewSession(nil, config.ModelConfig{Model: "m", MaxImages: 2}, config.SessionTuning{}, "SYS", nil, nil, 1, "test")
	img := func(label string) []map[string]interface{} {
		return []map[string]interface{}{
			{"type": "text", "text": label},
			{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,AAA"}},
		}
	}
	s.SetMessages([]ChatMessage{
		{Role: "user", Content: img("first")},
		{Role: "user", Content: img("second")},
		{Role: "user", Content: img("third")},
	})
	if n := s.pruneModelImages(); n != 1 {
		t.Fatalf("pruned = %d, want 1", n)
	}
	msgs := s.Messages()
	if hasImage(msgs[1]) || !hasImage(msgs[2]) || !hasImage(msgs[3]) {
		t.Fatalf("only the oldest image may be dropped: %+v", msgs)
	}
	if txt, _ := msgs[1].Content.(string); !strings.Contains(txt, "first") || !strings.Contains(txt, "max_images") {
		t.Fatalf("dropped image must keep text plus a re-view hint: %v", msgs[1].Content)
	}
	if n := s.pruneModelImages(); n != 0 {
		t.Fatalf("second call pruned %d, want 0", n)
	}
	// 0 = 不限制：没有模型上限时一张都不动。
	unlimited := NewSession(nil, config.ModelConfig{Model: "m"}, config.SessionTuning{}, "SYS", nil, nil, 1, "t2")
	unlimited.SetMessages([]ChatMessage{{Role: "user", Content: img("a")}})
	if n := unlimited.pruneModelImages(); n != 0 {
		t.Fatalf("unlimited model pruned %d", n)
	}
}

// 模型无视 tool_choice: none、预算耗尽后仍一直返回工具调用时，会话必须有界：
// 允许少量补跑以保持 wire 合法，之后就报错，不能无限空转（真实缺陷：整组
// 核验/核对会话曾因此挂死）。
func TestToolCallsAfterBudgetAreBounded(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"noop","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer srv.Close()
	stream := false
	cfg := config.ModelConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &stream}
	client := NewClient(cfg)
	tuning := config.SessionTuning{MaxToolRounds: 2, ToolRoundsGrace: -1}
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	s := NewSession(client, cfg, tuning, "SYS", []Tool{noopTool{}}, log, 1, "budget")
	_, runErr := s.Run(RunOptions{UserText: "go"})
	if runErr == nil {
		t.Fatal("want an error once the model keeps calling tools after the budget")
	}
	if n := calls.Load(); n > 5 {
		t.Fatalf("requests = %d, want <= 5 (budget 2 + 2 post-budget rounds + the failing check)", n)
	}
}

type noopTool struct{}

func (noopTool) Name() string { return "noop" }

func (noopTool) Definition() map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{"name": "noop", "description": "noop", "parameters": map[string]any{"type": "object"}}}
}

func (noopTool) Execute(string) (ToolResult, error) { return ToolResult{Text: "ok"}, nil }
