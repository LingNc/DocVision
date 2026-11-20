// Package queue 实现 append-only 任务队列：JSON 落盘、原子写、租约回收。
// 队列是任务层的唯一权威；同样的磁盘状态必然产生同样的派发序列。
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// Queue 是单项目的一条任务队列。
type Queue struct {
	Path  string                `json:"-"` // 队列文件路径
	Tasks map[string]*task.Task `json:"tasks"`

	mu sync.Mutex
}

// Load 从磁盘读队列；文件不存在时返回空队列。
func Load(path string) (*Queue, error) {
	q := &Queue{Path: path, Tasks: map[string]*task.Task{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return q, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, q); err != nil {
		return nil, fmt.Errorf("队列文件损坏: %w", err)
	}
	return q, nil
}

// Save 原子写队列（tmp + rename），中断安全。
func (q *Queue) Save() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	tmp := q.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, q.Path); err != nil {
		// FUSE 工作区（/mnt 挂载）拒绝同目录 rename（11-20 dvws 实跑暴露）——
		// 回退 copy+remove。原子性降级可接受：内核是唯一写者，读侧Load对半写文件
		// 有容错（json 解析失败视为空队列重建会丢任务——故 copy 前先写全量 tmp 再覆盖）。
		data, rerr := os.ReadFile(tmp)
		if rerr != nil {
			return fmt.Errorf("queue 落盘 rename 失败且回退读取失败: %v / %w", err, rerr)
		}
		if werr := os.WriteFile(q.Path, data, 0o644); werr != nil {
			return fmt.Errorf("queue 落盘 rename 失败且回退写入失败: %v / %w", err, werr)
		}
		_ = os.Remove(tmp)
	}
	return nil
}

// Enqueue 追加任务；同 id 幂等（已存在则忽略）。
func (q *Queue) Enqueue(t *task.Task) error {
	if t.Created.IsZero() {
		t.Created = time.Now()
	}
	t.Updated = time.Now()
	if _, ok := q.Tasks[t.ID]; ok {
		return nil
	}
	q.Tasks[t.ID] = t
	return q.Save()
}

// Claim 认领至多 n 个指定类型（空 = 任意）的可认领任务，附 leaseSeconds 秒租约。
// 派发顺序确定性：按 Created 升序——同样的磁盘状态产生同样的派发序列。
func (q *Queue) Claim(owner string, n int, leaseSeconds int, types ...task.Type) []*task.Task {
	now := time.Now()
	want := map[task.Type]bool{}
	for _, ty := range types {
		want[ty] = true
	}
	var pool []*task.Task
	for _, t := range q.Tasks {
		if len(want) > 0 && !want[t.Type] {
			continue
		}
		if t.Reclaimable(now) {
			pool = append(pool, t)
		}
	}
	// 确定性派发：主键创建时间，同时间戳（批量入队）按 id 字典序——
	// 同样的磁盘状态必然产生同样的派发序列。
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].Created.Equal(pool[j].Created) {
			return pool[i].ID < pool[j].ID
		}
		return pool[i].Created.Before(pool[j].Created)
	})
	if len(pool) > n {
		pool = pool[:n]
	}
	for _, t := range pool {
		t.Status = task.StatusLeased
		t.Lease = &task.Lease{
			Owner:     owner,
			ClaimedAt: now,
			ExpiresAt: now.Add(time.Duration(leaseSeconds) * time.Second),
		}
		t.Updated = now
	}
	if len(pool) > 0 {
		_ = q.Save()
	}
	return pool
}

// Get 取任务。
func (q *Queue) Get(id string) (*task.Task, error) {
	t, ok := q.Tasks[id]
	if !ok {
		return nil, fmt.Errorf("任务不存在: %s", id)
	}
	return t, nil
}

// RecordVerdict 记录一次验收结果并更新状态；verdict.Pass=false 时 attempts+1。
func (q *Queue) RecordVerdict(id string, v task.Verdict, maxAttempts int) (*task.Task, error) {
	t, err := q.Get(id)
	if err != nil {
		return nil, err
	}
	v.Attempt = t.Attempts + 1
	t.Verdicts = append(t.Verdicts, v)
	t.Updated = time.Now()
	if v.Pass {
		t.Status = task.StatusDone
		t.Lease = nil
	} else {
		t.Attempts++
		t.Lease = nil // 打回即释放租约，任务回 pending
		if maxAttempts > 0 && t.Attempts >= maxAttempts {
			t.Status = task.StatusFailed
		} else {
			t.Status = task.StatusPending
		}
	}
	return t, q.Save()
}

// Unblock 解除依赖：blockedBy 全部 done 时转 pending。返回被解锁的任务。
func (q *Queue) Unblock(doneID string) []*task.Task {
	var unlocked []*task.Task
	for _, t := range q.Tasks {
		if t.Status != task.StatusBlocked {
			continue
		}
		rem := t.BlockedBy[:0]
		for _, b := range t.BlockedBy {
			if b != doneID {
				rem = append(rem, b)
			}
		}
		t.BlockedBy = rem
		if len(rem) == 0 {
			t.Status = task.StatusPending
			t.Updated = time.Now()
			unlocked = append(unlocked, t)
		}
	}
	if len(unlocked) > 0 {
		_ = q.Save()
	}
	return unlocked
}

// Digest 输出全局蒸馏摘要（status 命令的数据源）。
func (q *Queue) Digest() map[string]any {
	byStatus := map[string]int{}
	byType := map[string]int{}
	for _, t := range q.Tasks {
		byStatus[string(t.Status)]++
		byType[string(t.Type)]++
	}
	return map[string]any{"by_status": byStatus, "by_type": byType, "total": len(q.Tasks)}
}

// ProjectQueuePath 项目队列文件路径。
func ProjectQueuePath(projectRoot string) string {
	return filepath.Join(projectRoot, "skills_queue.json")
}
