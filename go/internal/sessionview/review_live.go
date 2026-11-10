package sessionview

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"mineru-tools/internal/reviewpage"
)

// P64：图片核对页不能是"dedupe 收尾那一刻"的快照——那份静态页在 process 之前写，
// 上面的状态、等待转换、退回原图全是旧的，成品也还没生成。这里按**磁盘现状**现
// 渲染一份：groups.json 给分组，progress_items 给状态，figures/ 给成品，sessions/
// 与 tikz/ 给会话与代码。只读文件、不解码图片（缩略图是 dedupe 阶段留下的），
// 所以每次打开都重算得起。
//
// 页面里的相对地址都以 source/dedupe/ 为落点（与静态页同一套 ../ 口径），
// serveDedupeReview 会补 <base href="/media/<项目>/source/dedupe/">。

// liveImageProgress 是 progress_items 里一条判定记录里核对页要用的字段。
type liveImageProgress struct {
	MD             string `json:"md"`
	Img            string `json:"img"`
	Class          string `json:"class"`
	Status         string `json:"status"`
	FigPDF         string `json:"figure_pdf"`
	FigPNG         string `json:"figure_png"`
	FigSVG         string `json:"figure_svg"`
	EquivalentTo   string `json:"equivalent_to"`
	SharedTex      string `json:"shared_tex"`
	FallbackReason string `json:"fallback_reason"`
	Error          string `json:"error"`
	Absorbed       bool   `json:"absorbed"`
}

// liveDedupeGroups 是 groups.json 里核对页要用的子集（其余字段不读）。
type liveDedupeGroups struct {
	Fingerprint string `json:"fingerprint"`
	Refs        int    `json:"refs"`
	Distinct    int    `json:"distinct"`
	Rules       struct {
		AlignMin   float64 `json:"align_min"`
		AlignShift float64 `json:"align_shift"`
		IconAuto   float64 `json:"icon_auto"`
	} `json:"rules"`
	Groups []struct {
		KeyImage string `json:"key_image"`
		Class    string `json:"class"`
		Source   string `json:"source"`
		Members  []struct {
			Ref        string   `json:"ref"`
			Image      string   `json:"image"`
			Hash       string   `json:"hash"`
			Class      string   `json:"class"`
			AlignScore float64  `json:"align_score"`
			Duplicates []string `json:"duplicates"`
		} `json:"members"`
	} `json:"groups"`
	Independent []struct {
		Ref   string `json:"ref"`
		Image string `json:"image"`
		Hash  string `json:"hash"`
		Class string `json:"class"`
	} `json:"independent"`
}

