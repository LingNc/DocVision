package latex

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// T150：dedupe 的结论不能只有 JSON 和 CSV——分组对不对，最终得看图（用户：
// 「有没有在 dedupe 里面链接一下选过的图片，我想手动看一下」）。
//
// T154：复查页只写一次 source/dedupe/review.html，会话预览页（/dedupe?project=…）
// 直接把这份文件发出去、只补一个 <base href="/media/<项目>/source/dedupe/">，
// 于是同一份页面在两处都能用，图片与缩略图都是**链接**而不是 base64：
//
//	· 原图：相对路径 ../<图片相对路径>（页面在 source/dedupe/ 下）；
//	· 缩略图：source/dedupe/thumbs/<内容摘要前 12 位>.jpg（缺了就退回原图）。
//
// 分组是折叠的（默认收起）：一本书动辄几十组、单组上百张，全展开既慢又难扫。
// 每张缩略图点开是页内灯箱（滚轮/双指缩放、拖动平移、再点一下关闭），不再靠
// 浏览器跳转——file:// 下直接导航原图经常打不开，也回不来。
//
// 派生产物，写失败只警告（结论仍在 groups.json / progress 里）。

// motifReviewThumbWidth 是复查页缩略图的宽度（像素）。原图宽于它的等比缩小。
// 灯箱看的是原图，缩略图只求列表里看得清轮廓，220 够用且体积小。
const motifReviewThumbWidth = 220

// motifReviewProbeBudget 是复查页"补算分数"的记账上限：绝大多数对已经从
// pairs.csv 那一步拿到分数了，只有预算/前筛挡掉的对才需要补算，这里再给一个
// 小额度，避免复查页自己变成第二个关键路径。
const motifReviewProbeBudget = 200_000_000

// motifReviewProbe 按需要取两张图的重合度：先查已算过的分数表，没有再补算，
// 补算按 motifAlignCost 记账。
type motifReviewProbe struct {
	scores map[string]float64
	shift  int
	budget int
}

func (p *motifReviewProbe) of(a, b *motifCandidate) (float64, bool) {
	if p == nil || a == nil || b == nil || a.task == nil || b.task == nil {
		return 0, false
	}
	ka, kb := a.task.key(), b.task.key()
	if v, ok := p.scores[motifPairRef(ka, kb)]; ok {
		return v, true
	}
	ca, oka := a.coverage()
	cb, okb := b.coverage()
	if !oka || !okb {
		return 0, false
	}
	cost := motifAlignCost(ca, cb)
	if cost > p.budget {
		return 0, false
	}
	p.budget -= cost
	v := ca.overlap(cb, p.shift)
	if p.scores == nil {
		p.scores = map[string]float64{}
	}
	p.scores[motifPairRef(ka, kb)] = v
	p.scores[motifPairRef(kb, ka)] = v
	return v, true
}

// motifReviewItem 是复查页上的一个格子。
type motifReviewItem struct {
	Name  string // 文件名（同一页里够区分）
	Ref   string // 完整引用，悬浮说明用
	Image string // 相对 source/ 的图片路径（与 md 里的引用同一写法）
	Thumb string // 相对 source/dedupe/ 的缩略图路径；空 = 没有缩略图，直接用原图
	Note  string // 图下面那行小字（分数 / 副本数）
	Key   bool   // 代表图
}

// motifReviewGroup 是复查页上的一个折叠分组。
type motifReviewGroup struct {
	No      int
	Count   int
	Class   string
	Source  string
	Min     string
	Key     motifReviewItem
	Members []motifReviewItem
}

// motifReviewDoc 是复查页要显示的全部内容，与"用什么地址取图"无关：落盘页与
// 会话预览页共用这一份数据结构和同一个渲染器。
type motifReviewDoc struct {
	Title       string
	Intro       string
	Rules       string
	Stats       string
	Artifacts   string
	Groups      []motifReviewGroup
	Independent []motifReviewItem
}

