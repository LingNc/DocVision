package mdimg

import (
	"context"
	"encoding/json"
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

// imageRefRe 匹配 md 图片引用（旧线 analyze 同款正则），捕获 alt 与路径（对齐老线 altOfRef）。
var imageRefRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// Replacement 是一张图的替换决策（collect 阶段的中间表示）。
type Replacement struct {
	ImgRef  string // md 里的图片路径（images/xxx.jpg）
	Seq     int    // doc_index 条目 seq
	Text    string // 文字化内容（空=非文字化）
	Mermaid string // mermaid 源码（空=非 mermaid）
	ImgType string // SK-I22：[IMG_TYPE:] 类型（决定嵌入形态）
	Keep    bool   // 保留原图（内部容错，SK-I21 后仅限水印）
	Drop    bool   // 去水印（SK-I3）：整行删除不留占位（verdict.json "watermark":true）
}

// devTextLikeTypes：老线 embedBlockForRef 的纯文本直嵌类（text/latex/math/formula/table/code），
// 其余类型（illustration/graph 等）走 [Image]( body · P.N ) 文字锚（SK-I22 对齐 dev 契约）。
var devTextLikeTypes = map[string]bool{
	"text": true, "latex": true, "math": true,
	"formula": true, "table": true, "code": true,
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
		// verdict.json：水印（SK-I3 整行删除）+ 类型（SK-I22 决定嵌入形态）
		if vb, err := os.ReadFile(filepath.Join(sandbox, "verdict.json")); err == nil {
			var v struct {
				Result    string `json:"result"`
				ImgType   string `json:"img_type"`
				Watermark bool   `json:"watermark"`
			}
			if json.Unmarshal(vb, &v) == nil {
				r.ImgType = v.ImgType
				if v.Watermark {
					r.Drop = true
				}
			}
		}
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
	out, _ := AssembleWithShifts(md, repls)
	return out
}

// pageCommentRe 匹配 organize 的页界注释 <!-- pN -->（SK-I22：页码地面真值，输出原位转 [P. N]）。
var pageCommentRe = regexp.MustCompile(`^\s*<!--\s*p(\d+)\s*-->\s*$`)

// AssembleWithShifts 按决策重建 md（SK-I22 修订，对齐老线 dev 契约）：
//   - 页码单一系统：<!-- pN --> 行原位转 [P. N]（格式我们的，编号=organize 全页枚举，弃模糊对齐）
//   - 嵌入形态对齐老线 embedBlockForRef：text 类→纯 body；mermaid→[Image]( alt · P.N )+代码块；
//     其余→[Image]( body · P.N )（P.N=该行最近页，page<=0 不后缀）
//   - 去水印行整行删除。
func AssembleWithShifts(md string, repls []Replacement) (string, []embedShift) {
	byRef := map[string][]*Replacement{}
	for k := range repls {
		byRef[repls[k].ImgRef] = append(byRef[repls[k].ImgRef], &repls[k])
	}
	var shifts []embedShift
	lines := strings.Split(md, "\n")
	var sb strings.Builder
	curPage := 0
	for _, ln := range lines {
		if pm := pageCommentRe.FindStringSubmatch(ln); pm != nil {
			var pg int
			fmt.Sscanf(pm[1], "%d", &pg)
			curPage = pg
			sb.WriteString(fmt.Sprintf("[P. %d]\n", pg))
			continue
		}
		m := imageRefRe.FindStringSubmatch(ln)
		if m == nil || len(byRef[m[2]]) == 0 {
			sb.WriteString(ln + "\n")
			continue
		}
		r := byRef[m[2]][0]
		byRef[m[2]] = byRef[m[2]][1:]
		if r.Drop {
			shifts = append(shifts, embedShift{start: 0, delta: -(len(ln) + 1)})
			continue // 去水印：整行删除
		}
		var repl string
		switch {
		case r.Mermaid != "":
			repl = devImageAnchor(m[1], curPage) + "\n\n" + devMermaidFence(r.Mermaid) + "\n\n"
		case r.Text != "" && devTextLikeTypes[r.ImgType]:
			repl = "\n\n" + r.Text + "\n\n"
		case r.Text != "":
			repl = "\n\n" + devImageAnchor(r.Text, curPage) + "\n\n"
		default: // 内部容错（SK-I21 后 keep 仅限水印且走 Drop，不会到这）
			repl = ln + "\n"
		}
		sb.WriteString(repl)
	}
	return sb.String(), shifts
}


// devImageAnchor 老线 imageAnchorText：label · P.N（page<=0 不后缀）。
func devImageAnchor(label string, page int) string {
	label = strings.TrimSpace(label)
	if page > 0 {
		return fmt.Sprintf("[Image]( %s · P.%d )", label, page)
	}
	return fmt.Sprintf("[Image]( %s )", label)
}

// devMermaidFence 老线 withMermaidFence/ensureClosedFence：无围栏补 ```mermaid，有则不重复。
func devMermaidFence(body string) string {
	body = strings.TrimSpace(body)
	if strings.Contains(body, "```") {
		return body
	}
	return "```mermaid\n" + body + "\n```"
}

func LineStarts(md string) ([]string, []int) {
	lines := strings.Split(md, "\n")
	starts := make([]int, len(lines))
	offset := 0
	for i, ln := range lines {
		starts[i] = offset
		offset += len(ln) + 1
	}
	return lines, starts
}

// imageRefRef 取行内第一个图片引用的路径（无则空串）。
func imageRefRef(ln string) string {
	m := imageRefRe.FindStringSubmatch(ln)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
