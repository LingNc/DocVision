package mdimg

// pagemark.go：旧线 go/internal/img2text/pagemark.go 逐字移植（P21 页码标记）。
// 行级页码对齐引擎：md 行与 MinerU content_list 锚点规范化匹配、命中推进、
// 未命中继承上页、标记插在新页开始的行首、围栏内顺延、embed 改写位移平移、
// plan-apply-audit 三段。原文每一行保留（含设计取舍注释）。
//
// 数据源与 latex/docindex.go 的 docPartDirs 完全同款（MinerU 中间产物
// mineru_output/<subject>[_partN]/ 的 *_content_list.json：text 条目带
// text/page_idx，image 条目带 img_path/page_idx；global 页码 = 部件偏移 +
// page_idx + 1）。img2text 保持独立，不 import latex 包，这里只复制需要的
// ~30 行部件目录逻辑。
//
// 设计取舍：
//   - 不加新配置项（少即是多）；找不到 mineru 产物时 pageAnchors 返回 nil，
//     调用方静默跳过——老数据 / 无 mineru 产物时不该报错。
//   - 对齐按**行**做在原始 md（entry.content，图片引用还在）上：文本行与
//     text 锚点做规范化包含匹配（去空白后互相包含、命中推进锚点指针），
//     图片引用行用 imageRefRe 的捕获按文件名对 image 锚点；未命中的行继承
//     上一个已知页。
//   - 标记插入发生在 Final pass 的 embed 改写**之后**（行级扫描），围栏内
//     的插入点顺延到围栏结束后；embed 改写造成的字节偏移用每个替换的
//     delta 平移回原始行首位置，避免两套 offset 互相踩。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// pageMarkRe matches an existing page-marker line ([P. 42]) so a marker
// is never duplicated at the same position.
var pageMarkRe = regexp.MustCompile(`^\[P\. \d+\]$`)

// hasPageMarks reports whether an md file already carries page markers
// (T69: 已交付文件不回注成页码版——原来没页码的保持没页码)。
func hasPageMarks(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		if pageMarkRe.MatchString(strings.TrimSpace(line)) {
			return true
		}
	}
	return false
}

// pageAnchor is one ordered anchor from the MinerU content lists.
type pageAnchor struct {
	kind string // "text" | "image"
	text string // kind=text: normalized block text
	img  string // kind=image: image file name (no images/ prefix, no dir)
	page int    // global 1-based page across all parts
}

// pgPartNumRe extracts the trailing part number from "<base>_partN"
// (same shape as latex/docindex.go's partNumRe; kept local on purpose).
var pgPartNumRe = regexp.MustCompile(`_part(\d+)$`)

// pagePartDirs lists the MinerU part dirs for an md (same order as
// latex/docindex.go so the global page offset lines up).
func pagePartDirs(mineruOutput, mdName string) []string {
	base := strings.TrimSuffix(mdName, filepath.Ext(mdName))
	entries, err := os.ReadDir(mineruOutput)
	if err != nil {
		return nil
	}
	type part struct {
		num int
		dir string
	}
	var parts []part
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == base {
			parts = append(parts, part{num: 0, dir: name})
			continue
		}
		if !strings.HasPrefix(name, base+"_") {
			continue
		}
		if m := pgPartNumRe.FindStringSubmatch(name); m != nil {
			n, _ := strconv.Atoi(m[1])
			parts = append(parts, part{num: n, dir: name})
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].num < parts[j].num })
	dirs := make([]string, 0, len(parts))
	for _, p := range parts {
		dirs = append(dirs, p.dir)
	}
	return dirs
}

// pageContentListPath picks the part's content list (skips the heavier v2).
func pageContentListPath(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_content_list.json"))
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), "_v2") {
			return m
		}
	}
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// pageNormalize collapses all whitespace (incl. full-width) so OCR text and
// markdown text compare on content alone.
func pageNormalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func pageIdxOf(it map[string]any) int {
	if v, ok := it["page_idx"].(float64); ok {
		return int(v)
	}
	return 0
}

// pageAnchors loads every content list under the md's parts into ordered
// anchors (text blocks and image files), each carrying its global page.
// Returns nil when nothing is found — callers skip page marks silently.
func pageAnchors(mineruOutput, mdName string) []pageAnchor {
	var out []pageAnchor
	offset := 0
	for _, dir := range pagePartDirs(mineruOutput, mdName) {
		cl := pageContentListPath(filepath.Join(mineruOutput, dir))
		if cl == "" {
			continue
		}
		raw, err := os.ReadFile(cl)
		if err != nil {
			continue
		}
		var items []map[string]any
		if err := json.Unmarshal(raw, &items); err != nil {
			continue
		}
		pages := 0
		for _, it := range items {
			if pageIdxOf(it)+1 > pages {
				pages = pageIdxOf(it) + 1
			}
		}
		for _, it := range items {
			page := offset + pageIdxOf(it) + 1
			typ, _ := it["type"].(string)
			switch typ {
			case "text", "interline_equation":
				text, _ := it["text"].(string)
				if norm := pageNormalize(text); norm != "" {
					out = append(out, pageAnchor{kind: "text", text: norm, page: page})
				}
			default:
				img, _ := it["img_path"].(string)
				if img != "" {
					out = append(out, pageAnchor{kind: "image", img: filepath.Base(img), page: page})
				}
			}
		}
		offset += pages
	}
	return out
}