// renderMotifReviewHTML 渲染复查页。分组一律折叠、图片一律走相对 URL（不内嵌
// base64），灯箱在页内。
func renderMotifReviewHTML(doc motifReviewDoc) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>\n%s\n</style>\n</head>\n<body>\n", html.EscapeString(doc.Title), reviewStyle)
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(doc.Title))
	if doc.Intro != "" {
		fmt.Fprintf(&b, "<p class=\"intro\">%s</p>\n", html.EscapeString(doc.Intro))
	}
	b.WriteString("<ul class=\"meta\">\n")
	for _, line := range []string{doc.Rules, doc.Stats, doc.Artifacts} {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(&b, "<li>%s</li>\n", line)
	}
	b.WriteString("</ul>\n")

	b.WriteString("<div class=\"bar\"><h2>分组</h2><span class=\"spacer\"></span>")
	b.WriteString("<button type=\"button\" class=\"gb\" data-all=\"open\">全部展开</button>")
	b.WriteString("<button type=\"button\" class=\"gb\" data-all=\"close\">全部收起</button>")
	b.WriteString("</div>\n")
	if len(doc.Groups) == 0 {
		b.WriteString("<p class=\"empty\">没有分组（全部独立）。</p>\n")
	}
	for _, g := range doc.Groups {
		src := g.Source
		fmt.Fprintf(&b, "<details class=\"group\" data-no=\"%d\">\n<summary class=\"ghead\">", g.No)
		fmt.Fprintf(&b, "<span class=\"no\">#%d</span><span class=\"cnt\">%d 张</span>", g.No, g.Count)
		if g.Class != "" {
			fmt.Fprintf(&b, "<span class=\"chip\">%s</span>", html.EscapeString(g.Class))
		}
		if src != "" {
			fmt.Fprintf(&b, "<span class=\"chip src\">%s</span>", html.EscapeString(src))
		}
		if g.Min != "" {
			fmt.Fprintf(&b, "<span class=\"min\">%s</span>", html.EscapeString(g.Min))
		}
		fmt.Fprintf(&b, "<span class=\"name\">%s</span>", html.EscapeString(g.Key.Name))
		b.WriteString("</summary>\n<div class=\"gbody\">\n")
		b.WriteString("<div class=\"key\">")
		b.WriteString(motifReviewFigure(g.Key, "", "代表图 ★"))
		fmt.Fprintf(&b, "<div class=\"keystats\"><div class=\"kname\">%s</div><div class=\"sub\">%s</div></div>",
			html.EscapeString(g.Key.Name), html.EscapeString(g.Key.Note))
		b.WriteString("</div>\n<div class=\"members\">\n")
		for _, m := range g.Members {
			if m.Key {
				continue
			}
			b.WriteString(motifReviewFigure(m, "", m.Note))
			b.WriteString("\n")
		}
		b.WriteString("</div>\n</div>\n</details>\n")
	}

	b.WriteString("<h2>独立</h2>\n")
	if len(doc.Independent) == 0 {
		b.WriteString("<p class=\"empty\">没有独立图（全部进了分组）。</p>\n")
	}
	b.WriteString("<div class=\"indep\">\n")
	for _, it := range doc.Independent {
		b.WriteString(motifReviewFigure(it, it.Name, it.Note))
		b.WriteString("\n")
	}
	b.WriteString("</div>\n")

	b.WriteString(reviewLightboxHTML)
	fmt.Fprintf(&b, "<script>\n%s\n</script>\n</body>\n</html>\n", reviewScript)
	return b.String()
}

