package mdimg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// assemble.go：档位2 的 md 重组装——逐图任务完成后，按每图的决策替换 md 里的
// 图片引用：
//   - 文字化：out/replacement.md 的非空内容替换图片行（图片行本身删除）；
//   - mermaid：out/figure.mmd 包成 ```mermaid 围栏块替换图片行（kernel 侧
//     ValidateMermaid 校验，失败打回该图任务）；
//   - 保留：out/verdict.json {"result":"keep"} 或无任何产物 → 图片行原样保留。
// 替换按 md 中图片出现顺序与任务的 entry_seq 对应（gen-img2text 按 doc_index
// 序铺任务，md 中图片序即 doc_index 序）。

// imageRefRe 匹配 md 图片引用（旧线 analyze 同款正则）。
var imageRefRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// Replacement 是一张图的替换决策（collect 阶段的中间表示）。
type Replacement struct {
	ImgRef  string // md 里的图片路径（images/xxx.jpg）
	Seq     int    // doc_index 条目 seq
	Text    string // 文字化内容（空=非文字化）
	Mermaid string // mermaid 源码（空=非 mermaid）
	Keep    bool   // 保留原图
}

// LoadReplacements 从 done 的 img2txt_* 任务收集全部决策。
// 校验：mermaid 内容若本机有 mmdc 则逐块校验，失败返回错误（该图任务应回炉）。
func LoadReplacements(q *queue.Queue, projectRoot string) ([]Replacement, error) {
	var out []Replacement
	for _, tk := range q.Tasks {
		if tk.Type != task.TypeImg2Text || tk.Status != task.StatusDone || len(tk.Verdicts) == 0 {
			continue
		}
		last := tk.Verdicts[len(tk.Verdicts)-1]
		if !last.Pass {
			continue
		}
		seqF, _ := tk.Accept["entry_seq"].(float64)
		img, _ := tk.Accept["img"].(string)
		r := Replacement{Seq: int(seqF), ImgRef: img}
		sandbox := filepath.Join(projectRoot, tk.Sandbox, "out")
		if b, err := os.ReadFile(filepath.Join(sandbox, "replacement.md")); err == nil && strings.TrimSpace(string(b)) != "" {
			r.Text = strings.TrimSpace(string(b))
		} else if b, err := os.ReadFile(filepath.Join(sandbox, "figure.mmd")); err == nil && strings.TrimSpace(string(b)) != "" {
			r.Mermaid = strings.TrimSpace(string(b))
		} else {
			r.Keep = true
		}
		// mermaid 校验（有 mmdc 才校；没有则记 note 放行，doctor 会报）
		if r.Mermaid != "" {
			if v := ValidateMermaid(context.Background(), "```mermaid\n"+r.Mermaid+"\n```", "", 0); v.Available && !v.Valid {
				return nil, fmt.Errorf("img2txt_%d 的 mermaid 校验失败（回炉该图）: %s", int(seqF), v.Error)
			}
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// Assemble 按决策重建 md：逐行扫描，图片行按出现顺序替换（顺序=doc_index 序）。
// 决策多于图片行 → 多余项忽略（warning 由调用方报）；图片行多于决策 → 原样保留。
func Assemble(md string, repls []Replacement) string {
	lines := strings.Split(md, "\n")
	ri := 0
	var out []string
	for _, ln := range lines {
		m := imageRefRef(ln)
		if m == "" || ri >= len(repls) {
			out = append(out, ln)
			continue
		}
		r := repls[ri]
		ri++
		switch {
		case r.Text != "":
			out = append(out, r.Text)
		case r.Mermaid != "":
			out = append(out, "```mermaid", r.Mermaid, "```")
		default:
			out = append(out, ln) // keep
		}
	}
	return strings.Join(out, "\n")
}

// imageRefRef 取行内第一个图片引用的路径（无则空串）。
func imageRefRef(ln string) string {
	m := imageRefRe.FindStringSubmatch(ln)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
