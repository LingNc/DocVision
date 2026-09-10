package session

import (
	"strings"
	"testing"

	"mineru-tools/internal/config"
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