// renderLiveImageReview 按磁盘现状渲染核对页。返回 false 只表示这个项目**一张图
// 都没有**（图片阶段还没跑），调用方退回静态页或 404。
//
// T305：以前没有 `groups.json` 就直接 false——于是"整组核验跑完但一张都没并组"
// 的项目，页面上既没有阶段行按钮、`/dedupe?project=` 也是 404，等于图片核对页
// 没有入口。分组结论只是这个页面的一部分，**有图片记录就该能看**：没有结论时
// 分组为空、每张图按"未分组"列出来。
func renderLiveImageReview(sourceDir string) ([]byte, bool) {
	prog := loadLiveProgress(sourceDir)
	var gf liveDedupeGroups
	haveGroups := false
	if data, err := os.ReadFile(filepath.Join(sourceDir, "dedupe", "groups.json")); err == nil {
		if json.Unmarshal(data, &gf) == nil {
			haveGroups = true
		}
	}
	// 分组结论文档在、但里面一组、一张独立图都没有：等于没有结论，同样按未分组列。
	if haveGroups && len(gf.Groups) == 0 && len(gf.Independent) == 0 {
		haveGroups = false
	}
	if !haveGroups && len(prog) == 0 {
		return nil, false
	}
	page := reviewpageRig{source: sourceDir, prog: prog}

	doc := reviewpage.Doc{
		Title: "图片核对（按磁盘现状）",
		Intro: "每次打开都按磁盘重算：状态取 progress_items，成品取 figures/，分组取 groups.json。左原图、右成品并排；点任意一张放大。",
		Rules: page.rulesText(gf),
		Artifacts: "分组结论 dedupe/groups.json、逐图判定 progress_items/、成品 figures/、" +
			"LaTeX 源 tikz/、作图会话 sessions/vector_*.jsonl",
	}

	for i, g := range gf.Groups {
		if strings.TrimSpace(g.KeyImage) == "" || len(g.Members) == 0 {
			continue
		}
		src := mapLiveSource(g.Source)
		group := reviewpage.Group{No: i + 1, Count: len(g.Members), Class: g.Class, Source: src}
		minScore, have := 1.0, false
		for _, m := range g.Members {
			if m.AlignScore > 0 && m.AlignScore < minScore {
				minScore, have = m.AlignScore, true
			}
		}
		if have {
			group.Min = "组内最低分 " + trimFloat(minScore)
		}
		for _, m := range g.Members {
			it := page.item(m.Image, m.Hash)
			if it.Name == "" {
				continue
			}
			if m.Ref == g.KeyImage || m.Image == g.KeyImage {
				it.Key = true
				if n := len(m.Duplicates); n > 0 {
					it.Note = strings.TrimSpace(it.Note + " · 逐字节副本 " + itoa(n))
				}
				group.Key = it
				continue
			}
			if m.AlignScore > 0 {
				it.Note = strings.TrimSpace(it.Note + " · 与代表图 " + trimFloat(m.AlignScore))
			}
			group.Members = append(group.Members, it)
		}
		if group.Key.Name == "" {
			// 代表图不在 members 里（旧产物）：拿第一条当代表，别丢整组。
			for _, m := range group.Members {
				group.Key = m
				break
			}
		}
		if group.Key.Name == "" {
			continue
		}
		doc.Groups = append(doc.Groups, group)
	}

	for _, it := range gf.Independent {
		item := page.item(it.Image, it.Hash)
		if item.Name != "" {
			doc.Independent = append(doc.Independent, item)
		}
	}

	doc.Pending, doc.Degraded = page.looseSets()
	if !haveGroups {
		// 没有分组结论（整组核验没跑，或跑完一张都没并组）：所有图都列成"未分组"，
		// 页面仍然可用——这是"看图片"这件事本身，不该依赖 dedupe 的结论。
		doc.Rules = "还没有分组结论：下面的图全部按未分组列出（整组核验跑完、或一张都没并组时就是这样）。"
		doc.Stats = page.statsTextNoGroups(prog)
		doc.Independent = nil
		seen := map[string]bool{}
		keys := make([]string, 0, len(prog))
		for k := range prog {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			if it := page.item(k, ""); it.Name != "" {
				doc.Independent = append(doc.Independent, it)
			}
		}
	} else {
		doc.Stats = page.statsText(gf, prog)
	}
	return []byte(reviewpage.Page(doc)), true
}

// reviewpageRig 是"按磁盘事实造卡片"的小工具，把路径推断集中在一处。
type reviewpageRig struct {
	source string
	prog   map[string]*liveImageProgress // key: img 相对路径
}

