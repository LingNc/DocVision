package organize

// T331 页界索引（<subject>.pages.json）：organize 合并 md 时，用 MinerU 的
// layout.json 拿到**逐页块结构**，算出「每一页从合并后 md 的第几行开始」，
// 落盘成一份 sidecar 给 img2text 直接消费。
//
// 设计要点：
//   - md 正文一个字节都不动（不用 HTML 注释打标记），latex 那条线读到的 md
//     与 MinerU 逐字一致，零风险；页界只以行号形式存在 sidecar 里。
//   - 页号是跨部件的全局 1-based 页（部件页数累加），与 doc_search/view_pdf
//     同一套口径。
//   - 对齐只在**这里**做一次，而且是"页内首块 → md 行"的顺序匹配（页序单调、
//     只看首块、窗口有限），img2text 之后不再做任何对齐或反推。
//   - 页内首块对不上（OCR 差异、空页）时该页不落条目，消费侧按"页号连续"
//     补齐，绝不会缺号。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// pageIndexExt is the sidecar suffix written next to the merged markdown.
const pageIndexExt = ".pages.json"

// partText is one part's markdown text plus the MinerU directory it came
// from (needed to find that part's layout.json).
type partText struct {
	dir  string
	text string
}

type pageStartJSON struct {
	Page int `json:"page"`
	Line int `json:"line"`
}

// pageIndexJSON is the sidecar payload. Lines carries the merged md's line
// count so a consumer can tell a stale index from a fresh one.
type pageIndexJSON struct {
	Version int             `json:"version"`
	MD      string          `json:"md"`
	Lines   int             `json:"lines"`
	Pages   []pageStartJSON `json:"pages"`
}

// pageIndexPath is where the sidecar for an md lives.
func pageIndexPath(outputDir, subject string) string {
	return filepath.Join(outputDir, subject+pageIndexExt)
}

// layoutPageSigs reads one part's layout.json and returns, for every page
// in order, the candidate first-line signatures of that page's blocks (used
// to locate where the page starts in the markdown).
func layoutPageSigs(layoutPath string) [][]string {
	raw, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil
	}
	var lay struct {
		PDFInfo []struct {
			ParaBlocks []layoutBlock `json:"para_blocks"`
		} `json:"pdf_info"`
	}
	if err := json.Unmarshal(raw, &lay); err != nil {
		return nil
	}
	out := make([][]string, 0, len(lay.PDFInfo))
	for _, page := range lay.PDFInfo {
		var sigs []string
		for _, b := range page.ParaBlocks {
			if s := blockSig(b); s != "" {
				sigs = append(sigs, s)
			}
		}
		out = append(out, sigs)
	}
	return out
}

type layoutSpan struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	ImagePath string `json:"image_path"`
}

type layoutLine struct {
	Spans []layoutSpan `json:"spans"`
}

type layoutBlock struct {
	Type   string        `json:"type"`
	Lines  []layoutLine  `json:"lines"`
	Blocks []layoutBlock `json:"blocks"`
}

// blockSig renders the signature a block would contribute as the FIRST
// line of its markdown: plain text for text/title, the equation body for an
// interline equation, `![](…)` for an image, and the first line of a table
// body. Nested caption/body sub-blocks are searched for the image/table ones.
func blockSig(b layoutBlock) string {
	switch b.Type {
	case "image":
		for _, sub := range b.Blocks {
			if sub.Type != "image_body" {
				continue
			}
			for _, l := range sub.Lines {
				for _, s := range l.Spans {
					if s.Type == "image" && s.ImagePath != "" {
						// 只要文件名：md 里这一行是 `![](images/<名>)`，
						// 带不带 images/ 前缀都能命中。
						return s.ImagePath
					}
				}
			}
		}
		return ""
	case "table":
		for _, sub := range b.Blocks {
			if sub.Type != "table_body" {
				continue
			}
			for _, l := range sub.Lines {
				for _, s := range l.Spans {
					if s.Type == "table" && strings.TrimSpace(s.Content) != "" {
						return s.Content
					}
				}
			}
		}
		return ""
	}
	var sb strings.Builder
	for _, l := range b.Lines {
		for _, s := range l.Spans {
			switch s.Type {
			case "text":
				sb.WriteString(s.Content)
			case "inline_equation":
				sb.WriteString("$" + s.Content + "$")
			case "interline_equation":
				sb.WriteString(s.Content)
			}
		}
	}
	return sb.String()
}

