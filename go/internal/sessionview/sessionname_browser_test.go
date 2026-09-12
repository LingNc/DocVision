package sessionview

import (
	"html"
	"strings"
	"testing"
)

// 会话名里的 Markdown：名字来自转录文件名（或图注），单行显示，所以渲染必须
// **只走行内规则**——`code` / **粗** / _斜_ 变成对应元素，块级元素一个都不许
// 出现；没有标记的名字输出与改动前逐字一致；搜索仍按原始文本匹配。
//
// 夹具里有一个真实文件名带标记的会话（`*`、反引号本来就是合法文件名字符）、
// 一个只有图注带标记的逐图会话，以及三个**纯文本**对照会话（含 `<` / `&` /
// 落单的 `*`、反引号——它们必须继续按文本处理）。

// 反引号在 Go 的原始字符串里写不出来，夹具的文件名又必须有它。
const tick = "`"

// 标记会话：`**粗体**`、`_斜体_`、反引号 code 三样都在**文件名**里。
var (
	markedStem  = "convert_这是_斜体_与**粗体**与" + tick + "code" + tick + "的会话"
	markedFile  = markedStem + ".jsonl"
	markedTitle = "转换 · 这是_斜体_与**粗体**与" + tick + "code" + tick + "的会话"
	// 渲染后的可见文本：标记符号本身一个都不剩。
	markedText = "转换 · 这是斜体与粗体与code的会话"
)

// 危险字符会话：`<` / `>` / `&` / 落单的 `*` 与反引号——都不是标记，必须是文本。
// （文件名里不可能有 `/`，所以"闭合标签"那种形态只能写成 `<b>x>`。）
var (
	riskyStem  = "convert_<b>x>y&z*单" + tick + "反引号"
	riskyFile  = riskyStem + ".jsonl"
	riskyTitle = "转换 · <b>x>y&z*单" + tick + "反引号"
)

// 纯文本对照会话（真实工程里的命名形态：阶段 + 单下划线的章号）。
const (
	plainFile  = "convert_part1.jsonl"
	plainTitle = "转换 · part1"
	realFile   = "checker_chapter_01.jsonl"
	realTitle  = "核对 · chapter_01"
)

// 逐图会话：标题走「图注 → 可读标签 → 短写哈希」的第一档，图注里带标记。
const (
	imgSessionHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	imgSessionStem  = "vector_book__" + imgSessionHash + "__label"
	imgSessionFile  = imgSessionStem + ".jsonl"
	imgSessionTitle = "矢量图 · 如图 " + tick + "x" + tick + " 与 **粗**"
	imgSessionText  = "矢量图 · 如图 x 与 粗"
)

// sessionNameFixture 造一棵扫描根：一个项目、五个会话、一条带图注的 doc_index。
func sessionNameFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	body := transcript(
		`{"t":"msg","role":"user","text":"看一下这一章。"}`,
		`{"t":"msg","role":"assistant","text":"正文里有 `+tick+`inline`+tick+` 这样的行内代码。"}`,
	)
	// 阶段目录用 work/sessions（与真实工程一致），逐图会话用 source/sessions。
	for _, name := range []string{markedFile, riskyFile, plainFile, realFile} {
		writeFile(t, jsonl(root, "proj", "work", "sessions", name), body)
	}
	// 逐图会话 + 它的 doc_index 图注（图注里的标记同样要渲染）。
	writeFile(t, jsonl(root, "proj", "source", "sessions", imgSessionFile), transcript(
		`{"t":"msg","role":"user","text":"把这张图重画。"}`,
	))
	writeFile(t, jsonl(root, "proj", "doc_index", "doc_index.json"),
		`{"entries":[{"seq":1,"page":3,"type":"image","img":"images/book/`+imgSessionHash+`.jpg","text":"如图 `+
			tick+`x`+tick+` 与 **粗**"}]}`)
	return root
}

