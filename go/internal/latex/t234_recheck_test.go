package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T234：核对打回一轮之后，修复完必须**重新真的核一遍**。
//
// 旧行为是把同一条核对转录再跑一次，而那条转录末尾已经有一次 submit——
// replaySubmit 把它原样重放，模型一次都没被再问，回来的还是上一轮的 issues
// 原文。现场就是这条链：整合节点连续 6 轮收到同一条「脚注缺正文」，每一轮修复
// 都真的补上并提交，核对从没看过；6 轮耗尽 → 整次尝试作废 → 已核对通过的子章节
// 跟着重转。
func TestCheckerRecheckAfterFixRoundIsAFreshConversation(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		checkerStatuses: []string{"issues", "pass"},
	})
	if run.err != nil {
		t.Fatalf("第一轮问题修好、第二轮核对通过，这一章应当收下：%v", run.err)
	}
	if run.stats.checker != 2 {
		t.Errorf("核对请求 = %d，期望 2（打回后要重新问一次核对模型，不能重放上一轮的结论）", run.stats.checker)
	}
	if run.stats.convert != 2 {
		t.Errorf("转换请求 = %d，期望 2（首轮 + 一次反馈轮），不该整章重转", run.stats.convert)
	}
	texPath := filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")
	if _, err := os.Stat(texPath + ".checker"); !os.IsNotExist(err) {
		t.Errorf("核对已经通过，不该留下问题清单 %s.checker（stat err = %v）", texPath, err)
	}
	try2 := filepath.Join(run.proj, "work", "sessions", "checker_chapter_01.try2.jsonl")
	if _, err := os.Stat(try2); err != nil {
		t.Errorf("重核应当另开一条核对转录 %s：%v", try2, err)
	}
}