// item 把一张图造成页面上的一张卡片。hash 来自 groups.json（没有就只给原图）。
func (r reviewpageRig) item(imgRel, hash string) reviewpage.Item {
	imgRel = strings.TrimSpace(filepath.ToSlash(imgRel))
	if imgRel == "" {
		return reviewpage.Item{}
	}
	p := r.prog[imgRel]
	it := reviewpage.Item{Name: filepath.Base(imgRel), Ref: imgRel, Image: imgRel}
	if t := reviewpage.ThumbName(hash); t != "" {
		if fileExists(filepath.Join(r.source, "dedupe", filepath.FromSlash(t))) {
			it.Thumb = t
		}
	}
	if p == nil {
		// 没有判定记录（还没走到 process）：状态就是"未处理"，不给假信息。
		return it
	}
	it.Status = reviewpage.StatusLabel(p.Status)
	if p.Class != "" {
		it.Note = p.Class
	}
	base := figureStem(p.FigPNG)
	if base == "" {
		base = figureStem(p.FigPDF)
	}
	// 只有 status=done 才认成品：重画失败时旧 figures/tikz 文件不会被删除，
	// 把它们当"当前成品"显示就是在给一张退回原图的图配一张旧图。
	if base != "" && p.Status == reviewpage.StatusDone {
		if png := "figures/" + base + ".png"; fileExists(filepath.Join(r.source, "figures", base+".png")) {
			it.Figure = "../" + reviewpage.EscapeURLPath(png)
		}
	}
	if p.EquivalentTo != "" && p.SharedTex != "" {
		it.Note = joinNote(it.Note, "复用代表图成品（"+filepath.Base(p.EquivalentTo)+"）")
	} else if p.EquivalentTo != "" {
		it.Note = joinNote(it.Note, "跟随代表图 "+filepath.Base(p.EquivalentTo))
	}
	if p.Status == reviewpage.StatusFallback {
		if reason := strings.TrimSpace(p.FallbackReason); reason != "" {
			it.Note = joinNote(it.Note, reason)
		}
	}
	// T306：分类失败会把原因写进 error（Class 为空）。页面上要能直接看到
	// "为什么这张没结论"，否则只看到一个"失败"没法判断是接口还是图的问题。
	if reason := strings.TrimSpace(p.Error); reason != "" {
		if len([]rune(reason)) > 80 {
			reason = string([]rune(reason)[:80]) + "…"
		}
		if strings.TrimSpace(p.Class) == "" {
			reason = "未分类：" + reason
		}
		it.Note = joinNote(it.Note, reason)
	}
	it.Links = r.links(p, base)
	return it
}

// links 是这张图的验收材料入口：判定记录、LaTeX 源、作图会话、成品文件——存在
// 才给（不造假链接）。
func (r reviewpageRig) links(p *liveImageProgress, base string) []reviewpage.Link {
	var out []reviewpage.Link
	if p.MD != "" && p.Img != "" {
		rel := filepath.ToSlash(filepath.Join("progress_items", reviewpage.SanitizeName(p.MD), reviewpage.SanitizeName(p.Img)+".json"))
		if fileExists(filepath.Join(r.source, filepath.FromSlash(rel))) {
			out = append(out, reviewpage.Link{Label: "判定记录", Href: "../" + reviewpage.EscapeURLPath(rel)})
		}
	}
	done := p.Status == reviewpage.StatusDone
	if base != "" {
		// 成品类入口只在 status=done 时给：失败重画留下的旧 tikz/figures 不是
		// 这一版产物，给出去就成了"看起来画好了"。
		if done {
			if tex := filepath.ToSlash(filepath.Join("tikz", base+".tex")); fileExists(filepath.Join(r.source, filepath.FromSlash(tex))) {
				out = append(out, reviewpage.Link{Label: "LaTeX 源", Href: "../" + reviewpage.EscapeURLPath(tex)})
			}
		}
		// 会话转录总在（失败的那些也在），核对时要能点进那条会话。
		if tr := filepath.ToSlash(filepath.Join("sessions", "vector_"+base+".jsonl")); fileExists(filepath.Join(r.source, filepath.FromSlash(tr))) {
			out = append(out, reviewpage.Link{Label: "会话", Href: "../" + reviewpage.EscapeURLPath(tr)})
		}
	}
	if !done {
		return out
	}
	for _, f := range []struct{ label, rel string }{
		{"PDF", p.FigPDF},
		{"PNG", p.FigPNG},
		{"SVG", p.FigSVG},
	} {
		rel := strings.TrimSpace(filepath.ToSlash(f.rel))
		if rel == "" || !fileExists(filepath.Join(r.source, filepath.FromSlash(rel))) {
			continue
		}
		out = append(out, reviewpage.Link{Label: f.label, Href: "../" + reviewpage.EscapeURLPath(rel)})
	}
	return out
}

// looseSets 挑出"还没收尾"与"退回原图"两批：这正是核对时要盯的两类。
func (r reviewpageRig) looseSets() (pending, degraded []reviewpage.Item) {
	keys := make([]string, 0, len(r.prog))
	for k := range r.prog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := r.prog[k]
		if p == nil || p.Absorbed {
			continue
		}
		it := r.item(k, "")
		if it.Name == "" {
			continue
		}
		switch {
		case reviewpage.StatusSettled(p.Status):
			if reviewpage.StatusQuality(p.Status) == "degraded" {
				degraded = append(degraded, it)
			}
		default:
			pending = append(pending, it)
		}
	}
	return pending, degraded
}

