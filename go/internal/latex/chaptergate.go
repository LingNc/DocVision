package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// chapterGate 是章节提交的硬门禁（T165）：**先编译成功、再亲眼看编译结果**，
// 两件事都留下可核对的印记，`submit` 才放行。
//
// 为什么需要它：实测一本已经"跑完"的书里，有的章节（含递归最深的一层）
// 一次 `compile` 都没有就 `submit` 了，有的编译了但从没 `view_pdf`——
// 因为 `submit` 当时只检查「两个路径存在、没有 \documentclass、带 report」，
// 编译与看图全靠模型自觉。于是错误的 `.tex` 一路进到书里，到最后才在
// assemble 阶段炸掉（或者更糟：整章内容缺失却没人发现）。
//
// 印记写在会话工作区的 `.gate/` 下（不进提交产物）：
//
//	compile.json  {sources, pdf, pdf_hash, pages, at}
//	view.json     {pdf_hash, pages:[…], at}
//
// `sources` 是「章节 .tex + 它的 parts 目录」的内容摘要：编译之后再改文件，
// 摘要就对不上，必须重新编译——这正是「编译通过即采纳」曾经踩过的坑
// （改完不重编，队里拿到的还是旧的 PDF）。
type chapterGate struct {
	WorkRoot string // 会话可写工作区
	Base     string // 章节名（chapter_042 / chapter_042_child_02 …）
	// ChapterMD 是这一章的 markdown 原文。设了它，submit 还会做一次**内容
	// 覆盖率**核对（T168）：脚本机械转换漏掉整段/整张表时，md 的正文三字组
	// 在 PDF 文本里就凑不齐，当场打回并列出缺得最多的几段。
	ChapterMD string
	// NoSourceMount 表示这条会话**没有** source 挂载点（原书 PDF 视图没就绪，
	// 见 Runner.submitGate）。门禁照旧要求"编译成功 + 亲眼看产物"，但不再要求
	// "对照原书页"：那个要求在没有 source 挂载点的会话里永远满足不了（会话只
	// 会拿到 `未知挂载点 "source"`），硬留着只会把会话逼进"复盘 → 截断 → 还是
	// 过不去"的死循环。
	NoSourceMount bool
	// Warn 收一句告警（可选）。NoSourceMount 生效时用它把"这次不要求对照原书"
	// 说清楚，只提醒一次——静默放宽等于把一个顺序/配置问题藏起来。
	Warn func(string)
	// noSourceNoted 是 Warn 的"只说一次"标记（check 会被反复调用）。
	noSourceNoted bool
}

type gateCompileStamp struct {
	Sources string `json:"sources"`
	PDF     string `json:"pdf"`
	PDFHash string `json:"pdf_hash"`
	Pages   int    `json:"pages"`
	At      string `json:"at"`
}

// gateSourceStamp 记"看过原书的哪些页"（T168）。与 gateViewStamp 分开：
// 前者是别人的 PDF（原书 part），哈希随时可能变，只按 文件+页 记去重。
type gateSourceStamp struct {
	Views []gateSourceView `json:"views"`
	At    string           `json:"at"`
}

type gateSourceView struct {
	File string `json:"file"`
	Page int    `json:"page"`
}

type gateCoverageStamp struct {
	Coverage float64  `json:"coverage"`
	Missing  []string `json:"missing,omitempty"`
	// Checks 是"覆盖率不达标"的累计次数：第一次打回让模型补内容，第二次放行
	// 并把这个缺口记进报告（OCR 差异大的书可能永远到不了阈值，不能硬卡死）。
	Checks int    `json:"checks,omitempty"`
	At     string `json:"at"`
}

type gateViewStamp struct {
	PDFHash string `json:"pdf_hash"`
	Pages   []int  `json:"pages"`
	At      string `json:"at"`
}

func (g *chapterGate) dir() string {
	return filepath.Join(g.WorkRoot, ".gate")
}

func (g *chapterGate) compilePath() string  { return filepath.Join(g.dir(), "compile.json") }
func (g *chapterGate) viewPath() string     { return filepath.Join(g.dir(), "view.json") }
func (g *chapterGate) sourcePath() string   { return filepath.Join(g.dir(), "source.json") }
func (g *chapterGate) coveragePath() string { return filepath.Join(g.dir(), "coverage.json") }

