package prep

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// splitfast.go：SK-P31——旧线 T328 切分提速移植。
// 旧法（Split，本包另存）：每个分片对源文件做十余次 TrimFile 体积探测
// （每次全量解析），一万两千页的册子半小时连一个分片都写不出来。
// 新法（本文件）：
//   - 整本一次读入按页数切开——mutool merge 优先（机器上有就走），
//     否则 pdfcpu 的 SplitFile（Go 库一次全解析，写出全部分片）；
//   - 超体积分片在**它自己**的页上对半再切（不再回头探测源文件）；
//   - 后端可用环境变量 DOCVISION_PDF_SPLIT_TOOL=mutool|pdfcpu 强制。

// SplitFastConfig 是提速切分的参数。
type SplitFastConfig struct {
	PagesPerPart  int   // 每份页数
	MaxSizeBytes int64 // 分片体积上限（超过就在自己身上二分；0=不限）
}

type pdfToolKind int

const (
	toolPDFCPU pdfToolKind = iota
	toolMutool
)

// piece 是一个待落地的分片（页区间基于它自己的文件）。
type piece struct {
	path  string
	pages int
}

// pickPDFTool 选后端：环境变量强制 > PATH 有 mutool > pdfcpu。
func pickPDFTool() pdfToolKind {
	switch os.Getenv("DOCVISION_PDF_SPLIT_TOOL") {
	case "pdfcpu":
		return toolPDFCPU
	case "mutool":
		if _, err := exec.LookPath("mutool"); err == nil {
			return toolMutool
		}
	}
	if _, err := exec.LookPath("mutool"); err == nil {
		return toolMutool
	}
	return toolPDFCPU
}

// pageCount 取页数（pdfinfo，poppler）。
func pageCount(pdfPath string, run Runner) (int, error) {
	out, err := run("pdfinfo", pdfPath)
	if err != nil {
		return 0, fmt.Errorf("pdfinfo 失败: %v %.200s", err, out)
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(ln, "Pages:") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(ln, "Pages:")))
			if n > 0 {
				return n, nil
			}
		}
	}
	return 0, fmt.Errorf("pdfinfo 未给出页数")
}

// SplitFast 整本一次读入切分（T328 移植）：按 PagesPerPart 切 span，
// 超 MaxSizeMB 的分片在自己身上对半细化。返回分片名（<stem>_partN.pdf 排序）。
func SplitFast(pdfPath, outDir string, cfg SplitFastConfig, run Runner) ([]string, error) {
	if run == nil {
		run = DefaultRunner
	}
	if cfg.PagesPerPart <= 0 {
		cfg.PagesPerPart = 100
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, fmt.Errorf("PDF 不存在: %s", pdfPath)
	}
	stem := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	total, err := pageCount(pdfPath, run)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp(outDir, ".split-tmp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)

	tool := pickPDFTool()
	span := cfg.PagesPerPart
	if span > total {
		span = total
	}
	// 一次切分：整本 → span 页一份的 pieces（都在 scratch 里）
	pieces, err := splitIntoSpans(pdfPath, scratch, stem, span, total, tool, run)
	if err != nil {
		return nil, fmt.Errorf("一次切分失败: %w", err)
	}
	// 按体积细化：超体积分片在自己身上对半
	if cfg.MaxSizeBytes > 0 {
		pieces, err = refineParts(pieces, cfg.MaxSizeBytes, scratch, tool, run)
		if err != nil {
			return nil, fmt.Errorf("按体积细分失败: %w", err)
		}
	}
	// 落盘：pieces 顺序即 partN（细化会插入子片，保持整体页序）
	var names []string
	for i, p := range pieces {
		name := fmt.Sprintf("%s_part%d.pdf", stem, i+1)
		dst := filepath.Join(outDir, name)
		if _, err := os.Stat(dst); err == nil {
			names = append(names, name)
			continue // 幂等：已有分片跳过
		}
		if err := os.Rename(p.path, dst); err != nil {
			data, rerr := os.ReadFile(p.path)
			if rerr != nil {
				return names, fmt.Errorf("读取 %s 失败: %w", p.path, rerr)
			}
			if werr := os.WriteFile(dst, data, 0o644); werr != nil {
				return names, fmt.Errorf("写入 %s 失败: %w", dst, werr)
			}
		}
		names = append(names, name)
	}
	return names, nil
}

// splitIntoSpans 一次把整本切成 span 页一份（mutool 按区间循环调 merge，
// pdfcpu 一次 SplitFile 全出）。
func splitIntoSpans(pdfPath, scratch, stem string, span, total int, tool pdfToolKind, run Runner) ([]piece, error) {
	var pieces []piece
	if tool == toolMutool {
		for start := 1; start <= total; start += span {
			end := start + span - 1
			if end > total {
				end = total
			}
			p := filepath.Join(scratch, fmt.Sprintf("%s-span-%d.pdf", stem, len(pieces)))
			if out, err := run("mutool", "merge", "-o", p, pdfPath, fmt.Sprintf("%d-%d", start, end)); err != nil {
				return nil, fmt.Errorf("mutool merge %d-%d 失败: %v %.200s", start, end, err, out)
			}
			pieces = append(pieces, piece{path: p, pages: end - start + 1})
		}
		return pieces, nil
	}
	// pdfcpu：一次读入全出（SplitFile 写出 <base>_1.pdf, _2.pdf …）
	if err := pdfcpuapi.SplitFile(pdfPath, scratch, span, model.NewDefaultConfiguration()); err != nil {
		return nil, fmt.Errorf("pdfcpu SplitFile 失败: %w", err)
	}
	matches, _ := filepath.Glob(filepath.Join(scratch, "*.pdf"))
	// 排除 mutool 分支产物（同目录时按名字过滤 span- 前缀）
	var files []string
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), "-span-") {
			files = append(files, m)
		}
	}
	sort.Strings(files)
	for _, f := range files {
		pieces = append(pieces, piece{path: f, pages: span})
	}
	// 最后一份页数修正
	if len(pieces) > 0 {
		rem := total - span*(len(pieces)-1)
		if rem > 0 && rem < span {
			pieces[len(pieces)-1].pages = rem
		}
	}
	return pieces, nil
}

