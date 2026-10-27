package reviewpage

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strings"
)

// Link 是图下面的一枚材料入口（判定记录 / LaTeX 源 / 会话 / 成品）。
type Link struct {
	Label string
	Href  string
}

// Item 是页面上的一张图。Image 是原图（相对 source/），Thumb 是可选的缩略图，
// Figure 是可选的**成品**预览（相对 source/dedupe/，一般是 figures/<名>.png）。
// 原图与成品并排显示：核对时要看的就是"原来长什么样、画出来什么样"。
type Item struct {
	Name   string
	Ref    string
	Image  string
	Thumb  string
	Figure string
	Status string
	Note   string
	Links  []Link
	Key    bool
}

// Group 是一个折叠分组。
type Group struct {
	No      int
	Count   int
	Class   string
	Source  string
	Min     string
	Key     Item
	Members []Item
}

// Doc 是整页要显示的内容。
type Doc struct {
	Title       string
	Intro       string
	Rules       string
	Stats       string
	Artifacts   string
	Groups      []Group
	Independent []Item
	Pending     []Item
	Degraded    []Item
}

// Page 渲染整页：分组默认收起、图片一律走相对 URL（不内嵌 base64）、灯箱在页内。
func Page(doc Doc) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>\n%s\n</style>\n</head>\n<body>\n", html.EscapeString(doc.Title), Style)
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
		fmt.Fprintf(&b, "<details class=\"group\" data-no=\"%d\">\n<summary class=\"ghead\">", g.No)
		fmt.Fprintf(&b, "<span class=\"no\">#%d</span><span class=\"cnt\">%d 张</span>", g.No, g.Count)
		if g.Class != "" {
			fmt.Fprintf(&b, "<span class=\"chip\">%s</span>", html.EscapeString(g.Class))
		}
		if g.Source != "" {
			fmt.Fprintf(&b, "<span class=\"chip src\">%s</span>", html.EscapeString(g.Source))
		}
		if g.Min != "" {
			fmt.Fprintf(&b, "<span class=\"min\">%s</span>", html.EscapeString(g.Min))
		}
		fmt.Fprintf(&b, "<span class=\"name\">%s</span>", html.EscapeString(g.Key.Name))
		b.WriteString("</summary>\n<div class=\"gbody\">\n")
		b.WriteString("<div class=\"key\">")
		b.WriteString(Figure(g.Key))
		fmt.Fprintf(&b, "<div class=\"keystats\"><div class=\"kname\">%s</div><div class=\"sub\">%s</div></div>",
			html.EscapeString(g.Key.Name), html.EscapeString(g.Key.Note))
		b.WriteString("</div>\n<div class=\"members\">\n")
		for _, m := range g.Members {
			if m.Key {
				continue
			}
			b.WriteString(Figure(m))
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
		b.WriteString(Figure(it))
		b.WriteString("\n")
	}
	b.WriteString("</div>\n")

	Section(&b, "等待转换的原图", "这些图在 progress_items 里还没有收尾结论（未处理或上次失败），转换阶段会给它们出成品；点原图看大图。", doc.Pending)
	Section(&b, "退回原图的（已收尾但没画出成品）", "status=fallback：这一轮以原图交付，不再重画。核对时重点看这些，图注里写了退回原因。", doc.Degraded)

	b.WriteString(LightboxHTML)
	fmt.Fprintf(&b, "<script>\n%s\n</script>\n</body>\n</html>\n", Script)
	return b.String()
}

// Section 渲染一段按状态挑出来的图。空段不渲染：没有这类图时不该出现空标题。
func Section(b *strings.Builder, title, intro string, items []Item) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "<h2>%s（%d 张）</h2>\n", html.EscapeString(title), len(items))
	if intro != "" {
		fmt.Fprintf(b, "<p class=\"intro\">%s</p>\n", html.EscapeString(intro))
	}
	b.WriteString("<div class=\"indep\">\n")
	for _, it := range items {
		b.WriteString(Figure(it))
		b.WriteString("\n")
	}
	b.WriteString("</div>\n")
}

