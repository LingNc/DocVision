package latex

/*
 * P61：复盘会话本身"没结论"时的处置（B）与目标状态可见性（C）。
 *
 * B：复盘会话也是会话——它可能报错终止，也可能跑完不调 submit。它不是"目标
 * 没救了"，只是一次没拿到结论的调用：丢掉这条转录、重开一条新的复盘会话，最多
 * 3 次（1 + 2 次重来）；每次都记一行警告，全都没结论时记一行"放弃"，目标退回
 * 引入复盘之前的路径（照常提醒、仍不提交就判失败）。复盘自己的转录绝不进复盘。
 *
 * C：复盘一开始就往**目标**转录写 `chapter_activity: rescuing`（预览页显示
 * 「复盘处理中」）；结论是 truncate/retry 时先写 `queued` 再排队，日志里两步
 * 先后可对。truncate 之后那一章仍按失败收尾，但终态必须是 queued——队列马上会
 * 重跑它，界面不该停在"会话错误"。
 */

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// targetActivityStates 读一条转录里的 chapter_activity 状态序列（按写入顺序）。
func targetActivityStates(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读不到目标转录 %s: %v", path, err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"kind":"chapter_activity"`) {
			continue
		}
		var rec struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("活动标记不是 JSON: %q", line)
		}
		out = append(out, rec.State)
	}
	return out
}

// indexOfState 返回 states 里第一次出现 want 的下标（-1 = 没有）。
func indexOfState(states []string, want string) int {
	for i, s := range states {
		if s == want {
			return i
		}
	}
	return -1
}

// (B-1) 复盘会话报错/不提交时丢弃重来：两次没结论之后第三次拿到 truncate，
// 队列只排一条"从那一条记录继续"，两次失败的复盘转录都进了 .trash。
func TestRescueDiscardsNoVerdictReviewAndRetries(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		chapterError:      http.StatusBadRequest,
		chapterErrorBody:  `{"error":{"message":"model rejected the request"}}`,
		verdicts:          []string{"truncate"},
		from:              1,
		rescueFailures:    []int{1, 2}, // 第 1 次报错终止、第 2 次跑完不提交
		keepRescueRecords: true,
	})
	if run.err == nil {
		t.Fatal("报错终止且复盘判 truncate 时，这一章仍应当按失败收尾（继续跑由队列接手）")
	}
	if run.stats.rescue != 3 {
		t.Fatalf("复盘会话应当重来 2 次后拿到结论（共 3 次），实际 %d 次（请求 %d）",
			run.stats.rescue, run.stats.requests)
	}
	queued := rescueQueuedRequests(t, run.proj)
	if len(queued) != 1 {
		t.Fatalf("应当只排一条重试请求，实际 %d 条: %v", len(queued), queued)
	}
	if got := queued[0]["role"]; got != "continue" {
		t.Errorf("重试请求的角色 = %v，期望 continue（转换会话从这一条继续）", got)
	}
	// 只有拿到结论的那次复盘留着转录；两次没结论的都挪进了 .trash。
	live := rescueTranscripts(t, run.proj)
	if len(live) != 1 {
		t.Fatalf("应当只留一条有结论的复盘转录，实际 %v", live)
	}
	if !strings.HasSuffix(live[0], ".jsonl") || strings.Contains(live[0], "2026") {
		t.Errorf("复盘转录名应当与转换/核对同一种口径（rescue_<章>[.tryN].jsonl），实际 %q", live[0])
	}
	trash, _ := filepath.Glob(filepath.Join(run.proj, "work", "sessions", ".trash", "rescue_*.jsonl"))
	if len(trash) != 2 {
		t.Errorf("两次没结论的复盘转录都应当留在 .trash 里，实际 %v", trash)
	}
	// 复盘自己的转录绝不进复盘：只有 3 条复盘请求，没有第 4 条"复盘复盘"。
	if run.stats.rescue > 3 {
		t.Errorf("复盘会话自己不该再触发复盘，复盘请求 = %d", run.stats.rescue)
	}
}