// refineParts 超体积分片在自己身上对半（T328 核心：不回头探测源文件）。
func refineParts(pieces []piece, maxBytes int64, scratch string, tool pdfToolKind, run Runner) ([]piece, error) {
	limit := maxBytes
	out := make([]piece, 0, len(pieces))
	for _, p := range pieces {
		cur := []piece{p}
		for i := 0; i < len(cur); i++ {
			info, err := os.Stat(cur[i].path)
			if err != nil {
				return out, err
			}
			if info.Size() <= limit || cur[i].pages <= 1 {
				continue
			}
			// 对半：前半 pages/2，后半余下
			half := cur[i].pages / 2
			left, err := cutPages(cur[i].path, scratch, 1, half, tool, run)
			if err != nil {
				return out, err
			}
			right, err := cutPages(cur[i].path, scratch, half+1, cur[i].pages, tool, run)
			if err != nil {
				return out, err
			}
			cur = append(cur[:i], append([]piece{left, right}, cur[i+1:]...)...)
			i-- // 重查左半
		}
		out = append(out, cur...)
	}
	return out, nil
}

// cutPages 从 piece 文件里切出页区间 [from,to]（1-based，基于该文件自身）。
func cutPages(src, scratch string, from, to int, tool pdfToolKind, run Runner) (piece, error) {
	dst := filepath.Join(scratch, fmt.Sprintf("refined-%d.pdf", timeNowUnix()))
	if tool == toolMutool {
		if out, err := run("mutool", "merge", "-o", dst, src, fmt.Sprintf("%d-%d", from, to)); err != nil {
			return piece{}, fmt.Errorf("mutool 细分失败: %v %.200s", err, out)
		}
		return piece{path: dst, pages: to - from + 1}, nil
	}
	// pdfcpu：用 TrimFile 切出区间（会解析该分片一次——只在超体积分片上发生，
	// 数量极少，符合 T328 "不回头探测源文件"的权衡）。
	data, err := os.ReadFile(src)
	if err != nil {
		return piece{}, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return piece{}, err
	}
	if err := pdfcpuapi.TrimFile(dst, "", []string{fmt.Sprintf("%d-%d", from, to)}, model.NewDefaultConfiguration()); err != nil {
		return piece{}, fmt.Errorf("pdfcpu TrimFile 细分失败: %w", err)
	}
	return piece{path: dst, pages: to - from + 1}, nil
}

// timeNowUnix 细片命名用（单调不撞名）。
func timeNowUnix() int64 { return time.Now().UnixNano() }