// Figure 渲染一张卡片：原图永远在，成品（Figure）有就并排放在右边。两个图各自
// 可点开灯箱——核对就是来回比这两张。
func Figure(it Item) string {
	var b strings.Builder
	b.WriteString("<figure>")
	b.WriteString(pairShot(it, ImageHref(it.Image), strings.TrimSpace(it.Thumb), it.Name, "原图"))
	if fig := strings.TrimSpace(it.Figure); fig != "" {
		b.WriteString(pairShot(it, fig, "", it.Name, "成品"))
	}
	if it.Name != "" || it.Note != "" || it.Status != "" || len(it.Links) > 0 {
		b.WriteString("<figcaption>")
		if it.Name != "" {
			fmt.Fprintf(&b, "<span class=\"nm\">%s</span>", html.EscapeString(it.Name))
		}
		if it.Status != "" {
			fmt.Fprintf(&b, "<span class=\"st\">%s</span>", html.EscapeString(it.Status))
		}
		if it.Note != "" {
			fmt.Fprintf(&b, "<span class=\"nt\">%s</span>", html.EscapeString(it.Note))
		}
		if len(it.Links) > 0 {
			b.WriteString("<span class=\"lk\">")
			for i, l := range it.Links {
				if i > 0 {
					b.WriteString(" · ")
				}
				fmt.Fprintf(&b, "<a href=\"%s\" target=\"_blank\">%s</a>",
					html.EscapeString(l.Href), html.EscapeString(l.Label))
			}
			b.WriteString("</span>")
		}
		b.WriteString("</figcaption>")
	}
	b.WriteString("</figure>")
	return b.String()
}

// pairShot 是一张可点开放大的图。thumb 非空且 href 指向原图时，缩略图加载失败会
// 自动退回原图（原图软链挂了也不至于只剩破图标）。
func pairShot(it Item, href, thumb, name, kind string) string {
	title := it.Ref
	if title == "" {
		title = it.Name
	}
	var b strings.Builder
	thumbAttr := ""
	if thumb != "" {
		thumbAttr = fmt.Sprintf(" data-thumb=\"%s\"", html.EscapeString(EscapeURLPath(thumb)))
	}
	fmt.Fprintf(&b, "<a class=\"shot %s\" href=\"%s\" data-full=\"%s\"%s title=\"%s · %s · 点开看大图\">",
		kindClass(kind), href, href, thumbAttr, html.EscapeString(title), html.EscapeString(kind))
	if thumb == "" || kind == "成品" {
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"%s %s\" loading=\"lazy\" decoding=\"async\">", href, html.EscapeString(name), html.EscapeString(kind))
	} else {
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"%s\" loading=\"lazy\" decoding=\"async\" data-orig=\"%s\" onerror=\"this.onerror=null;this.src=this.dataset.orig\">",
			html.EscapeString(EscapeURLPath(thumb)), html.EscapeString(name), href)
	}
	b.WriteString("</a>")
	return b.String()
}

func kindClass(kind string) string {
	if kind == "成品" {
		return "conv"
	}
	return "orig"
}

// ImageHref 是页面到原图的相对链接：md 里的引用相对 source/，页面在
// source/dedupe/，所以加一层 ../（反斜杠统一成正斜杠，浏览器才认）。
func ImageHref(imgPath string) string {
	return "../" + EscapeURLPath(imgPath)
}

// EscapeURLPath 按路径段转义：中文、空格、# 与 ? 都得转义，否则浏览器会按另一
// 条路径去取文件。
func EscapeURLPath(p string) string {
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

// ThumbName 是缩略图的文件名：内容摘要前 12 位 + .jpg。摘要太短（测试数据、旧
// 产物）就返回空，页面直接用原图。
func ThumbName(hash string) string {
	h := strings.TrimSpace(hash)
	if len(h) < 12 {
		return ""
	}
	return "thumbs/" + h[:12] + ".jpg"
}

// SanitizeName 与 progress_items 的落盘命名同一口径：路径分隔符等不安全字符
// 换成下划线，最长 80 字节。核对页要按它拼出那张图的 json 路径。
func SanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case isSafeLetter(r):
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x4E00 && r <= 0x9FFF:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._")
	if out == "" {
		out = "img"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

// isSafeLetter 与 latex 那边的口径一致：只放行 ASCII 字母（CJK 单独处理，
// 这样生成的图片名仍然可读）。
func isSafeLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// 图片状态的词表（progress_items 的 status 字段决定一切）：product 侧写的是
// done / fallback / error / classified，raster 那类没画出成品也算收尾。核对页与
// latex 的进度统计共用这一份判断，避免"页面说未处理、进度行说做完了"。
const (
	StatusDone       = "done"
	StatusFallback   = "fallback"
	StatusError      = "error"
	StatusClassified = "classified"
)

// StatusSettled 报这一张是不是收尾了（不论好坏）。
func StatusSettled(status string) bool {
	return status == StatusDone || status == StatusFallback
}

// StatusQuality 把状态压成三档给页面用：ok / degraded / pending。
func StatusQuality(status string) string {
	switch status {
	case StatusDone:
		return "ok"
	case StatusFallback:
		return "degraded"
	}
	return "pending"
}

// StatusLabel 是给人看的词：一律以 status 为准，空状态明确写"未处理"。
func StatusLabel(status string) string {
	switch status {
	case StatusDone:
		return "已完成"
	case StatusFallback:
		return "退回原图"
	case StatusError:
		return "失败"
	case StatusClassified:
		return "已分类待处理"
	case "":
		return "未处理"
	}
	return status
}
