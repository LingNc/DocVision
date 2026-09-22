package img2text

// P21 页码标记：img2text 把 md（引用 PDF 转出的图片）转成最终产物时，在
// 原始内容分页的位置插入独立行 `[P. <页码>]`（纯 ASCII 方括号 + P. 前缀），
// 方便后续 AI 读取并向用户指明位置。
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

// pagePartDirs returns the MinerU part directories that contributed to a
// source markdown ("X.md" -> X/ then X_part1, X_part2, ...), in part
// order so global page numbers align. Mirrors docPartDirs.
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

// pageContentListPath picks the part's content list (skips the heavier v2),
// same rule as contentListPath in latex/docindex.go.
func pageContentListPath(dir string) string {
	// 不用 Glob：书名常带 [a4]/[正度纸] 这类方括号，Glob 会把它当字符
	// 类解析而静默失配（李-范/大雪深埋全部 0 锚点的根因）。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var first string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_content_list.json") {
			continue
		}
		if !strings.Contains(e.Name(), "_v2") {
			return filepath.Join(dir, e.Name())
		}
		if first == "" {
			first = filepath.Join(dir, e.Name())
		}
	}
	return first
}

// pageNormalize strips ALL whitespace (spaces are OCR noise; the md line
// and the content_list text differ in where they break lines) and
// lowercases, so containment matching is stable.
func pageNormalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '　' {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// pageIdxOf reads page_idx tolerating both real-data shapes: a JSON
// string ("39") and a plain number (39).
func pageIdxOf(it map[string]any) int {
	switch v := it["page_idx"].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

// pageAnchors scans the part directories of mdName in part order and
// returns the ordered anchor sequence. It returns nil (caller skips
// silently) when there is no mineru_output for this md or no readable
// content list — old data must never turn into an error.
func pageAnchors(mineruOutput, mdName string) []pageAnchor {
	dirs := pagePartDirs(mineruOutput, mdName)
	if len(dirs) == 0 {
		return nil
	}
	var out []pageAnchor
	offset := 0
	for _, d := range dirs {
		cl := pageContentListPath(filepath.Join(mineruOutput, d))
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
		maxPage := 0
		for _, it := range items {
			page := pageIdxOf(it)
			if page+1 > maxPage {
				maxPage = page + 1
			}
			if typ, _ := it["type"].(string); typ == "image" {
				p, _ := it["img_path"].(string)
				if p == "" {
					continue
				}
				out = append(out, pageAnchor{kind: "image",
					img: filepath.Base(p), page: offset + page + 1})
				continue
			}
			t, _ := it["text"].(string)
			if norm := pageNormalize(t); norm != "" {
				out = append(out, pageAnchor{kind: "text",
					text: norm, page: offset + page + 1})
			}
		}
		offset += maxPage
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// anchorTextMatch reports whether a normalized md line matches a
// normalized text anchor: mutual containment after whitespace stripping.
// Very short anchors (< 4 runes) only match on exact equality so a one-
// character block cannot latch onto arbitrary lines.
func anchorTextMatch(anchor, line string) bool {
	if anchor == "" || line == "" {
		return false
	}
	if anchor == line {
		return true
	}
	short, long := anchor, line
	if len(short) > len(long) {
		short, long = long, short
	}
	if len([]rune(short)) < 4 {
		return false
	}
	return strings.Contains(long, short)
}

// alignPages walks the md lines against the ordered anchors and returns
// the known page (1-based) for every line, 0 = unknown (the caller lets
// such a line inherit the previous known page). A hit advances the anchor
// pointer so the same anchor never matches twice.
// alignScanWindow bounds how far ahead one md line may reach for an
// anchor. Without it a TOC line ("第三章 认识论……029") matches the BODY
// heading anchor thousands of entries later and drags the pointer through
// the whole book — the "第 2 页后直接跳 243 页" symptom. A true next block
// sits within a few dozen anchors (tables/images between are counted too).
const alignScanWindow = 60

func alignPages(lines []string, anchors []pageAnchor) []int {
	pages := make([]int, len(lines))
	ai := 0
	last := 0 // 上一个已确认的页码；匹配不得回退（文档顺序页码非降）
	for i, line := range lines {
		if m := imageRefRe.FindStringSubmatch(line); m != nil {
			base := filepath.Base(m[1])
			if j := findImageAnchor(anchors, ai, base); j >= 0 && anchors[j].page >= last {
				pages[i] = anchors[j].page
				last = pages[i]
				ai = j + 1
			}
			continue
		}
		norm := pageNormalize(line)
		if norm == "" {
			continue
		}
		end := ai + alignScanWindow
		if end > len(anchors) {
			end = len(anchors)
		}
		for j := ai; j < end; j++ {
			if anchors[j].kind != "text" || anchors[j].page < last {
				continue
			}
			if anchorTextMatch(anchors[j].text, norm) {
				pages[i] = anchors[j].page
				last = pages[i]
				ai = j + 1
				break
			}
		}
	}
	return pages
}

// findImageAnchor returns the index of the next image anchor at/after
// from whose file name is base; when the pointer has run past the end it
// falls back to the first occurrence anywhere (an image referenced twice
// has two anchors; the pointer keeps them in order).
func findImageAnchor(anchors []pageAnchor, from int, base string) int {
	for j := from; j < len(anchors); j++ {
		if anchors[j].kind == "image" && anchors[j].img == base {
			return j
		}
	}
	for j := 0; j < from && j < len(anchors); j++ {
		if anchors[j].kind == "image" && anchors[j].img == base {
			return j
		}
	}
	return -1
}

// pageMark is one insertion request: before md line `line`, for page
// transitions to `page`. Positions may sit inside a code fence; the
// final-content inserter defers those to after the fence.
type pageMark struct {
	line int
	page int
}

// pageMarkPositions compares consecutive known pages and records an
// insertion request whenever the page INCREASES (only upward transitions
// mark a page break; unknown lines inherit and never fire).
func pageMarkPositions(lines []string, linePages []int) []pageMark {
	var out []pageMark
	prev := 0
	for i, p := range linePages {
		if p == 0 {
			continue
		}
		if prev > 0 && p > prev {
			out = append(out, pageMark{line: i, page: p})
		}
		prev = p
	}
	return out
}

// embedShift records the length change one embed replacement caused at an
// original byte offset, so marker positions (original line starts) can be
// translated into the rewritten content.
type embedShift struct {
	start int // original byte offset of the replaced image ref
	delta int // len(embed) - len(ref)
}

// shiftFor returns the total byte shift applied to an original offset by
// every embed replacement that starts BEFORE it (replacements never
// straddle a line start: a ref lives inside one line).
func shiftFor(pos int, shifts []embedShift) int {
	n := 0
	for _, s := range shifts {
		if s.start < pos {
			n += s.delta
		}
	}
	return n
}

// resolveFencePos returns pos unchanged when it is OUTSIDE a code fence;
// inside, it moves pos past the closing fence line so the marker never
// lands inside a fenced block. Fence parity counts complete fence lines
// (``` or ```lang) before pos; an unclosed trailing fence leaves pos as-is.
func resolveFencePos(content string, pos int) int {
	locs := fenceLineRe.FindAllStringIndex(content, -1)
	inside := false
	nextClose := -1
	for _, l := range locs {
		end := l[1]
		if end < len(content) && content[end] == '\n' {
			end++
		}
		if end <= pos {
			inside = !inside
			continue
		}
		nextClose = end
		break
	}
	if inside && nextClose >= 0 {
		return nextClose
	}
	return pos
}

// pageMarkText renders the marker line for a page.
func pageMarkText(page int) string {
	return "[P. " + strconv.Itoa(page) + "]\n"
}

// insertPageMarks inserts the marker lines into content (the FINAL
// rewritten md). marks refer to original line indices; starts is the
// original per-line byte table and shifts the embed deltas. Multiple
// requests resolving to the same position insert one line per page, in
// order; a position whose preceding line already carries the identical
// marker is skipped (no duplicates).
func insertPageMarks(content string, marks []pageMark, starts []int, shifts []embedShift) (string, int) {
	if len(marks) == 0 {
		return content, 0
	}
	type ins struct {
		pos  int
		text string
	}
	grouped := map[int][]int{} // pos -> pages in order
	order := []int{}
	for _, m := range marks {
		pos := resolveFencePos(content, starts[m.line]+shiftFor(starts[m.line], shifts))
		if _, ok := grouped[pos]; !ok {
			order = append(order, pos)
		}
		grouped[pos] = append(grouped[pos], m.page)
	}
	var insList []ins
	for _, pos := range order {
		pages := grouped[pos]
		// Dedupe: drop pages already announced by the line right before
		// the insertion point (re-running over marked output, or a
		// previous marker inserted at the same resolved position).
		prev := lastLineBefore(content, pos)
		prevIsMark := pageMarkRe.MatchString(prev)
		text := ""
		for _, p := range pages {
			mk := "[P. " + strconv.Itoa(p) + "]"
			if prev == mk {
				continue
			}
			t := pageMarkText(p)
			if pos > 0 && !prevIsMark && !strings.HasSuffix(content[:pos], "\n\n") {
				t = "\n" + t
			}
			text += t
			prev = mk
			prevIsMark = true
		}
		if text != "" {
			insList = append(insList, ins{pos: pos, text: text})
		}
	}
	// Insert right-to-left so earlier positions stay valid.
	sort.Slice(insList, func(i, j int) bool { return insList[i].pos > insList[j].pos })
	inserted := 0
	for _, in := range insList {
		if in.pos > len(content) {
			continue
		}
		content = content[:in.pos] + in.text + content[in.pos:]
		inserted += strings.Count(in.text, "[P. ")
	}
	return content, inserted
}

// lastLineBefore returns the last non-empty line strictly before pos.
func lastLineBefore(content string, pos int) string {
	if pos > len(content) {
		pos = len(content)
	}
	s := content[:pos]
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// pageMarkPlan is the per-md alignment result for one Final-pass rewrite.
type pageMarkPlan struct {
	anchors   []pageAnchor
	linePages []int // per original md line, 0 = unknown
}

// planPageMarks builds the alignment for one md (nil = no mineru output,
// caller skips silently).
func planPageMarks(mineruOutput, mdName string, lines []string) *pageMarkPlan {
	anchors := pageAnchors(mineruOutput, mdName)
	if anchors == nil {
		return nil
	}
	return &pageMarkPlan{anchors: anchors, linePages: alignPages(lines, anchors)}
}

// pageAt reports the known page of the md line containing original byte
// offset pos (0 = unknown; used to stamp [Image]( label · P.N ) embeds).
func (p *pageMarkPlan) pageAt(starts []int, pos int) int {
	if p == nil {
		return 0
	}
	return p.linePages[findLineIndex(starts, pos)]
}

// applyTo inserts the marker lines into the FINAL rewritten content.
// shifts carries the (offset, delta) of every embed replacement already
// applied. Returns the content and how many marker lines were inserted.
func (p *pageMarkPlan) applyTo(content string, lines []string, starts []int, shifts []embedShift) (string, []pageMark) {
	if p == nil {
		return content, nil
	}
	marks := pageMarkPositions(lines, p.linePages)
	out, _ := insertPageMarks(content, marks, starts, shifts)
	return out, marks
}

// pageMarkAudit is the runtime self-check printed next to the mark count:
// the marker pages must stay inside the book's page range and strictly
// increase. Anything else means the alignment drifted and the log says so
// instead of silently shipping wrong page numbers.
type pageMarkAudit struct {
	MaxPage    int // 该书总页数（部件偏移 + 末页）
	OutOfRange int // 越界标记数
	Backward   int // 页码非递增（回退/重复）标记数
}

func (p *pageMarkPlan) audit(marks []pageMark) pageMarkAudit {
	a := pageMarkAudit{}
	if p == nil {
		return a
	}
	for _, x := range p.anchors {
		if x.page > a.MaxPage {
			a.MaxPage = x.page
		}
	}
	prev := 0
	for _, m := range marks {
		if a.MaxPage > 0 && m.page > a.MaxPage {
			a.OutOfRange++
		}
		if prev > 0 && m.page <= prev {
			a.Backward++
		}
		prev = m.page
	}
	return a
}
