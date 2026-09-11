package sessionview

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// chromiumPath returns the browser used by the end-to-end viewer tests, or ""
// when the machine has none (the tests then skip instead of failing: the
// attribution rules are additionally pinned by source-level assertions in
// TestViewerMergesToolCallsAndImageTurns).
func chromiumPath() string {
	for _, cand := range []string{"/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	if p, err := exec.LookPath("chromium"); err == nil {
		return p
	}
	return ""
}

// selectorScript clicks the session row and then runs extra (a JS snippet run
// 120ms later, still inside the page's own event loop).
func selectorScript(sessionSuffix, extra string) string {
	return `
<script>
window.addEventListener('load', function () {
  setTimeout(function () {
    var row = document.querySelector('.session-row[data-id$="` + sessionSuffix + `"]');
    if (row) { row.click(); }
    setTimeout(function () {` + extra + `
    }, 120);
  }, 60);
});
</script>
`
}

// renderViewerDOM exports a static page for root, opens it in headless
// chromium and returns the rendered DOM (with the inlined <script>/<style>
// blocks stripped, so an assertion can never be satisfied by the source text).
func renderViewerDOM(t *testing.T, root, sessionSuffix, extra string) string {
	t.Helper()
	browser := chromiumPath()
	if browser == "" {
		t.Skip("没有 chromium：跳过真实渲染验证")
	}
	dir := t.TempDir()
	page := filepath.Join(dir, "page.html")
	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := WriteStaticHTML(root, page, sessions); err != nil {
		t.Fatalf("WriteStaticHTML: %v", err)
	}
	html, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	open := filepath.Join(dir, "open.html")
	if err := os.WriteFile(open, []byte(strings.Replace(string(html), "</body>",
		selectorScript(sessionSuffix, extra)+"</body>", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(browser, "--headless=new", "--no-sandbox", "--disable-gpu",
		"--user-data-dir="+filepath.Join(dir, "cr"), "--crash-dumps-dir="+filepath.Join(dir, "cr"),
		"--virtual-time-budget=4000", "--dump-dom", "file://"+open)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	dom := string(out)
	dom = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`).ReplaceAllString(dom, "")
	dom = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`).ReplaceAllString(dom, "")
	return dom
}

// TestImageViewerAttributionInChromium is the behaviour test for requirement 8:
// a tool image turn and the session-opening figure feed must land in the right
// block, and the trajectory must say **how** the attribution was made.
//
// The transcript deliberately carries the OLD handle (no call id), which is what
// every already-written transcript looks like.
func TestImageViewerAttributionInChromium(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "proj", "work", "media", "a.jpg"), "jpeg")
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_01.jsonl"), transcript(
		// 会话开头的原图投喂轮（没有任何工具调用可配）
		`{"t":"msg","role":"user","text":"把附带的图重画成矢量图。 ORIGINAL FIGURE SIZE: 36.9mm x 20.1mm","images":["file://media/a.jpg"],"ts":"2025-01-02T03:00:00Z"}`,
		`{"t":"msg","role":"user","text":"先看这一章的结构。","ts":"2025-01-02T03:00:01Z"}`,
		`{"t":"msg","role":"assistant","text":"跑一下编译。","ts":"2025-01-02T03:00:02Z","tool_calls":[{"id":"call_1","type":"function","function":{"name":"view_pdf","arguments":"{\"path\":\"work/a.tex\"}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_1","text":"PDF page work/a.pdf p1 (crop 0%,0%-100%,100%, width 900px) (page 1/1) attached.","ts":"2025-01-02T03:00:03Z"}`,
		// 旧句柄：没有 call id，只能按顺序推断到 call_1
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/a.jpg"],"ts":"2025-01-02T03:00:04Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_01.jsonl", "")

	// ① 工具图片回执：挂进那次调用的卡片（附件段），并注明归属是推断的。
	card := dom[strings.Index(dom, `class="io-card"`):]
	if end := strings.Index(card, `</details>`); end > 0 {
		card = card[:end]
	}
	if !strings.Contains(card, "附件（user 轮）") {
		t.Error("工具展开区里没有附件段：旧转录的图片回执没有归到那次调用上")
	}
	if !strings.Contains(card, "归属：由顺序推断（本轮的 call call_1）") {
		t.Errorf("附件段没有写明归属依据；卡片片段：%s", card)
	}
	// ② 会话开头这一轮**自带任务正文**：它自己就是任务块（本会话任务），不塞给工具。
	//    （纯图片行排在任务提示前面的那条路见
	//    TestTaskFeedBeforeTaskAndOldHandleInChromium。）
	sys := dom[strings.Index(dom, `class="msg msg-system"`):]
	sys = sys[:strings.Index(sys, "</section>")]
	if !strings.Contains(sys, "附件（user 轮）") {
		t.Error("会话开头的原图轮没有归到任务块")
	}
	if !strings.Contains(sys, "归属：本会话任务") {
		t.Errorf("任务块的附件没有写明归属；片段：%s", sys)
	}
	if strings.Contains(sys, "归属：call") {
		t.Error("会话开头的原图轮被硬塞给了某个工具调用")
	}

	// ③ 轨迹页：真实行 + 归属证据（推断 vs 精确）。
	traj := renderViewerDOM(t, root, "convert_01.jsonl", "\n      document.getElementById('tab-traj').click();")
	if !strings.Contains(traj, "归属：由顺序推断（本轮的 call call_1）") {
		t.Error("轨迹页没有标出'由顺序推断'")
	}
	if !strings.Contains(traj, "归属：本会话任务") {
		t.Error("轨迹页没有标出会话首图归到任务")
	}
	if !strings.Contains(traj, "kind-result") {
		t.Error("轨迹页把工具调用与工具结果合并了（应当各占一行）")
	}
}