// sessionNameProbeJS 采集三处显示位置（侧栏行 / 面包屑 / 详情栏）的**结构**
// 与可见文本、悬浮说明、行内 code 的最终样式。
//
// 只采 textContent 与标签名（不采 outerHTML）：探针的正文会再被序列化一次，
// 采 HTML 源码就要来回转义，而"结构 + 文本"已经足以钉住"没有块级元素""标记
// 变成了 <strong>/<code>""纯文本名字就是一个文本节点"这几件事。
const sessionNameProbeJS = `
      var out = [];
      var push = function (k, v) { out.push(k + '=' + String(v).replace(/\s+/g, ' ').trim()); };
      var BLOCK = /^(P|DIV|H1|H2|H3|H4|H5|H6|UL|OL|LI|BR|HR|TABLE|SECTION|PRE|BLOCKQUOTE|DL|DT|DD)$/;
      var tagsIn = function (node) {
        if (!node) { return 'no-node'; }
        var hit = [];
        Array.prototype.forEach.call(node.querySelectorAll('*'), function (n) { hit.push(n.tagName); });
        return hit.length ? hit.join(',') : 'none';
      };
      var blocksIn = function (node) {
        if (!node) { return 'no-node'; }
        var hit = [];
        Array.prototype.forEach.call(node.querySelectorAll('*'), function (n) {
          if (BLOCK.test(n.tagName)) { hit.push(n.tagName); }
        });
        return hit.length ? hit.join(',') : 'none';
      };
      var rowOf = function (suffix) {
        return document.querySelector('.session-row[data-id$="' + suffix + '"]');
      };
      var titleOf = function (suffix) {
        var r = rowOf(suffix);
        return r ? r.querySelector('.row-title') : null;
      };
      var styleOf = function (sel) {
        var n = document.querySelector(sel);
        if (!n) { return 'no-node'; }
        var cs = getComputedStyle(n);
        return cs.fontFamily + ' / ' + cs.fontSize + ' / ' + cs.lineHeight;
      };
      var marked = rowOf('@MARK@');
      var mt = marked ? marked.querySelector('.row-title') : null;
      push('markedText', mt ? mt.textContent : 'no-row');
      push('markedTags', tagsIn(mt));
      // String.fromCharCode(96) 就是反引号（Go 原始字符串里写不出来）。
      push('markedMarkerLeft', mt ? (mt.textContent.indexOf('*') >= 0 ||
        mt.textContent.indexOf(String.fromCharCode(96)) >= 0) : 'no-row');
      push('markedCodeCls', (function () {
        var c = mt ? mt.querySelector('code') : null;
        return c ? c.className : 'no-code';
      })());
      push('markedTip', marked ? (marked.getAttribute('title') || '') : 'no-row');
      push('markedBlocks', blocksIn(mt));
      push('markedWrap', mt ? getComputedStyle(mt).whiteSpace : 'no-row');
      var plain = titleOf('@PLAIN@');
      push('plainText', plain ? plain.textContent : 'no-row');
      push('plainKids', plain ? plain.childNodes.length : -1);
      push('plainTextKids', plain ? plain.childNodes[0].nodeType : -1);
      push('plainElemKids', plain ? plain.children.length : -1);
      var real = titleOf('@REAL@');
      push('realText', real ? real.textContent : 'no-row');
      push('realElemKids', real ? real.children.length : -1);
      var risky = titleOf('@RISKY@');
      push('riskyText', risky ? risky.textContent : 'no-row');
      push('riskyTags', tagsIn(risky));
      var img = titleOf('@IMAGE@');
      push('imgSessionText', img ? img.textContent : 'no-row');
      push('imageTags', tagsIn(img));
      push('imageTip', (function () {
        var r = rowOf('@IMAGE@');
        return r ? (r.getAttribute('title') || '') : 'no-row';
      })());
      var crumb = document.querySelector('.crumb-current');
      push('crumbText', crumb ? crumb.textContent : 'no-crumb');
      push('crumbTags', tagsIn(crumb));
      push('crumbTip', crumb ? (crumb.getAttribute('title') || '') : 'no-crumb');
      // 侧栏在窄视口里会折成轨道（display:none），量不到矩形；中栏的面包屑
      // 一直可见，用它量"名字就是一行"（nowrap + 单行盒）。
      push('crumbRects', crumb ? crumb.getClientRects().length : -1);
      push('crumbBlocks', blocksIn(crumb));
      var dd = null, ddTip = 'no-dd';
      Array.prototype.forEach.call(document.querySelectorAll('.detail-kv dt'), function (dt) {
        if (dt.textContent === '会话') {
          dd = dt.nextElementSibling;
          ddTip = dd ? (dd.getAttribute('title') || '') : 'no-dd';
        }
      });
      push('detailText', dd ? dd.textContent : 'no-dd');
      push('detailTags', tagsIn(dd));
      push('detailTip', ddTip);
      push('detailBlocks', blocksIn(dd));
      push('chatCodeStyle', styleOf('.md-body .md-inline-code'));
      push('nameCodeStyle', styleOf('.row-title .md-inline-code'));
      // 轮询路径：静态模式下"重新扫描"走的也是 refreshIndex -> refreshList，
      // 签名没变时只该走 patchList()。给行与渲染出来的 <strong> 打上记号，刷两次
      // 再回来看：记号还在（节点没被重建）才算没破坏列表签名守卫。
      marked.setAttribute('data-keep', '1');
      var keepStrong = mt.querySelector('strong');
      if (keepStrong) { keepStrong.setAttribute('data-keep', '1'); }
      // 先把根目录那行改掉：刷新真的跑了才会被 refreshIndex 写回根路径，
      // 否则下面两条"没被重建"的断言就是空转。
      document.getElementById('root-path').textContent = 'probe';
      document.getElementById('refresh').click();
      document.getElementById('refresh').click();
      push('refreshRan', document.getElementById('root-path').textContent === 'probe' ? 'no' : 'yes');
      push('keepRow', (marked.getAttribute('data-keep') || 'gone') + '/' +
        (document.body.contains(marked) && document.body.contains(mt) ? 'attached' : 'detached'));
      push('keepStrong', (keepStrong ? (keepStrong.getAttribute('data-keep') || 'gone') : 'no-strong') + '/' +
        (keepStrong && document.body.contains(keepStrong) ? 'attached' : 'detached'));
      push('keepText', mt.textContent);
      var o = document.createElement('div');
      o.id = 'name-probe';
      o.textContent = out.join(' ;; ');
      document.body.appendChild(o);
`