// anchorTextMatch: normalized containment in either direction (OCR text may
// drop punctuation; md lines may carry markdown punctuation).
func anchorTextMatch(anchor, line string) bool {
	a, l := pageNormalize(anchor), pageNormalize(line)
	if a == "" || l == "" {
		return false
	}
	if strings.Contains(l, a) {
		return true
	}
	if strings.Contains(a, l) {
		return true
	}
	la, ll := len([]rune(a)), len([]rune(l))
	shorter, longer := a, l
	if la > ll {
		shorter, longer = l, a
	}
	return len([]rune(shorter)) >= 8 && strings.Contains(longer, shorter)
}

// alignPages walks the md lines top to bottom, advancing an anchor cursor.
// Text lines match text anchors; image lines match image anchors by file
// name; unmatched lines inherit the previous page (0 = still unknown).
func alignPages(lines []string, anchors []pageAnchor) []int {
	linePages := make([]int, len(lines))
	cursor := 0
	lastPage := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			linePages[i] = lastPage
			continue
		}
		if m := imageRefRe.FindStringSubmatch(trimmed); m != nil {
			base := filepath.Base(strings.TrimSpace(m[2])) // SK-I22 正则增 alt 组，路径在 m[2]
			page, found := 0, false
			for j := cursor; j < len(anchors); j++ {
				if anchors[j].kind != "image" {
					continue
				}
				if anchors[j].img == base {
					page, cursor, found = anchors[j].page, j+1, true
					break
				}
			}
			if found {
				linePages[i] = page
				lastPage = page
			} else {
				linePages[i] = lastPage
			}
			continue
		}
		norm := pageNormalize(trimmed)
		if norm == "" {
			linePages[i] = lastPage
			continue
		}
		page, found := 0, false
		for j := cursor; j < len(anchors); j++ {
			if anchors[j].kind != "text" {
				continue
			}
			if anchorTextMatch(anchors[j].text, norm) {
				page, cursor, found = anchors[j].page, j+1, true
				break
			}
		}
		if found {
			linePages[i] = page
			lastPage = page
		} else {
			linePages[i] = lastPage
		}
	}
	return linePages
}

// findImageAnchor resolves the nearest image anchor at or after cursor whose
// file name matches; used when an image line failed the forward scan.
func findImageAnchor(anchors []pageAnchor, from int, base string) int {
	for j := from; j < len(anchors); j++ {
		if anchors[j].kind == "image" && anchors[j].img == base {
			return j
		}
	}
	return -1
}

// pageMarkPositions compares consecutive known pages and records an
// insertion request whenever the page INCREASES (only upward transitions
// mark a new page; repeated/earlier pages never get a marker).
func pageMarkPositions(lines []string, linePages []int) []pageMark {
	var marks []pageMark
	prev := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if pageMarkRe.MatchString(trimmed) {
			if n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(trimmed, "[P. "), "]")); err == nil {
				prev = n
			}
			continue
		}
		if linePages[i] > prev {
			marks = append(marks, pageMark{line: i, page: linePages[i]})
			prev = linePages[i]
		}
	}
	return marks
}

// pageMark records one page-marker insertion request against the ORIGINAL
// md line number (0-based).
type pageMark struct {
	line int // 原文行号（0-based）
	page int // global 1-based 页码
}

// embedShift records one image-ref→embed replacement's byte delta so page
// marks computed on the ORIGINAL md stay correct after the rewrite.
type embedShift struct {
	start int // original byte offset of the replaced image ref
	delta int // len(embed) - len(ref)
}

// shiftFor returns the total byte shift applied to an original offset by
// every embed replacement that starts BEFORE it (replacements never
// overlap, so the sum is exact).
func shiftFor(pos int, shifts []embedShift) int {
	delta := 0
	for _, s := range shifts {
		if s.start < pos {
			delta += s.delta
		}
	}
	return delta
}

// resolveFencePos walks the final content and, when pos lands inside a
// fenced code block, slides it to just after the closing fence so the page
// marker is never injected into a code block.
func resolveFencePos(content string, pos int) int {
	lines := strings.Split(content, "\n")
	offset := 0
	inFence := false
	fenceChar := byte(0)
	for _, line := range lines {
		if offset >= pos {
			break
		}
		trimmed := strings.TrimSpace(line)
		if !inFence && len(trimmed) >= 3 && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			inFence = true
			fenceChar = trimmed[0]
		} else if inFence && len(trimmed) >= 3 && trimmed[0] == fenceChar {
			inFence = false
		}
		offset += len(line) + 1
	}
	if !inFence {
		return pos
	}
	for offset < len(content) {
		end := strings.IndexByte(content[offset:], '\n')
		if end < 0 {
			return len(content)
		}
		offset += end + 1
		trimmed := strings.TrimSpace(content[offset:])
		if len(trimmed) >= 3 && trimmed[0] == fenceChar {
			return offset
		}
	}
	return len(content)
}