// TestImageHandleAttributionIsPreciseWhenCallIDPresent covers the new-transcript
// path: the handle names the call, so the attribution is exact rather than
// inferred from order.
func TestImageHandleAttributionIsPreciseWhenCallIDPresent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "proj", "work", "media", "a.jpg"), "jpeg")
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_02.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"看这一章。","ts":"2025-01-02T03:00:01Z"}`,
		// 同一轮两个调用：句柄点名了第二个，归属必须精确落到 view_pdf（call_b），
		// 而不是顺序推断给出的 call_a。
		`{"t":"msg","role":"assistant","text":"两个都看一下。","ts":"2025-01-02T03:00:02Z","tool_calls":[{"id":"call_a","type":"function","function":{"name":"view_image","arguments":"{}"}},{"id":"call_b","type":"function","function":{"name":"view_pdf","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_a","text":"Image images/proj/a.jpg (crop 0%,0%-100%,100%, width 900px) attached.","ts":"2025-01-02T03:00:03Z"}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_b","text":"PDF page work/a.pdf p1 (crop 0%,0%-100%,100%, width 900px) (page 1/1) attached.","ts":"2025-01-02T03:00:04Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output from view_pdf (call call_b) (for your visual review):","images":["file://media/a.jpg"],"ts":"2025-01-02T03:00:05Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_02.jsonl", "\n      document.getElementById('tab-traj').click();")
	if !strings.Contains(dom, "归属：call call_b（view_pdf）") {
		t.Error("轨迹页没有把句柄里的 call id 认成精确归属")
	}
	if strings.Contains(dom, "归属：由顺序推断") {
		t.Error("有 call id 的句柄被当成了顺序推断")
	}
}

// TestTaskImageTurnStaysWithTaskInChromium pins the shape every real vector
// session has (source/sessions/vector_<书>__<哈希>__<图>.jsonl, 2nd line): the
// **session task itself carries the original figure** — a role:"user" line with
// images whose text is the task prompt, so it neither starts with
// `Tool image output` nor is a bare image line.
//
// That line used to fall through to 「归属：未识别」and render as a lone image
// row. It must be attributed to 本会话任务 and rendered as that task's own
// attachment — never folded into a tool call.
func TestTaskImageTurnStaysWithTaskInChromium(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "proj", "source", "media", "f6ce799176eb4fa1.jpg"), "jpeg")
	writeFile(t, jsonl(root, "proj", "source", "sessions",
		"vector_测试__"+strings.Repeat("ab12", 16)+"__Mind_map.jsonl"), transcript(
		`{"t":"meta","kind":"system","model":"m","text":"You are an expert LaTeX vector illustrator.","tools":[]}`,
		// ← 真实矢量图会话的第 2 行：任务提示自己带着原图。
		`{"t":"msg","role":"user","text":"Redraw the attached image as TikZ.\n\nORIGINAL FIGURE SIZE: 108.3mm x 189.6mm on the page,","images":["file://media/f6ce799176eb4fa1.jpg"],"ts":"2025-01-02T03:00:00Z"}`,
		`{"t":"msg","role":"assistant","text":"I'll start by studying the original image carefully.","ts":"2025-01-02T03:00:01Z","tool_calls":[{"id":"call_00_view","type":"function","function":{"name":"view_image","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_00_view","text":"Image images/测试/f6ce799176eb4fa1.jpg (crop 0%,0%-100%,100%, width 900px) attached.","ts":"2025-01-02T03:00:02Z"}`,
		// 旧转录的工具图片回执（只有前缀）→ 仍然只能按顺序推断。
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/f6ce799176eb4fa1.jpg"],"ts":"2025-01-02T03:00:03Z"}`,
	))
	dom := renderViewerDOM(t, root, "Mind_map.jsonl", "")

	// ① 归属：任务行自带的图 → 本会话任务，绝不落到未识别。
	if !strings.Contains(dom, "归属：本会话任务") {
		t.Error("任务行自带的图没有归到「本会话任务」")
	}
	if strings.Contains(dom, "归属：未识别") {
		t.Error("还有图片轮落到「归属：未识别」（任务行自带的图应当归到本会话任务）")
	}

	// ② 呈现：整块是系统消息/任务块（msg-system），图片作为**它的附件**收在里面。
	sys := dom[strings.Index(dom, `class="msg msg-system"`):]
	sys = sys[:strings.Index(sys, "</section>")]
	if !strings.Contains(sys, "附件（user 轮）") {
		t.Errorf("任务块里没有附件段；任务块片段：%s", sys)
	}
	if !strings.Contains(sys, `class="thumb"`) {
		t.Error("任务块的附件里没有图片缩略图")
	}
	if !strings.Contains(sys, "归属：本会话任务") {
		t.Errorf("任务块的附件没有写明归属；片段：%s", sys)
	}
	if strings.Contains(sys, "归属：call") {
		t.Error("任务行自带的图被塞给了某个工具调用")
	}

	// ③ 工具调用卡片只收旧句柄那张图（由顺序推断），任务提示正文不得出现在卡片里。
	card := dom[strings.Index(dom, `class="io-card"`):]
	if end := strings.Index(card, `</details>`); end > 0 {
		card = card[:end]
	}
	if !strings.Contains(card, "归属：由顺序推断（本轮的 call call_00_view）") {
		t.Errorf("旧式句柄没有按顺序推断到本轮调用；卡片片段：%s", card)
	}
	if strings.Contains(card, "Redraw the attached image as TikZ.") {
		t.Error("任务行的正文跑到工具调用卡片里去了")
	}

	// ④ 轨迹页：这一行也标「本会话任务」。
	traj := renderViewerDOM(t, root, "Mind_map.jsonl", "\n      document.getElementById('tab-traj').click();")
	if !strings.Contains(traj, "归属：本会话任务") {
		t.Error("轨迹页没有把任务行自带的图标成本会话任务")
	}
	if strings.Contains(traj, "归属：未识别") {
		t.Error("轨迹页还有未识别的图片轮")
	}
}

