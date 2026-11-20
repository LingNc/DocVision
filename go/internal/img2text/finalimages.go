package img2text

// T334：交付的 md 里凡是有 `[url](images/<书>/<文件>)` 的原图，都复制到
// `finally/` 下同一条相对路径，让交付目录自成一套（md + images 一起拷走就
// 能用），而不是只在 `paths.images_dir` 里留一份、离开流水线就断链。
//
// 只认交付 md 里**已经出现**的 url 行：文本图/表格图那种直接嵌入的没有这
// 一行，也就不会把用不上的原图塞进 finally。

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mineru-tools/internal/config"
)

// deliveredURLRe 匹配交付 md 里的原图行（`[url](images/<书>/<文件>)`），
// 容忍尖括号包裹与行尾空白。
var deliveredURLRe = regexp.MustCompile(`(?m)^\[url\]\(\s*<?([^()\s>]+)>?\s*\)`)

// deliveredImageTargets returns the unique image targets referenced by url
// lines, in first-appearance order.
func deliveredImageTargets(content string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range deliveredURLRe.FindAllStringSubmatch(content, -1) {
		t := strings.TrimSpace(m[1])
		if t == "" || seen[t] {
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
	}
	return copied, nil
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
