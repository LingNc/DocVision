package latex

import (
	"testing"

	"mineru-tools/internal/session"
)

// 用户口径（2026-10-22）：中断后回来重跑是**原样续行、不插任何内容**，
// 也不该被 P58 的复盘/重试链路改变。复盘只在"这一轮非正常收尾"之后才触发
// （remindOrRescue / rescueAfterError），续跑的起点永远是空 UserText。
func TestResumeInsertsNoContent(t *testing.T) {
	history := []session.ChatMessage{{Role: "user", Content: "task"}, {Role: "tool", Content: "ok"}}
	if got := resumeUserText(history, "The session was interrupted earlier. Continue."); got != "" {
		t.Fatalf("有历史时续跑必须不插内容，得到 %q", got)
	}
	// 只有全新会话（历史为空）才用首句任务文本。
	if got := resumeUserText(nil, "fresh task"); got != "fresh task" {
		t.Fatalf("空历史应使用首句任务文本，得到 %q", got)
	}
}
