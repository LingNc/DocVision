package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// Issue 是一条核对/验收发现的问题（回炉队列的载体，跨会话存活）。
type Issue struct {
	ID       string    `json:"id"`
	TaskID   string    `json:"task_id"`
	Project  string    `json:"project"`
	Desc     string    `json:"desc"`      // 必须含位置指针（核对任务的验收 spec 保证）
	Status   string    `json:"status"`    // open / resolved
	Created  time.Time `json:"created"`
	ReworkID string    `json:"rework_id"` // 由本问题生成的回炉任务
}

// IssueStore 是项目级问题存储（kernel 拥有，agent 经 issue_add/issue_resolve 写入）。
type IssueStore struct {
	Path   string            `json:"-"`
	Issues map[string]*Issue `json:"issues"`
}

// IssueStorePath 项目问题存储路径。
func IssueStorePath(projectRoot string) string {
	return filepath.Join(projectRoot, "skills_issues.json")
}

// LoadIssues 读问题存储；不存在则初始化。
func LoadIssues(path string) (*IssueStore, error) {
	s := &IssueStore{Path: path, Issues: map[string]*Issue{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("问题存储损坏: %w", err)
	}
	return s, nil
}

// Save 原子写。
func (s *IssueStore) Save() error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Add 追加问题（幂等：同 id 忽略）。
func (s *IssueStore) Add(iss *Issue) error {
	if iss.Created.IsZero() {
		iss.Created = time.Now()
	}
	if _, ok := s.Issues[iss.ID]; ok {
		return nil
	}
	s.Issues[iss.ID] = iss
	return s.Save()
}

// Resolve 标记解决。
func (s *IssueStore) Resolve(id string) error {
	iss, ok := s.Issues[id]
	if !ok {
		return fmt.Errorf("问题不存在: %s", id)
	}
	iss.Status = "resolved"
	return s.Save()
}

// SpawnRework 由问题生成回炉任务：原任务 blocked 于回炉，回炉任务继承原验收 spec + 逐条复核问题。
// 延迟核对模式的核心：问题落盘即入队，绝不靠 agent 记住回炉指令。
func (q *Queue) SpawnRework(iss *Issue) (*task.Task, error) {
	orig, err := q.Get(iss.TaskID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	rw := &task.Task{
		ID:      fmt.Sprintf("rework_%s", iss.ID),
		Type:    task.TypeRework,
		Project: orig.Project,
		Stage:   orig.Stage,
		Sandbox: orig.Sandbox, // 回原任务沙箱，scratch/notes.md 重入 O(1)
		Accept:  map[string]any{},
		Status:  task.StatusPending,
		Created: now,
		Updated: now,
	}
	for k, v := range orig.Accept {
		rw.Accept[k] = v
	}
	rw.Accept["rework_issue"] = iss.ID
	rw.Accept["rework_desc"] = iss.Desc
	if err := q.Enqueue(rw); err != nil {
		return nil, err
	}
	// 原任务挂起等回炉（若已在 rework 则幂等跳过）
	for _, b := range orig.BlockedBy {
		if b == rw.ID {
			return rw, q.Save()
		}
	}
	orig.BlockedBy = append(orig.BlockedBy, rw.ID)
	orig.Status = task.StatusBlocked
	orig.Lease = nil // 挂起即放弃租约
	orig.Updated = now
	iss.ReworkID = rw.ID
	return rw, q.Save()
}
