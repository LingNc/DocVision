package gen

import (
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// final.go：SK-P32 终审任务类型（T7）——旧线 fixSession/finalReview 的终审部分
// 固化为任务。语义：全书 convert/check 全部落定后，派一个**全书级只读终审**——
// 看装配产物整体（章节衔接/书签/目录/跨章一致性），与 per-chapter check 互补：
// check 看章内，final 看全书。编译修复（fixSession）不固化——验收打回走 rework。

// FinalSpec 是 T7 验收 spec（verdict 语义同 check：pass/issues/no_verdict，问题带位置）。
var FinalSpec = map[string]any{
	"required_files":  []any{"verdict.json"},
	"verdict_schema":  "final",
	"min_coverage":    false,
	"require_compile": false,
}

// FinalReport 是终审生成器回执。
type FinalReport struct {
	Enqueued   bool   `json:"enqueued"`
	Reason     string `json:"reason,omitempty"` // 未触发原因
	BlockedNow int    `json:"blocked_now"`
}

// GenFinal 触发条件：有 convert 任务且全部 done、全部 check done、无 pending
// 的 draw/img2text（未竟的图工作不算完书）→ 铺 fin_book 终审任务，
// blocked 于全部 conv/chk（已 done 则直接可领）。幂等。
func GenFinal(q *queue.Queue) (*FinalReport, error) {
	rep := &FinalReport{}
	var convDone, convOpen, checkDone, checkOpen int
	var deps []string
	for _, tk := range q.Tasks {
		switch tk.Type {
		case task.TypeConvert:
			if tk.Status == task.StatusDone {
				convDone++
				deps = append(deps, tk.ID)
			} else {
				convOpen++
			}
		case task.TypeCheck:
			if tk.Status == task.StatusDone {
				checkDone++
				deps = append(deps, tk.ID)
			} else {
				checkOpen++
			}
		case task.TypeDraw, task.TypeImg2Text:
			if tk.Status != task.StatusDone && tk.Status != task.StatusFailed {
				convOpen++ // 未竟图工作视作开放项
			}
		}
	}
	if convDone == 0 {
		rep.Reason = "尚无完成的 convert 任务"
		return rep, nil
	}
	if checkDone == 0 {
		rep.Reason = "尚无完成的核对（先 gen-check 跑过核对）"
		return rep, nil
	}
	if convOpen > 0 || checkOpen > 0 {
		rep.Reason = "仍有未完成的转换/核对/图工作"
		return rep, nil
	}
	// 全部落定：铺终审（依赖边仅含未完成项——此处全 done，直接可领）。
	// 幂等：已铺过直接回（Enqueue 同 id 跳过不报错，但不能谎称"本次入队"）。
	if _, err := q.Get("fin_book"); err == nil {
		return rep, nil
	}
	now := time.Now()
	dep := depsUnfinished(q, deps)
	st := task.StatusPending
	if len(dep) > 0 {
		st = task.StatusBlocked
	}
	err := q.Enqueue(&task.Task{
		ID:            "fin_book",
		Type:          task.TypeFinal,
		Stage:         "final",
		Sandbox:       "tasks/fin_book",
		Inputs:        []string{},
		WhitelistRead: []string{"build/", "chapters/", "style/", "reports/"},
		Accept:        FinalSpec,
		BlockedBy:     dep,
		Status:        st,
		Created:       now,
		Updated:       now,
	})
	if err != nil {
		return rep, err
	}
	rep.Enqueued = true
	if st == task.StatusBlocked {
		rep.BlockedNow = len(dep)
	}
	return rep, nil
}

// depsUnfinished 过滤出未完成依赖（全部 done 时返回 nil）。
func depsUnfinished(q *queue.Queue, deps []string) []string {
	var out []string
	for _, id := range deps {
		if tk, err := q.Get(id); err == nil && tk.Status != task.StatusDone {
			out = append(out, id)
		}
	}
	return out
}