var mdHeadingPrefixRe = regexp.MustCompile(`^#{1,6}\s+`)

// mdLineSig normalizes one markdown line for comparison with a block
// signature: heading markers dropped, markdown escapes removed, whitespace
// (incl. ideographic space) stripped, case folded.
func mdLineSig(line string) string {
	s := mdHeadingPrefixRe.ReplaceAllString(strings.TrimSpace(line), "")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '*', '`', '~', '$', '_', '#', '[', ']':
				i++
			}
		}
		r := rune(s[i])
		if unicode.IsSpace(r) || r == '　' {
			continue
		}
		b.WriteString(strings.ToLower(string(r)))
	}
	return b.String()
}

// buildPageIndex aligns every part's per-page signatures against its own
// markdown text and returns the global page starts of the MERGED md.
func buildPageIndex(mineruOutput string, parts []partText, separator string) ([]pageStartJSON, int, error) {
	var out []pageStartJSON
	page := 0
	var acc strings.Builder
	for i, part := range parts {
		if i > 0 {
			acc.WriteString(separator)
		}
		// 该部件第一行在合并后 md 里的行号 = 前缀里已有的换行数。
		base := strings.Count(acc.String(), "\n")
		acc.WriteString(part.text)

		sigs := layoutPageSigs(filepath.Join(mineruOutput, part.dir, "layout.json"))
		lines := strings.Split(part.text, "\n")
		norm := make([]string, len(lines))
		for j, l := range lines {
			norm[j] = mdLineSig(l)
		}
		ptr := 0
		for _, pageSigs := range sigs {
			page++
			start := -1
			// 先整串相等（最可靠），整页都不中再用前缀包含兜一次。
			for pass := 0; pass < 2 && start < 0; pass++ {
				for _, sig := range pageSigs {
					want := mdLineSig(sig)
					if want == "" {
						continue
					}
					if j := findSigLine(norm, ptr, want, pass == 1); j >= 0 {
						start = j
						break
					}
				}
			}
			if start >= 0 {
				ptr = start + 1
				out = append(out, pageStartJSON{Page: page, Line: base + start})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, page, nil
}

// sigPrefixKey returns the comparison key for prefix matching: the first 24
// non-empty characters, or the whole signature when it is shorter (then only
// exact equality is used — "1." 这种短签名做包含匹配会乱中).
func sigPrefixKey(want string) string {
	r := []rune(want)
	if len(r) < 12 {
		return ""
	}
	if len(r) > 24 {
		r = r[:24]
	}
	return string(r)
}

// findSigLine locates a page's first block in the md lines: exact equality
// first, then (when exact fails for every candidate) a prefix containment.
//
// md 会把一个块按列表行/断行规则拆成多行，也会在某行末尾留出行内公式后的
// 空格，整串相等会漏掉这些页；前缀包含补上它们，而页序单调 + 窗口有限保证
// 不会认到隔了好几页的同名标题上。
func findSigLine(norm []string, from int, want string, prefix bool) int {
	const pageSigLookahead = 400
	end := from + pageSigLookahead
	if end > len(norm) {
		end = len(norm)
	}
	if !prefix {
		for i := from; i < end; i++ {
			if norm[i] == want {
				return i
			}
		}
		return -1
	}
	key := sigPrefixKey(want)
	if key == "" {
		return -1
	}
	for i := from; i < end; i++ {
		if norm[i] == want || strings.Contains(norm[i], key) {
			return i
		}
	}
	return -1
}

// writePageIndex writes the sidecar for one subject ("md 正文不动，页界在
// 这里钉死"). It is best effort: any part without a readable layout.json
// simply contributes no page starts, and a subject with nothing usable gets
// no sidecar at all (img2text then falls back to its own alignment).
func writePageIndex(mineruOutput, outputDir, subject string, parts []partText, separator string, totalLines int) error {
	pages, _, err := buildPageIndex(mineruOutput, parts, separator)
	if err != nil {
		return err
	}
	path := pageIndexPath(outputDir, subject)
	if pages == nil {
		pages = []pageStartJSON{}
	}
	// 即使一页都没算出也照写照删失败都无所谓：文件存在即代表"这版 md 的
	// 页界已经算过"，下次命中指纹时据此判断要不要重算。
	payload := pageIndexJSON{Version: 1, MD: subject + ".md", Lines: totalLines, Pages: pages}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
