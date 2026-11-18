package split

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// partFile 是一份还没改成 `_partN.pdf` 的分片：落在临时目录里、页数已知。
type partFile struct {
	path  string
	pages int
}

// spanNameRe 从 pdfcpu SplitFile 的产物名里取页段：
// `{base}_{from}-{thru}.pdf`（单页是 `{base}_{n}.pdf`）。base 本身可能带
// 数字与连字符，所以只从尾部认。
var spanNameRe = regexp.MustCompile(`_(\d+)(?:-(\d+))?\.pdf$`)

// pdfTool 是"取一段页"的实际执行者（T328）。一万多页、两百多兆的册子，
// pdfcpu 的 TrimFile 慢到不可用：实测对同一份源文件取一页要跑几十分钟，
// 而 mutool 取一段 200 页只要半秒。所以能外挂 mutool 就外挂，装不上再退回
// pdfcpu——退回时也不用"逐片 Trim 源文件"（旧实现每个分片还要二分探测
// 体积，等于对源文件反复全量解析），而是一次 SplitFile 把整本切开。
type pdfTool struct {
	name string
}

// pickPDFTool 选后端：mutool（mupdf-tools）优先，否则 pdfcpu。
//
// DOCVISION_PDF_SPLIT_TOOL=mutool|pdfcpu 可以强制指定，便于在两种后端上各跑
// 一遍（回归与现场排查用；不写就是上面这套自动选择）。
func pickPDFTool() pdfTool {
	switch os.Getenv("DOCVISION_PDF_SPLIT_TOOL") {
	case "mutool":
		return pdfTool{name: "mutool"}
	case "pdfcpu":
		return pdfTool{name: "pdfcpu"}
	}
	if _, err := exec.LookPath("mutool"); err == nil {
		return pdfTool{name: "mutool"}
	}
	return pdfTool{name: "pdfcpu"}
}

// writeRange 把 src 的 [from,to]（1-based，含两端）页写到 out。
func (t pdfTool) writeRange(src, out string, from, to int) error {
	rng := strconv.Itoa(from)
	if to > from {
		rng = fmt.Sprintf("%d-%d", from, to)
	}
	if t.name == "mutool" {
		cmd := exec.Command("mutool", "merge", "-o", out, src, rng)
		if b, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mutool merge %s 失败: %v（%s）", rng, err, strings.TrimSpace(string(b)))
		}
		return nil
	}
	if err := api.TrimFile(src, out, []string{rng}, pdfConf); err != nil {
		return fmt.Errorf("trim %s: %w", rng, err)
	}
	return nil
}

// splitIntoSpans 按 span 页一份把整本切成若干片，写到 dir，按页序返回。
//
// mutool：每片一次调用（各自读一遍源文件的 xref，但只取这一段页，百页级
// 半秒量级）；pdfcpu：一次 SplitFile 把整本切开，源文件只解析一遍。
func (t pdfTool) splitIntoSpans(src, dir string, span, totalPages int) ([]partFile, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if t.name != "mutool" {
		if err := api.SplitFile(src, dir, span, pdfConf); err != nil {
			return nil, err
		}
		return collectSpans(dir)
	}
	var out []partFile
	n := 0
	for start := 1; start <= totalPages; start += span {
		end := start + span - 1
		if end > totalPages {
			end = totalPages
		}
		n++
		p := filepath.Join(dir, fmt.Sprintf("span-%05d.pdf", n))
		if err := t.writeRange(src, p, start, end); err != nil {
			return nil, err
		}
		out = append(out, partFile{path: p, pages: end - start + 1})
	}
	return out, nil
}

// collectSpans 收齐 dir 里的分片，按起始页排序（pdfcpu 的产物名带页段，
// 字典序会把 1001-1200 排到 201-400 前面，所以按数字排）。
func collectSpans(dir string) ([]partFile, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		return nil, err
	}
	type span struct {
		p    partFile
		from int
	}
	spans := make([]span, 0, len(names))
	for _, n := range names {
		m := spanNameRe.FindStringSubmatch(n)
		if m == nil {
			continue
		}
		from, _ := strconv.Atoi(m[1])
		thru := from
		if m[2] != "" {
			thru, _ = strconv.Atoi(m[2])
		}
		pages := thru - from + 1
		if pages <= 0 {
			continue
		}
		spans = append(spans, span{p: partFile{path: n, pages: pages}, from: from})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	out := make([]partFile, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.p)
	}
	return out, nil
}

// refineParts 把超过体积上限的分片就地再对半切（在**分片自己**身上切，
// 不再回头碰源文件），直到每片都装得下或只剩一页。
func (t pdfTool) refineParts(parts []partFile, scratch string, maxSizeMB float64) ([]partFile, error) {
	if maxSizeMB <= 0 {
		return parts, nil
	}
	out := make([]partFile, 0, len(parts))
	for _, p := range parts {
		more, err := t.refineOne(p, scratch, maxSizeMB)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func (t pdfTool) refineOne(p partFile, scratch string, maxSizeMB float64) ([]partFile, error) {
	st, err := os.Stat(p.path)
	if err != nil {
		return nil, err
	}
	if float64(st.Size())/(1<<20) <= maxSizeMB || p.pages <= 1 {
		return []partFile{p}, nil
	}
	mid := p.pages / 2
	sub := filepath.Join(scratch, "refine-"+strings.TrimSuffix(filepath.Base(p.path), ".pdf"))
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return nil, err
	}
	var out []partFile
	for _, seg := range []struct{ a, b, n int }{{1, mid, mid}, {mid + 1, p.pages, p.pages - mid}} {
		f := filepath.Join(sub, fmt.Sprintf("%d-%d.pdf", seg.a, seg.b))
		if err := t.writeRange(p.path, f, seg.a, seg.b); err != nil {
			return nil, err
		}
		more, err := t.refineOne(partFile{path: f, pages: seg.n}, sub, maxSizeMB)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

// sizeLimitText 给日志用：没设体积上限时说明"不限大小"。
func sizeLimitText(maxSizeMB float64) string {
	if maxSizeMB <= 0 {
		return "，不限大小"
	}
	return fmt.Sprintf("，每份 ≤ %.0fMB", maxSizeMB)
}
