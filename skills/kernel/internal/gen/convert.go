package gen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// ConvertSpec 给一章生成转换任务的验收 spec（与 verify/texcheck 对应）：
// 主产物 <章名>.tex 编译通过 + 内容覆盖率（inputs/chapter.md 是对照源，
// 正是 verify.NewChecker 的默认 CoverageSource）。
func convertSpec(name string) map[string]any {
	return map[string]any{
		"required_files":  []any{name + ".tex"},
		"main_file":       name + ".tex",
		"require_compile": true,
		"min_coverage":    true,
	}
}

// ConvertReport 是转换任务生成器的回执。
type ConvertReport struct {
	ChaptersDir string `json:"chapters_dir"`
	TasksTotal  int    `json:"tasks_total"`
	Added       int    `json:"added"`
}

// GenConvert 扫描项目 chaptersRel 目录的章节 md，为每章幂等铺一个 convert 任务：
// 章节 md 拷进任务沙箱 inputs/chapter.md（覆盖率对照源），任务 id = conv_<章名>。
// 幂等：同 id 跳过；已拷的 inputs 不覆盖（agent 可能已改过分章内容？不——inputs
// 是 kernel 的只读输入，重拷保持与章节源一致，覆盖是正确语义）。
func GenConvert(q *queue.Queue, projectRoot, chaptersRel string) (*ConvertReport, error) {
	before := countType(q, task.TypeConvert)
	dir := filepath.Join(projectRoot, filepath.FromSlash(chaptersRel))
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var names []string
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	now := time.Now()
	for _, name := range names {
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		sandbox := "tasks/conv_" + stem
		inDir := filepath.Join(projectRoot, filepath.FromSlash(sandbox), "inputs")
		if err := os.MkdirAll(inDir, 0o755); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(inDir, "chapter.md"), data, 0o644); err != nil {
			return nil, err
		}
		t := &task.Task{
			ID:      "conv_" + stem,
			Type:    task.TypeConvert,
			Stage:   "convert",
			Sandbox: sandbox,
			Inputs:  []string{"inputs/chapter.md"},
			Accept:  convertSpec(stem),
			Status:  task.StatusPending,
			Created: now,
			Updated: now,
		}
		if err := q.Enqueue(t); err != nil {
			return nil, err
		}
	}
	after := countType(q, task.TypeConvert)
	return &ConvertReport{
		ChaptersDir: filepath.ToSlash(chaptersRel),
		TasksTotal:  after,
		Added:       after - before,
	}, nil
}
