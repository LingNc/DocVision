package gen

import (
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// CheckSpec 是 T4 核对任务的验收 spec（与 verify.schema.go 的 check 分支对应）。
var CheckSpec = map[string]any{
	"required_files":  []any{"verdict.json"},
	"verdict_schema":  "check",
	"min_coverage":    false,
	"require_compile": false,
}

// CheckReport 是核对任务生成器的回执。
type CheckReport struct {
	TasksTotal int `json:"tasks_total"`
	Added      int `json:"added"`
	BlockedNow int `json:"blocked_now"` // 生成时 blocked 于未完成 convert 的任务数
}

// GenCheck 为队列里每个 convert 任务铺一个核对（check）任务：blocked 于该 convert
// （T4 延迟核对模式的结构表达——转换任务 done 后由 Unblock 自动解锁核对）。
// 幂等：同 id 跳过。只枚举队列中现存（未失败）的 convert 任务。
func GenCheck(q *queue.Queue) (*CheckReport, error) {
	before := countType(q, task.TypeCheck)
	now := time.Now()
	for _, ct := range q.Tasks {
		if ct.Type != task.TypeConvert || ct.Status == task.StatusFailed {
			continue
		}
		name := ct.ID[len("conv_"):]
		// convert 已 done 时不挂依赖边（顺序生成场景：先跑完再铺核对），直接可领。
		dep := []string{ct.ID}
		st := task.StatusBlocked
		if ct.Status == task.StatusDone {
			dep, st = nil, task.StatusPending
		}
		t := &task.Task{
			ID:        "chk_" + name,
			Type:      task.TypeCheck,
			Stage:     "check",
			Sandbox:   "tasks/chk_" + name,
			Inputs:    []string{}, // 核对是只读任务：输入经白名单路径给，不拷进沙箱
			WhitelistRead: []string{ct.Sandbox, "style/", "reports/"},
			Accept:    CheckSpec,
			BlockedBy: dep,
			Status:    st,
			Created:   now,
			Updated:   now,
		}
		if err := q.Enqueue(t); err != nil {
			return nil, err
		}
	}
	blocked := 0
	for _, t := range q.Tasks {
		if t.Type == task.TypeCheck && len(t.BlockedBy) > 0 && t.Status == task.StatusBlocked {
			blocked++
		}
	}
	return &CheckReport{
		TasksTotal: countType(q, task.TypeCheck),
		Added:      countType(q, task.TypeCheck) - before,
		BlockedNow: blocked,
	}, nil
}