// motifReviewFigure 渲染一张缩略图：永远套一层指向原图的链接，点了开灯箱。
// name / note 各占一行——合成一句再靠 CSS 折行会把「0.648」这类数字从中间劈开。
func motifReviewFigure(it motifReviewItem, name, note string) string {
	full := html.EscapeString(motifReviewImageHref(it.Image))
	thumb := strings.TrimSpace(it.Thumb)
	if thumb != "" {
		thumb = escapeURLPath(thumb)
	}
	title := it.Ref
	if title == "" {
		title = it.Name
	}
	// 灯箱里原图取不到时退回缩略图（比如原图软链挂了）：页面不至于只剩个破图标。
	thumbAttr := ""
	if thumb != "" {
		thumbAttr = fmt.Sprintf(" data-thumb=\"%s\"", html.EscapeString(thumb))
	}
	var b strings.Builder
	b.WriteString("<figure>")
	fmt.Fprintf(&b, "<a class=\"shot\" href=\"%s\" data-full=\"%s\"%s title=\"%s  · 点开看大图\">",
		full, full, thumbAttr, html.EscapeString(title))
	if thumb == "" {
		// 没有缩略图就直接用原图（老跑批的产物没有 thumbs/，也不能出现空图）。
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"%s\" loading=\"lazy\" decoding=\"async\">", full, html.EscapeString(it.Name))
	} else {
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"%s\" loading=\"lazy\" decoding=\"async\" data-orig=\"%s\" onerror=\"this.onerror=null;this.src=this.dataset.orig\">",
			html.EscapeString(thumb), html.EscapeString(it.Name), full)
	}
	b.WriteString("</a>")
	if name != "" || note != "" {
		b.WriteString("<figcaption>")
		if name != "" {
			fmt.Fprintf(&b, "<span class=\"nm\">%s</span>", html.EscapeString(name))
		}
		if note != "" {
			fmt.Fprintf(&b, "<span class=\"nt\">%s</span>", html.EscapeString(note))
		}
		b.WriteString("</figcaption>")
	}
	b.WriteString("</figure>")
	return b.String()
}

// motifReviewThumbName 是缩略图的文件名：内容摘要前 12 位 + .jpg。groups.json
// 里每条结论都带完整的内容摘要，所以预览页不用额外存映射就能拼出同一个名字。
// 摘要太短（测试数据、旧产物）就不写缩略图，页面直接用原图。
func motifReviewThumbName(hash string) string {
	h := strings.TrimSpace(hash)
	if len(h) < 12 {
		return ""
	}
	return "thumbs/" + h[:12] + ".jpg"
}

// motifReviewThumb 是一份待写盘的缩略图。
type motifReviewThumb struct {
	Name string // 相对 source/dedupe/
	Data []byte
}

