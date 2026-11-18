package gen

import (
	"path/filepath"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// img2text.go：T6 逐图文字化任务生成器（SK-P29）。
// 输入 doc_index，为每个图片块铺一个 img2txt_<seq> 小轮转任务：
//   - agent 用 ctx-up/ctx-down 检索该图前后文（kernel 从 doc_index 取邻块）；
//   - 用 page-render 看图（任务包带 global 页+bbox）；
//   - 写 out/description.md 提交，kernel 汇总。
// 与 classify/draw 明确分工：classify 判三态（结构化），img2text 产自然语言描述。

// Img2TextSpec 是 T6 验收 spec。
var Img2TextSpec = map[string]any{
	"required_files":  []any{"description.md"},
	"min_coverage":    false,
	"require_compile": false,
}

// Img2TextReport 是生成器回执。
type Img2TextReport struct {
	TasksTotal int      `json:"tasks_total"`
	Added      int      `json:"added"`
	ImageSeqs  []int    `json:"image_seqs"` // 已铺任务的 doc_index 条目 seq（排序）
}

// GenImg2Text 从 doc_index 的图片条目铺 T6 任务（按 seq 去重，幂等）。
// 任务包 Accept 带 entry_seq/global/bbox/img 指针，ctx 工具按 entry_seq 取邻块。
func GenImg2Text(q *queue.Queue, projectRoot string) (*Img2TextReport, error) {
	before := countType(q, task.TypeImg2Text)
	rep := &Img2TextReport{}
	idx, err := docindex.Load(filepath.Join(projectRoot, "doc_index.json"))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	seen := map[string]bool{}
	for _, e := range idx.Entries {
		if e.Img == "" || seen[e.Img] {
			continue
		}
		seen[e.Img] = true
		rep.ImageSeqs = append(rep.ImageSeqs, e.Seq)
		id := itoaSeq(e.Seq)
		t := &task.Task{
			ID:      "img2txt_" + id,
			Type:    task.TypeImg2Text,
			Stage:   "img2text",
			Sandbox: "tasks/img2txt_" + id,
			Inputs:  []string{},
			WhitelistRead: []string{"doc_index.json", "style/"},
			Accept: map[string]any{
				"required_files":  []any{"description.md"},
				"min_coverage":      false,
				"require_compile":   false,
				"entry_seq":         float64(e.Seq),
				"global":            float64(e.Global),
				"img":               e.Img,
				"figure_hint_bbox":  e.BBox,
			},
			Status:  task.StatusPending,
			Created: now,
			Updated: now,
		}
		if err := q.Enqueue(t); err != nil {
			return rep, err
		}
	}
	rep.TasksTotal = countType(q, task.TypeImg2Text)
	rep.Added = rep.TasksTotal - before
	return rep, nil
}

func itoaSeq(n int) string {
	// seq 直接用十进制串进任务 id（排序稳定、人类可读）
	digits := "0123456789"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}
