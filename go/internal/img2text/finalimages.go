package img2text

// T334：交付的 md 里凡是有 `[url](images/<书>/<文件>)` 的原图，都复制到
// `finally/` 下同一条相对路径，让交付目录自成一套（md + images 一起拷走就
// 能用），而不是只在 `paths.images_dir` 里留一份、离开流水线就断链。
//
// 只认交付 md 里**已经出现**的 url 行：文本图/表格图那种直接嵌入的没有这
// 一行，也就不会把用不上的原图塞进 finally。

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/internal/imagemeta"
)

// deliveredImageTargets returns the unique image targets referenced by url
// lines (`[url](images/<书>/<文件>)`), in first-appearance order. Parsed by
// line with linkTargetOf so目录名里的空格与半角括号（如 "xx(下)"）不会把
// 路径截断——旧的正则 `[^()\s>]+` 会在第一个空格或 "(" 处停下，把文件
// 名和扩展名一起丢掉。
func deliveredImageTargets(content string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "[url](") {
			continue
		}
		t, ok := linkTargetOf(line)
		if !ok || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// resolveDeliveredImage finds the source file for a target reference. The
// reference is written as `images/<书>/<文件>`; the file itself lives under
// paths.images_dir (organize 收集的就是那儿)，兼容 output_dir 下直接放。
func resolveDeliveredImage(cfg *config.Config, target string) string {
	cands := []string{filepath.Join(cfg.Paths.OutputDir, target)}
	if rel := strings.TrimPrefix(target, "images/"); rel != target {
		cands = append([]string{filepath.Join(cfg.Paths.ImagesDir, rel)}, cands...)
	}
	cands = append(cands, filepath.Join(cfg.Paths.ImagesDir, target))
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			return c
		}
	}
	return ""
}

// copyDeliveredImages copies every referenced original into
// `<finally>/<target>`, preserving the relative path. Already-present files of
// the same size are left alone (reruns stay cheap), and references that cannot
// be resolved are skipped silently — the md link is still what the reader sees.
func copyDeliveredImages(cfg *config.Config, content string) (int, error) {
	targets := deliveredImageTargets(content)
	if len(targets) == 0 {
		return 0, nil
	}
	copied := 0
	// T335：按目录收集原始量，复制完统一写 sizes.csv。
	raws := map[string]map[string]imagemeta.Raw{}
	for _, t := range targets {
		if filepath.IsAbs(t) || strings.Contains(t, "..") {
			continue // 只接受 md 里的相对路径，别的形态不动
		}
		dst := filepath.Join(cfg.Paths.FinallyDir, t)
		if st, err := os.Stat(dst); err == nil {
			if src := resolveDeliveredImage(cfg, t); src != "" {
				if sst, err := os.Stat(src); err == nil && sst.Size() == st.Size() {
					continue
				}
			} else {
				continue
			}
		}
		src := resolveDeliveredImage(cfg, t)
		if src == "" {
			continue
		}
		if err := copyImageFile(dst, src); err != nil {
			return copied, fmt.Errorf("复制 %s: %w", t, err)
		}
		copied++
		if r, ok := imagemeta.LookupImage(filepath.Base(t)); ok {
			dir := filepath.Dir(dst)
			if raws[dir] == nil {
				raws[dir] = map[string]imagemeta.Raw{}
			}
			raws[dir][filepath.Base(t)] = r
		}
	}
	for dir, m := range raws {
		if err := writeSizeCSV(dir, m); err != nil {
			return copied, err
		}
	}
	return copied, nil
}

// writeSizeCSV 写一份 <目录>/sizes.csv（T335/T336）：每张图一行
// `file,width_mm,height_mm,page_w_mm`（只给 mm；pt 需要时 ×25.4/72 反推；
// page_w_mm 是该图所在页的页宽，用来算“占页宽比例”——这个比值与页框标错无关），外带一行注释给出本册的换算比例
// `scale_px_per_pt`（px 每 pt）。比例按本册全部图的中位数定标——实测同一
// 册内几乎恒定（如 1.65–1.66），跨册不同（1.20 / 1.41 / 1.65），所以必须
// 按册算不能用全局常数。图片宽高比原样保留：两者同除一个比例。
func writeSizeCSV(dir string, raws map[string]imagemeta.Raw) error {
	type rec struct {
		file     string
		pxW, pxH int
		bboxW    float64
		pageW    float64 // pt
	}
	recs := make([]rec, 0, len(raws))
	var ratios []float64
	for n, r := range raws {
		pxW, pxH := pixelSize(filepath.Join(dir, n))
		if pxW <= 0 || pxH <= 0 {
			continue
		}
		recs = append(recs, rec{file: n, pxW: pxW, pxH: pxH, bboxW: r.BBox[2] - r.BBox[0], pageW: r.PageW})
		if r.BBox[2] > r.BBox[0] {
			ratios = append(ratios, float64(pxW)/(r.BBox[2]-r.BBox[0]))
		}
	}
	if len(recs) == 0 {
		return nil
	}
	scale := medianFloat(ratios)
	if scale <= 0 {
		scale = 1
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].file < recs[j].file })
	var b strings.Builder
	fmt.Fprintf(&b, "# scale_px_per_pt=%.4f\n", scale)
	b.WriteString("file,width_mm,height_mm,page_w_mm\n")
	for _, r := range recs {
		fmt.Fprintf(&b, "%s,%.2f,%.2f,%.2f\n", r.file,
			float64(r.pxW)/scale*mmPerPt, float64(r.pxH)/scale*mmPerPt, r.pageW*mmPerPt)
	}
	return os.WriteFile(filepath.Join(dir, "sizes.csv"), []byte(b.String()), 0o644)
}

// mmPerPt 是 1pt 对应的毫米数。
const mmPerPt = 25.4 / 72.0

// medianFloat 返回中位数（空切片返回 0）。
func medianFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	return c[len(c)/2]
}

// pixelSize 读位图尺寸（不解码像素，只读头）。
func pixelSize(p string) (w, h int) {
	f, err := os.Open(p)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// copyImageFile copies src to dst, creating parent directories.
func copyImageFile(dst, src string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