// TestTaskFeedBeforeTaskAndOldHandleInChromium covers the two neighbouring
// paths that must keep working next to the task-own-figure case:
//
//	① 会话开头的**纯图片行**（没有正文）排在任务提示前面 → 仍归到本会话任务，
//	   经 pendingTaskImages 挂到那条任务行上；
//	② 旧式 `Tool image output (for your visual review):` → 仍归「由顺序推断」。
func TestTaskFeedBeforeTaskAndOldHandleInChromium(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "proj", "work", "media", "a.jpg"), "jpeg")
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_03.jsonl"), transcript(
		// ① 纯图片行排在任务**前面**：渲染到它时任务块还没建出来。
		`{"t":"msg","role":"user","text":"","images":["file://media/a.jpg"],"ts":"2025-01-02T03:00:00Z"}`,
		`{"t":"msg","role":"user","text":"先看这一章的结构。","ts":"2025-01-02T03:00:01Z"}`,
		`{"t":"msg","role":"assistant","text":"跑一下编译。","ts":"2025-01-02T03:00:02Z","tool_calls":[{"id":"call_1","type":"function","function":{"name":"view_pdf","arguments":"{\"path\":\"work/a.tex\"}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_1","text":"PDF page work/a.pdf p1 (crop 0%,0%-100%,100%, width 900px) (page 1/1) attached.","ts":"2025-01-02T03:00:03Z"}`,
		// ② 旧式句柄：既没有 call id 也没有工具名 → 该轮里还没被认领的那次调用。
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/a.jpg"],"ts":"2025-01-02T03:00:04Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_03.jsonl", "")

	task := dom[strings.Index(dom, `class="msg msg-system"`):]
	task = task[:strings.Index(task, "</section>")]
	if !strings.Contains(task, "附件（user 轮）") {
		t.Errorf("会话开头的纯图片行没有挂到任务块上；任务块片段：%s", task)
	}
	if !strings.Contains(task, "归属：本会话任务") {
		t.Errorf("纯图片行的归属不是本会话任务；片段：%s", task)
	}
	if strings.Contains(task, "归属：call") {
		t.Error("纯图片行被塞给了某个工具调用")
	}
	if !strings.Contains(dom, "归属：由顺序推断（本轮的 call call_1）") {
		t.Error("旧式句柄不再按顺序推断了")
	}
	if strings.Contains(dom, "归属：未识别") {
		t.Error("纯图片行或旧式句柄落到了未识别")
	}
}

