package sessionview

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
		`{"t":"msg","role":"tool","tool_call_id":"call_1","text":"ok (12 行)","ts":"2025-01-02T03:00:03Z"}`,
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
	// ② 会话开头的原图轮：归到第一条任务，不塞给工具。
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
		`{"t":"msg","role":"tool","tool_call_id":"call_a","text":"image attached","ts":"2025-01-02T03:00:03Z"}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_b","text":"page 1","ts":"2025-01-02T03:00:04Z"}`,
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