// pageMarkText renders the marker line.
func pageMarkText(page int) string {
	return "[P. " + strconv.Itoa(page) + "]"
}

// insertPageMarks splices the marker lines into the final content at the
// recorded byte positions (already shifted back through shiftFor and
// resolveFencePos). Returns the rewritten content plus how many markers
// were inserted.
func insertPageMarks(content string, marks []pageMark, starts []int, shifts []embedShift) (string, int) {
	if len(marks) == 0 {
		return content, 0
	}
	positions := make([]int, len(marks))
	for i, m := range marks {
		lineStart := starts[m.line]
		positions[i] = resolveFencePos(content, lineStart+shiftFor(lineStart, shifts))
	}
	sort.Ints(positions)
	// 插入语义：标记是"加进去"的不是"替换"——续写偏移就是 positions[k] 本身。
	// 旧实现续写偏移多算了 len(标记)+1，把每个 [P. N] 后 7~8 个字节吞掉
	// （11-20 dvws3 实跑暴露：ASCII 夹具 silently 吞字符，CJK 直接非法 UTF-8）。
	out := content[:positions[0]] + pageMarkText(marks[0].page) + "\n"
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			continue
		}
		out += content[positions[i-1]:positions[i]] + pageMarkText(marks[i].page) + "\n"
	}
	last := positions[len(positions)-1]
	out += content[last:]
	return out, len(positions)
}

// lastLineBefore returns the nearest non-empty line above pos (for logs).
func lastLineBefore(content string, pos int) string {
	if pos > len(content) {
		pos = len(content)
	}
	prefix := content[:pos]
	lines := strings.Split(strings.TrimRight(prefix, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// pageMarkPlan bundles the alignment inputs for one md.
type pageMarkPlan struct {
	anchors   []pageAnchor
	linePages []int // per original md line, 0 = unknown
}

// planPageMarks builds the alignment for one md (nil = no mineru output,
// caller skips silently).
func planPageMarks(mineruOutput, mdName string, lines []string) *pageMarkPlan {
	anchors := pageAnchors(mineruOutput, mdName)
	if len(anchors) == 0 {
		return nil
	}
	return &pageMarkPlan{anchors: anchors, linePages: alignPages(lines, anchors)}
}

// pageAt returns the aligned page of an original line (0 = unknown).
func (p *pageMarkPlan) pageAt(starts []int, pos int) int {
	line := sort.Search(len(starts), func(i int) bool { return starts[i] > pos }) - 1
	if line < 0 || line >= len(p.linePages) {
		return 0
	}
	return p.linePages[line]
}

// applyTo inserts the page markers into the final rewritten content and
// returns the new content plus the inserted marks (for the audit).
func (p *pageMarkPlan) applyTo(content string, lines []string, starts []int, shifts []embedShift) (string, []pageMark) {
	marks := pageMarkPositions(lines, p.linePages)
	if len(marks) == 0 {
		return content, nil
	}
	out, n := insertPageMarks(content, marks, starts, shifts)
	return out, marks[:n]
}

// pageMarkAudit summarizes the applied marks for the batch report.
type pageMarkAudit struct {
	MaxPage    int // 该书总页数（部件偏移 + 末页）
	OutOfRange int // 越界标记数
	Backward   int // 页码非递增（回退/重复）标记数
}

func (p *pageMarkPlan) audit(marks []pageMark) pageMarkAudit {
	var audit pageMarkAudit
	maxPage := 0
	for _, a := range p.anchors {
		if a.page > maxPage {
			maxPage = a.page
		}
	}
	audit.MaxPage = maxPage
	prev := 0
	for _, m := range marks {
		if m.page > maxPage {
			audit.OutOfRange++
		}
		if m.page <= prev {
			audit.Backward++
		}
		prev = m.page
	}
	return audit
}

// —— 以下为新架构导出适配层（薄壳，内部即上文逐字实现）——

// PageMark 是导出的标记类型。
type PageMark = pageMark

// HasPageMarks 导出 hasPageMarks。
func HasPageMarks(content []byte) bool { return hasPageMarks(content) }

// PlanPageMarks 导出 planPageMarks。
func PlanPageMarks(mineruOutput, mdName string, lines []string) *pageMarkPlan {
	return planPageMarks(mineruOutput, mdName, lines)
}

// ApplyTo 导出 applyTo。
func (p *pageMarkPlan) ApplyTo(content string, lines []string, starts []int, shifts []embedShift) (string, []PageMark) {
	return p.applyTo(content, lines, starts, shifts)
}

// Audit 导出 audit。
func (p *pageMarkPlan) Audit(marks []PageMark) pageMarkAudit { return p.audit(marks) }