// writeMotifReview 把本次 dedupe 的分组写成 source/dedupe/review.html（+ 缩略图）。
func (r *Runner) writeMotifReview(dir string, leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, scores map[string]float64) error {
	probe := &motifReviewProbe{scores: scores, shift: r.motifAlignShift(), budget: motifReviewProbeBudget}
	doc, thumbs := r.motifReviewDocOf(leaders, copies, applied, probe)
	for _, t := range thumbs {
		if t.Name == "" || len(t.Data) == 0 {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(t.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, t.Data, 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review.html"), []byte(renderMotifReviewHTML(doc)), 0o644)
}

// motifReviewDocOf 从本次运行的分组里整理出复查页内容，并顺手编好缩略图字节。
func (r *Runner) motifReviewDocOf(leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, probe *motifReviewProbe) (motifReviewDoc, []motifReviewThumb) {
	rules := r.motifCurrentRules()
	thumbs := make([]motifReviewThumb, 0, len(leaders))
	seen := map[string]bool{}
	item := func(c *motifCandidate, note string) motifReviewItem {
		if c == nil || c.task == nil {
			return motifReviewItem{}
		}
		it := motifReviewItem{Name: motifShortName(c), Image: c.task.imgPath}
		if c.task != nil {
			it.Ref = c.task.key()
		}
		if name := motifReviewThumbName(c.sourceHash); name != "" && !seen[name] {
			if data, err := encodeJPEGBytes(scaleToWidth(c.image, motifReviewThumbWidth)); err == nil {
				seen[name] = true
				thumbs = append(thumbs, motifReviewThumb{Name: name, Data: data})
				it.Thumb = name
			}
		} else if name != "" {
			it.Thumb = name
		}
		it.Note = note
		return it
	}
	doc := motifReviewDoc{
		Title:     "dedupe 复查",
		Intro:     "分组默认收起，点开一组看大图；点缩略图在页内放大（滚轮/双指缩放、拖动平移、再点一下关闭）。",
		Artifacts: "逐对明细 <code>metrics/pairs.csv</code>、桶 <code>metrics/buckets.json</code>、汇总 <code>report.md</code>、结论 <code>groups.json</code>",
	}
	doc.Rules = fmt.Sprintf("判据：与组中心重合 ≥ %.2f、平移 %.0f%%、本地自动定组 %.2f（0 = 关闭）",
		rules.AlignMin, rules.AlignShift*100, rules.IconAuto)

	inGroup := map[*motifCandidate]bool{}
	for _, g := range applied {
		if g.Key == nil {
			continue
		}
		for _, m := range g.Members {
			inGroup[m] = true
		}
	}
	independent := make([]*motifCandidate, 0, len(leaders))
	for _, c := range leaders {
		if !inGroup[c] {
			independent = append(independent, c)
		}
	}
	refs := 0
	for _, c := range leaders {
		refs += 1 + len(copies[c])
	}
	doc.Stats = fmt.Sprintf("引用 %d 条 → 不同内容 %d 份、分组 %d 个、独立 %d 份", refs, len(leaders), len(applied), len(independent))

	origin := map[string]string{"auto": "本地自动（≥阈值）", "session": "核验会话", "attached": "挂靠已有组", "folded": "组间收口"}
	for i, g := range applied {
		if g.Key == nil {
			continue
		}
		src := origin[g.Source]
		if src == "" {
			src = g.Source
		}
		if src == "" {
			src = "核验会话"
		}
		key := item(g.Key, "")
		key.Key = true
		key.Note = fmt.Sprintf("%d 条引用 · %d 份副本", 1+len(copies[g.Key]), len(copies[g.Key]))
		group := motifReviewGroup{No: i + 1, Count: len(g.Members), Class: g.Key.progress.Class, Source: src, Key: key}
		minScore, have := 1.0, false
		for _, m := range g.Members {
			if m == g.Key {
				continue
			}
			if v, ok := probe.of(m, g.Key); ok && v < minScore {
				minScore, have = v, true
			}
		}
		if have {
			group.Min = fmt.Sprintf("组内最低分 %.3f", minScore)
		}
		members := append([]*motifCandidate{}, g.Members...)
		sort.SliceStable(members, func(x, y int) bool {
			if members[x] == g.Key {
				return true
			}
			if members[y] == g.Key {
				return false
			}
			vx, _ := probe.of(members[x], g.Key)
			vy, _ := probe.of(members[y], g.Key)
			return vx > vy
		})
		for _, m := range members {
			if m == g.Key {
				// 代表图单独一张大卡片，成员列表里不再重复。
				continue
			}
			note := "未测量"
			if v, ok := probe.of(m, g.Key); ok {
				note = fmt.Sprintf("%.3f", v)
			}
			if n := len(copies[m]); n > 0 {
				note = fmt.Sprintf("%s · 副本 %d", note, n)
			}
			group.Members = append(group.Members, item(m, note))
		}
		doc.Groups = append(doc.Groups, group)
	}

	for _, c := range independent {
		best, bestKey, have := 0.0, (*motifCandidate)(nil), false
		for _, g := range applied {
			if g.Key == nil || g.Key == c {
				continue
			}
			if v, ok := probe.of(c, g.Key); ok && (!have || v > best) {
				best, bestKey, have = v, g.Key, true
			}
		}
		note := "最像：未测量"
		if have {
			note = fmt.Sprintf("最像 %.3f（%s）", best, motifShortName(bestKey))
		}
		doc.Independent = append(doc.Independent, item(c, note))
	}
	return doc, thumbs
}

// motifReviewImageHref 是复查页到原图的相对链接：md 里的引用相对 source/，
// 复查页在 source/dedupe/，所以加一层 ../（反斜杠统一成正斜杠，浏览器才认）。
func motifReviewImageHref(imgPath string) string {
	return "../" + escapeURLPath(imgPath)
}

// escapeURLPath 按路径段转义：中文书名、空格、# 与 ? 都得转义，否则浏览器会
// 按另一条路径去取图（曾经直接拼原字符串，路径里有空格的书就全打不开）。
func escapeURLPath(p string) string {
	p = strings.TrimSpace(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// motifShortName 是复查页上显示的名字：只要文件名（同一页里够区分）。
func motifShortName(c *motifCandidate) string {
	if c == nil || c.task == nil {
		return ""
	}
	return filepath.Base(c.task.imgPath)
}
