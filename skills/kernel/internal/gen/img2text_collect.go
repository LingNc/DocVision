package gen

import (
	"os"
	"path/filepath"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// img2text_collect.go：img2text 汇总回填（T6 设计里"kernel 整理"的落点）——
// 把 done 的 img2txt_* 任务的 out/description.md 回填进 doc_index 对应条目的
// Content 字段（按 entry_seq 匹配）。回填后 doc_search 能按描述搜到图、
// style 素材可由 agent 从索引自取。幂等：已有 Content 不覆盖（以先到的为准）。

// CollectImg2Text 回填全部 done img2text 任务到 doc_index，返回回填数。
func CollectImg2Text(q *queue.Queue, projectRoot string) (int, error) {
	idxPath := filepath.Join(projectRoot, "doc_index.json")
	idx, err := docindex.Load(idxPath)
	if err != nil {
		return 0, err
	}
	bySeq := map[int]*docindex.DocEntry{}
	for i := range idx.Entries {
		bySeq[idx.Entries[i].Seq] = &idx.Entries[i]
	}
	filled := 0
	for _, tk := range q.Tasks {
		if tk.Type != task.TypeImg2Text || tk.Status != task.StatusDone || len(tk.Verdicts) == 0 {
			continue
		}
		last := tk.Verdicts[len(tk.Verdicts)-1]
		if !last.Pass {
			continue
		}
		seqF, ok := tk.Accept["entry_seq"].(float64)
		if !ok {
			continue
		}
		desc, err := os.ReadFile(filepath.Join(projectRoot, tk.Sandbox, "out", "description.md"))
		if err != nil || len(desc) == 0 {
			continue
		}
		e, ok := bySeq[int(seqF)]
		if !ok || e.Content != "" {
			continue
		}
		e.Content = string(desc)
		filled++
	}
	if filled == 0 {
		return 0, nil
	}
	// 落盘（回填即增量更新索引）
	if err := docindex.Save(idx, idxPath); err != nil {
		return filled, err
	}
	return filled, nil
}