func (r reviewpageRig) rulesText(gf liveDedupeGroups) string {
	return "判据：与组中心重合 ≥ " + trimFloat(gf.Rules.AlignMin) +
		"、平移 " + trimFloat(gf.Rules.AlignShift*100) + "%、本地自动定组 " + trimFloat(gf.Rules.IconAuto) +
		"（0 = 关闭）；结论指纹 " + orDash(gf.Fingerprint)
}

func (r reviewpageRig) statsText(gf liveDedupeGroups, prog map[string]*liveImageProgress) string {
	var settled, ok, fallback, pending, errs int
	for _, p := range prog {
		switch {
		case reviewpage.StatusSettled(p.Status):
			settled++
			if p.Status == reviewpage.StatusDone {
				ok++
			} else {
				fallback++
			}
		case p.Status == reviewpage.StatusError:
			errs++
		default:
			pending++
		}
	}
	return "引用 " + itoa(gf.Refs) + " 条 → 不同内容 " + itoa(gf.Distinct) + " 份、分组 " + itoa(len(gf.Groups)) +
		"、" + "独立 " + itoa(len(gf.Independent)) + " 份；全书 " + itoa(settled) + "/" + itoa(len(prog)) +
		" 已收尾（画好 " + itoa(ok) + "、退回原图 " + itoa(fallback) + "、失败 " + itoa(errs) + "、未处理 " + itoa(pending) + "）"
}

// statsTextNoGroups 是没有分组结论时的一行统计：只报"有多少张图、各自什么状态"，
// 不编造 refs/distinct/分组数（那些数来自 groups.json，没有就是没有）。
func (r reviewpageRig) statsTextNoGroups(prog map[string]*liveImageProgress) string {
	var settled, ok, fallback, pending, errs int
	for _, p := range prog {
		switch {
		case reviewpage.StatusSettled(p.Status):
			settled++
			if p.Status == reviewpage.StatusDone {
				ok++
			} else {
				fallback++
			}
		case p.Status == reviewpage.StatusError:
			errs++
		default:
			pending++
		}
	}
	return "共 " + itoa(len(prog)) + " 张图；已收尾 " + itoa(settled) + "（画好 " + itoa(ok) +
		"、退回原图 " + itoa(fallback) + "）、失败 " + itoa(errs) + "、未处理 " + itoa(pending) +
		"；还没有分组结论"
}

// loadLiveProgress 读一遍 progress_items（每个 md 一个子目录、每图一个 json）。
// 读不到的条目跳过：核对页宁可少一行，也不能整体打不开。
func loadLiveProgress(sourceDir string) map[string]*liveImageProgress {
	out := map[string]*liveImageProgress{}
	root := filepath.Join(sourceDir, "progress_items")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		sub := filepath.Join(root, d.Name())
		files, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(sub, f.Name()))
			if err != nil {
				continue
			}
			var p liveImageProgress
			if json.Unmarshal(data, &p) != nil || strings.TrimSpace(p.Img) == "" {
				continue
			}
			key := filepath.ToSlash(strings.TrimSpace(p.Img))
			out[key] = &p
		}
	}
	return out
}

func mapLiveSource(src string) string {
	switch src {
	case "auto":
		return "本地自动（≥阈值）"
	case "session":
		return "核验会话"
	case "attached":
		return "挂靠已有组"
	case "folded":
		return "组间收口"
	case "":
		return "核验会话"
	}
	return src
}

// figureStem 从 figures/<名>.png|pdf 取出 <名>：LaTeX 源与作图会话都以它命名。
func figureStem(rel string) string {
	base := filepath.Base(strings.TrimSpace(filepath.ToSlash(rel)))
	if base == "" || base == "." {
		return ""
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func joinNote(note, extra string) string {
	switch {
	case strings.TrimSpace(note) == "":
		return extra
	case strings.TrimSpace(extra) == "":
		return note
	}
	return note + " · " + extra
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

func itoa(n int) string { return strconv.Itoa(n) }

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