// sessionNameSearchProbeJS 把查询词依次打进搜索框，记录命中数与标记会话是否还在。
// （`tickPh` 占位，装载时换成真反引号——Go 的原始字符串写不出反引号。）
const sessionNameSearchProbeJS = `
      var out = [];
      var input = document.getElementById('search');
      var push = function (k, v) { out.push(k + '=' + String(v).replace(/\s+/g, ' ').trim()); };
      var probe = function (k, q) {
        input.value = q;
        input.dispatchEvent(new Event('input', { bubbles: true }));
        push(k, document.querySelectorAll('.session-row').length +
          (document.querySelector('.session-row[data-id$="@MARK@"]') ? '/命中' : '/未命中'));
      };
      probe('q1', '**粗体**');
      probe('q2', '` + tickPh + `code` + tickPh + `');
      probe('q3', '粗体');
      probe('q4', '斜体');
      probe('q5', '这个词不存在');
      input.value = '';
      input.dispatchEvent(new Event('input', { bubbles: true }));
      var o = document.createElement('div');
      o.id = 'name-probe';
      o.textContent = out.join(' ;; ');
      document.body.appendChild(o);
`

// tickPh 是装载探针脚本时的反引号占位符。
const tickPh = "\u0000"

// nameProbeValue 从渲染后的 DOM 里取一个探针字段（HTML 实体还原一次：探针正文
// 里只有文本，转义只发生在浏览器序列化这一步）。
func nameProbeValue(t *testing.T, dom, key string) string {
	t.Helper()
	i := strings.Index(dom, `id="name-probe"`)
	if i < 0 {
		t.Fatalf("页面里没有探针输出（渲染可能失败）；DOM 片段：%s", head(dom, 400))
	}
	probe := dom[i:]
	if gt := strings.Index(probe, ">"); gt >= 0 {
		probe = probe[gt+1:]
	}
	if end := strings.Index(probe, "</div>"); end > 0 {
		probe = probe[:end]
	}
	probe = html.UnescapeString(probe)
	for _, part := range strings.Split(probe, ";;") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(part, key+"="))
		}
	}
	t.Fatalf("探针里没有 %s：%s", key, probe)
	return ""
}

// nameProbeFilled 把夹具里的文件名与反引号占位符塞进探针脚本。
func nameProbeFilled(js string) string {
	return strings.NewReplacer(
		"@MARK@", markedFile,
		"@PLAIN@", plainFile,
		"@REAL@", realFile,
		"@RISKY@", riskyFile,
		"@IMAGE@", imgSessionFile,
		tickPh, tick,
	).Replace(js)
}

