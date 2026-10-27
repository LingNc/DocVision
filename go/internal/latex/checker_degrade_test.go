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

	"mineru-tools/internal/session"
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

	// T231：这一章是**核对**没给结论，不是转换出错。以前失败只记在转换转录里、
	// 状态还是 error，页面上就是"转换 · 会话错误"——转换明明 submit 成功、run_end
	// 为 ok。现在两份转录各写 no_verdict，任何一条都不再读作会话错误。
	convertTr := filepath.Join(run.proj, "work", "sessions", "convert_chapter_01.jsonl")
	if got := lastChapterActivity(t, convertTr); got != "no_verdict" {
		t.Errorf("转换转录最后一条状态 = %q，期望 no_verdict（页面上这一行不该显示会话错误）", got)
	}
	checkers, _ := filepath.Glob(filepath.Join(run.proj, "work", "sessions", "checker_chapter_01*.jsonl"))
	if len(checkers) == 0 {
		t.Fatal("没有核对转录")
	}
	sawNoVerdict := false
	for _, path := range checkers {
		switch got := lastChapterActivity(t, path); got {
		case "no_verdict":
			sawNoVerdict = true
		case "error":
			t.Errorf("%s 最后一条状态仍是 error：没给结论不是会话错误", filepath.Base(path))
		}
	}
	if !sawNoVerdict {
		t.Errorf("核对那一边也该留下 no_verdict，实际转录 %v", checkers)
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

// T230（用户报的「核对#9 那条会话为什么一直不重试、复盘也不触发」）：核对转录
// 里已经重复到"提醒了还是不提交"之后，旧代码仍然把**同一份历史**回放给下一次
// 核对，于是同一段坏尾巴被一轮又一轮地追加：现场那条转录 319 行里有 34 次
// run_end、74 条章节活动、18 条核对锚点，全是"把 submit 写进正文 → 被提醒 →
// 再写一遍"。T233 起收口分两步：这条打满提醒的转录**不再回放**（也不再往上
// 写），但整章不因此失败——自动换的下一条全新尝试照常再核（默认 6 条），产物
// 一直留着；用户要的正是"没反馈就重新核对，而不是原地卡死"。
func TestCheckerTranscriptWithRepeatedRemindersIsNotResumed(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		checkerNoSubmit:      true,
		seedCheckerReminders: checkerNoVerdictAttempts,
	})

	// 那条已到线的转录不再被回放：换的是新的尝试文件，不是同一条历史。
	if run.stats.checker < 1 {
		t.Fatalf("打满提醒之后应当换一条全新核对尝试再核，实际核对请求 %d 次", run.stats.checker)
	}
	if run.stats.checker > 1+checkerNoVerdictSelfHeal {
		t.Errorf("自动重核次数应当有上限（<= %d），实际 %d", 1+checkerNoVerdictSelfHeal, run.stats.checker)
	}
	// 转录没被追加：没有新的提醒、没有新的章节活动。
	trPath := filepath.Join(run.proj, "work", "sessions", "checker_chapter_01.jsonl")
	data, err := os.ReadFile(trPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), checkerSubmitReminderMark); n != checkerNoVerdictAttempts {
		t.Fatalf("转录里提交提醒应为 %d 条（没有被追加），实际 %d 条", checkerNoVerdictAttempts, n)
	}
	// 产物保留、`.checker` 里写的是"没结论"那类文案：重跑或页面「重新核对」
	// 会接着核这一份，而不是被当成已通过。
	if !fileExists(filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")) {
		t.Fatal("核对没给结论不该作废产物")
	}
	marker, err := os.ReadFile(filepath.Join(run.proj, "work", "chapters", "chapter_01.tex.checker"))
	if err != nil {
		t.Fatal("应留下 .checker 标记，重跑时先重核这一份:", err)
	}
	if !checkerNoVerdict(string(marker)) {
		t.Fatalf(".checker 里的文案应属于「没结论」那一类，实际: %s", marker)
	}
	if run.err == nil {
		t.Fatal("核对始终没有结论应如实上报失败")
	}
}

// 判据只看"真实的提醒消息"，不看正文：把 submit 写进正文的那些轮次本来就不该
// 被数成提醒，也别把别的会话的提醒（"You have NOT submitted yet."）算进来。
func TestCountCheckerSubmitReminders(t *testing.T) {
	msgs := []session.ChatMessage{
		{Role: "system", Content: "You are a checker."},
		{Role: "assistant", Content: `{"submit": {"report": {"status": "pass"}}}`},
		{Role: "user", Content: `"submit": {"report": {"status": "pass"}}`},
		{Role: "user", Content: "  " + checkerSubmitReminderMark + " Use the `submit` TOOL."},
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: []map[string]any{{"type": "text", "text": "not a string body"}}},
		{Role: "user", Content: "You have NOT submitted yet."},
	}
	if got := countCheckerSubmitReminders(msgs); got != 1 {
		t.Fatalf("提交提醒应数到 1 条，实际 %d", got)
	}
}