// writePNG writes a real PNG of the given size next to a transcript, so the
// browser tests can measure the *rendered* image box (a 1x1 file would only
// prove that max-width does not enlarge anything).
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// measureBlock is the injected JS of the geometry test: it appends the measured
// numbers as a <pre id="measure">…</pre> so the assertions run on the DOM dump.
const measureBlock = `
      (function () {
        Array.prototype.forEach.call(document.querySelectorAll('#timeline img'),
                                     function (im) { im.loading = 'eager'; });
        var tries = 0;
        var shot = function () {
        var root = getComputedStyle(document.documentElement);
        var tl = document.getElementById('timeline');
        var rows = Array.prototype.slice.call(tl.querySelectorAll('.disclosure > summary'));
        var heights = {}, sameGaps = {}, cross = {}, sizes = { preview: [], attach: [] };
        var prev = null;
        rows.forEach(function (sm) {
          var r = sm.getBoundingClientRect();
          heights[Math.round(r.height)] = 1;
          var sec = sm.closest('section.msg');
          if (prev) {
            var gap = Math.round((r.top - prev.r.bottom) * 100) / 100;
            if (prev.sec === sec) { sameGaps[gap] = 1; } else { cross[gap] = 1; }
          }
          prev = { r: r, sec: sec };
        });
        Array.prototype.forEach.call(tl.querySelectorAll('.preview-strip img'), function (im) {
          var b = im.getBoundingClientRect();
          sizes.preview.push(Math.round(b.width) + 'x' + Math.round(b.height));
        });
        Array.prototype.forEach.call(tl.querySelectorAll('.attach-section img, .out-section img'), function (im) {
          var b = im.getBoundingClientRect();
          sizes.attach.push(Math.round(b.width) + 'x' + Math.round(b.height));
        });
        var borders = {};
        Array.prototype.forEach.call(tl.querySelectorAll('section.msg'), function (sec, i) {
          var cs = getComputedStyle(sec);
          var key = (i === 0 ? 'first' : cs.borderTopStyle + '/' + cs.marginTop + '/' + cs.paddingTop);
          borders[key] = (borders[key] || 0) + 1;
        });
        var pre = document.createElement('pre');
        pre.id = 'measure';
        var snapshot = function () { return {
          rowH: root.getPropertyValue('--row-h').trim(),
          rowGap: root.getPropertyValue('--row-gap').trim(),
          dividerGap: root.getPropertyValue('--divider-gap').trim(),
          previewH: root.getPropertyValue('--preview-h').trim(),
          previewW: root.getPropertyValue('--preview-w').trim(),
          attachH: root.getPropertyValue('--attach-img-h').trim(),
          attachW: root.getPropertyValue('--attach-img-w').trim(),
          heights: Object.keys(heights), sameGaps: Object.keys(sameGaps),
          crossGaps: Object.keys(cross), borders: borders,
          loaded: Array.prototype.filter.call(document.querySelectorAll('#timeline img'),
                                              function (im) { return im.naturalWidth > 0; }).length,
          sizes: sizes
        }; };
        // 折叠态先量一遍，再把所有折叠行展开量一遍：行高与回合分隔线必须一模一样。
        var firstSnap = snapshot();
        Array.prototype.forEach.call(document.querySelectorAll('#timeline details'),
                                     function (d) { d.open = true; });
        setTimeout(function () {
          pre.textContent = JSON.stringify({ collapsed: firstSnap, expanded: snapshot() });
          document.body.appendChild(pre);
        }, 80);
        };
        var poll = function () {
          var imgs = document.querySelectorAll('#timeline img');
          var pending = Array.prototype.filter.call(imgs, function (im) { return !im.complete; });
          if (imgs.length && pending.length && tries++ < 40) { setTimeout(poll, 40); return; }
          shot();
        };
        poll();
      })();
`

