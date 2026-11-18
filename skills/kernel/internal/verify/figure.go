package verify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/config"
	"github.com/LingNc/DocVision/skills/kernel/internal/figcheck"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// figure.go：T2 作图验收的数值判据接线——软墨迹对齐（figcheck）跑在
// "agent 交出的 figure.pdf" 与 "原图" 之间，低于按图阈值打回；accept_min_submits
// 交够 N 次后 below 阈值放行并记 note（旧线 accept_as_is 判据）。

// FigureSpec 里可带的验收键（DrawSpec 之上由 claim/生成器补全）：
//   figure_ref    string  原图相对项目根路径（必填才触发本判据）
//   fig_mm_w/h    float64 原图印刷尺寸 mm（缺省按像素比退化宽高比判据）
//   min_overlap   float64 墨迹重合度下限（缺省 0.50）
//   max_scale_dev float64 尺寸偏差上限（缺省 0.10；T312 mm 口径抓"看着对上了其实窄了一截"）
//   accept_min_submits int 交够 N 次才允许 below 阈值放行（缺省 3）

// checkFigure 是 Run 的第 7 步（verdict_schema 之后）：figure_ref 存在才跑。
func (c *Checker) checkFigure(t *task.Task, sandboxAbs string) ([]string, map[string]string) {
	notes := map[string]string{}
	refRel, _ := t.Accept["figure_ref"].(string)
	if refRel == "" {
		return nil, notes
	}
	root := c.ProjectRoot
	refPath := filepath.Join(root, filepath.FromSlash(refRel))
	if _, err := os.Stat(refPath); err != nil {
		return []string{"figure_ref 原图不存在: " + refRel}, notes
	}
	// 渲染 agent 交的 figure.pdf（pdftoppm，150dpi 固定）；缺工具跳过不卡死。
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		notes["figure_skip"] = "pdftoppm 不可用，跳过作图数值判据"
		return nil, notes
	}
	pdfPath := filepath.Join(sandboxAbs, "out", "figure.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		return []string{"缺少产物: out/figure.pdf（作图任务必须以 figure.pdf 提交）"}, notes
	}
	kvDir := filepath.Join(sandboxAbs, ".kv")
	if err := os.MkdirAll(kvDir, 0o755); err != nil {
		return []string{"内核临时目录创建失败: " + err.Error()}, notes
	}
	defer os.RemoveAll(kvDir)
	cmd := exec.Command("pdftoppm", "-png", "-r", "150", "-singlefile", pdfPath, filepath.Join(kvDir, "figure"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return []string{fmt.Sprintf("figure.pdf 渲染失败: %v %.300s", err, out)}, notes
	}

	refImg, err := figcheck.DecodeImage(refPath)
	if err != nil {
		return []string{"原图不可读: " + refRel + ": " + err.Error()}, notes
	}
	candImg, err := figcheck.DecodeImage(filepath.Join(kvDir, "figure.png"))
	if err != nil {
		return []string{"渲染图不可读: " + err.Error()}, notes
	}
	mm := figcheck.CanvasMM{}
	if w, ok := t.Accept["fig_mm_w"].(float64); ok {
		mm.RefW = w
	}
	if h, ok := t.Accept["fig_mm_h"].(float64); ok {
		mm.RefH = h
	}
	// 候选画布物理尺寸：从 PDF 页尺寸换算（pdfinfo，pt→mm；拿不到则 mm 口径只量原图→退化 aspect）。
	if _, err := exec.LookPath("pdfinfo"); err == nil {
		if out, err := exec.Command("pdfinfo", pdfPath).Output(); err == nil {
			if w, h := pdfPageSizeMM(string(out)); w > 0 && h > 0 {
				mm.CandW, mm.CandH = w, h
			}
		}
	}
	r, _, err := figcheck.CompareImagesMM(refImg, candImg, mm)
	if err != nil {
		return []string{"软墨迹对齐失败: " + err.Error()}, notes
	}
	notes["figure_similarity"] = fmt.Sprintf("%.3f", r.Similarity)
	notes["figure_runs"] = figcheck.FormatRuns(r.Runs)
	notes["figure_size"] = figcheck.SizeSentence(r)

	// 阈值三级来源：验收 spec（最具体） > 用户配置（skills_config.json） > 内置默认。
	// agent 无权改配置——config 命令不进 MCP 工具面（用户纪律，SK-I2）。
	minOverlap := 0.17
	maxDev := 0.10
	acceptAfter := 3
	if cfg, err := config.Load(root); err == nil {
		if cfg.Figure.MinOverlap > 0 {
			minOverlap = cfg.Figure.MinOverlap
		}
		if cfg.Figure.MaxScaleDev > 0 {
			maxDev = cfg.Figure.MaxScaleDev
		}
		if cfg.Figure.AcceptMinSubmits > 0 {
			acceptAfter = cfg.Figure.AcceptMinSubmits
		}
	}
	if v, ok := t.Accept["min_overlap"].(float64); ok && v > 0 {
		minOverlap = v
	}
	if v, ok := t.Accept["max_scale_dev"].(float64); ok && v > 0 {
		maxDev = v
	}
	if v, ok := t.Accept["accept_min_submits"].(float64); ok && int(v) > 0 {
		acceptAfter = int(v)
	}

	var errs []string
	if r.Similarity < minOverlap {
		errs = append(errs, fmt.Sprintf("墨迹重合度 %.1f%% 低于 %.0f%%（%s）",
			r.Similarity*100, minOverlap*100, figcheck.FormatRuns(r.Runs)))
	}
	if r.SizeUnit == "mm" && r.ScaleDev > maxDev {
		errs = append(errs, fmt.Sprintf("尺寸偏差 %.1f%% 超过 %.0f%%（%s）",
			r.ScaleDev*100, maxDev*100, figcheck.SizeSentence(r)))
	}
	if len(errs) == 0 {
		return nil, notes
	}
	// accept_as_is 判据：交够 N 次后 below 阈值放行（note 留证据，不静默）。
	if t.Attempts+1 >= acceptAfter {
		notes["figure_accept_as_is"] = fmt.Sprintf("第 %d 次提交，按 accept_min_submits=%d 放行（判据仍不达标，证据见上）", t.Attempts+1, acceptAfter)
		return nil, notes
	}
	return errs, notes
}

// pdfPageSizeMM 从 pdfinfo 输出解析首页尺寸（pt → mm）。
func pdfPageSizeMM(info string) (float64, float64) {
	for _, ln := range strings.Split(info, "\n") {
		if !strings.HasPrefix(ln, "Page size:") {
			continue
		}
		// 形如 "Page size:       595 x 842 pts (A4)"
		f := strings.Fields(ln)
		for i := 0; i+2 < len(f); i++ {
			if f[i] == "x" {
				w, err1 := strconv.ParseFloat(f[i-1], 64)
				h, err2 := strconv.ParseFloat(f[i+1], 64)
				if err1 == nil && err2 == nil {
					return w * 25.4 / 72, h * 25.4 / 72
				}
			}
		}
	}
	return 0, 0
}
