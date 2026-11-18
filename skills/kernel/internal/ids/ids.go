// Package ids 是人类可读编号体系（SK-P28）：imgxxx=原图、vecxxx=矢量成品、
// pdfpxx=页码。agent 内部用 sha/路径（它对 sha 更熟）；编号是给**人**的固定
// 指代——用户与 agent 讨论时说"img012"，任何一方查表即知是哪张图/哪一页。
// 映射表落盘 <projectRoot>/skills_ids.json。
package ids

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Kind 是编号类别。
type Kind string

const (
	KindImg Kind = "img" // 原图（classify 登记）
	KindVec Kind = "vec" // 矢量成品（draw 登记）
	KindPDF Kind = "pdfp" // 页码（pdfp<全局页>，由页表派生，不占用序列）
)

// Record 是一条编号映射。
type Record struct {
	ID     string `json:"id"`     // img001 / vec001 / pdfp137
	Kind   Kind   `json:"kind"`
	SHA    string `json:"sha"`    // 内容哈希（agent 内部指代）
	Path   string `json:"path"`   // 相对项目根路径
	Page   int    `json:"page"`   // 全书页码（图）；pdfp 的页
	BBox   [4]float64 `json:"bbox,omitempty"` // 页内坐标（点）
	TaskID string `json:"task_id,omitempty"` // 关联任务
}

// Table 是项目编号表。
type Table struct {
	Path    string            `json:"-"`
	Records map[string]*Record `json:"records"`
	Seq     map[Kind]int      `json:"seq"`

	mu sync.Mutex
}

// PathOf 映射表路径。
func PathOf(projectRoot string) string {
	return filepath.Join(projectRoot, "skills_ids.json")
}

// Load 读表（缺文件初始化空表）。
func Load(projectRoot string) (*Table, error) {
	t := &Table{Path: PathOf(projectRoot), Records: map[string]*Record{}, Seq: map[Kind]int{}}
	raw, err := os.ReadFile(t.Path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, t); err != nil {
		return nil, fmt.Errorf("编号表损坏: %w", err)
	}
	return t, nil
}

// Save 原子写。
func (t *Table) Save() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := t.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.Path)
}

// Register 登记一个编号：同 SHA 幂等返回已有编号（重复图共享 id）。
func (t *Table) Register(kind Kind, sha, path string, page int, bbox [4]float64, taskID string) (*Record, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if sha != "" {
		for _, r := range t.Records {
			if r.SHA == sha && r.Kind == kind {
				return r, nil
			}
		}
	}
	t.Seq[kind]++
	id := fmt.Sprintf("%s%03d", kind, t.Seq[kind])
	r := &Record{ID: id, Kind: kind, SHA: sha, Path: path, Page: page, BBox: bbox, TaskID: taskID}
	t.Records[id] = r
	if err := t.saveLocked(); err != nil {
		return nil, err
	}
	return r, nil
}

// PageID 返回页码编号（pdfp<全局页>——页不占序列，派生即得）。
func PageID(global int) string { return fmt.Sprintf("pdfp%d", global) }

// Get 查编号。
func (t *Table) Get(id string) (*Record, bool) {
	r, ok := t.Records[id]
	return r, ok
}

// BySHA 反查（agent 内部指代 → 人类编号）。
func (t *Table) BySHA(sha string) *Record {
	for _, r := range t.Records {
		if r.SHA == sha {
			return r
		}
	}
	return nil
}

// List 按类别列出（排序，确定性）。
func (t *Table) List(kind Kind) []*Record {
	var out []*Record
	for _, r := range t.Records {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	sortByID(out)
	return out
}

func sortByID(rs []*Record) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j-1].ID > rs[j].ID; j-- {
			rs[j-1], rs[j] = rs[j], rs[j-1]
		}
	}
}

func (t *Table) saveLocked() error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := t.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.Path)
}

var _ = strings.Contains // 占位