// TestViewerSessionNameMarkdownInChromium 是这条需求的行为测试：会话名里的
// Markdown 在**侧栏行 / 面包屑 / 详情栏**三处都渲染成行内元素，标记符号不再
// 显示；纯文本名字（含 `<`、`&`、落单的 `*`、反引号）输出与改动前一模一样；
// 悬浮说明保持原文；名字仍然只有一行（没有块级元素）。
func TestViewerSessionNameMarkdownInChromium(t *testing.T) {
	root := sessionNameFixture(t)
	dom := renderViewerDOM(t, root, markedFile, nameProbeFilled(sessionNameProbeJS))

	// ① 侧栏行：名字渲染成 em/strong/code，可见文本里没有 * 与反引号残渣。
	if got := nameProbeValue(t, dom, "markedText"); got != markedText {
		t.Errorf("侧栏名字的可见文本 = %q，期望 %q（标记符号应当被渲染掉）", got, markedText)
	}
	if got := nameProbeValue(t, dom, "markedTags"); got != "EM,STRONG,CODE" {
		t.Errorf("侧栏名字的元素结构 = %q，期望 EM,STRONG,CODE（斜体/粗体/行内 code）", got)
	}
	if got := nameProbeValue(t, dom, "markedMarkerLeft"); got != "false" {
		t.Errorf("* 或反引号还留在可见文本里（markedMarkerLeft=%s）", got)
	}
	if got := nameProbeValue(t, dom, "markedCodeCls"); got != "md-inline-code" {
		t.Errorf("行内 code 的类名 = %q，期望共用的 md-inline-code", got)
	}

	// ② 单行：没有任何块级元素，且名字不换行（CSS 的 nowrap 仍生效）。
	if got := nameProbeValue(t, dom, "markedBlocks"); got != "none" {
		t.Errorf("名字里出现了块级元素 %q（必须保持单行）", got)
	}
	if got := nameProbeValue(t, dom, "crumbRects"); got != "1" {
		t.Errorf("名字的矩形数 = %s，期望 1（一行，不能折行）", got)
	}
	if got := nameProbeValue(t, dom, "markedWrap"); got != "nowrap" {
		t.Errorf("名字的 white-space = %q，期望 nowrap", got)
	}

	// ③ 悬浮说明：原始名字（标记本身一个不少），且是纯文本。
	tip := nameProbeValue(t, dom, "markedTip")
	if !strings.Contains(tip, markedTitle) {
		t.Errorf("行悬浮说明里没有原始名字；实际：%s", tip)
	}
	for _, want := range []string{"**粗体**", tick + "code" + tick} {
		if !strings.Contains(tip, want) {
			t.Errorf("悬浮说明丢了标记 %q（title 必须保持原文）；实际：%s", want, tip)
		}
	}

	// ④ 纯文本对照：输出与改动前逐字一致（整串就是一个文本节点，没有元素）。
	if got := nameProbeValue(t, dom, "plainText"); got != plainTitle {
		t.Errorf("纯文本会话名的可见文本 = %q，期望 %q", got, plainTitle)
	}
	if got := nameProbeValue(t, dom, "plainKids"); got != "1" {
		t.Errorf("纯文本会话名的子节点数 = %s，期望 1", got)
	}
	if got := nameProbeValue(t, dom, "plainTextKids"); got != "3" {
		t.Errorf("纯文本会话名的子节点类型 = %s，期望 3（文本节点）", got)
	}
	if got := nameProbeValue(t, dom, "plainElemKids"); got != "0" {
		t.Errorf("纯文本会话名里冒出了元素（%s 个）", got)
	}
	if got := nameProbeValue(t, dom, "realText"); got != realTitle {
		t.Errorf("真实命名形态（单下划线章号）的可见文本 = %q，期望 %q", got, realTitle)
	}
	if got := nameProbeValue(t, dom, "realElemKids"); got != "0" {
		t.Errorf("单下划线的章号被当成了强调（%s 个元素）", got)
	}

	// ⑤ 危险字符：`<b>` 不是一个元素、`&` 只是文本、落单的 * 与反引号保持原样。
	if got := nameProbeValue(t, dom, "riskyText"); got != riskyTitle {
		t.Errorf("含 < & * 反引号的名字可见文本 = %q，期望 %q", got, riskyTitle)
	}
	if got := nameProbeValue(t, dom, "riskyTags"); got != "none" {
		t.Errorf("名字里的 <b> 被当成标签解析了（元素：%s）", got)
	}

	// ⑥ 逐图会话：标题先按「图注 → 标签 → 短写哈希」定好，再渲染图注里的标记。
	if got := nameProbeValue(t, dom, "imgSessionText"); got != imgSessionText {
		t.Errorf("逐图会话名的可见文本 = %q，期望 %q", got, imgSessionText)
	}
	if got := nameProbeValue(t, dom, "imageTags"); got != "CODE,STRONG" {
		t.Errorf("图注里的标记没有渲染：元素 = %q，期望 CODE,STRONG", got)
	}
	if got := nameProbeValue(t, dom, "imageTip"); !strings.Contains(got, imgSessionTitle) {
		t.Errorf("逐图会话的悬浮说明里没有原始图注标题：%s", got)
	}

	// ⑦ 中栏面包屑：同一套渲染，且它的悬浮说明（完整路径）保持原文。
	if got := nameProbeValue(t, dom, "crumbText"); got != markedText {
		t.Errorf("面包屑的可见文本 = %q，期望 %q", got, markedText)
	}
	if got := nameProbeValue(t, dom, "crumbTags"); got != "EM,STRONG,CODE" {
		t.Errorf("面包屑的元素结构 = %q，期望 EM,STRONG,CODE", got)
	}
	if got := nameProbeValue(t, dom, "crumbBlocks"); got != "none" {
		t.Errorf("面包屑里出现了块级元素 %q", got)
	}
	if got := nameProbeValue(t, dom, "crumbTip"); !strings.Contains(got, markedFile) {
		t.Errorf("面包屑的悬浮说明不再是完整路径：%s", got)
	}

	// ⑧ 右侧详情栏「会话信息」：名字渲染，悬浮说明仍是完整路径。
	if got := nameProbeValue(t, dom, "detailText"); got != markedText {
		t.Errorf("详情栏会话名的可见文本 = %q，期望 %q", got, markedText)
	}
	if got := nameProbeValue(t, dom, "detailTags"); got != "SPAN,EM,STRONG,CODE" {
		t.Errorf("详情栏的元素结构 = %q，期望 SPAN,EM,STRONG,CODE（外层是名字落点的 span）", got)
	}
	if got := nameProbeValue(t, dom, "detailBlocks"); got != "none" {
		t.Errorf("详情栏的名字里出现了块级元素 %q", got)
	}
	if got := nameProbeValue(t, dom, "detailTip"); !strings.Contains(got, markedFile) {
		t.Errorf("详情栏的悬浮说明不再是完整路径：%s", got)
	}

	// ⑨ 样式：名字里的 code 与页面其它行内 code 同一套 token（同字号、同行高、
	//    同字体族）——没有为会话名新造一套样式。
	chat := nameProbeValue(t, dom, "chatCodeStyle")
	name := nameProbeValue(t, dom, "nameCodeStyle")
	if chat == "no-node" {
		t.Fatal("消息正文里没有行内 code，样式比对没有参照物")
	}
	if name != chat {
		t.Errorf("名字里的 code 样式 = %q，页面其它行内 code = %q（必须同一套 token）", name, chat)
	}

	// ⑩ 轮询不重建：刷新两次（签名没变 → patchList）后，行与渲染出来的
	//    <strong> 还是原来那个节点，名字文本也没变。
	if got := nameProbeValue(t, dom, "refreshRan"); got != "yes" {
		t.Fatal("重新扫描没有真的跑（下面的断言会空转）")
	}
	if got := nameProbeValue(t, dom, "keepRow"); got != "1/attached" {
		t.Errorf("刷新后侧栏行被重建了（%s）：渲染不能进列表签名/patchList", got)
	}
	if got := nameProbeValue(t, dom, "keepStrong"); got != "1/attached" {
		t.Errorf("刷新后名字里的 <strong> 被重建了（%s）", got)
	}
	if got := nameProbeValue(t, dom, "keepText"); got != markedText {
		t.Errorf("刷新后的名字文本 = %q，期望 %q", got, markedText)
	}
}