func (g *chapterGate) enabled() bool { return g != nil && g.WorkRoot != "" && g.Base != "" }

// sourceDigest 把「主 .tex + parts 目录里的每个文件」按路径排序摘要一遍。
// 任何一处改动都会换掉摘要，从而作废旧印记。
func (g *chapterGate) sourceDigest() string {
	h := sha256.New()
	add := func(rel string, data []byte) {
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		h.Write(data)
	}
	mainPath := filepath.Join(g.WorkRoot, g.Base+".tex")
	if data, err := os.ReadFile(mainPath); err == nil {
		add(g.Base+".tex", data)
	}
	partsDir := filepath.Join(g.WorkRoot, g.Base)
	var files []string
	_ = filepath.Walk(partsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		files = append(files, p)
		return nil
	})
	sort.Strings(files)
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		rel, rerr := filepath.Rel(g.WorkRoot, p)
		if rerr != nil {
			rel = p
		}
		add(filepath.ToSlash(rel), data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// markCompiled 在编译成功且产物存在时记下这一轮的「源码摘要 + 产物指纹」。
func (g *chapterGate) markCompiled(pdfPath string) error {
	if !g.enabled() {
		return nil
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return err
	}
	hash, err := fileSHA256(pdfPath)
	if err != nil {
		return err
	}
	pages, _ := pdfPageCount(pdfPath)
	stamp := gateCompileStamp{
		Sources: g.sourceDigest(),
		PDF:     filepath.Base(pdfPath),
		PDFHash: hash,
		Pages:   pages,
		At:      nowStamp(),
	}
	if err := os.MkdirAll(g.dir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(stamp, "", "  ")
	// 重新编译即作废上一次的"已看过"：新产物还没人看。
	_ = os.Remove(g.viewPath())
	return os.WriteFile(g.compilePath(), data, 0o644)
}

// markViewed 记录「这一份编译产物被真的渲染出来看过了」。
func (g *chapterGate) markViewed(pdfPath string, page int) {
	if !g.enabled() || page <= 0 {
		return
	}
	var cs gateCompileStamp
	if data, err := os.ReadFile(g.compilePath()); err != nil || json.Unmarshal(data, &cs) != nil {
		return
	}
	hash, err := fileSHA256(pdfPath)
	if err != nil || hash != cs.PDFHash {
		return // 看的不是这一轮的编译产物
	}
	var vs gateViewStamp
	if data, err := os.ReadFile(g.viewPath()); err == nil {
		_ = json.Unmarshal(data, &vs)
	}
	vs.PDFHash = hash
	seen := false
	for _, p := range vs.Pages {
		if p == page {
			seen = true
			break
		}
	}
	if !seen {
		vs.Pages = append(vs.Pages, page)
		sort.Ints(vs.Pages)
	}
	vs.At = nowStamp()
	if err := os.MkdirAll(g.dir(), 0o755); err != nil {
		return
	}
	data, _ := json.MarshalIndent(vs, "", "  ")
	_ = os.WriteFile(g.viewPath(), data, 0o644)
}

// check 返回拒绝文案；"" 表示门禁通过。
// markSourceViewed 记下"用 view_pdf 看过了原书的这一页"。只在工作区之外的
// PDF（source 挂载里的原书 part）上调用——阶段一的目标是让人真的把产物和原书
// 对一遍，而不是只盯着自己的 PDF 自我感觉良好。
func (g *chapterGate) markSourceViewed(pdfPath string, page int) {
	if !g.enabled() || page <= 0 {
		return
	}
	name := filepath.Base(pdfPath)
	var ss gateSourceStamp
	if data, err := os.ReadFile(g.sourcePath()); err == nil {
		_ = json.Unmarshal(data, &ss)
	}
	for _, v := range ss.Views {
		if v.File == name && v.Page == page {
			return
		}
	}
	ss.Views = append(ss.Views, gateSourceView{File: name, Page: page})
	ss.At = nowStamp()
	if err := os.MkdirAll(g.dir(), 0o755); err != nil {
		return
	}
	data, _ := json.MarshalIndent(ss, "", "  ")
	_ = os.WriteFile(g.sourcePath(), data, 0o644)
}

func (g *chapterGate) sourceViews() []gateSourceView {
	var ss gateSourceStamp
	if data, err := os.ReadFile(g.sourcePath()); err != nil || json.Unmarshal(data, &ss) != nil {
		return nil
	}
	return ss.Views
}

// needSourcePages 是要对照的原书页数：产物越长，要求抽查的原书页越多（2–4 页）。
// 下限 2（开头 + 一处中段/结尾），上限 4（再长也没必要把成本堆在抽样上；真要
// 逐页核对的时候模型自己会看更多）。
func needSourcePages(ownPages int) int {
	want := ownPages / 3
	if want < 2 {
		want = 2
	}
	if want > 4 {
		want = 4
	}
	return want
}

// compiledPDF 返回本次编译的产物路径与页数（没有编译记录时 ok=false）。
func (g *chapterGate) compiledPDF() (string, int, bool) {
	var cs gateCompileStamp
	if data, err := os.ReadFile(g.compilePath()); err != nil || json.Unmarshal(data, &cs) != nil {
		return "", 0, false
	}
	return filepath.Join(g.WorkRoot, cs.PDF), cs.Pages, true
}

// lastCoverage 读上一次内容核对的结果（供日志展示）。
func (g *chapterGate) lastCoverage() (float64, bool) {
	var cv gateCoverageStamp
	if data, err := os.ReadFile(g.coveragePath()); err != nil || json.Unmarshal(data, &cv) != nil {
		return 0, false
	}
	return cv.Coverage, true
}

// coverageGap 返回"已经放行、但仍低于阈值"的那次核对（供 submit 记账）。
func (g *chapterGate) coverageGap() (gateCoverageStamp, bool) {
	var cv gateCoverageStamp
	if data, err := os.ReadFile(g.coveragePath()); err != nil || json.Unmarshal(data, &cv) != nil {
		return cv, false
	}
	if cv.Checks == 0 || cv.Coverage >= minChapterCoverage {
		return cv, false
	}
	return cv, true
}

// checkCoverage 把章节 md 的正文与编译出的 PDF 文本做一次覆盖率核对。返回
// 打回文案即拒绝；工具缺失（pdftotext）时返回空并放行——不能因为环境缺件
// 把整本书卡死。
func (g *chapterGate) checkCoverage() string {
	if !g.enabled() || strings.TrimSpace(g.ChapterMD) == "" {
		return ""
	}
	pdf, _, ok := g.compiledPDF()
	if !ok {
		return "REJECTED: 还没有编译记录，无法核对内容。先 compile 再 submit。"
	}
	cov, missing, err := chapterCoverage(g.ChapterMD, pdf)
	if err != nil {
		return ""
	}
	prev := gateCoverageStamp{}
	if data, err := os.ReadFile(g.coveragePath()); err == nil {
		_ = json.Unmarshal(data, &prev)
	}
	reject, checks := coverageDecision(cov, minChapterCoverage, prev.Checks)
	cv := gateCoverageStamp{Coverage: cov, Missing: missing, Checks: checks, At: nowStamp()}
	if err := os.MkdirAll(g.dir(), 0o755); err == nil {
		if data, merr := json.MarshalIndent(cv, "", "  "); merr == nil {
			_ = os.WriteFile(g.coveragePath(), data, 0o644)
		}
	}
	if !reject {
		// 达标，或第二次仍然不达标：都放行；后者由 submit 把这个缺口写进报告。
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "REJECTED: 内容和原稿对不上——这一章 md 的正文只有 %.0f%% 出现在你编译出的 PDF 文本里（低于 %.0f%%）。", cov*100, minChapterCoverage*100)
	b.WriteString("常见原因：拿脚本把 md 机械包成 tex，公式/表格/整段内容根本没进去，或者整段被漏掉。\n缺得最多的是这几段：\n")
	for _, l := range missing {
		b.WriteString("  - " + l + "\n")
	}
	b.WriteString("把缺的内容按手册补进 .tex（不要用脚本直接包），再 compile + view_pdf 对照原书页，然后 submit。")
	b.WriteString("\n如果你确认这些内容本来就该与原文不同（OCR 差异大、重复页眉页脚、公式被排成图），下一次 submit 仍会放行，")
	b.WriteString("请在 report.coverage_note 里写一句原因——它连同覆盖率数字一起记进本报告的未解问题，向上合并。")
	return b.String()
}

func (g *chapterGate) check() string {
	if !g.enabled() {
		return ""
	}
	var cs gateCompileStamp
	data, err := os.ReadFile(g.compilePath())
	if err != nil || json.Unmarshal(data, &cs) != nil {
		return "REJECTED: 这一章还没有编译过。先用 compile 编译 " + g.Base + "_wrapper.tex（或在 compile 里给 path），" +
			"确认 COMPILE OK 后用 view_pdf 看过结果，再 submit。"
	}
	if want := g.sourceDigest(); cs.Sources != want {
		return "REJECTED: 编译之后 .tex 或 parts/ 又改过了——现在这份还没编译。重新 compile，确认 OK 后再 submit。"
	}
	pdfPath := filepath.Join(g.WorkRoot, cs.PDF)
	if _, err := os.Stat(pdfPath); err != nil {
		return "REJECTED: 编译产物 " + cs.PDF + " 不见了。重新 compile 后再 submit。"
	}
	if hash, err := fileSHA256(pdfPath); err != nil || hash != cs.PDFHash {
		return "REJECTED: 编译产物 " + cs.PDF + " 与上次编译的记录不一致。重新 compile 后再 submit。"
	}
	var vs gateViewStamp
	vdata, verr := os.ReadFile(g.viewPath())
	if verr != nil || json.Unmarshal(vdata, &vs) != nil || vs.PDFHash != cs.PDFHash {
		return "REJECTED: 你还没有看过这次编译的结果。用 view_pdf {path: " + quoteGate(cs.PDF) + ", page: 1} 看过" +
			"（至少第 1 页；建议再看最后一页），确认版式与内容无误后再 submit。"
	}
	want := 1
	if cs.Pages >= 2 {
		want = 2
	}
	if len(vs.Pages) < want {
		return fmt.Sprintf("REJECTED: 这次编译的 PDF 一共 %d 页，你只看了 %d 页。至少看 %d 页（第 1 页与最后一页最容易看出问题）后再 submit。",
			cs.Pages, len(vs.Pages), want)
	}
	// 第二道（T168）：和**原书**对一遍。只看自己的产物等于自说自话——脚本
	// 机械转换漏掉整段内容时，产物内部毫无矛盾，只有和原书比才看得出来。
	// 例外：这次会话根本没有 source 挂载点（NoSourceMount）。那时它不是"偷懒
	// 不看原书"，而是**看不了**——要求它满足一个物理上做不到的条件，只会让
	// 会话在"提醒 / 复盘 / 截断"里空转。
	need := needSourcePages(cs.Pages)
	if g.NoSourceMount {
		if !g.noSourceNoted {
			g.noSourceNoted = true
			if g.Warn != nil {
				g.Warn(fmt.Sprintf("本次会话没有 source 挂载点（原书 PDF 视图没就绪），不要求对照原书（这一章 %d 页，原本要 %d 页原书页）", cs.Pages, need))
			}
		}
		return g.checkCoverage()
	}
	if have := len(g.sourceViews()); have < need {
		return fmt.Sprintf("REJECTED: 还没有和原书对照过。这一章的 PDF 有 %d 页，至少用 view_pdf 打开原书对应页 %d 页"+
			"（source:<原书 part>.pdf, page: N），逐段核对文字、公式、表格与插图顺序——你已经看了 %d 页。"+
			"用 list_source_pages / doc_search 定位本章在原书的页，把产物与原书比一遍再 submit。",
			cs.Pages, need, have)
	}
	return g.checkCoverage()
}

func quoteGate(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func nowStamp() string {
	return time.Now().Format("2006-01-02T15:04:05")
}

// pathWithin 判断 p 是否在 root 之内（用于区分"自己的产物"与"原书页"）。
func pathWithin(root, p string) bool {
	if root == "" {
		return false
	}
	absRoot, err1 := filepath.Abs(root)
	absP, err2 := filepath.Abs(p)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
