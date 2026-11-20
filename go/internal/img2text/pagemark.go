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
//   - 标记与图片锚在 Final pass 的**同一遍行级扫描**里产出（T330）：每行
//     先按对齐结果补齐它之前的页标记（从 P.1 连续覆盖，空白页也补），再
//     输出该行；锚的 `· P.N` 与标记共用同一个 curPage，不再有"先按字节
//     偏移改写、再把标记位置平移回去"的第二步。围栏内的标记顺延到围栏后。

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
	// 类解析而静默失配（实测两本书全部 0 锚点的根因）。
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

// pageMark is one marker actually emitted: before md line `line`, the
// page announced. Used for the count and the self-check (a marker that
// would land inside a code fence is deferred to after the fence, so the
// recorded line is the deferred one).
type pageMark struct {
	line int
	page int
}

// pageMarkCoverage turns the per-line known pages into the page list to
// announce before each line (T330). Every page between the last announced
// one and this line's page is listed, so blank pages and image-only pages
// — the ones the old "only on an upward transition" rule silently skipped
// — still get their marker: the delivered markers are contiguous 1..N.
func pageMarkCoverage(linePages []int) [][]int {
	out := make([][]int, len(linePages))
	announced := 0
	for i, p := range linePages {
		if p <= announced {
			continue
		}
		pages := make([]int, 0, p-announced)
		for q := announced + 1; q <= p; q++ {
			pages = append(pages, q)
		}
		out[i] = pages
		announced = p
	}
	return out
}

// fenceBodyFlags marks the lines that sit INSIDE a fenced code block
// (fence lines themselves excluded). A page marker landing there would
// break the fence, so markLines defers it.
func fenceBodyFlags(lines []string) []bool {
	out := make([]bool, len(lines))
	inside := false
	for i, line := range lines {
		if fenceLineRe.MatchString(line) {
			inside = !inside
			continue
		}
		out[i] = inside
	}
	return out
}

// markLines maps each line to the page markers to emit before it.
func (p *pageMarkPlan) markLines(lines []string) [][]int {
	if p == nil {
		return nil
	}
	cov := pageMarkCoverage(p.linePages)
	inside := fenceBodyFlags(lines)
	out := make([][]int, len(lines))
	for i, pages := range cov {
		if len(pages) == 0 {
			continue
		}
		j := i
		// 围栏体内（以及收尾的 ``` 那一行）都不能插，顺延到围栏之后。
		for j < len(lines) && (inside[j] || fenceLineRe.MatchString(lines[j])) {
			j++
		}
		if j >= len(lines) {
			j = len(lines) - 1
		}
		out[j] = append(out[j], pages...)
	}
	return out
}

// pageOfLine is the ground-truth page of md line i (0 = unknown, the
// caller keeps the running page).
func (p *pageMarkPlan) pageOfLine(i int) int {
	if p == nil || i < 0 || i >= len(p.linePages) {
		return 0
	}
	return p.linePages[i]
}

// pageMarkText renders the marker line for a page.
func pageMarkText(page int) string {
	return "[P. " + strconv.Itoa(page) + "]"
}

// pageMarkPlan is the per-md alignment result for one Final-pass rewrite.
type pageMarkPlan struct {
	anchors   []pageAnchor // 对齐路径的锚点（sidecar 路径为空）
	linePages []int        // per original md line, 0 = unknown
	maxPage   int          // 总页数（sidecar 给的，用于自检）
}

// planPageMarks builds the page table for one md. T331：organize 在合并 md
// 时已经用 layout.json 把页界钉成了 `<md>.pages.json`（每页起始行），有这份
// sidecar 就**直接消费**、一页都不用推；老数据（升级前的 md）没有 sidecar，
// 才退回按 content_list 对齐。nil = 两者都没有，调用方静默跳过。
func planPageMarks(mineruOutput, outputDir, mdName string, lines []string) *pageMarkPlan {
	if p := planFromPageIndex(outputDir, mdName, lines); p != nil {
		return p
	}
	anchors := pageAnchors(mineruOutput, mdName)
	if anchors == nil {
		return nil
	}
	return &pageMarkPlan{anchors: anchors, linePages: alignPages(lines, anchors)}
}

// pageIndexFile mirrors the sidecar organize writes (`<md>.pages.json`) —
// kept as a small local struct so img2text does not import organize.
type pageIndexFile struct {
	Version int `json:"version"`
	Lines   int `json:"lines"`
	Pages   []struct {
		Page int `json:"page"`
		Line int `json:"line"`
	} `json:"pages"`
}

// planFromPageIndex expands the sidecar into a per-line page table: page N
// owns every line from its start until the next page's start. A stale index
// (merged md's line count changed) is ignored, and lines before the first
// page start stay unknown.
func planFromPageIndex(outputDir, mdName string, lines []string) *pageMarkPlan {
	if outputDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(outputDir, strings.TrimSuffix(mdName, ".md")+".pages.json"))
	if err != nil {
		return nil
	}
	var idx pageIndexFile
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil
	}
	if idx.Version != 1 || idx.Lines != len(lines) || len(idx.Pages) == 0 {
		return nil // 过期或空索引：当作没有，退回对齐
	}
	table := make([]int, len(lines))
	maxPage := 0
	next := 0
	cur := 0
	for i := range table {
		for next < len(idx.Pages) && idx.Pages[next].Line <= i {
			cur = idx.Pages[next].Page
			next++
		}
		table[i] = cur
	}
	for _, p := range idx.Pages {
		if p.Page > maxPage {
			maxPage = p.Page
		}
	}
	return &pageMarkPlan{linePages: table, maxPage: maxPage}
}

// pageMarkAudit is the runtime self-check printed next to the mark count:
// the marker pages must stay inside the book's page range and strictly
// increase. Anything else means the alignment drifted and the log says so
// instead of silently shipping wrong page numbers.
type pageMarkAudit struct {
	MaxPage    int // 该书总页数（部件偏移 + 末页）
	OutOfRange int // 越界标记数
	Backward   int // 页码非递增（回退/重复）标记数
	Missing    int // 该有却没有的页标记数（标记应连续覆盖 1..MaxPage）
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
	if p.maxPage > a.MaxPage {
		a.MaxPage = p.maxPage
	}
	for _, m := range marks {
		if m.page > a.MaxPage {
			a.MaxPage = m.page
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
	if a.MaxPage > len(marks) {
		a.Missing = a.MaxPage - len(marks)
	}
	return a
}
