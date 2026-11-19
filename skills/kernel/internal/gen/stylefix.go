// stylefix.go：SK-I19 全书级样式问题后段——assess 分块 + stylefix 逐章适配。
// 触发：gen-stylefix。阶段一铺 assess（前置=convert/check/图工作全落定，仿 GenFinal）；
// assess 交 blocks.json 后，同命令重跑进入阶段二按块铺 stylefix（幂等按 block_id）。
package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

type StylefixReport struct {
	AssessAdded  int      `json:"assess_added"`
	AssessSkipped string  `json:"assess_skipped,omitempty"`
	BlocksAdded  int      `json:"blocks_added"`
	BlockIDs     []string `json:"block_ids,omitempty"`
	Skipped      []string `json:"skipped,omitempty"`
}

// assessOpenDeps 统计未竟的 convert/check/图工作（仿 GenFinal 前置语义）。
func assessOpenDeps(q *queue.Queue) (open int, doneConv, doneCheck int) {
	for _, tk := range q.Tasks {
		switch tk.Type {
		case task.TypeConvert, task.TypeCheck:
			if tk.Status == task.StatusDone {
				if tk.Type == task.TypeConvert {
					doneConv++
				} else {
					doneCheck++
				}
			} else {
				open++
			}
		case task.TypeDraw, task.TypeImg2Text:
			if tk.Status != task.StatusDone && tk.Status != task.StatusFailed {
				open++
			}
		}
	}
	return
}

// GenStylefixAssess 阶段一：全书落定后铺一个 stylefix-assess 任务（幂等：已有则不重复）。
func GenStylefixAssess(q *queue.Queue) (*StylefixReport, error) {
	rep := &StylefixReport{}
	open, doneConv, doneCheck := assessOpenDeps(q)
	if open > 0 {
		return rep, fmt.Errorf("全书尚未落定：还有 %d 个未竟 convert/check/图任务（已完成 convert %d / check %d）——先跑完再 gen-stylefix", open, doneConv, doneCheck)
	}
	if doneConv == 0 {
		return rep, fmt.Errorf("队列里没有已完成的 convert——还没到 stylefix 阶段")
	}
	for _, tk := range q.Tasks {
		if tk.Type == task.TypeStylefixAssess {
			rep.AssessSkipped = "assess 已铺（" + tk.ID + " " + string(tk.Status) + "）"
			return rep, nil
		}
	}
	id := "sfx_assess"
	q.Enqueue(&task.Task{ID: id, Type: task.TypeStylefixAssess, Sandbox: "tasks/" + id,
		Accept: map[string]any{"verdict_schema": "stylefix-assess", "done_conv": doneConv, "done_check": doneCheck}})
	rep.AssessAdded = 1
	return rep, nil
}

type sfxBlock struct {
	BlockID  string   `json:"block_id"`
	Problems []string `json:"problems"`
	Chapters []string `json:"chapters"`
}

// GenStylefixBlocks 阶段二：assess done 后按 blocks.json 铺 stylefix 任务（幂等按 block_id）。
func GenStylefixBlocks(q *queue.Queue, root string) (*StylefixReport, error) {
	rep := &StylefixReport{}
	var assess *task.Task
	for _, tk := range q.Tasks {
		if tk.Type == task.TypeStylefixAssess {
			assess = tk
		}
	}
	if assess == nil {
		return rep, fmt.Errorf("没有 stylefix-assess 任务——先 gen-stylefix 铺 assess")
	}
	if assess.Status != task.StatusDone {
		return rep, fmt.Errorf("assess 未 done（当前 %s）——先让 assess 会话交 blocks.json", assess.Status)
	}
	raw, err := os.ReadFile(filepath.Join(root, assess.Sandbox, "out", "blocks.json"))
	if err != nil {
		return rep, fmt.Errorf("assess 沙箱缺 out/blocks.json: %w", err)
	}
	var blocks []sfxBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return rep, fmt.Errorf("blocks.json 解析失败: %w", err)
	}
	if len(blocks) == 0 {
		return rep, fmt.Errorf("blocks.json 为空数组——assess 没分出块")
	}
	existing := map[string]bool{}
	for _, tk := range q.Tasks {
		if tk.Type == task.TypeStylefix && tk.Accept != nil {
			if bid, ok := tk.Accept["block_id"].(string); ok {
				existing[bid] = true
			}
		}
	}
	for _, b := range blocks {
		if b.BlockID == "" {
			return rep, fmt.Errorf("blocks.json 有缺 block_id 的块")
		}
		if existing[b.BlockID] {
			rep.Skipped = append(rep.Skipped, b.BlockID)
			continue
		}
		id := "sfx_" + b.BlockID
		q.Enqueue(&task.Task{ID: id, Type: task.TypeStylefix, Sandbox: "tasks/" + id,
			Accept: map[string]any{"verdict_schema": "stylefix", "block_id": b.BlockID, "chapters": b.Chapters, "problems": b.Problems}})
		rep.BlocksAdded++
		rep.BlockIDs = append(rep.BlockIDs, b.BlockID)
	}
	return rep, nil
}
