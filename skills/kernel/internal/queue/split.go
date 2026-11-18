package queue

import (
	"fmt"
	"sort"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// Segment 是一个拆分提案里的段落区间（行号，含端点，从 1 起）。
type Segment struct {
	ID    string `json:"id"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// ValidateSplit 机械验证拆分提案（沿用旧线判据：2–16 段、完整、连续、无重叠）。
// chapterLines 是该章源文件总行数；返回错误清单（空 = 通过）。
func ValidateSplit(segs []Segment, chapterLines int) []string {
	var errs []string
	n := len(segs)
	if n < 2 {
		errs = append(errs, fmt.Sprintf("拆分必须至少 2 段，实际 %d 段", n))
	}
	if n > 16 {
		errs = append(errs, fmt.Sprintf("拆分最多 16 段，实际 %d 段", n))
	}
	if chapterLines <= 0 {
		errs = append(errs, "章节行数非法")
		return errs
	}
	sorted := make([]Segment, n)
	copy(sorted, segs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	for i, s := range sorted {
		if s.Start < 1 || s.End > chapterLines || s.Start > s.End {
			errs = append(errs, fmt.Sprintf("段 %s 行号越界 [%d,%d]（章节共 %d 行）", s.ID, s.Start, s.End, chapterLines))
			continue
		}
		if i == 0 {
			if s.Start != 1 {
				errs = append(errs, "第一段必须从第 1 行开始（拆分须完整覆盖章节）")
			}
		} else {
			prev := sorted[i-1]
			if s.Start != prev.End+1 {
				errs = append(errs, fmt.Sprintf("段 %s 与段 %s 不连续/重叠（%d 后面是 %d）", prev.ID, s.ID, prev.End, s.Start))
			}
		}
	}
	if n > 0 && sorted[n-1].End != chapterLines {
		errs = append(errs, fmt.Sprintf("最后一段必须结束于章节末行 %d，实际 %d（拆分须完整覆盖）", chapterLines, sorted[n-1].End))
	}
	return errs
}

// FanOutSplit 把验证过的拆分提案扇出为子任务：子任务入队，父任务转 blocked。
// 父任务在全部子任务 pass 后由 Unblock 解锁（队列依赖边，替代旧线会话树编排）。
func (q *Queue) FanOutSplit(parentID string, segs []Segment) ([]*task.Task, error) {
	parent, err := q.Get(parentID)
	if err != nil {
		return nil, err
	}
	if len(segs) < 2 {
		return nil, fmt.Errorf("拆分必须至少 2 段")
	}
	now := time.Now()
	parent.BlockedBy = parent.BlockedBy[:0]
	var children []*task.Task
	for _, s := range segs {
		child := &task.Task{
			ID:      fmt.Sprintf("%s/%s", parentID, s.ID),
			Type:    parent.Type,
			Project: parent.Project,
			Stage:   parent.Stage,
			Sandbox: fmt.Sprintf("%s/%s", parent.Sandbox, s.ID),
			Accept:  parent.Accept,
			Status:  task.StatusPending,
			Created: now,
			Updated: now,
		}
		if err := q.Enqueue(child); err != nil {
			return nil, err
		}
		children = append(children, child)
		parent.BlockedBy = append(parent.BlockedBy, child.ID)
	}
	parent.Status = task.StatusBlocked
	parent.Updated = now
	return children, q.Save()
}
