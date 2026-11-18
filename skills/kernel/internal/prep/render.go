package prep

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
)

// render.go：原书页渲染/裁剪（view_pdf 等价物）。
// 数据面纪律：命令只回路径+元数据，图由 agent 用自己的看图工具打开。

// LoadPageIndex 读页表（skills_pages.json）。
func LoadPageIndex(path string) (*docindex.PageIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return docindex.LoadPageIndex(data)
}

// PageRender 是一张渲染结果的路径与元数据。
type PageRender struct {
	PDF    string  `json:"pdf"`    // 原书 PDF 路径
	Page   int     `json:"page"`   // PDF 内页码（1-based）
	Global int     `json:"global"` // 全书页码
	Image  string  `json:"image"`  // 渲染出的 PNG 路径
	MMW    float64 `json:"mm_w"`   // 渲染图实际印刷宽（按 150dpi 换算）
	MMH    float64 `json:"mm_h"`
}

// RenderPage 把全书第 global 页渲染成 PNG（可选 bbox 裁剪）。
// outDir 必须存在；150dpi。bbox 为 MinerU 页内坐标（点，原点在左上）。
func RenderPage(pages *docindex.PageIndex, global int, bbox *[4]float64, outDir string, run Runner) (*PageRender, error) {
	if run == nil {
		run = DefaultRunner
	}
	src, pdfPage, err := findPage(pages, global)
	if err != nil {
		return nil, err
	}
	base := fmt.Sprintf("page-%d", global)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	outPrefix := filepath.Join(outDir, base)
	if _, err := run("pdftoppm", "-png", "-r", "150", "-f", itoa(pdfPage), "-l", itoa(pdfPage), "-singlefile", src, outPrefix); err != nil {
		return nil, fmt.Errorf("pdftoppm 页 %d 失败: %v", global, err)
	}
	pngPath := outPrefix + ".png"
	res := &PageRender{PDF: src, Page: pdfPage, Global: global, Image: pngPath}
	if bbox != nil {
		if err := cropPNG(pngPath, *bbox); err != nil {
			return res, fmt.Errorf("裁剪失败: %w", err)
		}
	}
	// 150dpi → 1px = 25.4/150 mm；A4 整页 1240x1754px ≈ 210x297mm。
	if info, err := os.Stat(pngPath); err == nil {
		_ = info
	}
	res.MMW, res.MMH = pngSizeMM(pngPath, 150)
	return res, nil
}

// findPage 把全书页码映射到（原书 PDF，PDF 内页码）。
func findPage(pages *docindex.PageIndex, global int) (string, int, error) {
	for _, s := range pages.Srcs {
		if global >= s.First && global < s.First+s.Count {
			return s.PDF, global - s.First + 1, nil
		}
	}
	return "", 0, fmt.Errorf("全书页码 %d 超出页表范围（共 %d 页）", global, pages.Total)
}

// cropPNG 按 bbox（点，左上原点）裁剪 PNG 原位覆盖。
func cropPNG(path string, bbox [4]float64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	const ptPerInch = 72.0
	dpi := 150.0
	scale := dpi / ptPerInch
	b := img.Bounds()
	// 单位 sanity：页面 595pt 却给 99999pt 的 bbox = agent 传错坐标系，报错
	// 比静默裁整页好（错一点点照常 clamp，错一个量级必须显形）。
	pageWPT := float64(b.Dx()) / scale
	pageHPT := float64(b.Dy()) / scale
	if bbox[0] >= pageWPT || bbox[1] >= pageHPT || bbox[2] > pageWPT*2 || bbox[3] > pageHPT*2 {
		return fmt.Errorf("bbox 疑似坐标系错误: %v（页面约 %.0fx%.0f pt）", bbox, pageWPT, pageHPT)
	}
	x0 := int(bbox[0] * scale)
	y0 := int(bbox[1] * scale)
	x1 := int(bbox[2]*scale + 0.5)
	y1 := int(bbox[3]*scale + 0.5)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > b.Dx() {
		x1 = b.Dx()
	}
	if y1 > b.Dy() {
		y1 = b.Dy()
	}
	if x1 <= x0 || y1 <= y0 {
		return fmt.Errorf("bbox 越界: %v（图 %dx%dpx）", bbox, b.Dx(), b.Dy())
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := 0; y < y1-y0; y++ {
		for x := 0; x < x1-x0; x++ {
			out.Set(x, y, img.At(b.Min.X+x0+x, b.Min.Y+y0+y))
		}
	}
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	return png.Encode(w, out)
}

// pngSizeMM 按 dpi 换算 PNG 的印刷尺寸（mm）。
func pngSizeMM(path string, dpi float64) (float64, float64) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(f)
	_ = f.Close()
	if err != nil {
		return 0, 0
	}
	mm := 25.4 / dpi
	return float64(cfg.Width) * mm, float64(cfg.Height) * mm
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [12]byte{}
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

var _ = strings.Contains // 占位：后续接入 CropHint 自动取 bbox 时复用

var _ = exec.Command // 保留默认 runner 依赖说明
