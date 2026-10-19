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
}

type gateCompileStamp struct {
	Sources string `json:"sources"`
	PDF     string `json:"pdf"`
	PDFHash string `json:"pdf_hash"`
	Pages   int    `json:"pages"`
	At      string `json:"at"`
}

type gateViewStamp struct {
	PDFHash string `json:"pdf_hash"`
	Pages   []int  `json:"pages"`
	At      string `json:"at"`
}

func (g *chapterGate) dir() string {
	return filepath.Join(g.WorkRoot, ".gate")
}

func (g *chapterGate) compilePath() string { return filepath.Join(g.dir(), "compile.json") }
func (g *chapterGate) viewPath() string    { return filepath.Join(g.dir(), "view.json") }

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
	return ""
}

func quoteGate(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func nowStamp() string {
	return time.Now().Format("2006-01-02T15:04:05")
}
