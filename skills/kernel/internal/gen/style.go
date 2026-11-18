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
)

// style.go：style 阶段群的素材管道（"选材"环节的数据面）。
// styled_text 分类的提取内容（印刷在图里的文字：题记/题字/特殊排版文本）
// 从 classify verdict 汇总进 style/styled_text/ 素材池——写 style 会话（选材）
// 与转换会话（引用原书排版文本）从这里取料，不再各翻各的 verdict.json。

// StyleReport 是素材收集回执。
type StyleReport struct {
	Collected int      `json:"collected"`
	Materials []string `json:"materials"` // 素材文件名（排序）
	Pool      string   `json:"pool"`      // 素材池路径
}

// CollectStyleMaterials 把 done classify 任务里 styled_text 的提取内容汇总成
// 素材文件 style/styled_text/<task_id>.md（一图一条；幂等覆盖=提取内容以 verdict 为准）。
func CollectStyleMaterials(q *queue.Queue, projectRoot string) (*StyleReport, error) {
	rep := &StyleReport{Pool: "style/styled_text"}
	poolDir := filepath.Join(projectRoot, "style", "styled_text")
	if err := os.MkdirAll(poolDir, 0o755); err != nil {
		return rep, err
	}
	var ids []string
	for _, tk := range q.Tasks {
		if tk.Type != task.TypeClassify || tk.Status != task.StatusDone || len(tk.Verdicts) == 0 {
			continue
		}
		last := tk.Verdicts[len(tk.Verdicts)-1]
		if !last.Pass {
			continue
		}
		b, err := os.ReadFile(filepath.Join(projectRoot, tk.Sandbox, "out", "verdict.json"))
		if err != nil {
			continue
		}
		var v struct {
			Class     string `json:"class"`
			Extracted string `json:"extracted"`
		}
		if err := json.Unmarshal(b, &v); err != nil || v.Class != "styled_text" || v.Extracted == "" {
			continue
		}
		name := tk.ID + ".md"
		body := fmt.Sprintf("<!-- 来源: %s | 图: %s | 汇总: %s -->\n\n%s\n",
			tk.ID, firstInput(tk), time.Now().Format("2006-01-02 15:04"), v.Extracted)
		if err := os.WriteFile(filepath.Join(poolDir, name), []byte(body), 0o644); err != nil {
			return rep, err
		}
		ids = append(ids, name)
		rep.Collected++
	}
	sort.Strings(ids)
	rep.Materials = ids
	return rep, nil
}

// firstInput 取任务第一个 input 的文件名（溯源用）。
func firstInput(tk *task.Task) string {
	if len(tk.Inputs) == 0 {
		return ""
	}
	return filepath.Base(tk.Inputs[0])
}