// (B-2, T233) 复盘会话自己没给结论**不算**这条目标的复盘次数：头三次复盘都
// 不给结论之后，代码把占用的那一次退回去、只记一笔"复盘自己失败"，下一次提醒
// 照旧换一条新复盘会话——所以第 4 条复盘拿到了结论，目标按正常路径收尾。
//
// 以前这里是把目标的复盘预算一次用光，"复盘自己坏掉"直接变成"这条目标再也别
// 复盘"，等于把复盘的问题算到目标头上；用户要求的正是这两件事分开计。
func TestRescueOwnFailuresDoNotExhaustTargetBudget(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		noSubmit:          true,
		rescueFailures:    []int{1, 2, 1},
		keepRescueRecords: true,
	})
	if run.err == nil || !strings.Contains(run.err.Error(), "未提交") {
		t.Fatalf("这一章最终仍应按老路径判失败（未提交），实际 %v", run.err)
	}
	// 复盘自己失败的次数不进目标的预算：第 4 条复盘照常起得来并给出结论。
	if run.stats.rescue != 4 {
		t.Fatalf("复盘自身失败应退回预算、再换一条复盘会话（共 4 条），实际 %d 条（请求 %d）",
			run.stats.rescue, run.stats.requests)
	}
	if max := rescueMaxOwnFailures * rescueMaxReviewAttempts; run.stats.rescue > max {
		t.Errorf("复盘自身失败的自我重试也要有上限（<= %d），实际 %d", max, run.stats.rescue)
	}
	// 第 4 条给了 retry：只够再发一次提醒，不会把转换会话反复重开。
	if run.stats.convert < 2 || run.stats.convert > 3 {
		t.Errorf("拿到 retry 后应当只多发一次提醒（转换请求 2–3），实际 %d", run.stats.convert)
	}
	// 拿不到结论的那几条复盘转录照旧丢弃，留下有结论的那一条。
	trash, _ := filepath.Glob(filepath.Join(run.proj, "work", "sessions", ".trash", "rescue_*.jsonl"))
	if len(trash) != 3 {
		t.Errorf("3 条没结论的复盘转录都应当进 .trash，实际 %d", len(trash))
	}
	if got := len(rescueTranscripts(t, run.proj)); got != 1 {
		t.Errorf("应当只留有结论的那条复盘转录，实际 %d 条", got)
	}
	// 目标转录的终态是 error（没提交判失败），不残留 rescuing。
	target := filepath.Join(run.proj, "work", "sessions", "convert_chapter_01.jsonl")
	states := targetActivityStates(t, target)
	if last := states[len(states)-1]; last == rescueActivityState {
		t.Errorf("复盘收尾后目标状态不该停在 rescuing：%v", states)
	}
}

// (C-1) 复盘一开始就把目标状态写成 rescuing，判 truncate 时先写 queued 再排队，
// 且失败终态保持 queued（队列马上会重跑这一章）。
func TestRescuePublishesTargetActivityBeforeQueueing(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		chapterError:     http.StatusBadRequest,
		chapterErrorBody: `{"error":{"message":"model rejected the request"}}`,
		verdicts:         []string{"truncate"},
		from:             1,
	})
	states := targetActivityStates(t, filepath.Join(run.proj, "work", "sessions", "convert_chapter_01.jsonl"))
	rescuing := indexOfState(states, rescueActivityState)
	if rescuing < 0 {
		t.Fatalf("复盘开始时应当往目标转录写 rescuing，实际状态序列 %v", states)
	}
	// queued 有两次：一次是转换进槽位时、一次是复盘判 truncate 之后。复盘那次
	// 一定在 rescuing 之后。
	after := indexOfState(states[rescuing:], "queued")
	if after < 0 {
		t.Fatalf("复盘判 truncate 后应当写 queued，实际状态序列 %v", states)
	}
	if last := states[len(states)-1]; last != "queued" {
		t.Fatalf("truncate 之后目标终态应当是 queued（队列接手），实际 %v", states)
	}
	// 顺序可审计：queued 写在排进重试队列之前——请求文件已存在，而转录里最后
	// 一行就是那次 queued。
	queuedReqs := rescueQueuedRequests(t, run.proj)
	if len(queuedReqs) != 1 {
		t.Fatalf("应当排一条 continue 请求，实际 %d 条", len(queuedReqs))
	}
	if got := queuedReqs[0]["chapter"]; got != "chapter_01" {
		t.Errorf("重试请求的章节 = %v，期望 chapter_01", got)
	}
}

// (C-2) 结论是 retry 时目标也先写 queued，再进提醒轮（提醒轮拿到槽位后写 running）。
func TestRescueRetryPublishesQueuedThenRunning(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{
		noSubmit: true,
		verdicts: []string{"retry"},
	})
	states := targetActivityStates(t, filepath.Join(run.proj, "work", "sessions", "convert_chapter_01.jsonl"))
	rescuing := indexOfState(states, rescueActivityState)
	if rescuing < 0 {
		t.Fatalf("复盘开始时应当写 rescuing，实际 %v", states)
	}
	tail := states[rescuing:]
	if indexOfState(tail, "queued") < 0 {
		t.Fatalf("判 retry 后应当写 queued（提醒轮排队之前），实际 %v", states)
	}
	if indexOfState(tail, "running") < 0 {
		t.Fatalf("提醒轮拿到槽位后应当写 running，实际 %v", states)
	}
	if indexOfState(tail, "queued") > indexOfState(tail, "running") {
		t.Fatalf("queued 必须写在 running 之前（先排队、再进槽位），实际 %v", states)
	}
}