// measure returns the <pre id="measure"> payload as plain text.
func measure(t *testing.T, dom string) string {
	t.Helper()
	const open = `<pre id="measure">`
	i := strings.Index(dom, open)
	if i < 0 {
		t.Fatal("测量脚本没有产出 #measure（页面没渲染出来？）")
	}
	rest := dom[i+len(open):]
	j := strings.Index(rest, "</pre>")
	if j < 0 {
		t.Fatal("测量结果没有闭合")
	}
	return strings.NewReplacer("&quot;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(rest[:j])
}

// callIDBefore returns the tool-call id of the nearest preceding tool row.
var callIDRe = regexp.MustCompile(`data-call-id="([^"]+)"`)

func callIDBefore(t *testing.T, dom string, idx int) string {
	t.Helper()
	all := callIDRe.FindAllStringSubmatch(dom[:idx], -1)
	if len(all) == 0 {
		t.Fatal("锚点前面没有任何工具行")
	}
	return all[len(all)-1][1]
}

// TestViewPreviewStripFollowsLastViewRowInChromium pins requirement D: the
// thumbnails of the image-producing tool calls sit **directly under that row**
// (visible while collapsed), and when one round calls view several times they
// are all collected — in transcript order, side by side — under the LAST view
// row of that round, never one strip per row.
func TestViewPreviewStripFollowsLastViewRowInChromium(t *testing.T) {
	root := t.TempDir()
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "a.png"), 800, 1200)
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "b.png"), 1400, 400)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_d1.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"看一下这一章。","ts":"2025-01-02T03:00:00Z"}`,
		// 一轮里两次看图调用（外加一次不产图的 bash），图片回执一条一条按顺序回来
		`{"t":"msg","role":"assistant","text":"先看两张。","ts":"2025-01-02T03:00:01Z","tool_calls":[{"id":"call_v1","type":"function","function":{"name":"view_pdf","arguments":"{}"}},{"id":"call_bash","type":"function","function":{"name":"bash","arguments":"{}"}},{"id":"call_v2","type":"function","function":{"name":"view_image","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_v1","text":"PDF page work/a.pdf p1 (crop 0%,0%-100%,100%, width 900px) (page 1/1) attached.","ts":"2025-01-02T03:00:02Z"}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_bash","text":"ok","ts":"2025-01-02T03:00:03Z"}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_v2","text":"Image images/proj/a.png (crop 0%,0%-100%,100%, width 1400px) attached.","ts":"2025-01-02T03:00:04Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/a.png"],"ts":"2025-01-02T03:00:05Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/b.png"],"ts":"2025-01-02T03:00:06Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_d1.jsonl", "")

	if n := strings.Count(dom, "class=\"preview-strip\""); n != 1 {
		t.Fatalf("一轮里两次 view 应当只有一条缩略图行，实际 %d 条", n)
	}
	strip := strings.Index(dom, "class=\"preview-strip\"")
	if id := callIDBefore(t, dom, strip); id != "call_v2" {
		t.Errorf("缩略图行没有挂在最后一个 view 行下面，而是 %q", id)
	}
	if !strings.Contains(dom, "<summary") || strings.Index(dom, "class=\"preview-strip\"") < 0 {
		t.Fatal("缩略图行缺失")
	}
	// 折叠态就可见：它必须是 <details> 的兄弟节点，而不是卡片里的内容。
	if !strings.Contains(dom, `</details><div class="preview-strip">`) {
		t.Error("缩略图行不在工具行（<details>）之外，折叠时看不到")
	}
	// 顺序 = 转录顺序（先 a.png 后 b.png），两张并排，且都在这一条缩略图行里。
	stripHTML := dom[strip:]
	if end := strings.Index(stripHTML, "</section>"); end > 0 {
		stripHTML = stripHTML[:end]
	}
	first, second := strings.Index(stripHTML, `alt="file://media/a.png"`), strings.Index(stripHTML, `alt="file://media/b.png"`)
	if first < 0 || second < 0 || first > second {
		t.Errorf("缩略图顺序不对（应当按转录先后 a.png、b.png）：%s", stripHTML)
	}
	if n := strings.Count(stripHTML, `class="preview-thumb"`); n != 2 {
		t.Errorf("缩略图数量不对，期望 2 张，实际 %d：%s", n, stripHTML)
	}
	if strings.Contains(dom, "归属：未识别") {
		t.Error("FIFO 归属留下了未识别的图片轮")
	}
	// FIFO：第一条图归这一轮里**最早**那次还没被认领的产图调用（call_v1），
	// 第二条归下一次（call_v2）——而不是都就近挂到同一轮。
	cardV1 := dom[strings.Index(dom, `data-call-id="call_v1"`):]
	cardV1 = cardV1[:strings.Index(cardV1, "</details>")]
	if !strings.Contains(cardV1, "归属：由顺序推断（本轮的 call call_v1）") {
		t.Errorf("第一条图没有按 FIFO 归到 call_v1；卡片片段：%s", cardV1)
	}
	cardBash := dom[strings.Index(dom, `data-call-id="call_bash"`):]
	cardBash = cardBash[:strings.Index(cardBash, "</details>")]
	if strings.Contains(cardBash, "附件（user 轮）") {
		t.Error("不产图的 bash 调用被塞进了图片附件")
	}
}