// TestViewerSessionNameSearchInChromium 钉住"搜索仍按原始文本匹配"：渲染把
// `**粗体**` 变成了粗体，但过滤用的是渲染**前**的字段，所以拿标记本身照样
// 能搜到，拿不存在的词仍然是空列表。
func TestViewerSessionNameSearchInChromium(t *testing.T) {
	root := sessionNameFixture(t)
	dom := renderViewerDOM(t, root, markedFile, nameProbeFilled(sessionNameSearchProbeJS))

	cases := []struct{ key, want string }{
		{"q1", "1/命中"}, // `**粗体**`：标记本身就能搜到（原文匹配）
		{"q2", "1/命中"}, // 反引号 code
		{"q3", "1/命中"}, // 渲染后的可见文字同样能搜到
		{"q4", "1/命中"},
		{"q5", "0/未命中"},
	}
	for _, c := range cases {
		if got := nameProbeValue(t, dom, c.key); got != c.want {
			t.Errorf("搜索探针 %s = %q，期望 %q（筛选必须按原始文本）", c.key, got, c.want)
		}
	}
}

// TestViewerSessionNameSourceRules 钉住源码契约（真实渲染的行为断言在上面）：
// 行内渲染只有一个入口且复用 mdInline、不碰块级解析；三处显示位置共用同一个
// 落点；渲染既不进列表签名也不进 patchList（轮询不会因此重建侧栏 DOM）；
// 搜索串仍是原始字段；没有为会话名新造样式。
func TestViewerSessionNameSourceRules(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")

	inline := jsFunc(t, js, "renderInlineMarkdown")
	if !strings.Contains(inline, "mdInline(frag, text, 0);") {
		t.Error("行内渲染没有复用既有的 mdInline")
	}
	if strings.Contains(inline, "mdInlineLines") || strings.Contains(inline, "renderMarkdown(") {
		t.Error("行内渲染混进了块级路径（会造出 <br>/<p> 之类的块级元素）")
	}
	if !strings.Contains(inline, "document.createDocumentFragment()") {
		t.Error("行内渲染没有返回 DocumentFragment")
	}
	if n := strings.Count(js, "function mdInline("); n != 1 {
		t.Errorf("行内渲染器有 %d 份实现，期望 1（复用，不写第二套）", n)
	}
	if !strings.Contains(jsFunc(t, js, "nameNode"), "node.appendChild(renderInlineMarkdown(name));") {
		t.Error("会话名的落点没有走行内渲染入口")
	}

	// 三处显示位置 + 悬浮说明（悬浮说明仍是原始字符串）。
	for _, want := range []string{
		"row.appendChild(nameNode('span', 'row-title', sessionTitleOf(s)));",
		"var name = nameNode('span', 'crumb crumb-current', sessionTitleOf(cur));",
		"['会话', nameNode('span', 'detail-name', sessionTitleOf(cur)), cur.id, true],",
		"var tip = [sessionTitleOf(s), s.id];",
		"row.title = sessionTip(s);",
		"if (p[1] && p[1].nodeType) {",
		"dd.appendChild(p[1]);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("会话名的显示位置/悬浮说明缺 %q", want)
		}
	}
	if !strings.Contains(jsFunc(t, js, "sessionTitleOf"), "return s.title || s.label || s.name;") {
		t.Error("sessionTitleOf 不再返回先截断定好的那个字符串")
	}

	// 渲染不进轮询：签名与最小修补都不碰名字节点（名字只在整表重建时造一次）。
	for _, name := range []string{"listSignature", "patchList"} {
		body := jsFunc(t, js, name)
		for _, bad := range []string{"renderInlineMarkdown", "nameNode", "row-title", "sessionTitleOf"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s 里出现了 %s：轮询会因此重建侧栏 DOM", name, bad)
			}
		}
	}
	if !strings.Contains(jsFunc(t, js, "sessionHaystack"), "s.name") {
		t.Error("搜索串不再是原始字段（渲染不该影响过滤结果）")
	}

	// 没有为会话名新造样式：名字里的 code 落到共用的 .md-inline-code。
	for _, bad := range []string{".row-title code", ".crumb code", ".detail-name"} {
		if strings.Contains(css, bad) {
			t.Errorf("样式表里出现了 %s：会话名不该另造一套字号/颜色", bad)
		}
	}
	rule := cssRule(t, css, ".md-inline-code")
	for _, want := range []string{"font-size: var(--code-font);", "line-height: var(--code-line);"} {
		if !strings.Contains(rule, want) {
			t.Errorf("行内 code 没有用 --code-font/--code-line 那套 token：缺 %q", want)
		}
	}
}
