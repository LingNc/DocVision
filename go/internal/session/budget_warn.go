package session

// 输出预算与"思考"的相容性告警（T159）。
//
// max_tokens 是"思考 + 正文"的总预算，不是正文的长度限制。开着思考的模型
// 在预算偏小时会把预算全花在思考上、正文一个字都不吐：现场两例连续两轮
// finish=length、8192/8192、content 为空，会话被判 "empty final answer" 而
// 失败——看起来像"输出被截断"，其实是配置偏小。
//
// 代码无法替用户决定成本，所以只在会话创建时告警一次（同模型同预算只报
// 一次，避免几十个章节会话刷屏），并在被截断时兜底翻倍重试（见 Run）。

import (
	"fmt"
	"sync"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// thinkingBudgetFloor is the smallest completion budget we consider safe for
// a reasoning model: below it, thinking alone can consume the whole reply.
const thinkingBudgetFloor = 16384

var warnedBudget sync.Map // "model|budget" → true

// thinkingEnabled reports whether the model entry asks the vendor to think.
func thinkingEnabled(m config.ModelConfig) bool {
	t, ok := m.Thinking["type"].(string)
	return ok && (t == "enabled" || t == "adaptive")
}

// budgetWarning renders the one-line warning for a session whose model thinks
// but whose completion budget is too small to leave room for an answer.
// Returns "" when there is nothing to warn about.
func budgetWarning(model string, m config.ModelConfig, maxTokens int) string {
	if maxTokens <= 0 || maxTokens >= thinkingBudgetFloor || !thinkingEnabled(m) {
		return ""
	}
	return fmt.Sprintf("模型 %s 开启了思考，而本次会话 max_tokens=%d：思考会把预算吃光、正文可能为空（被截断时只会以空回复告终）。建议把该会话的 max_tokens 调到 32768 以上",
		model, maxTokens)
}

// warnBudgetOnce emits the warning at most once per model+budget pair, so a
// book with dozens of chapter sessions does not repeat the same line.
func warnBudgetOnce(log *logger.Logger, tid int, model string, m config.ModelConfig, maxTokens int) {
	msg := budgetWarning(model, m, maxTokens)
	if msg == "" || log == nil {
		return
	}
	key := fmt.Sprintf("%s|%d", model, maxTokens)
	if _, dup := warnedBudget.LoadOrStore(key, true); dup {
		return
	}
	log.LogWarning(tid, "[session] "+msg)
}