// TestExactHandleRendersImageAsToolOutputInChromium pins requirement E: with an
// exact handle (`(call <id>)`) the image IS that call's output — it renders
// directly under the 输出 text inside the same column, there is no separate
// 「附件（user 轮）」 section, and only one small attribution line follows.
//
// The contrast case (an inferred handle keeps the attachment section) is pinned
// by TestViewPreviewStripFollowsLastViewRowInChromium and
// TestFifoHandleKeepsAttachmentSectionLayoutInChromium.
func TestExactHandleRendersImageAsToolOutputInChromium(t *testing.T) {
	root := t.TempDir()
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "a.png"), 800, 1200)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_e1.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"看一下这一页。","ts":"2025-01-02T03:00:00Z"}`,
		`{"t":"msg","role":"assistant","text":"看看第 10 页。","ts":"2025-01-02T03:00:01Z","tool_calls":[{"id":"call_p","type":"function","function":{"name":"view_pdf","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_p","text":"PDF page work/a.pdf p10 (crop 0%,0%-100%,100%, width 1400px) (page 10/14) attached.","ts":"2025-01-02T03:00:02Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output from view_pdf (call call_p) (for your visual review):","images":["file://media/a.png"],"ts":"2025-01-02T03:00:03Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_e1.jsonl", "")

	card := dom[strings.Index(dom, `data-call-id="call_p"`):]
	card = card[:strings.Index(card, "</details>")]
	if strings.Contains(card, "附件（user 轮）") {
		t.Errorf("精确匹配还把图片拆成了「附件（user 轮）」；卡片片段：%s", card)
	}
	if !strings.Contains(card, `class="io-section out-section"`) {
		t.Error("「输出」段没有自己的标记（图片没法贴着它渲染）")
	}
	outAt := strings.Index(card, `class="io-section out-section"`)
	textAt := strings.Index(card[outAt:], "io-text")
	imgAt := strings.Index(card[outAt:], `class="images"`)
	noteAt := strings.Index(card[outAt:], `class="attach-note"`)
	if textAt < 0 || imgAt < 0 || noteAt < 0 || !(textAt < imgAt && imgAt < noteAt) {
		t.Errorf("精确匹配的图片没有紧跟在输出正文下面、脚注也没有在最后（text=%d img=%d note=%d）", textAt, imgAt, noteAt)
	}
	if !strings.Contains(card, "归属：call call_p（view_pdf，精确匹配）") {
		t.Errorf("精确匹配的归属说明不对；卡片片段：%s", card)
	}
	if !strings.Contains(dom, `</details><div class="preview-strip">`) {
		t.Error("精确匹配的卡片下面没有缩略图预览行")
	}
}

// TestFifoHandleKeepsAttachmentSectionLayoutInChromium pins the presentation the
// inferred path must keep (requirement B): a separate 「附件（user 轮）」 section
// whose caption is first, whose image sits on its own line and whose attribution
// footnote is the LAST child — the section is a single-column block, never the
// `label | content` grid of 输入/输出.
func TestFifoHandleKeepsAttachmentSectionLayoutInChromium(t *testing.T) {
	root := t.TempDir()
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "a.png"), 800, 1200)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_b1.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"看一下这一页。","ts":"2025-01-02T03:00:00Z"}`,
		`{"t":"msg","role":"assistant","text":"看看第 10 页。","ts":"2025-01-02T03:00:01Z","tool_calls":[{"id":"call_p","type":"function","function":{"name":"view_pdf","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_p","text":"PDF page work/a.pdf p10 (crop 0%,0%-100%,100%, width 1400px) (page 10/14) attached.","ts":"2025-01-02T03:00:02Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/a.png"],"ts":"2025-01-02T03:00:03Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_b1.jsonl", "")

	card := dom[strings.Index(dom, `data-call-id="call_p"`):]
	card = card[:strings.Index(card, "</details>")]
	if !strings.Contains(card, `class="io-section attach-section"`) {
		t.Errorf("推断出来的图片没有单独的附件段（单列块）；卡片片段：%s", card)
	}
	labelAt := strings.Index(card, "附件（user 轮）")
	imgAt := strings.Index(card, `class="images"`)
	noteAt := strings.Index(card, `class="attach-note"`)
	if labelAt < 0 || imgAt < 0 || noteAt < 0 || !(labelAt < imgAt && imgAt < noteAt) {
		t.Errorf("附件段顺序不对（说明在前、图片自成一行、脚注在末）：label=%d img=%d note=%d", labelAt, imgAt, noteAt)
	}
	if !strings.Contains(card, "归属：由顺序推断（本轮的 call call_p）") {
		t.Errorf("附件段没有写明归属依据；卡片片段：%s", card)
	}
	// 图片必须落在**附件段**里，而不是被当成「输出」的一部分（那是精确匹配才有的待遇）。
	attachAt := strings.Index(card, `class="io-section attach-section"`)
	if attachAt < 0 || imgAt < attachAt {
		t.Errorf("推断出来的图片没有落在附件段里（attach=%d img=%d）", attachAt, imgAt)
	}
}

// TestToolRowGeometryComesFromOneTokenSetInChromium pins requirement C: every
// collapsed row has the same height and the same vertical spacing regardless of
// receipts/attachments/status, the dashed turn divider appears only between
// messages (never between rows of one message) and the image boxes are capped by
// their tokens. The measured numbers must equal the CSS tokens themselves, so a
// later edit can only change them in one place.
func TestToolRowGeometryComesFromOneTokenSetInChromium(t *testing.T) {
	root := t.TempDir()
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "a.png"), 800, 1200)
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "b.png"), 1400, 400)
	writeFile(t, jsonl(root, "proj", "work", "sessions", "convert_c1.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"看一下这一章。","ts":"2025-01-02T03:00:00Z"}`,
		// 同一轮：思考行 + 一个失败的看图调用（无附件、无摘要）+ 两个成功的看图调用
		`{"t":"msg","role":"assistant","reasoning":"先看清楚这一页。","text":"跑一下编译。","ts":"2025-01-02T03:00:01Z","tool_calls":[{"id":"call_fail","type":"function","function":{"name":"view_pdf","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_fail","text":"TOOL ERROR: 文件不存在: work/a.pdf","ts":"2025-01-02T03:00:02Z"}`,
		`{"t":"msg","role":"assistant","text":"","ts":"2025-01-02T03:00:03Z","tool_calls":[{"id":"call_v1","type":"function","function":{"name":"view_pdf","arguments":"{}"}},{"id":"call_v2","type":"function","function":{"name":"view_image","arguments":"{}"}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_v1","text":"PDF page work/a.pdf p1 (crop 0%,0%-100%,100%, width 900px) (page 1/1) attached.","ts":"2025-01-02T03:00:04Z"}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_v2","text":"Image images/proj/a.png (crop 0%,0%-100%,100%, width 1400px) attached.","ts":"2025-01-02T03:00:05Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output from view_pdf (call call_v1) (for your visual review):","images":["file://media/a.png"],"ts":"2025-01-02T03:00:06Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/b.png"],"ts":"2025-01-02T03:00:07Z"}`,
	))
	dom := renderViewerDOM(t, root, "convert_c1.jsonl", measureBlock)
	m := measure(t, dom)

	for _, want := range []string{`"rowH":"24px"`, `"rowGap":"6px"`, `"dividerGap":"8px"`,
		`"previewH":"108px"`, `"previewW":"240px"`, `"attachH":"420px"`, `"attachW":"520px"`} {
		if !strings.Contains(m, want) {
			t.Errorf("间距/尺寸 token 不对，缺 %s；实测 %s", want, m)
		}
	}
	// 所有折叠行同一高度，且就等于 --row-h；展开卡片也不改变行高。
	for _, state := range []string{"collapsed", "expanded"} {
		chunk := m[strings.Index(m, `"`+state+`":`):]
		if next := strings.Index(chunk[1:], `"collapsed":`); next > 0 {
			chunk = chunk[:next+1]
		}
		if !strings.Contains(chunk, `"heights":["24"]`) {
			t.Errorf("%s 状态下工具行高度不唯一（应当只有 --row-h 一个值）；实测 %s", state, chunk)
		}
		if !strings.Contains(chunk, `"dashed/8px/8px"`) || !strings.Contains(chunk, `"first"`) {
			t.Errorf("%s 状态下回合分隔线不是唯一一套（8px + dashed + 8px）；实测 %s", state, chunk)
		}
	}
	// 同一条消息里的相邻行间距只有一个值（= --row-gap）；回合之间由分隔线负责。
	// （展开态下卡片会占位置，这一条只对折叠态成立。）
	if !strings.Contains(m, `"sameGaps":["6"]`) {
		t.Errorf("同一条消息里的行间距不唯一（应当只有 --row-gap）；实测 %s", m)
	}
	// 图片按原比例被 token 收口：a.png 是 800x1200 → 附件框 280x420、预览 72x108
	// （既没被拉变形、也没留空框）；b.png（1400x400）的附件框按宽 520 收口。
	sizePairs := func(list string) [][2]int {
		got := regexp.MustCompile(`"(\d+)x(\d+)"`).FindAllStringSubmatch(list, -1)
		out := make([][2]int, 0, len(got))
		for _, g := range got {
			w, errW := strconv.Atoi(g[1])
			h, errH := strconv.Atoi(g[2])
			if errW != nil || errH != nil {
				t.Fatalf("尺寸解析失败 %q", g[0])
			}
			out = append(out, [2]int{w, h})
		}
		return out
	}
	if !strings.Contains(m, `"loaded":`) {
		t.Errorf("没有图片加载完成，尺寸无从测量；实测 %s", m)
	}
	previewAt, attachAt := strings.Index(m, `"preview":[`), strings.Index(m, `"attach":[`)
	if previewAt < 0 || attachAt < previewAt {
		t.Fatalf("尺寸没有测到；实测 %s", m)
	}
	// 两个夹具图的比例：a.png 800x1200 = 2/3（竖版），b.png 1400x400 = 3.5（横版）。
	ratioOK := func(wh [2]int) bool {
		got := float64(wh[0]) / float64(wh[1])
		return math.Abs(got-2.0/3.0) < 0.08*2.0/3.0 || math.Abs(got-3.5) < 0.08*3.5
	}
	tallPreview, tallAttach := false, false
	for _, wh := range sizePairs(m[previewAt:attachAt]) {
		if wh[0] > 240 || wh[1] > 108 {
			t.Errorf("预览缩略图 %dx%d 超出 --preview-w/--preview-h（240x108）", wh[0], wh[1])
		}
		if !ratioOK(wh) {
			t.Errorf("预览缩略图 %dx%d 不是原比例（被拉变形了）", wh[0], wh[1])
		}
		if wh[1] == 108 && wh[0] < 240 {
			tallPreview = true
		}
	}
	for _, wh := range sizePairs(m[attachAt:]) {
		if wh[0] > 520 || wh[1] > 420 {
			t.Errorf("附件/输出里的图片 %dx%d 超出 --attach-img-w/--attach-img-h（520x420）", wh[0], wh[1])
		}
		if !ratioOK(wh) {
			t.Errorf("附件/输出里的图片 %dx%d 不是原比例（被拉变形了）", wh[0], wh[1])
		}
		if wh[1] == 420 && wh[0] < 520 {
			tallAttach = true
		}
	}
	if !tallPreview || !tallAttach {
		t.Errorf("竖版图没有正好顶到 token 的高度上限（preview=%v attach=%v）；实测 %s", tallPreview, tallAttach, m)
	}
	// 图片在容器里也不许撑破：每个框都在 token 与容器宽度之内（上面已逐项判定），
	// 且至少有一张真的渲染出来了。
	if !strings.Contains(m, `"loaded":`) || strings.Contains(m, `"loaded":0`) {
		t.Errorf("图片没有加载完成，尺寸不算实测；实测 %s", m)
	}
}

