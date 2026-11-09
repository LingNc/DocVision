package latex

import (
	"fmt"
	"mineru-tools/internal/reviewpage"
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

// 复查页的类型与渲染器都在 internal/reviewpage（会话预览页要按磁盘现状现渲染，
// 与 dedupe 收尾写静态页共用同一份）。这里只保留别名与"数据怎么来"。
// escapeURLPath 与核对页同一套路径转义（figures/、media 链接等多处要用）。
func escapeURLPath(p string) string { return reviewpage.EscapeURLPath(p) }

type motifReviewItem = reviewpage.Item
type motifReviewLink = reviewpage.Link
type motifReviewGroup = reviewpage.Group
type motifReviewDoc = reviewpage.Doc

// renderMotifReviewHTML 渲染复查页（分组默认收起、图片走相对 URL、灯箱在页内）。
func renderMotifReviewHTML(doc motifReviewDoc) string { return reviewpage.Page(doc) }

func motifReviewThumbName(hash string) string { return reviewpage.ThumbName(hash) }

// motifReviewThumb 是一份待写盘的缩略图。
type motifReviewThumb struct {
	Name string // 相对 source/dedupe/
	Data []byte
}

// writeMotifReview 把本次 dedupe 的分组写成 source/dedupe/review.html（+ 缩略图）。
func (r *Runner) writeMotifReview(dir string, leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, scores map[string]float64, prog map[string]*imageProgress) error {
	probe := &motifReviewProbe{scores: scores, shift: r.motifAlignShift(), budget: motifReviewProbeBudget}
	doc, thumbs := r.motifReviewDocOf(leaders, copies, applied, probe, prog)
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
func (r *Runner) motifReviewDocOf(leaders []*motifCandidate, copies map[*motifCandidate][]*motifCandidate, applied []motifAppliedGroup, probe *motifReviewProbe, prog map[string]*imageProgress) (motifReviewDoc, []motifReviewThumb) {
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
		if pr := c.progress; pr != nil {
			it.Status = imageStatusLabel(pr)
			it.Links = imageArtifactLinks(pr)
			if pr.Class != "" {
				note = strings.TrimSpace(pr.Class + " · " + note)
			}
		}
		it.Note = note
		return it
	}
	doc := motifReviewDoc{
		Title:     "dedupe 复查",
		Intro:     "分组默认收起，点开一组看大图；点缩略图在页内放大（滚轮/双指缩放、拖动平移、再点一下关闭）。这是整组核验那一刻的快照（那时还没转换）：状态与成品要看会话预览页里同一个项目的核对页，那里按磁盘现状重算、原图与成品并排。",
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
	doc.Pending, doc.Degraded = r.motifReviewLooseSets(prog, applied, item)
	return doc, thumbs
}

// motifReviewLooseSets 从 progress_items 里挑出两批图（P64）：还没收尾的
// （等待转换）与已收尾但退回原图的（fallback）。两批都直接给原图与判定记录，
// 因为核对比对时真正想看的就是这两类。
func (r *Runner) motifReviewLooseSets(prog map[string]*imageProgress, applied []motifAppliedGroup, item func(*motifCandidate, string) motifReviewItem) (pending, degraded []motifReviewItem) {
	inGroup := map[string]string{}
	for i, g := range applied {
		if g.Key == nil || g.Key.task == nil {
			continue
		}
		_ = i
		for _, m := range g.Members {
			if m.task != nil {
				inGroup[m.task.key()] = motifShortName(g.Key)
			}
		}
	}
	keys := make([]string, 0, len(prog))
	for k := range prog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := prog[k]
		if p == nil || p.Absorbed {
			continue
		}
		it := motifReviewItem{Name: filepath.Base(p.ImgPath), Ref: k, Image: p.ImgPath, Status: imageStatusLabel(p), Links: imageArtifactLinks(p)}
		if g, ok := inGroup[k]; ok {
			it.Note = "归组：" + g
		}
		switch {
		case imageStatusSettled(p.Status):
			if imageStatusQuality(p.Status) == "degraded" {
				// 退回原图的图必须说清为什么退：状态只说"没画出成品"，
				// 工具链缺失 / 校验没过 / 接口出错的处理方式完全不同。
				if r := strings.TrimSpace(p.FallbackReason); r != "" {
					it.Note = strings.TrimSpace(it.Note + " · " + r)
				}
				degraded = append(degraded, it)
			}
		default:
			if p.Class == "" {
				it.Note = "还没分类"
			}
			pending = append(pending, it)
		}
	}
	return pending, degraded
}

// imageStatusLabel 是给页面看的状态词：一律以 progress_items 的 status 为准，
// 空状态明确写成"未处理"，不留一个空白让读者自己猜。
func imageStatusLabel(p *imageProgress) string {
	if p == nil {
		return ""
	}
	switch p.Status {
	case ImageStatusDone:
		return "已完成"
	case ImageStatusFallback:
		return "退回原图"
	case ImageStatusError:
		return "失败"
	case ImageStatusClassified:
		return "已分类待处理"
	case "":
		return "未处理"
	}
	return p.Status
}

// imageArtifactLinks 列出一张图的验收材料：判定记录（progress_items 里的那条
// json）与转换产物（TikZ/PDF/PNG/SVG）。链接相对 source/dedupe/。
func imageArtifactLinks(p *imageProgress) []motifReviewLink {
	if p == nil {
		return nil
	}
	var out []motifReviewLink
	if p.MDName != "" && p.ImgPath != "" {
		rel := "../progress_items/" + reviewpage.EscapeURLPath(sanitizeName(p.MDName)) + "/" + reviewpage.EscapeURLPath(sanitizeName(p.ImgPath)) + ".json"
		out = append(out, motifReviewLink{Label: "判定记录", Href: rel})
	}
	// 转换结果与它的来源：PDF/PNG/SVG 在 figures/，LaTeX 源在 tikz/，作图会话的
	// 转录在 sessions/vector_<名>.jsonl——三样都能从 figures/<名>.pdf 推出来，
	// 所以只有真画过的图才有这几个入口。
	if base := figureBaseName(p.FigPDF); base != "" {
		out = append(out, motifReviewLink{Label: "LaTeX 源", Href: "../tikz/" + reviewpage.EscapeURLPath(base) + ".tex"})
		out = append(out, motifReviewLink{Label: "会话", Href: "../sessions/" + reviewpage.EscapeURLPath("vector_"+base+".jsonl")})
	}
	for _, f := range []struct{ label, href string }{
		{"PDF", p.FigPDF},
		{"PNG", p.FigPNG},
		{"SVG", p.FigSVG},
	} {
		if strings.TrimSpace(f.href) == "" {
			continue
		}
		h := f.href
		if !strings.HasPrefix(h, "http") {
			h = "../" + strings.TrimPrefix(filepath.ToSlash(h), "/")
		}
		out = append(out, motifReviewLink{Label: f.label, Href: h})
	}
	return out
}

// figureBaseName 从 figures/<名>.pdf 取出 <名>：LaTeX 源与作图转录都以它命名。
func figureBaseName(figPDF string) string {
	base := filepath.Base(strings.TrimSpace(filepath.ToSlash(figPDF)))
	if base == "" || base == "." {
		return ""
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// motifShortName 是复查页上显示的名字：只要文件名（同一页里够区分）。
func motifShortName(c *motifCandidate) string {
	if c == nil || c.task == nil {
		return ""
	}
	return filepath.Base(c.task.imgPath)
}
