package verify

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/texcheck"
)

// assemble.go：assemble-verify——消费 verdict 里的 artifact_sha256 快照，
// 复核 done 任务的 out/ 自验收以来未被篡改/替换/删除。这是"验收后完整性"的
// 最后一块：验收可信，但验收之后到装配之间产物可能被手滑改动。

// ArtifactDrift 是一个 done 任务的产物漂移报告。
type ArtifactDrift struct {
	TaskID      string   `json:"task_id"`
	Mismatched  []string `json:"mismatched"`  // 哈希对不上（被改/被替换）
	Missing     []string `json:"missing"`     // 快照里有、盘上没了（被删）
	Extra       []string `json:"extra"`       // 快照后新增（可能是合法补充）
	NoSnapshot  bool   `json:"no_snapshot"` // verdict 无哈希快照（无法复核）
}

// CheckDrift 复核全部 done 任务的 out/ 与各自最后一次 pass verdict 的快照。
func CheckDrift(tasks map[string]*task.Task, projectRoot string) []ArtifactDrift {
	var out []ArtifactDrift
	for _, t := range tasks {
		if t.Status != task.StatusDone || len(t.Verdicts) == 0 {
			continue
		}
		last := t.Verdicts[len(t.Verdicts)-1]
		snapRaw, ok := last.Notes["artifact_sha256"]
		if !ok || snapRaw == "" {
			out = append(out, ArtifactDrift{TaskID: t.ID, NoSnapshot: true})
			continue
		}
		var snap map[string]string
		if err := json.Unmarshal([]byte(snapRaw), &snap); err != nil {
			out = append(out, ArtifactDrift{TaskID: t.ID, NoSnapshot: true})
			continue
		}
		now, err := texcheck.Sha256Dir(filepath.Join(projectRoot, t.Sandbox, "out"))
		if err != nil {
			now = map[string]string{}
		}
		d := ArtifactDrift{TaskID: t.ID}
		for rel, h := range snap {
			cur, ok := now[rel]
			if !ok {
				d.Missing = append(d.Missing, rel)
			} else if cur != h {
				d.Mismatched = append(d.Mismatched, rel)
			}
		}
		for rel := range now {
			if _, ok := snap[rel]; !ok {
				d.Extra = append(d.Extra, rel)
			}
		}
		if len(d.Mismatched) > 0 || len(d.Missing) > 0 {
			out = append(out, d)
		}
	}
	return out
}

// DriftSummary 把漂移报告压缩成一行人类可读结论（status 命令/assemble 前调用）。
func DriftSummary(drifts []ArtifactDrift) (clean bool, lines []string) {
	lines = []string{}
	for _, d := range drifts {
		switch {
		case d.NoSnapshot:
			lines = append(lines, d.TaskID+": 无快照（无法复核）")
		case len(d.Mismatched) > 0 || len(d.Missing) > 0:
			lines = append(lines, d.TaskID+": 漂移——改/替换 "+itoa(len(d.Mismatched))+" 项，缺失 "+itoa(len(d.Missing))+" 项")
		}
	}
	return len(lines) == 0, lines
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [8]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

var _ = os.Getenv // 占位：后续接入修复命令时复用
