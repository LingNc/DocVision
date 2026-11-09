package latex

import (
	"path/filepath"
	"testing"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// fakeSubmitTool 模拟 submit 类工具的真实面目（T298）：被门禁打回时回一句
// REJECTED 的**正常回执**（error 为 nil），只有真收下时才置 Terminal。
type fakeSubmitTool struct{ accepted bool }

func (f *fakeSubmitTool) Definition() map[string]any { return nil }
func (f *fakeSubmitTool) Name() string               { return "submit_style" }
func (f *fakeSubmitTool) Execute(string) (session.ToolResult, error) {
	if f.accepted {
		return session.ToolResult{Text: "SUBMITTED.", Terminal: true}, nil
	}
	return session.ToolResult{Text: "REJECTED: 计划清单还有 1 项未完成：\n4. 还有一项没做完"}, nil
}

// 重放已记录的提交时，"被打回"不等于"收账"——否则阶段会带着未提交的状态
// 继续跑（T298 的真事故）。
func TestReplaySubmitDistinguishesRejectedFromAccepted(t *testing.T) {
	log, err := logger.NewLogger(filepath.Join(t.TempDir(), "run.log"), filepath.Join(t.TempDir(), "err.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	st := session.ResumeStats{SubmitName: "submit_style", SubmitArgs: `{"cls":"x.cls"}`}

	if replaySubmit(log, 1, "style", &fakeSubmitTool{accepted: false}, st) {
		t.Fatal("被打回的提交不该算重放成功")
	}
	if !replaySubmit(log, 1, "style", &fakeSubmitTool{accepted: true}, st) {
		t.Fatal("真收下的提交应算重放成功")
	}
}

// 没有记录过提交时什么都不做（保持原有语义）。
func TestReplaySubmitWithoutRecord(t *testing.T) {
	log, err := logger.NewLogger(filepath.Join(t.TempDir(), "run.log"), filepath.Join(t.TempDir(), "err.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if replaySubmit(log, 1, "style", &fakeSubmitTool{accepted: true}, session.ResumeStats{}) {
		t.Fatal("没有提交记录时应返回 false")
	}
}

// 回执首行截短：日志里只留一眼能看懂的那半句。
func TestReceiptLine(t *testing.T) {
	if got := receiptLine("REJECTED: 计划清单还有 1 项未完成：\n4. 细节"); got != "REJECTED: 计划清单还有 1 项未完成：" {
		t.Fatalf("首行截取不对: %q", got)
	}
	long := ""
	for i := 0; i < 40; i++ {
		long += "一二三四五六七八九十"
	}
	if n := len([]rune(receiptLine(long))); n > 121 {
		t.Fatalf("过长未截断: %d", n)
	}
}
