// Package gen 把项目制品铺进任务队列（接线层：制品 → 队列）。
// 每种任务类型一个生成器；全部幂等（同 id 已存在则跳过）。
package gen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// ClassifySpec 是 T1 分类任务的验收 spec（与 verify.schema.go 的 classify 分支对应）。
var ClassifySpec = map[string]any{
	"required_files":  []any{"verdict.json"},
	"verdict_schema":  "classify",
	"min_coverage":    false, // 无编译无覆盖率
	"require_compile": false,
}

// ClassifyReport 是生成器的结构化回执。
type ClassifyReport struct {
	ImagesDir  string `json:"images_dir"`
	TasksTotal int    `json:"tasks_total"` // 生成后 classify 任务总数（含已存在）
	Added      int    `json:"added"`         // 本次新入队
}

// GenClassify 扫描项目 imagesRel 目录，为每张图幂等铺一个 classify 任务
// （沙箱 inputs/<图> 引用 + classify 验收 spec）。同批生成同批派发（按文件名
// 排序入队，Created 相同 → 确定性派发顺序）。
func GenClassify(q *queue.Queue, projectRoot, imagesRel string) (*ClassifyReport, error) {
	before := countType(q, task.TypeClassify)
	// 图源两级发现：① 项目 source/images（抽图已归集）；② 真实 MinerU 产物的
	// mineru/*/auto/images/（未归集时直接发现，路径按项目相对登记）。
	type imgSrc struct {
		name string // 文件名
		path string // 项目内绝对路径
		rel  string // 项目相对路径（ids 登记用）
	}
	var srcs []imgSrc
	dir := filepath.Join(projectRoot, filepath.FromSlash(imagesRel))
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
				srcs = append(srcs, imgSrc{name: e.Name(), path: filepath.Join(dir, e.Name()),
					rel: filepath.ToSlash(filepath.Join(imagesRel, e.Name()))})
			}
		}
	}
	if len(srcs) == 0 {
		// ② MinerU 直出：mineru/<part>/auto/images/*
		matches, _ := filepath.Glob(filepath.Join(projectRoot, "mineru", "*", "auto", "images", "*.*"))
		for _, m := range matches {
			ext := strings.ToLower(filepath.Ext(m))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
				rel, rerr := filepath.Rel(projectRoot, m)
				if rerr == nil {
					srcs = append(srcs, imgSrc{name: filepath.Base(m), path: m, rel: filepath.ToSlash(rel)})
				}
			}
		}
	}
	sort.Slice(srcs, func(i, j int) bool { return srcs[i].rel < srcs[j].rel })
	now := time.Now()
	idTable, _ := ids.Load(projectRoot) // 人类编号表（SK-P28）；读失败不挡铺队
	for _, src := range srcs {
		name := src.name
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		sandbox := "tasks/cls_" + stem
		// 图拷进沙箱 inputs（记忆泡泡自包含——与 GenConvert 同原则）。
		inDir := filepath.Join(projectRoot, filepath.FromSlash(sandbox), "inputs")
		if err := os.MkdirAll(inDir, 0o755); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(src.path)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(inDir, name), data, 0o644); err != nil {
			return nil, err
		}
		// 登记人类编号 imgxxx（同 SHA 幂等共享编号）。
		if idTable != nil {
			h := sha256.Sum256(data)
			_, _ = idTable.Register(ids.KindImg, fmt.Sprintf("%x", h), src.rel, 0, [4]float64{}, "cls_"+stem)
		}
		t := &task.Task{
			ID:      "cls_" + stem,
			Type:    task.TypeClassify,
			Stage:   "images",
			Sandbox: sandbox,
			Inputs:  []string{"inputs/" + name},
			Accept:  ClassifySpec,
			Status:  task.StatusPending,
			Created: now,
			Updated: now,
		}
		if err := q.Enqueue(t); err != nil {
			return nil, err
		}
	}
	after := countType(q, task.TypeClassify)
	return &ClassifyReport{
		ImagesDir:  filepath.ToSlash(imagesRel),
		TasksTotal: after,
		Added:      after - before,
	}, nil
}

func countType(q *queue.Queue, ty task.Type) int {
	n := 0
	for _, t := range q.Tasks {
		if t.Type == ty {
			n++
		}
	}
	return n
}
