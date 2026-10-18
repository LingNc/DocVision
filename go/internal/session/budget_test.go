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

// 输出预算必须给输入让路：厂商按 prompt + max_tokens 之和卡窗口。真实事故
// （2026-10-17 某项目整组核验）：窗口 262144、max_tokens=131072，prompt
// 涨过 131072 之后每一轮都是 HTTP 400，整个 336 张图的核验会话白跑 18 分钟。
func TestOutputBudgetLeavesRoomForPrompt(t *testing.T) {
	// 装得下就一点都不动（不能无缘无故把输出预算砍小）。
	if got := sessionOutputBudget(131072, 262144, 100000); got != 131072 {
		t.Fatalf("fits: got %d, want 131072", got)
	}
	// 事故现场：prompt=131073 时输出必须退到 125829（262144-131073-5242）。
	if got := sessionOutputBudget(131072, 262144, 131074); got != 262144-131074-5242 {
		t.Fatalf("overflow: got %d, want %d", got, 262144-131074-5242)
	}
	// 没有窗口配置（或没配输出预算）时保持原样：不能凭空编一个上限。
	if got := sessionOutputBudget(131072, 0, 999999); got != 131072 {
		t.Fatalf("no limit: got %d", got)
	}
	if got := sessionOutputBudget(0, 262144, 999999); got != 0 {
		t.Fatalf("no budget: got %d", got)
	}
	// 窗口小到连余量都放不下时，仍要发一个合法（>0）的输出预算。
	if got := sessionOutputBudget(131072, 4000, 3900); got != 512 {
		t.Fatalf("tiny window: got %d, want 512", got)
	}
	// 任意规模下都不许把 prompt+max_tokens 送出窗口（除非窗口小到连保底都
	// 放不下，那已经不是预算问题，压缩阶段该负责）。
	for prompt := 0; prompt <= 262144; prompt += 4096 {
		out := sessionOutputBudget(131072, 262144, prompt)
		if out == 131072 && prompt+131072+5242 > 262144 {
			t.Fatalf("prompt=%d unchanged at %d: would exceed the window", prompt, out)
		}
		if out != 131072 && prompt+out > 262144 && out != 512 {
			t.Fatalf("prompt=%d out=%d: %d > 262144", prompt, out, prompt+out)
		}
	}
	// 会话级：上次厂商实测的 prompt_tokens 也算进当前规模。
	s := NewSession(nil, config.ModelConfig{Model: "m"},
		config.SessionTuning{MaxTokens: 131072, ContextLimit: 262144}, "SYS", nil, nil, 1, "budget")
	if got := s.outputBudget(); got != 131072 {
		t.Fatalf("empty session: got %d", got)
	}
	s.lastPromptTokens = 200000
	if got := s.outputBudget(); got != 262144-200000-5242 {
		t.Fatalf("measured prompt: got %d, want %d", got, 262144-200000-5242)
	}
	// 反复调用结果一致（日志只解释一次，不影响数值）。
	if got := s.outputBudget(); got != 262144-200000-5242 {
		t.Fatalf("idempotent: got %d", got)
	}
}
