package latex

// T209 现场（用户报的截图）：核对会话退化时，界面上一句话是「checker 会话已退化，
// 已排队从干净轮次继续核对」，另一句却是「The checker reviewed your submitted
// chapter and found problems that must be fixed」——同一个转换会话里，代码一边说
// 已经排队重核、一边又让它去改内容，而那一刻**没有任何内容问题被指出来**。
//
// 根因是判断「核对有没有给结论」只看三个前缀，退化/放弃/视图准备失败这三种都漏在
// 外面，于是它们照样进了反馈轮。
//
// T222 现场（同一件事的另一半）：跳过假反馈之后，代码仍然作废这一版产物、换全新
// 会话整章重转——而产物根本还没被判定过。那句「已排队从干净轮次继续核对」承诺的
// 是重核，做的是重转，两头对不上；核对模型持续退化时，这一章会被作废重转最多 6 次、
// 每次再开若干核对会话，全是白烧。现在改成：**保留产物、只重开核对会话**，仍没有
// 结论就以「checker 不可用」收尾；带这种标记的章节重跑时先重核，绝不静默采纳。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckerDegradeDoesNotFeedConvertSession(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		checkerNoSubmit: true,
		verdicts:        []string{"truncate"},
		from:            1,
	})

	// 退化不是内容问题：转换会话不该收到「checker 发现这些问题」。
	for i, text := range run.stats.userTexts {
		if strings.Contains(text, "The checker reviewed your submitted chapter") {
			t.Fatalf("核对会话从没给出结论，转换会话却收到了第 %d 条反馈消息：\n%s", i+1, text)
		}
	}
	// 但它确实退化过：复盘跑过（`latex.rescue.max_per_attempt` 默认 2，所以每轮
	// 核对各复盘一次，结论是「排一条重核请求」——与页面 ⤓ 同一条通路）。
	if run.stats.rescue < 1 {
		t.Fatalf("核对退化应触发复盘，实际 %d 次（请求 %d）", run.stats.rescue, run.stats.requests)
	}
	reqs := rescueQueuedRequests(t, run.proj)
	if len(reqs) == 0 {
		t.Fatal("truncate 应排一条重核请求，实际一条都没有")
	}
	for _, req := range reqs {
		if req["role"] != "checker" || req["chapter"] != "chapter_01" {
			t.Errorf("排队请求不对：%+v", req)
		}
	}

	// T222：核对没给结论 → **重开核对会话**（至少两轮：初核 + 一次重核）。
	if run.stats.checker < 2 {
		t.Errorf("核对退化后应重开核对会话，实际只跑了 %d 个核对会话", run.stats.checker)
	}
	// 但**不重转**：转换会话只跑过开头那一次，产物没被作废重做。
	if run.stats.convert != 1 {
		t.Errorf("核对没给结论不该整章重转，实际转换请求 %d 次", run.stats.convert)
	}
	// 产物留在原处，归档目录为空：这一版没被判定过，不该被丢掉。
	texPath := filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")
	if _, err := os.Stat(texPath); err != nil {
		t.Fatalf("产物应保留在原处，实际 stat err = %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(run.proj, "work", "attempts")); err == nil && len(entries) > 0 {
		t.Errorf("核对没给结论不应归档产物，实际 work/attempts/ 有 %d 项", len(entries))
	}
	// 跳过反馈轮不等于放行：这一章照旧判失败，且失败原因指向"核对没结论"，
	// 并把这句话留在 `.checker` 里（重跑时靠它认出"这份产物还没被判定过"）。
	if run.err == nil {
		t.Fatal("核对始终没给结论时应按失败收尾")
	}
	if !strings.Contains(run.err.Error(), "没有给出结论") {
		t.Errorf("失败原因应说明核对没给结论：%v", run.err)
	}
	note, err := os.ReadFile(texPath + ".checker")
	if err != nil {
		t.Fatalf("应在 .checker 里留下结论，实际 stat err = %v", err)
	}
	if !checkerNoVerdict(string(note)) {
		t.Errorf(".checker 里应是「没结论」那类文案，实际：%q", string(note))
	}
}

// TestPendingNoVerdictProductIsRecheckedNotSkipped 钉住另一半：产物在、但上次核对
// 没给结论时，重跑不能走「已有产物，跳过」——跳过意味着整章连核对都不跑，一份
// 从没被判定过的东西直接进书。应当先重核这一份，通过就把 `.checker` 收掉。
func TestPendingNoVerdictProductIsRecheckedNotSkipped(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{preseedNoVerdict: true})
	if run.err != nil {
		t.Fatalf("重核通过应正常收尾，实际 %v", run.err)
	}
	if run.stats.convert != 0 {
		t.Errorf("产物已在时不该重转，实际转换请求 %d 次", run.stats.convert)
	}
	if run.stats.checker != 1 {
		t.Errorf("应重核这一份产物，实际核对会话 %d 个", run.stats.checker)
	}
	texPath := filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")
	if _, err := os.Stat(texPath + ".checker"); !os.IsNotExist(err) {
		t.Errorf("重核通过后应收掉 .checker，实际 stat err = %v", err)
	}
}
