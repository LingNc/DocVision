// Package task 定义 skills 内核的任务模型。
// 任务 = 队列里的一个工作单元；任务是 agent 与 kernel 之间唯一的工作契约。
package task

import "time"

// Type 是任务类别，对应 docs/02-task-protocol.md 的五类任务。
type Type string

const (
	TypeClassify Type = "classify" // T1 图片三态分类
	TypeDraw     Type = "draw"     // T2 矢量图重绘
	TypeConvert  Type = "convert"  // T3 章节转换
	TypeCheck    Type = "check"    // T4 核对（只读）
	TypeRework   Type = "rework"   // T5 复盘回炉
	TypeImg2Text Type = "img2text" // T6 逐图文字化（小轮转：上下文up/down+看图+提交描述）
)

// Status 是任务生命周期状态。
type Status string

const (
	StatusPending Status = "pending" // 可认领
	StatusLeased  Status = "leased"  // 已认领（租约有效期内）
	StatusBlocked Status = "blocked" // 等待依赖（子任务/rework）
	StatusDone    Status = "done"    // 验收通过
	StatusFailed  Status = "failed"  // 验收连续打回达上限
)

// Task 是一个任务的完整记录。全部字段可 JSON 序列化落盘。
type Task struct {
	ID            string         `json:"task_id"`
	Type          Type           `json:"type"`
	Project       string         `json:"project"`
	Stage         string         `json:"stage"`        // 所属阶段（images/style/chapters/convert/check/...）
	Sandbox       string         `json:"sandbox"`        // 任务沙箱目录（相对项目根）
	Inputs        []string       `json:"inputs"`         // kernel 铺好的输入文件（相对沙箱）
	WhitelistRead []string       `json:"whitelist_read"` // 只读白名单（相对项目根）
	Accept        map[string]any `json:"acceptance"`     // 验收 spec（kernel 解释）
	BlockedBy     []string       `json:"blocked_by"`     // 依赖的任务 id
	Status        Status         `json:"status"`
	Lease         *Lease         `json:"lease,omitempty"`
	Attempts      int            `json:"attempts"` // 提交被打回次数
	Verdicts      []Verdict      `json:"verdicts"` // 历史验收记录（append-only）
	Created       time.Time      `json:"created"`
	Updated       time.Time      `json:"updated"`
}

// Lease 是任务租约：agent 崩溃/离开时租约过期，任务自动回队列。
type Lease struct {
	Owner     string    `json:"owner"` // 认领者标识（agent 会话名，仅记账）
	ClaimedAt time.Time `json:"claimed_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Expired 报告租约是否已过期。
func (l *Lease) Expired(now time.Time) bool { return now.After(l.ExpiresAt) }

// Verdict 是一次验收的落盘记录。
type Verdict struct {
	At      time.Time         `json:"at"`
	Pass    bool              `json:"pass"`
	Errors  []string          `json:"errors,omitempty"` // 机器可读的错误清单
	Notes   map[string]string `json:"notes,omitempty"`  // 备注：覆盖率放行说明/缺口、哈希快照、跳过原因
	Attempt int               `json:"attempt"`
}

// Reclaimable 报告任务是否可认领：pending，或 leased 但租约已过期。
// 注意：有未完成依赖（BlockedBy 非空）的任务不可认领（P82 判据）。
func (t *Task) Reclaimable(now time.Time) bool {
	if len(t.BlockedBy) > 0 {
		return false
	}
	if t.Status == StatusLeased {
		return t.Lease == nil || t.Lease.Expired(now)
	}
	return t.Status == StatusPending
}
