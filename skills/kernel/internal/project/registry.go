// Package project 实现项目注册表与书级档案。
// 注册表管生命周期（active/paused/archived），profile.md 管书的长期记忆。
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Lifecycle 是项目生命周期状态。
type Lifecycle string

const (
	Active   Lifecycle = "active"   // 可派任务
	Paused   Lifecycle = "paused"   // 暂停（停派新任务，任务边界原子）
	Archived Lifecycle = "archived" // 封存（队列冻结只读）
)

// Project 是注册表里的一本书。
type Project struct {
	Name         string    `json:"name"`
	Root         string    `json:"root"`
	Lifecycle    Lifecycle `json:"lifecycle"`
	CurrentStage string    `json:"current_stage"`
	LastActive   time.Time `json:"last_active"`
}

// Registry 是项目注册表。
type Registry struct {
	Path     string              `json:"-"`
	Projects map[string]*Project `json:"projects"`
}

// RegistryPath 注册表文件路径（workspace 根下）。
func RegistryPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, "skills_registry.json")
}

// Load 读注册表；不存在则初始化空表。
func Load(path string) (*Registry, error) {
	r := &Registry{Path: path, Projects: map[string]*Project{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, fmt.Errorf("注册表损坏: %w", err)
	}
	return r, nil
}

// Save 原子写注册表（目录不存在时先建——workspace 根可能是全新目录）。
func (r *Registry) Save() error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	tmp := r.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.Path)
}

// Register 登记一本书（幂等）。
func (r *Registry) Register(name, root string) (*Project, error) {
	if p, ok := r.Projects[name]; ok {
		return p, nil
	}
	p := &Project{Name: name, Root: root, Lifecycle: Paused, LastActive: time.Now()}
	r.Projects[name] = p
	return p, r.Save()
}

// Activate 切换 active 指针：目标置 active，其余 active 的置 paused（一次只跑一本）。
func (r *Registry) Activate(name string) (*Project, error) {
	p, ok := r.Projects[name]
	if !ok {
		return nil, fmt.Errorf("项目未登记: %s", name)
	}
	for _, o := range r.Projects {
		if o.Lifecycle == Active && o.Name != name {
			o.Lifecycle = Paused
		}
	}
	p.Lifecycle = Active
	p.LastActive = time.Now()
	return p, r.Save()
}

// Pause 暂停项目：停派新任务（已领任务在任务边界自然完成）。
func (r *Registry) Pause(name string) (*Project, error) {
	p, ok := r.Projects[name]
	if !ok {
		return nil, fmt.Errorf("项目未登记: %s", name)
	}
	p.Lifecycle = Paused
	return p, r.Save()
}

// ProfilePath 书级档案路径。
func (p *Project) ProfilePath() string {
	return filepath.Join(p.Root, "profile.md")
}

// ProfileAppend 追加书级档案（append-only；文件不存在则创建带头部的文件）。
func (p *Project) ProfileAppend(text string) error {
	f, err := os.OpenFile(p.ProfilePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n## %s\n\n%s\n", time.Now().Format("2006-01-02 15:04"), text)
	return err
}

// Digest 全局蒸馏（status 命令数据源的一部分）。
func (r *Registry) Digest() map[string]any {
	out := map[string]any{}
	for name, p := range r.Projects {
		out[name] = map[string]any{
			"lifecycle":     p.Lifecycle,
			"current_stage": p.CurrentStage,
			"last_active":   p.LastActive,
		}
	}
	return out
}