// TestSpacingTokensAreDeclaredOnce pins the token values in the stylesheet, so
// the geometry test above can only be satisfied by the shared token set.
func TestSpacingTokensAreDeclaredOnce(t *testing.T) {
	css, err := assets.ReadFile("assets/viewer.css")
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	for _, want := range []string{
		"--row-h: 24px;", "--row-gap: 6px;", "--divider-gap: 8px;",
		"--preview-h: 108px;", "--preview-w: 240px;",
		"--attach-img-w: 520px;", "--attach-img-h: 420px;",
		"min-height: var(--row-h);",
		"gap: var(--row-gap);",
		"margin-top: var(--divider-gap);",
		"padding-top: var(--divider-gap);",
		"border-top: .5px dashed var(--border);",
		"max-width: min(100%, var(--attach-img-w));",
		"max-height: var(--attach-img-h);",
		"max-width: var(--preview-w);",
		"max-height: var(--preview-h);",
		".io-section.attach-section {",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("样式表里缺少统一 token/用法 %q", want)
		}
	}
	// 行高与间距不允许再有硬编码（除了 token 自己的定义行）。
	for _, bad := range []string{"min-height: 24px;", "gap: 16px;", "margin-top: 16px;",
		"max-height: 160px;", "max-width: 160px;", "max-height: 220px;", "max-width: 220px;\n  }\n  .images"} {
		if strings.Contains(text, bad) {
			t.Errorf("间距/尺寸还有硬编码 %q（应当走 token）", bad)
		}
	}
}
