package latex

// T209 现场（用户报的截图）：核对会话退化时，界面上一句话是「checker 会话已退化，
// 已排队从干净轮次继续核对」，另一句却是「The checker reviewed your submitted
// chapter and found problems that must be fixed」——同一个转换会话里，代码一边说
// 已经排队重核、一边又让它去改内容，而那一刻**没有任何内容问题被指出来**。
//
// 根因是判断「核对有没有给结论」只看三个前缀，退化/放弃/视图准备失败这三种都漏在
// 外面，于是它们照样进了反馈轮。这里直接跑一章真实的 `convertOneChapter`（假客户端）
// 钉住行为：核对会话每轮都跑完不提交 → 复盘判 truncate → 排一条 role=checker 的重核
// 请求，而转换会话**一条反馈消息都收不到**；章还是按失败收尾、仍会走换会话重转换
// 那一条路（T217 的边界没动，只是不再先喂一轮无内容的反馈）。

import (
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
	// 但它确实退化过：复盘跑过（`latex.rescue.max_per_attempt` 默认 2，所以两次
	// 尝试各复盘一次，之后预算用完就只走"重开核对会话"），结论是「排一条重核
	// 请求」（与页面 ⤓ 同一条通路）。
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
	// 跳过反馈轮不等于放行：这一章照旧判失败，且真的又换会话重转过（T217 的
	// 「作废产物、换全新会话」仍然发生）——否则"跳过反馈轮"就成了静默采纳。
	if run.err == nil {
		t.Fatal("核对始终没给结论时应按失败收尾")
	}
	if !strings.Contains(run.err.Error(), "checker") {
		t.Errorf("失败原因应指向 checker：%v", run.err)
	}
	if run.stats.convert < 2 {
		t.Errorf("换会话重转换没有发生（转换请求 %d 次）", run.stats.convert)
	}
}
