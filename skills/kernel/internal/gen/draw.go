package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

// DrawSpec 给一张矢量图生成作图任务的验收 spec。
// T2 数值判据（figcheck 软墨迹对齐 + accept_min_submits）在 verify 的
// figure 接线批接入；这里先把任务结构与"编译通过"门禁立起来。
func DrawSpec(stem string) map[string]any {
	return map[string]any{
		"required_files":  []any{"figure.tex"},
		"main_file":       "figure.tex",
		"require_compile": true,
		"min_coverage":    false,
		"figure_stem":     stem,
	}
}

// DrawReport 是作图任务生成器的回执。
type DrawReport struct {
	TasksTotal   int      `json:"tasks_total"`
	Added        int      `json:"added"`
	VectorStems  []string `json:"vector_stems"`  // 本次驱动出的矢量图（排序）
	StyledText   int      `json:"styled_text"`   // styled_text 分类数（无需作图）
	Raster       int      `json:"raster"`        // raster 分类数（无需重绘）
	NoVerdict    int      `json:"no_verdict"`    // done 但 verdict.json 缺失/损坏（异常计数）
}

// GenDraw 从队列里已 done 的 classify 任务读回三态结论，为 class=vector 的图
// 铺一个 draw 任务（第一个"上游产物驱动下游任务"的生成器：分类结论就是
// dedupe 之后的 key 图清单）。图文件从 classify 沙箱 inputs 拷进 draw 沙箱
// inputs（记忆泡泡自包含）。幂等：同 id 跳过。
func GenDraw(q *queue.Queue, projectRoot string) (*DrawReport, error) {
	before := countType(q, task.TypeDraw)
	rep := &DrawReport{}
	now := time.Now()

	var cls []*task.Task
	for _, t := range q.Tasks {
		if t.Type == task.TypeClassify && t.Status == task.StatusDone {
			cls = append(cls, t)
		}
	}
	sort.Slice(cls, func(i, j int) bool { return cls[i].ID < cls[j].ID })

	for _, ct := range cls {
		stem := ct.ID[len("cls_"):]
		vb, err := os.ReadFile(filepath.Join(projectRoot, ct.Sandbox, "out", "verdict.json"))
		if err != nil {
			rep.NoVerdict++
			continue
		}
		var v verify.ClassifyVerdict
		if err := json.Unmarshal(vb, &v); err != nil {
			rep.NoVerdict++
			continue
		}
		switch v.Class {
		case "vector":
			rep.VectorStems = append(rep.VectorStems, stem)
			if err := enqueueDraw(q, projectRoot, ct, stem, now); err != nil {
				return rep, err
			}
		case "styled_text":
			rep.StyledText++
		case "raster":
			rep.Raster++
		}
	}
	sort.Strings(rep.VectorStems)
	rep.TasksTotal = countType(q, task.TypeDraw)
	rep.Added = rep.TasksTotal - before
	return rep, nil
}

// enqueueDraw 铺一个 draw 任务并拷贝图文件（classify 沙箱 inputs → draw 沙箱 inputs）。
func enqueueDraw(q *queue.Queue, projectRoot string, ct *task.Task, stem string, now time.Time) error {
	if len(ct.Inputs) == 0 {
		return fmt.Errorf("classify 任务 %s 无输入", ct.ID)
	}
	src := filepath.Join(projectRoot, ct.Sandbox, ct.Inputs[0])
	sandbox := "tasks/drw_" + stem
	dstDir := filepath.Join(projectRoot, filepath.FromSlash(sandbox), "inputs")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	name := filepath.Base(src)
	if err := os.WriteFile(filepath.Join(dstDir, name), data, 0o644); err != nil {
		return err
	}
	return q.Enqueue(&task.Task{
		ID:            "drw_" + stem,
		Type:          task.TypeDraw,
		Stage:         "draw",
		Sandbox:       sandbox,
		Inputs:        []string{"inputs/" + name},
		WhitelistRead: []string{"style/", "figures/"},
		Accept:        DrawSpec(stem),
		Status:        task.StatusPending,
		Created:       now,
		Updated:       now,
	})
}
