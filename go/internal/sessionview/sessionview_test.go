package sessionview

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeFile creates a file (and its parent directories) with the given bytes.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// transcript builds a JSONL body from raw lines.
func transcript(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func jsonl(root string, parts ...string) string {
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestLabelForAndTitleFor(t *testing.T) {
	cases := []struct {
		rel   string
		label string
		title string
	}{
		{"work/style_session.jsonl", "style", "样式"},
		{"work/sessions/chapters.jsonl", "chapters", "章节划分"},
		{"work/sessions/convert_chapter_01.jsonl", "convert:chapter_01", "转换 · chapter_01"},
		{
			"source/sessions/vector_1000ti__" + strings.Repeat("ab12", 16) + "__Mind_map___knowledge_structure_diagram.jsonl",
			"vector:knowledge structure diagram",
			"矢量图 · knowledge structure diagram",
		},
		{"sessions/vector_figure_3.jsonl", "vector:figure 3", "矢量图 · figure 3"},
		{"work/sessions/whatever.jsonl", "会话:whatever", "whatever"},
	}
	for _, c := range cases {
		if got := LabelFor(c.rel); got != c.label {
			t.Errorf("LabelFor(%q) = %q, want %q", c.rel, got, c.label)
		}
		if got := TitleFor(c.rel); got != c.title {
			t.Errorf("TitleFor(%q) = %q, want %q", c.rel, got, c.title)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KB", 3 * 1024 * 1024: "3.00 MB"}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestHumanTokenEst pins the display form of a LOCAL estimate, which must match
// the viewer's fmtTokens (assets/viewer.js): k below a million (no decimal from
// 10k up), two decimals of M above it, and always the ≈ — a local estimate is
// never a provider number. `sessions --list` shows exactly these strings.
func TestHumanTokenEst(t *testing.T) {
	cases := map[int]string{
		0:       "≈ 0",
		12:      "≈ 12",
		999:     "≈ 999",
		1000:    "≈ 1.0k",
		1512:    "≈ 1.5k",
		9999:    "≈ 10.0k",
		10000:   "≈ 10k",
		62384:   "≈ 62k",
		999999:  "≈ 1000k",
		1000000: "≈ 1.00M",
		2880000: "≈ 2.88M",
	}
	for in, want := range cases {
		if got := HumanTokenEst(in); got != want {
			t.Errorf("HumanTokenEst(%d) = %q, want %q", in, got, want)
		}
	}
}

// TestScanLabelsCountingAndOrder covers the three things the sidebar depends
// on: which files are sessions at all, how they are labelled, and the order.
func TestScanLabelsCountingAndOrder(t *testing.T) {
	root := t.TempDir()

	stylePath := jsonl(root, "work", "style_session.jsonl")
	writeFile(t, stylePath, transcript(
		`{"t":"msg","role":"user","text":"请分析样式","images":["file://media/ab12.jpg"]}`,
		`{"t":"msg","role":"assistant","text":"好的","reasoning_content":"先看目录","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"book.md\"}"}}]}`,
	))
	convertPath := jsonl(root, "work", "sessions", "convert_chapter_01.jsonl")
	writeFile(t, convertPath, transcript(
		`{"t":"msg","role":"user","text":"转换第一章"}`,
		`{"t":"msg","role":"tool","tool_call_id":"c1","text":"ok (34 行)"}`,
		`{"t":"msg","role":"assistant","text":"已提交"}`,
	))
	chaptersPath := jsonl(root, "work", "sessions", "chapters.jsonl")
	writeFile(t, chaptersPath, transcript(`{"t":"msg","role":"user","text":"划分章节"}`))
	vectorPath := jsonl(root, "source", "sessions", "vector_book__"+strings.Repeat("f0", 32)+"__venn_diagram.jsonl")
	writeFile(t, vectorPath, transcript(`{"t":"msg","role":"user","text":"画图"}`))

	// Things a scan must ignore.
	writeFile(t, jsonl(root, "work", "sessions", "media", "decoy.jsonl"), transcript(`{"t":"msg","role":"user","text":"不该出现"}`))
	writeFile(t, jsonl(root, "work", "book.md"), "# not a transcript\n")

	// A deterministic order: newest mtime first.
	old := time.Now().Add(-2 * time.Hour)
	for _, p := range []string{stylePath, convertPath, chaptersPath, vectorPath} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	newest := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(vectorPath, newest, newest); err != nil {
		t.Fatal(err)
	}

	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var ids []string
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	want := []string{
		"source/sessions/vector_book__" + strings.Repeat("f0", 32) + "__venn_diagram.jsonl",
		"work/sessions/chapters.jsonl",
		"work/sessions/convert_chapter_01.jsonl",
		"work/style_session.jsonl",
	}
	if strings.Join(ids, "|") != strings.Join(want, "|") {
		t.Fatalf("Scan ids = %v, want %v", ids, want)
	}

	byID := map[string]SessionInfo{}
	for _, s := range sessions {
		byID[s.ID] = s
	}
	if len(byID) != 4 {
		t.Fatalf("scanned %d sessions, want 4 (media/ and non-jsonl files are skipped)", len(byID))
	}
	if got := byID["work/style_session.jsonl"]; got.Label != "style" || got.Title != "样式" || got.Messages != 2 {
		t.Errorf("style session = %+v, want label=style title=样式 messages=2", got)
	}
	if got := byID["work/sessions/convert_chapter_01.jsonl"]; got.Label != "convert:chapter_01" || got.Messages != 3 {
		t.Errorf("convert session = %+v, want convert:chapter_01 with 3 messages", got)
	}
	if got := byID["work/sessions/chapters.jsonl"]; got.Label != "chapters" || got.Messages != 1 {
		t.Errorf("chapters session = %+v, want chapters with 1 message", got)
	}
	if got := byID["source/sessions/vector_book__"+strings.Repeat("f0", 32)+"__venn_diagram.jsonl"]; got.Title != "矢量图 · venn diagram" || !got.Live {
		t.Errorf("vector session = %+v, want 矢量图 · venn diagram and live", got)
	}
	if got := byID["work/style_session.jsonl"]; got.Live || got.Bytes == 0 || got.ModTime.IsZero() {
		t.Errorf("stale session still reported live or without metadata: %+v", got)
	}
}

// TestReadSessionIncremental pins the (fromLine, nextFrom) contract the live
// viewer polls with, including the treatment of a half-written last line.
func TestReadSessionIncremental(t *testing.T) {
	root := t.TempDir()
	path := jsonl(root, "work", "sessions", "convert_chapter_01.jsonl")
	writeFile(t, path, transcript(
		`{"t":"msg","role":"user","text":"转换第一章","images":["file://media/ab12.jpg"]}`,
		`{"t":"msg","role":"assistant","reasoning_content":"先看手册","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"manual.md\"}"}}]}`,
		`this line is not json at all`,
		`{"t":"msg","role":"tool","tool_call_id":"c1","text":"ok (12 行)"}`,
		`{"t":"meta","label":"convert"}`,
	))

	lines, next, err := ReadSession(path, 0)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if next != 5 || len(lines) != 5 {
		t.Fatalf("read %d lines nextFrom=%d, want 5/5", len(lines), next)
	}
	if !lines[2].Bad || len(lines[2].Raw) == 0 {
		t.Errorf("line 3 should be reported as bad with its raw bytes: %+v", lines[2])
	}
	if lines[0].Role != "user" || len(lines[0].Images) != 1 || lines[0].Images[0] != "file://media/ab12.jpg" {
		t.Errorf("user line not parsed: %+v", lines[0])
	}
	if lines[1].Reasoning != "先看手册" || len(lines[1].Calls) != 1 || lines[1].Calls[0].Function.Name != "read_file" {
		t.Errorf("assistant line lost its reasoning or tool call: %+v", lines[1])
	}
	if lines[3].CallID != "c1" {
		t.Errorf("tool line lost tool_call_id: %+v", lines[3])
	}
	if lines[4].Type != "meta" {
		t.Errorf("meta line type = %q, want meta", lines[4].Type)
	}

	// Incremental: only what was appended since the last read.
	lines, next, err = ReadSession(path, next)
	if err != nil {
		t.Fatalf("ReadSession(from=5): %v", err)
	}
	if len(lines) != 0 || next != 5 {
		t.Fatalf("incremental read returned %d lines nextFrom=%d, want 0/5", len(lines), next)
	}

	writeFile(t, path, transcript(
		`{"t":"msg","role":"user","text":"转换第一章"}`,
		`{"t":"msg","role":"assistant","text":"好的"}`,
	))
	lines, next, err = ReadSession(path, 5)
	if err != nil {
		t.Fatalf("ReadSession after rewrite: %v", err)
	}
	if len(lines) != 0 || next != 2 {
		t.Fatalf("shrunk file: %d lines nextFrom=%d, want 0/2", len(lines), next)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"t":"msg","role":"user","text":"再来一次"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	lines, next, err = ReadSession(path, next)
	if err != nil {
		t.Fatalf("ReadSession appended: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "再来一次" || next != 3 {
		t.Fatalf("append read = %+v nextFrom=%d, want the new line / 3", lines, next)
	}
}

// TestReadSessionHalfWrittenTail proves a torn last line is not consumed: the
// next poll (after the writer finishes the line) still sees it.
func TestReadSessionHalfWrittenTail(t *testing.T) {
	root := t.TempDir()
	path := jsonl(root, "work", "style_session.jsonl")
	writeFile(t, path, `{"t":"msg","role":"user","text":"完整的一行"}`+"\n"+`{"t":"msg","role":"user","text":"写到一半`)

	lines, next, err := ReadSession(path, 0)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if len(lines) != 1 || next != 1 {
		t.Fatalf("read %d lines nextFrom=%d, want the torn line to be left alone (1/1)", len(lines), next)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\"}\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	lines, next, err = ReadSession(path, next)
	if err != nil {
		t.Fatalf("ReadSession after the line was finished: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "写到一半" || next != 2 {
		t.Fatalf("finished line = %+v nextFrom=%d, want the completed message / 2", lines, next)
	}
}

func TestWriteStaticHTML(t *testing.T) {
	root := t.TempDir()
	stylePath := jsonl(root, "work", "style_session.jsonl")
	// The text carries both an HTML tag and a script terminator: the second is
	// what would break out of the data block if it were not escaped.
	writeFile(t, stylePath, transcript(
		`{"t":"msg","role":"user","text":"看图 <b>加粗</b> 与 </script><img src=x onerror=alert(1)>"}`,
		`{"t":"msg","role":"assistant","text":"收到","reasoning_content":"思考中"}`,
	))
	writeFile(t, jsonl(root, "work", "sessions", "convert_chapter_01.jsonl"), transcript(
		`{"t":"msg","role":"tool","tool_call_id":"c1","text":"ok (12 行)"}`,
	))
	writeFile(t, jsonl(root, "work", "media", "ab12.jpg"), "not really a jpeg")

	out := filepath.Join(root, "sessions.html")
	if err := WriteStaticHTML(root, out, nil); err != nil {
		t.Fatalf("WriteStaticHTML: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	if !strings.Contains(page, `<script type="application/json" id="dsh-data">`) {
		t.Error("static page has no embedded data block")
	}
	if !strings.Contains(page, `\u003c/script\u003e`) {
		t.Error("a </script> inside a transcript was not escaped in the data block")
	}
	if strings.Contains(page, `</script><img src=x`) {
		t.Error("transcript content escaped the data block")
	}
	if !strings.Contains(page, "DocVision session viewer") {
		t.Error("the script was not inlined, so the snapshot is not self-contained")
	}
	if !strings.Contains(page, "--mono:") || !strings.Contains(page, `html[data-theme="dark"]`) {
		t.Error("the stylesheet was not inlined")
	}
	if strings.Contains(page, "viewer.js") || strings.Contains(page, "viewer.css") {
		t.Error("the static page still references external assets")
	}

	var payload pageData
	start := strings.Index(page, `id="dsh-data">`) + len(`id="dsh-data">`)
	end := strings.Index(page[start:], `</script>`)
	if err := json.Unmarshal([]byte(page[start:start+end]), &payload); err != nil {
		t.Fatalf("embedded data is not valid JSON: %v", err)
	}
	if payload.Mode != "static" || payload.MediaRoot != "" {
		t.Errorf("payload mode/mediaRoot = %q/%q, want static/empty (the page sits in root)", payload.Mode, payload.MediaRoot)
	}
	if len(payload.Sessions) != 2 {
		t.Fatalf("embedded %d sessions, want 2", len(payload.Sessions))
	}
	if payload.Sessions[0].ID != "work/sessions/convert_chapter_01.jsonl" && payload.Sessions[1].ID != "work/sessions/convert_chapter_01.jsonl" {
		t.Errorf("convert session missing from the snapshot: %+v", payload.Sessions)
	}
	for _, s := range payload.Sessions {
		if s.ID == "work/style_session.jsonl" && len(s.Lines) != 2 {
			t.Errorf("style session embedded %d lines, want 2", len(s.Lines))
		}
	}

	// An export outside root still points at the media directories via a
	// relative prefix.
	elsewhere := filepath.Join(t.TempDir(), "snap", "sessions.html")
	if err := WriteStaticHTML(root, elsewhere, nil); err != nil {
		t.Fatalf("WriteStaticHTML(outside root): %v", err)
	}
	other, err := os.ReadFile(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(other), `"mediaRoot":"../../`) {
		t.Errorf("mediaRoot for an out-of-tree export is wrong: %s", firstLineWith(string(other), "mediaRoot"))
	}
}

func firstLineWith(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return "(not found)"
	}
	end := i + 80
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}

func TestServeAPI(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "proj")
	stylePath := jsonl(root, "work", "style_session.jsonl")
	writeFile(t, stylePath, transcript(
		`{"t":"msg","role":"user","text":"分析样式"}`,
		`{"t":"msg","role":"assistant","text":"好的"}`,
	))
	writeFile(t, jsonl(root, "work", "sessions", "chapters.jsonl"), transcript(`{"t":"msg","role":"user","text":"划分章节"}`))
	writeFile(t, jsonl(root, "work", "media", "ab12.jpg"), "jpeg-bytes")
	// A real file outside the root, so a traversal attempt would succeed if
	// path validation were missing.
	writeFile(t, filepath.Join(base, "outside-secret.txt"), "secret")

	srv := httptest.NewServer(newViewerServer(root))
	defer srv.Close()

	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		var buf strings.Builder
		if _, err := io.Copy(&buf, resp.Body); err != nil {
			t.Fatal(err)
		}
		return resp, buf.String()
	}

	// The page itself: live mode means no embedded data and a local script.
	resp, page := get("/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d", resp.StatusCode)
	}
	if strings.Contains(page, `id="dsh-data"`) {
		t.Error("the served page must not carry embedded data (that is what selects static mode)")
	}
	if !strings.Contains(page, "viewer.js") {
		t.Error("the served page does not reference the external script")
	}

	// /api/index reports every session with the size+mtime the page polls on.
	resp, body := get("/api/index")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/index = %d", resp.StatusCode)
	}
	var idx indexResponse
	if err := json.Unmarshal([]byte(body), &idx); err != nil {
		t.Fatalf("/api/index is not JSON: %v", err)
	}
	if len(idx.Sessions) != 2 {
		t.Fatalf("/api/index returned %d sessions, want 2", len(idx.Sessions))
	}
	found := false
	for _, s := range idx.Sessions {
		if s.ID == "work/style_session.jsonl" {
			found = true
			if s.Messages != 2 || s.Bytes == 0 || s.ModTime.IsZero() {
				t.Errorf("index entry lacks metadata: %+v", s)
			}
		}
	}
	if !found {
		t.Fatalf("style session missing from /api/index: %+v", idx.Sessions)
	}

	// Incremental reads.
	resp, body = get("/api/session?id=work/style_session.jsonl&from=0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/session = %d", resp.StatusCode)
	}
	var first sessionResponse
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatalf("/api/session is not JSON: %v", err)
	}
	if len(first.Lines) != 2 || first.NextFrom != 2 {
		t.Fatalf("first read: %d lines nextFrom=%d, want 2/2", len(first.Lines), first.NextFrom)
	}
	_, body = get("/api/session?id=work/style_session.jsonl&from=" + strconv.Itoa(first.NextFrom))
	var empty sessionResponse
	if err := json.Unmarshal([]byte(body), &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.Lines) != 0 || empty.NextFrom != 2 {
		t.Fatalf("no-op read: %d lines nextFrom=%d, want 0/2", len(empty.Lines), empty.NextFrom)
	}

	f, err := os.OpenFile(stylePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"t":"msg","role":"tool","tool_call_id":"c1","text":"ok (1 行)"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, body = get("/api/session?id=work/style_session.jsonl&from=2")
	var second sessionResponse
	if err := json.Unmarshal([]byte(body), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Lines) != 1 || second.Lines[0].CallID != "c1" || second.NextFrom != 3 {
		t.Fatalf("incremental read = %+v nextFrom=%d, want the appended tool line / 3", second.Lines, second.NextFrom)
	}
	// The cached message count must follow the file, not the first scan.
	_, body = get("/api/index")
	if err := json.Unmarshal([]byte(body), &idx); err != nil {
		t.Fatal(err)
	}
	for _, s := range idx.Sessions {
		if s.ID == "work/style_session.jsonl" && s.Messages != 3 {
			t.Errorf("cached message count is stale: %+v", s)
		}
	}

	// Media is served read-only from the root.
	resp, body = get("/media/work/media/ab12.jpg")
	if resp.StatusCode != http.StatusOK || body != "jpeg-bytes" {
		t.Fatalf("GET /media/... = %d %q", resp.StatusCode, body)
	}
	resp, _ = get("/file/work/media/ab12.jpg")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /file/... = %d, want 200", resp.StatusCode)
	}

	// Traversal and unknown ids are refused.
	for _, path := range []string{
		"/media/../outside-secret.txt",
		"/media/work/../../outside-secret.txt",
		"/media/%2e%2e/outside-secret.txt",
		"/file/..%2foutside-secret.txt",
	} {
		resp, _ := get(path)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 400 or 404", path, resp.StatusCode)
		}
	}
	resp, _ = get("/api/session?id=../outside-secret.txt&from=0")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown session id = %d, want 404", resp.StatusCode)
	}
	resp, _ = get("/api/session?id=work/style_session.jsonl&from=abc")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad from = %d, want 400", resp.StatusCode)
	}
	resp, _ = get("/media/work/sessions")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("directory request = %d, want 404 (no listings)", resp.StatusCode)
	}
}

func TestResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	ok := []string{"work/style_session.jsonl", "/work/style_session.jsonl", "a/./b.txt"}
	for _, rel := range ok {
		if _, got := resolveUnderRoot(root, rel); !got {
			t.Errorf("resolveUnderRoot(%q) rejected a legitimate path", rel)
		}
	}
	bad := []string{"", "/", "..", "../secret", "work/../../secret", "/../secret", "a/../../b"}
	for _, rel := range bad {
		if _, got := resolveUnderRoot(root, rel); got {
			t.Errorf("resolveUnderRoot(%q) accepted an escaping path", rel)
		}
	}
}

func TestIndexOnMissingRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("Scan of a missing root should fail so the CLI can report it")
	}
}

/* ===================== 第二十四批：meta 行 / 项目分组 / 主题 ===================== */

// readAsset reads one embedded viewer resource, so the assertions below run
// against exactly what the binary ships.
func readAsset(t *testing.T, name string) string {
	t.Helper()
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		t.Fatalf("读取内嵌资源 %s: %v", name, err)
	}
	return string(data)
}

func TestProjectFor(t *testing.T) {
	cases := map[string]string{
		"latex_project/work/style_session.jsonl":                    "latex_project",
		"latex_project_0909/work/sessions/convert_chapter_01.jsonl": "latex_project_0909",
		"latex_project/书名/work/sessions/convert_chapter_01.jsonl":   "latex_project",
		"sessions.jsonl":   RootProject,
		"./sessions.jsonl": RootProject,
		"":                 RootProject,
		"a/b/c/d.jsonl":    "a",
	}
	for rel, want := range cases {
		if got := ProjectFor(rel); got != want {
			t.Errorf("ProjectFor(%q) = %q, want %q", rel, got, want)
		}
	}
}

// TestScanProjectsAndMeta pins what the grouped sidebar needs: the project of
// every session, the newest meta line summarised, meta lines never counted as
// messages, and a transcript without any meta line still scanning cleanly.
func TestScanProjectsAndMeta(t *testing.T) {
	root := t.TempDir()

	// A session with two meta lines (a resumed run rewrote the prompt): only
	// the newest one may drive the summary.
	stylePath := jsonl(root, "latex_project", "work", "style_session.jsonl")
	writeFile(t, stylePath, transcript(
		`{"t":"meta","kind":"system","session_label":"style","model":"glm-4.6-OLD","system_sha":"0000000000000000","text":"旧提示词","tools":[{"name":"read_file","description":"读文件","parameters":{"type":"object"}}]}`,
		`{"t":"msg","role":"user","text":"请分析样式"}`,
		`{"t":"meta","kind":"system","session_label":"style","model":"glm-4.6","system_sha":"ab12cd34deadbeef","text":"新提示词一二三","tools":[{"name":"read_file","description":"读文件","parameters":{"type":"object","properties":{"path":{"type":"string"}}}},{"name":"submit","description":"提交"}]}`,
		`{"t":"msg","role":"assistant","text":"好的","reasoning_content":"先看目录"}`,
	))
	// A transcript written before meta lines existed: it must scan without a
	// meta summary and without an error.
	oldPath := jsonl(root, "latex_project_0909", "work", "sessions", "convert_chapter_01.jsonl")
	writeFile(t, oldPath, transcript(
		`{"t":"msg","role":"user","text":"转换第一章"}`,
		`{"t":"msg","role":"assistant","text":"已提交"}`,
	))
	// A nested book under the same project: still the same project group.
	bookPath := jsonl(root, "latex_project", "书名", "work", "sessions", "chapters.jsonl")
	writeFile(t, bookPath, transcript(`{"t":"msg","role":"user","text":"划分章节"}`))
	// A loose transcript directly in the scan root.
	loosePath := jsonl(root, "loose.jsonl")
	writeFile(t, loosePath, transcript(`{"t":"msg","role":"user","text":"根目录会话"}`))

	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byID := map[string]SessionInfo{}
	for _, s := range sessions {
		byID[s.ID] = s
	}
	if len(byID) != 4 {
		t.Fatalf("扫描到 %d 个会话，want 4", len(byID))
	}

	style := byID["latex_project/work/style_session.jsonl"]
	if style.Project != "latex_project" {
		t.Errorf("style 会话 project = %q, want latex_project", style.Project)
	}
	if style.Messages != 2 {
		t.Errorf("style 会话 messages = %d, want 2（meta 行不计入消息数）", style.Messages)
	}
	if style.Meta == nil {
		t.Fatalf("style 会话没有 meta 摘要：%+v", style)
	}
	if style.Meta.Count != 2 {
		t.Errorf("meta count = %d, want 2", style.Meta.Count)
	}
	if style.Meta.Model != "glm-4.6" || style.Meta.SystemSHA != "ab12cd34deadbeef" || style.Meta.SessionLabel != "style" {
		t.Errorf("meta 摘要没有取最新一条：%+v", style.Meta)
	}
	if style.Meta.PromptChars != len([]rune("新提示词一二三")) {
		t.Errorf("promptChars = %d, want %d", style.Meta.PromptChars, len([]rune("新提示词一二三")))
	}
	if style.Meta.Tools != 2 {
		t.Errorf("meta tools = %d, want 2", style.Meta.Tools)
	}

	if got := byID["latex_project_0909/work/sessions/convert_chapter_01.jsonl"]; got.Project != "latex_project_0909" || got.Meta != nil || got.Messages != 2 {
		t.Errorf("老转录（无 meta 行）扫描结果不对：%+v", got)
	}
	// 多项目布局：书名目录本身就是工作区（含 work/），因此它自己是一个组，
	// 组名带上容器（输出根），否则一个输出根下的所有书会挤成一组。
	if got := byID["latex_project/书名/work/sessions/chapters.jsonl"]; got.Project != "latex_project/书名" {
		t.Errorf("书名目录的 project = %q, want latex_project/书名", got.Project)
	}
	// latex_project 下直接有 work/：它是旧版单项目工程本身，标成 legacy。
	if !style.ProjectLegacy {
		t.Errorf("旧版单项目组的 legacy 标记 = false，want true（%+v）", style)
	}
	if got := byID["loose.jsonl"]; got.Project != RootProject {
		t.Errorf("根目录直挂的 project = %q, want %q", got.Project, RootProject)
	}
}

// TestReadSessionMetaLine checks the parsed shape of a meta line: the viewer
// needs the prompt, the model, the short hash and the tool definitions, while
// the line must stay out of the message stream (no role).
func TestReadSessionMetaLine(t *testing.T) {
	root := t.TempDir()
	path := jsonl(root, "p", "work", "style_session.jsonl")
	writeFile(t, path, transcript(
		`{"t":"meta","kind":"system","session_label":"convert:chapter_01","model":"glm-4.6","system_sha":"ab12cd34deadbeef","text":"你是转换助手。","tools":[{"name":"read_file","description":"读文件","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]}`,
		`{"t":"msg","role":"user","text":"开始"}`,
	))

	lines, next, err := ReadSession(path, 0)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if next != 2 || len(lines) != 2 {
		t.Fatalf("读出 %d 行 nextFrom=%d, want 2/2", len(lines), next)
	}
	meta := lines[0]
	if !meta.IsMeta() || meta.Type != "meta" {
		t.Fatalf("第一行不是 meta 行：%+v", meta)
	}
	if meta.Role != "" {
		t.Errorf("meta 行不该有 role（否则会被当成一条正文消息渲染）：%q", meta.Role)
	}
	if meta.Text != "你是转换助手。" || meta.Model != "glm-4.6" || meta.SystemSHA != "ab12cd34deadbeef" || meta.SessionLabel != "convert:chapter_01" || meta.Kind != "system" {
		t.Errorf("meta 字段解析不完整：%+v", meta)
	}
	if len(meta.Tools) != 1 || meta.Tools[0].Name != "read_file" || meta.Tools[0].Description != "读文件" {
		t.Fatalf("meta 行的工具定义丢失：%+v", meta.Tools)
	}
	if !strings.Contains(meta.Tools[0].Parameters, `"path"`) {
		t.Errorf("meta 行的 parameters 没有保留原始 JSON：%q", meta.Tools[0].Parameters)
	}
	if lines[1].IsMeta() {
		t.Errorf("普通消息被当成 meta 行：%+v", lines[1])
	}

	// 老转录（一条 meta 行都没有）必须照旧可读。
	oldPath := jsonl(root, "p", "work", "sessions", "convert_chapter_01.jsonl")
	writeFile(t, oldPath, transcript(
		`{"t":"msg","role":"user","text":"老会话"}`,
		`{"t":"msg","role":"assistant","text":"好的"}`,
	))
	oldLines, _, err := ReadSession(oldPath, 0)
	if err != nil {
		t.Fatalf("读取老转录失败：%v", err)
	}
	for _, l := range oldLines {
		if l.IsMeta() || len(l.Tools) != 0 {
			t.Errorf("老转录里出现了 meta 行：%+v", l)
		}
	}
	if sessions, err := Scan(root); err != nil {
		t.Fatalf("扫描含老转录的根目录失败：%v", err)
	} else {
		for _, s := range sessions {
			if strings.Contains(s.ID, "convert_chapter_01") && s.Meta != nil {
				t.Errorf("没有 meta 行的转录却有 meta 摘要：%+v", s.Meta)
			}
		}
	}
}

// TestStaticHTMLMetaAndGroups proves the exported snapshot carries everything
// the grouped sidebar and the meta card need, and that the shipped page has the
// theme switch and the meta card renderer in it.
func TestStaticHTMLMetaAndGroups(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "latex_project", "work", "style_session.jsonl"), transcript(
		`{"t":"meta","kind":"system","session_label":"style","model":"glm-4.6","system_sha":"ab12cd34deadbeef","text":"系统提示词正文","tools":[{"name":"read_file","description":"读文件","parameters":{"type":"object"}}]}`,
		`{"t":"msg","role":"user","text":"分析样式"}`,
		`{"t":"msg","role":"assistant","reasoning_content":"思考一下","text":"好的"}`,
	))
	writeFile(t, jsonl(root, "latex_project_0909", "work", "sessions", "convert_chapter_01.jsonl"), transcript(
		`{"t":"msg","role":"user","text":"转换"}`,
	))

	out := filepath.Join(root, "sessions.html")
	if err := WriteStaticHTML(root, out, nil); err != nil {
		t.Fatalf("WriteStaticHTML: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	// 页面资源里的关键标识：主题切换、项目分组容器、meta 卡片、思考滚动区。
	for _, marker := range []string{
		`id="theme-toggle"`,
		`data-theme="light"`,
		"dsh.sessionview.theme",
		"proj-group",
		"meta-card",
		"系统提示词（本次运行快照，不参与回放）",
		"reasoning-scroll",
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("静态页面缺少关键标识 %q", marker)
		}
	}

	start := strings.Index(page, `id="dsh-data">`) + len(`id="dsh-data">`)
	end := strings.Index(page[start:], `</script>`)
	var payload pageData
	if err := json.Unmarshal([]byte(page[start:start+end]), &payload); err != nil {
		t.Fatalf("内嵌数据不是合法 JSON: %v", err)
	}
	if len(payload.Sessions) != 2 {
		t.Fatalf("内嵌 %d 个会话, want 2", len(payload.Sessions))
	}
	byID := map[string]staticSession{}
	for _, s := range payload.Sessions {
		byID[s.ID] = s
	}
	style, ok := byID["latex_project/work/style_session.jsonl"]
	if !ok {
		t.Fatalf("内嵌数据里没有 style 会话：%+v", byID)
	}
	if style.Project != "latex_project" {
		t.Errorf("内嵌数据的 project = %q, want latex_project（左侧分组依赖它）", style.Project)
	}
	if style.Meta == nil || style.Meta.Count != 1 || style.Meta.PromptChars != 7 || style.Meta.Model != "glm-4.6" || style.Meta.Tools != 1 {
		t.Fatalf("内嵌数据的 meta 摘要不对：%+v", style.Meta)
	}
	var metaLine *Line
	for i := range style.Lines {
		if style.Lines[i].IsMeta() {
			metaLine = &style.Lines[i]
		}
	}
	if metaLine == nil {
		t.Fatalf("内嵌数据里没有 meta 行：%+v", style.Lines)
	}
	if metaLine.Text != "系统提示词正文" || metaLine.Model != "glm-4.6" || metaLine.SystemSHA != "ab12cd34deadbeef" {
		t.Errorf("内嵌的 meta 行少了字段：%+v", metaLine)
	}
	if len(metaLine.Tools) != 1 || metaLine.Tools[0].Name != "read_file" || !strings.Contains(metaLine.Tools[0].Parameters, "object") {
		t.Errorf("内嵌的 meta 行没有工具定义：%+v", metaLine.Tools)
	}
	var reasoning *Line
	for i := range style.Lines {
		if style.Lines[i].Reasoning != "" {
			reasoning = &style.Lines[i]
		}
	}
	if reasoning == nil || reasoning.Reasoning != "思考一下" {
		t.Errorf("内嵌数据丢了推理内容：%+v", reasoning)
	}

	if got := byID["latex_project_0909/work/sessions/convert_chapter_01.jsonl"]; got.Project != "latex_project_0909" || got.Meta != nil {
		t.Errorf("老转录的内嵌数据不对：project=%q meta=%+v", got.Project, got.Meta)
	}
}

// TestViewerAssetsThemeAndMeta is the guard for the two things this batch is
// about: the theme switch must default to daylight (dark is an override), and
// the meta/reasoning rendering must stay in the shipped assets.
func TestViewerAssetsThemeAndMeta(t *testing.T) {
	css := readAsset(t, "viewer.css")
	js := readAsset(t, "viewer.js")
	html := readAsset(t, "viewer.html")

	// 浅色是默认：:root 里就是浅色底，深色只是 html[data-theme="dark"] 的一层覆盖。
	if !strings.Contains(css, `html[data-theme="dark"]`) {
		t.Error("样式表里没有深色主题覆盖块")
	}
	// 从 ":root {" 量到深色覆盖块（文件头的注释里也提到过这个选择器）。
	rootBlock := css[strings.Index(css, ":root {"):strings.Index(css, `html[data-theme="dark"] {`)]
	if strings.Contains(rootBlock, "--bg: #0f0f10") {
		t.Error("深色底出现在 :root 里：默认主题必须是白天模式")
	}
	// :root 必须是浅色默认（页面底色为白/近白），深色只在覆盖块里出现。
	if !strings.Contains(rootBlock, "--bg: #ffffff") || !strings.Contains(rootBlock, "--text: #0f1115") {
		t.Error(":root 不是浅色默认配色")
	}
	if !strings.Contains(css, "--mono:") || !strings.Contains(css, ".reasoning-scroll, .prompt-scroll { max-height") {
		t.Error("思考内容缺少等宽字体变量或最大高度滚动")
	}

	// 主题切换只改根元素上的一个属性，并写进 localStorage。
	if !strings.Contains(html, `data-theme="light"`) {
		t.Error("页面没有默认的浅色主题属性")
	}
	if !strings.Contains(html, "dsh.sessionview.theme") || !strings.Contains(html, "setAttribute('data-theme'") {
		t.Error("首帧前的主题应用脚本缺失")
	}
	if !strings.Contains(html, "var theme = 'light';") {
		t.Error("首帧前的主题回退值不是浅色：默认必须是白天模式")
	}
	if !strings.Contains(js, "setAttribute('data-theme'") {
		t.Error("脚本没有通过根元素属性切换主题")
	}
	if !strings.Contains(js, "THEME_KEY = 'theme'") || !strings.Contains(js, "DEFAULT_THEME = 'light'") {
		t.Error("主题记忆键或默认值不对（必须默认浅色）")
	}
	if !strings.Contains(js, "function toggleTheme") || !strings.Contains(js, "themeToggle.addEventListener") {
		t.Error("主题切换按钮没有接线")
	}

	// 思考过程：折叠时就是**一行**（标签 + 圆点 + 一行摘要 + 字符数），展开后
	// 正文进带最大高度的滚动容器，并整体缩进 22px。
	if !strings.Contains(js, "!state.forceCollapse && remembered") {
		t.Error("思考块不再默认折叠（「折叠全部思考」开关会失效）")
	}
	if !strings.Contains(js, "countText(line.reasoning.length, estOf(line).reasoning)") {
		t.Error("思考摘要行没有写清计数值（按显示单位：token 估算 / 字符）")
	}
	if !strings.Contains(js, "reasoning-scroll") {
		t.Error("思考正文没有放进带最大高度的滚动容器")
	}
	if !strings.Contains(js, "function disclosureLine") ||
		!strings.Contains(css, ".thinking-body { padding: 4px 0 6px 22px; }") {
		t.Error("思考块没有走 DSH 的单行折叠行形态（disclosureLine + 展开缩进）")
	}

	// meta 卡片：标题、最新一条、工具二级折叠。这一块**搬到了右侧详情栏**，
	// 消息流里只剩开头一行极简摘要，所以这里断的是挂在详情栏而不是时间线上。
	for _, marker := range []string{
		"系统提示词（本次运行快照，不参与回放）",
		"共 ' + m.count + ' 条，显示最新",
		"tool-schema",
		"schema-params",
		"var meta = metaCard();",
		"refs.detailsBody.appendChild(b);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少 meta 卡片关键逻辑 %q", marker)
		}
	}

	// 侧栏按项目/阶段分组，折叠状态写进记忆并在重建后还原。
	for _, marker := range []string{
		"proj-group",
		"stage-group",
		"state.collapsed",
		"function bindCollapse",
		"node.dataset.open = wantOpen ? '1' : '0'",
		"setCollapsed(key, !node.open);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少项目分组关键逻辑 %q", marker)
		}
	}
	// 多项目布局：「输出根/书名」以书名为标题（容器名弱化成前缀），旧版
	// 单项目根标一句说明——两者在侧栏里必须能一眼分清。
	for _, marker := range []string{"proj-prefix", "proj-title", "'旧版单项目'"} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少多项目布局的组标题逻辑 %q", marker)
		}
	}
	if !strings.Contains(css, ".proj-prefix") {
		t.Error("样式表没有容器前缀样式（书名应当是视觉主体）")
	}
	// 静态快照的会话字段**整份带走**，只减掉每会话的 lines 大块。这里以前是
	// 一个手写白名单，每加一个字段（projectLegacy、stage、projectStages、
	// imageOrder、cost…）都要补一行，漏掉就在静态页里静默少一块 UI
	// （--serve 实时模式却正常）。现在钉住的是"只减字段"的写法本身。
	if !strings.Contains(js, "if (k !== 'lines') { copy[k] = s[k]; }") {
		t.Error("静态模式的会话字段映射不再是「整份带走、只减 lines」")
	}
	if strings.Contains(js, "projectLegacy: s.projectLegacy") {
		t.Error("静态模式又回到了手写字段白名单")
	}
	if !strings.Contains(js, "含系统提示词快照") {
		t.Error("会话行没有提示含系统提示词快照")
	}
}

// TestViewerMatchesDSHStructure pins the *structure* this batch rebuilt the
// preview page around: the DSH-style three-column frame with draggable
// separators, a breadcrumb + tab header, the trajectory table, the right-hand
// details panel, and — the actual user-visible bug — a polling path that does
// not rebuild the sidebar DOM when nothing changed.
func TestViewerMatchesDSHStructure(t *testing.T) {
	css := readAsset(t, "viewer.css")
	js := readAsset(t, "viewer.js")
	html := readAsset(t, "viewer.html")

	// 1. 三栏骨架：侧栏 + 中栏 + 详情栏，两条可拖分隔条。
	for _, id := range []string{
		`id="frame"`, `id="sidebar-col"`, `id="handle-sidebar"`,
		`id="center-col"`, `id="handle-details"`, `id="details-col"`,
	} {
		if !strings.Contains(html, id) {
			t.Errorf("页面缺少三栏骨架元素 %s", id)
		}
	}
	if !strings.Contains(css, "grid-template-columns") {
		t.Error("样式表没有用 grid 布三栏")
	}
	if !strings.Contains(css, "cursor: col-resize") {
		t.Error("分隔条没有拖拽光标")
	}
	for _, marker := range []string{
		"function computeColumns",
		"var SIDEBAR_AUTO_COLLAPSE = 1024",
		"var RAIL_W = 56",
		"var CENTER_MIN = 640",
		"var DETAILS_MIN = 300",
		"var DETAILS_MAX = 520",
		"function wireHandle",
		"setPointerCapture",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少三栏布局关键逻辑 %q", marker)
		}
	}

	// 2. 中栏表头 = 面包屑 + 页签（对话 / 轨迹），开关挪到页签行右端。
	for _, id := range []string{`id="crumbs"`, `id="tab-chat"`, `id="tab-traj"`, `id="details-toggle"`, `id="side-toggle"`} {
		if !strings.Contains(html, id) {
			t.Errorf("页面表头缺少 %s", id)
		}
	}
	if !strings.Contains(js, "function renderHeader") || !strings.Contains(css, ".crumb-current") {
		t.Error("面包屑没有渲染逻辑或当前段样式")
	}
	if !strings.Contains(css, "max-width: 220px") || !strings.Contains(css, ".tabs { display: flex; gap: 36px;") {
		t.Error("面包屑截断宽度或页签间距不是 DSH 的取值")
	}

	// 3. 轨迹页签是一张真表格，可筛选、可展开、能跳回对话。
	for _, marker := range []string{
		"function renderTrajectory",
		"function jumpToLine",
		"traj-table",
		"traj-chip",
		"traj-disclose",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少轨迹页关键逻辑 %q", marker)
		}
	}
	if !strings.Contains(css, ".kind-tag") || !strings.Contains(css, "table-layout: fixed") {
		t.Error("轨迹表格缺少事件类型标签或固定表格布局")
	}

	// 4. 右侧详情栏承载元信息与指标，消息流里只留一行摘要。
	for _, marker := range []string{
		"function renderDetails",
		"function detailsSessionBlock",
		"function detailsStatsBlock",
		"function toggleDetails",
		"stats-table",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少详情栏关键逻辑 %q", marker)
		}
	}
	if !strings.Contains(html, `id="details-body"`) {
		t.Error("页面没有详情栏容器")
	}

	// 7. 消息形态：**全部左对齐**（右侧气泡已去掉）、助手不套卡片、
	//    工具/思考/图片都是一行折叠行。
	for _, marker := range []string{
		"--content-w: clamp(680px, 64%, 920px)",
		".msg-system { align-items: stretch; }",
		".sys-badge",
		".disclosure-image > summary .line-name",
		"function toolDisclosure",
		"function thinkingDisclosure",
		"io-card",
	} {
		if !strings.Contains(css, marker) && !strings.Contains(js, marker) {
			t.Errorf("缺少消息形态关键逻辑 %q", marker)
		}
	}
	if strings.Contains(css, ".msg-user") || strings.Contains(css, "--user-bubble") {
		t.Error("用户消息又套回右侧浅蓝气泡了（那套已取消，消息一律左对齐）")
	}
	// 消息区里不许再出现右对齐（页签行自己用 flex-end 对齐下划线，不算）。
	msgCSS := css[strings.Index(css, ".msg { display: flex"):strings.Index(css, "/* ============================ 轨迹")]
	if strings.Contains(msgCSS, "flex-end") {
		t.Error("消息区里还有右对齐（用户消息必须和其他消息一样靠左）")
	}

	// 6. 轮询入口：签名没变就走 patchList()，一个 DOM 节点都不重建。
	if !strings.Contains(js, "function listSignature") || !strings.Contains(js, "function patchList") {
		t.Error("缺少列表签名或最小修补函数")
	}
	if !strings.Contains(js, "if (sig === state.listSig && refs.list.childElementCount) {\n      patchList();\n      return;\n    }") {
		t.Error("轮询仍然无条件重建侧栏 DOM（用户折叠的组会被刷新掉）")
	}
	// 折叠/溢出的记忆键属于视图状态，不许进数据签名，否则点一下组头就让整份列表作废。
	sig := js[strings.Index(js, "function listSignature"):]
	sig = sig[:strings.Index(sig, "\n  }")]
	if strings.Contains(sig, "state.collapsed") || strings.Contains(sig, "state.overflow") {
		t.Error("列表签名里混进了视图状态（折叠/溢出）：点组头会触发整表重建")
	}
	if strings.Contains(sig, "s.mtime") || strings.Contains(sig, "s.live") {
		t.Error("列表签名里混进了每 2 秒就变的字段（mtime/live）：轮询会一直重建 DOM")
	}
	if !strings.Contains(js, "refs.list.scrollTop = scroll;") {
		t.Error("侧栏重建后没有恢复滚动位置")
	}

	// 9. 消息正文的 Markdown 预览：开关在页签行右端、默认开启，正文走
	//    bodyBlock()（详细断言见 TestViewerMarkdownRendering）。
	for _, marker := range []string{
		"function renderMarkdown(text)",
		"function bodyBlock(text, previewLines, key, extraClass, tokens)",
		"if (!state.markdown) { return collapsibleText(text, previewLines, key, extraClass, tokens); }",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("缺少 Markdown 预览关键逻辑 %q", marker)
		}
	}
	if !strings.Contains(html, `id="md-toggle"`) {
		t.Error("页签行右端没有 Markdown 开关")
	}
}

// TestViewerMergesToolCallsAndImageTurns pins the message layout the user asked
// for: nothing is right-aligned any more, a role:"user" line is either a task
// block (no images) or an image/attachment turn (with images) that folds into
// the call it belongs to, and one tool call occupies exactly one row (its
// receipt is merged in, not printed as a second "result" row).
func TestViewerMergesToolCallsAndImageTurns(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")

	// 1. 带图 user 行 = 图片轮/回执行，不是任务块，也不是右对齐气泡。
	for _, marker := range []string{
		"var isImageTurn = line.role === 'user' && !!(line.images && line.images.length);",
		"function imageTurnRow(line, attr)",
		"disclosureLine('disclosure-image', '图片（user 轮）', imageTurnSummary(line),",
		"storeGet(key) === '1'", // 默认折叠（只认"用户展开过"）
		"function imageTurnSummary(line)",
		"bits.push(names.slice(0, 2).join('、')",
		"function originalFigureSize(text)",
		"ORIGINAL\\s+FIGURE\\s+SIZE\\s*:\\s*([0-9.]+\\s*mm\\s*[x×]\\s*[0-9.]+\\s*mm)",
		"el('section', 'msg msg-image')",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("图片轮渲染缺少 %q", marker)
		}
	}
	// 2. 不带图的 user 行 = **系统消息**（左对齐、无气泡），保留"user 轮"这个事实，
	//    并且与思考/工具同一套折叠：「系统」标签 + user 轮次级标签 + 展开全文。
	for _, marker := range []string{
		"function systemTurnSection(line)",
		"el('section', 'msg msg-system')",
		"el('span', 'sys-badge', '系统')",
		"el('span', 'sys-meta', 'user 轮')",
		"var long = lines.length > SYSTEM_PREVIEW_LINES;",
		"scroll.classList.toggle('folded', long && !expanded);",
		"toggle.textContent = foldLabel(expanded, lines.length, raw.length, tokens);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("系统消息块缺少 %q", marker)
		}
	}
	if strings.Contains(js, "task-label") || strings.Contains(css, ".msg-task") || strings.Contains(css, ".msg-user") {
		t.Error("还在用「任务」块或右侧气泡那一套（不带图的 user 行应当按系统消息呈现）")
	}
	if !strings.Contains(css, ".sys-scroll.folded") {
		t.Error("系统消息没有默认折叠的渐隐遮罩")
	}

	// 3. 图片归属：按**句柄 + 顺序**推断（第八条，不改 wire）。顺序那一半是 **FIFO**：
	//    转录里"会产生图片的调用"按出现次序排队（回执有 `Image … attached.` 证据最硬，
	//    没回执才退回工具名 view_image/view_pdf 推定），每条还没归属的图片行领走队里
	//    最早那个还没被认领、且**排在它前面**的调用；有 `(call <id>)` 的句柄精确匹配优先；
	//    带正文的图片行是任务自己的图（FIFO 不许抢）。归到调用就并进那次调用的卡片，
	//    归到任务就并进任务块，配不上才单独成行。
	for _, marker := range []string{
		"function imageAttributions()",
		"var IMAGE_TOOLS = { view_image: 1, view_pdf: 1 };",
		"var IMAGE_RESULT_RE = /^(?:Image\\s+\\S+|PDF page\\s+\\S+)[^\\n]*\\battached\\b/im;",
		"var nextUnclaimed = function (lineN) {",
		"candRound[c.id] = c.lineN;",
		"roundLast[c.lineN] = c.id;",
		"var IMAGE_HANDLE_RE = /^Tool image output\\b/i;",
		"var IMAGE_CALL_RE = /\\(call\\s+([A-Za-z0-9_.:-]+)\\)/;",
		"var IMAGE_FROM_RE = /\\bfrom\\s+([A-Za-z0-9_.:-]+)/i;",
		"function attributionText(attr)",
		"'归属：call ' + attr.callId",
		"'归属：由顺序推断（本轮的 call ' + attr.callId + '）'",
		"'归属：本会话任务（这一段是投喂给任务的原图）'",
		"function attachImages(host, line, attr)",
		"section.appendChild(el('div', 'io-label', '附件（user 轮）'));",
		"'这一轮是 user 轮发出的（' + (attr && attr.kind === 'task' ? '会话开头的原图投喂，作为任务的输入' : '工具的输入/附件') +",
		"attachImages(callNode, line, attr);",
		"attachImages(taskNode, line, attr);",
		"pendingTaskImages[attr.taskLineN].push({ line: line, attr: attr });",
		"attachImages(msg, w.line, w.attr);",
		"return anchor(imageTurnSection(line, attr), line);",
		"host.attachments = (host.attachments || 0) + imgs.length;",
		// 第 E 条：精确匹配时图片算这次调用的**输出**——贴在输出正文下面（text-wrap 里、
		// 滚动区之外），不另立附件段；卡片里只剩一行小字归属说明。
		"host.card.querySelector('.io-section.out-section')",
		"col.appendChild(imageStrip(line));",
		"'归属：call ' + attr.callId + (attr.name ? '（' + attr.name + '，精确匹配）' : '（精确匹配）')",
		// 第 D 条：看图类调用的缩略图预览行，紧跟在那一行下面、折叠态就可见；
		// 一轮多 view 时全挂在这一轮**最后一个** view 行下面（按行号重排后再画）。
		"function addPreview(host, line)",
		"sec.insertBefore(strip, host.details.nextSibling);",
		"strip.__items.sort(function (a, b) { return a.n - b.n; });",
		"function previewHost(host, attr)",
		"if (host.details) { addPreview(previewHost(host, attr), line); }",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("图片归属逻辑缺少 %q", marker)
		}
	}
	// 归属结果要缓存（渲染是增量的），并且跟着会话/行数失效。
	if !strings.Contains(js, "state.imgAttr = { sig: sig, map: map, candRound: candRound, roundLast: roundLast };") {
		t.Error("归属结果没有按会话 + 行数缓存")
	}
	// 带图 user 行归到工具调用时只登记锚点：卡片已经在它那条消息的 section 里，
	// 再把它交给调用方 appendChild 会被拽到 .stream 上，行距/回合分隔线就跟别的工具行不一样。
	if !strings.Contains(js, "anchor(callNode.details, line);\n            return null;") {
		t.Error("图片轮的卡片被从消息 section 里拽出来了（行间距会与其他工具行不一致）")
	}

	// 4. 一次调用 = 一行：回执并进同一个卡片（返回 null，不再另起一行），
	//    折叠态一行写全 `输入 → 输出 字符数 · ok/error`；配不上的回执注明。
	for _, marker := range []string{
		"function updateCallTail(node)",
		"tail = countText(node.argsChars, node.argsTokens) + ' → ' +",
		"'(未配对的工具回执)'",
		"detail: { input: prettyJSON(args) || args }",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("调用/回执合并逻辑缺少 %q", marker)
		}
	}
	// 配对上的回执行必须**不产生**第二个节点。
	if !strings.Contains(js, "if (node) {\n        attachResult(node, line);") {
		t.Error("回执没有并入调用卡片")
	}

	// 5. 轨迹页**保持真实结构**：一行一步、类型照真实角色（用户/助手/思考/工具/
	//    结果/元信息/用量），工具调用与结果各自独立两行；唯一增量是给带图的
	//    user 轮标出 `图片 ×N`，并且点行跳回对话里合并后的那一块。
	for _, marker := range []string{
		"{ id: 'user', label: '用户' }",
		"{ id: 'tool', label: '工具' }",
		"{ id: 'result', label: '结果' }",
		"kind: 'user', tag: '用户', name: '用户',",
		"summary: (fromTool ? '接上一行工具回执 · ' : '') + '图片 ×' + line.images.length + ' · ' +",
		"title: IMAGE_WIRE_TITLE + '\\n' + attributionText(attr) +",
		"var jumpTo = attr.kind === 'call' && attr.callId && state.callNodes[attr.callId]",
		"images: line.images,",
		"kind: 'result', tag: '结果',",
		"labels = { prompt: '系统提示词', thinking: '思考', user: '用户消息', message: '助手消息', input: '输入', output: '输出', request: '用量行' }",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("轨迹页缺少 %q", marker)
		}
	}
	if strings.Contains(js, "{ id: 'task', label: '任务' }") || strings.Contains(js, "{ id: 'image', label: '图片' }") {
		t.Error("轨迹页被改成了对话页那套合并分类（轨迹要保持真实结构）")
	}
	if !strings.Contains(js, "if (k === 'input' || k === 'request' || k === 'output')") {
		t.Error("轨迹的输入/输出没有走高亮渲染")
	}
	// 轨迹里的带图 user 轮要写明 wire 真相，并与上一行工具回执互指（但仍是两行）。
	for _, marker := range []string{
		"var IMAGE_WIRE_TITLE = 'user 消息承载图片（tool 消息的 content 只能文本，OpenAI 兼容 schema 限制）'",
		" · text + image_url(data:image/jpeg;base64,…)'",
		"var IMAGE_PLACEHOLDER = /\\[\\s*image\\b|\\[\\s*图片|图片见|image omitted/i;",
		"' · 图片见下一行用户轮'",
		"' · 图片在下一行用户轮里'",
		"if (imageNext) { imageAfterTool[after.n] = true; }",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("轨迹里的图片 wire 说明缺少 %q", marker)
		}
	}

	// 6. 「仅看工具调用」也要留着图片行（合并后的工具行 + 图片行）。
	if !strings.Contains(js, "if (state.onlyTools && line.role !== 'tool' && !isToolCall && !isImageTurn) { return null; }") {
		t.Error("「仅看工具调用」会把图片轮一并过滤掉")
	}
	// 7. 搜索：图片轮的摘要文本与文件名都要能命中。
	for _, marker := range []string{
		"s.imageShort, s.imageFile, s.imageLabel",
		"function imageDisplayName(s)",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("图片的文件名/短名没有进搜索结果 %q", marker)
		}
	}
}

// TestViewerSidebarGroupsStartCollapsed pins the new sidebar default: a project
// group and a stage group are collapsed unless the memory explicitly says the
// user opened them — except the groups holding the selected session, which open
// themselves (and stay shut once the user folds them by hand).
func TestViewerSidebarGroupsStartCollapsed(t *testing.T) {
	js := readAsset(t, "viewer.js")

	for _, marker := range []string{
		// 记忆是三态：true=折过、false=展开过、缺省=收起
		"function isCollapsed(key) { return state.collapsed[key] === true; }",
		"function hasCollapseMemory(key)",
		"state.collapsed[key] = !!collapsed;",
		// 默认收起 + 当前会话所在组自动展开
		"function groupWantOpen(key, holdsCurrent)",
		"if (hasCollapseMemory(key)) { return !isCollapsed(key); }",
		"return !!holdsCurrent;",
		"function syncGroupOpen()",
		"setGroupOpen(node, groupWantOpen(key, holdsGroup));",
		// 两个层级都走同一套判定
		"bindCollapse(wrap, key, g.matched ? true : groupWantOpen(key, holdsCurrent), !!g.matched)",
		"bindCollapse(det, skey, groupWantOpen(skey, holdsCurrent && curStage === stg), false)",
		// 用户点击仍然是唯一写回记忆的入口
		"setCollapsed(key, !node.open);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少「默认收起」关键逻辑 %q", marker)
		}
	}

	// 旧默认（wantOpen 来自 !isCollapsed(key)）不许回来。
	if strings.Contains(js, "g.matched ? true : !isCollapsed(key)") {
		t.Error("项目组又变回默认展开（wantOpen 仍来自 !isCollapsed(key)）")
	}
	if strings.Contains(js, "bindCollapse(det, skey, !isCollapsed(skey)") {
		t.Error("阶段组又变回默认展开")
	}
	// 展开状态必须**显式**落盘（false 也是记录）；靠 delete 回到"缺省"的话，
	// 缺省会被当成"未选择"，当前会话所在组就没法既自动展开又尊重手动折叠。
	if strings.Contains(js, "delete state.collapsed[key]") {
		t.Error("展开状态没有显式落盘（仍然用 delete 回到缺省）")
	}

	// 切换会话时只改 open、不重建侧栏 DOM：syncGroupOpen() 必须在 patchList 里
	// 先跑（selectSession → patchList），且不写回记忆。
	if !strings.Contains(js, "function patchList() {\n    syncGroupOpen();") {
		t.Error("切换会话没有同步组的展开状态（当前会话所在的组应当自动展开）")
	}
	if !strings.Contains(js, "node.dataset.open = open ? '1' : '0';") {
		t.Error("程序化开/关没有同步 dataset.open（会被 toggle 事件误当成用户操作写回记忆）")
	}
	// 过滤时是 frozen 强制展开，不许跟它抢。
	if !strings.Contains(js, "if (state.filter) { return; }  // 过滤时全部强制展开") {
		t.Error("syncGroupOpen 会在过滤（强制展开）时跟记忆抢 open")
	}
}

// TestViewerMarkdownRendering pins the Markdown preview: the renderer is our own
// (no library, no CDN), it builds the tree with DOM nodes only — so markup inside
// a transcript stays text instead of becoming elements — and the toggle defaults
// to on, remembers its state, and falls back to plain pre-wrap when switched off.
func TestViewerMarkdownRendering(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")
	html := readAsset(t, "viewer.html")

	// 1. 转义：整份脚本里不许出现任何 HTML 注入入口——正文只能经
	//    createElement / createTextNode 落树（`<script>` 因此就是一个文本节点，
	//    不会变成元素）。注释里可以写这个 API 的名字，赋值/调用形式一律不许出现。
	for _, bad := range []string{".innerHTML", "insertAdjacentHTML", ".outerHTML", "document.write(", "createContextualFragment"} {
		if strings.Contains(js, bad) {
			t.Errorf("脚本里出现 %s：Markdown 渲染有注入路径（正文必须只经 DOM API 落树）", bad)
		}
	}
	for _, marker := range []string{
		"function renderMarkdown(text)",
		"var frag = document.createDocumentFragment();",
		"parent.appendChild(document.createTextNode(",
		"document.createTextNode(String(text || ''))",
		"node = el('code', 'md-inline-code', m[2]);",
		"mdInline(node, m[3], (depth || 0) + 1);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 渲染器缺少「DOM 构树 / 文本节点」逻辑 %q", marker)
		}
	}
	// 链接协议白名单：`javascript:` 之类不生成 href。
	for _, marker := range []string{
		"function mdSafeURL(url)",
		"if (/^[a-z][a-z0-9+.-]*:/i.test(s)) { return ''; }",
		"node.setAttribute('rel', 'noopener noreferrer');",
		"node.setAttribute('target', '_blank');",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 链接缺少安全处理 %q", marker)
		}
	}

	// 2. 支持的语法范围：标题 / 代码块（语言标签 + 复制）/ 列表 / 引用 / 水平线 /
	//    表格 / 粗斜体删除线 / 行内代码。
	for _, marker := range []string{
		"var MD_HEADING = /^(#{1,6})\\s+(.*?)\\s*#*\\s*$/;",
		"function mdFence(line)",
		"function mdCodeBlock(code, lang)",
		"head.appendChild(el('span', 'md-code-lang', lang || 'text'));",
		"head.appendChild(copyButton(code));",
		"function mdListMarker(line)",
		"el('blockquote', 'md-quote')",
		"el('hr', 'md-hr')",
		"function mdTable(header, rows)",
		"MD_HR.test(trimmed)",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 渲染器缺少语法 %q", marker)
		}
	}
	if !strings.Contains(js, "|\\*\\*([^*]+)\\*\\*|__([^_]+)__|~~([^~]+)~~") {
		t.Error("Markdown 行内规则里没有粗体 / 删除线")
	}

	// 3. 应用位置：助手正文、用户正文、详情栏的系统提示词走渲染器；
	//    思考与工具输入输出仍然是等宽纯文本（不许 Markdown 化）。
	for _, marker := range []string{
		"var md = el('div', 'md-body sys-md');",
		"scroll.appendChild(el('pre', 'body-text sys-text', raw));",
		"msg.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'asst.' + line.n, null, estOf(line).text));",
		"promptMD.appendChild(renderMarkdown(promptText));",
		// 关掉开关就回到原来的纯文本 pre-wrap
		"if (!state.markdown) { return collapsibleText(text, previewLines, key, extraClass, tokens); }",
		"scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'));",
		// 思考 / 工具 IO 保持等宽纯文本
		"scroll.appendChild(el('pre', 'body-text reasoning-text', line.reasoning));",
		"card.appendChild(ioSection('输入', prettyJSON(argsText) || '(无参数)', false, 'in.' + (call.id || ''), argsTokens));",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 的应用位置不对：缺少 %q", marker)
		}
	}

	// 4. 开关：默认开启、落盘、能关；关掉后重新渲染整条消息流。
	for _, marker := range []string{
		"state.markdown = storeGet('markdown') !== '0';",
		"markdown: true,",
		"storeSet('markdown', state.markdown ? '1' : '0');",
		"function applyMarkdownToggle()",
		"refs.mdToggle.addEventListener('click', function () {",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 开关缺少 %q", marker)
		}
	}
	if !strings.Contains(html, `id="md-toggle" class="tab-toggle" type="button" aria-pressed="true"`) {
		t.Error("Markdown 开关不是默认按下的状态（必须默认开启）")
	}
	// 长文本折叠 / 展开沿用同一套记忆键与按钮语义。
	for _, marker := range []string{
		"function markdownText(text, previewLines, key, extraClass, tokens)",
		"var expanded = storeGet('text.' + key) === '1';",
		"storeSet('text.' + key, expanded ? '1' : '0');",
		"body.style.maxHeight = clamped ? (previewLines * 24) + 'px' : '';",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("Markdown 下的长文本折叠缺少 %q", marker)
		}
	}
	// 5. 样式：代码块 banner / 等宽 11px·19px / 单块内滚 / 表格 / 圆角用既有 token。
	for _, marker := range []string{
		".md-code-block {",
		"border-radius: var(--radius);",
		".md-code-lang {",
		"font-family: var(--mono);",
		"font-size: var(--code-font);\n  line-height: var(--code-line);",
		"max-height: 260px;\n  overflow: auto;",
		".md-table th, .md-table td {",
		".md-body.clamped",
	} {
		if !strings.Contains(css, marker) {
			t.Errorf("Markdown 样式缺少 %q", marker)
		}
	}
}

// TestServeAPIMetaAndProject covers the live path of the same two features: the
// index must carry the project/meta summary the sidebar groups on, the session
// endpoint must deliver the meta line itself (it is the transcript's first
// line, so from=0 returns it), and the served page must carry the theme switch.
func TestServeAPIMetaAndProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "latex_project", "work", "style_session.jsonl"), transcript(
		`{"t":"meta","kind":"system","session_label":"style","model":"glm-4.6","system_sha":"ab12cd34deadbeef","text":"系统提示词","tools":[{"name":"read_file","description":"读文件","parameters":{"type":"object"}}]}`,
		`{"t":"msg","role":"user","text":"分析样式"}`,
	))
	srv := httptest.NewServer(newViewerServer(root))
	defer srv.Close()

	getJSON := func(path string, out any) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("GET %s 不是合法 JSON: %v", path, err)
		}
	}

	var idx indexResponse
	getJSON("/api/index", &idx)
	if len(idx.Sessions) != 1 {
		t.Fatalf("/api/index 返回 %d 个会话, want 1", len(idx.Sessions))
	}
	got := idx.Sessions[0]
	if got.Project != "latex_project" {
		t.Errorf("/api/index 的 project = %q, want latex_project", got.Project)
	}
	if got.Meta == nil || got.Meta.Count != 1 || got.Meta.Model != "glm-4.6" || got.Meta.Tools != 1 || got.Meta.PromptChars == 0 {
		t.Fatalf("/api/index 的 meta 摘要不对：%+v", got.Meta)
	}
	if got.Messages != 1 {
		t.Errorf("/api/index 的 messages = %d, want 1（meta 行不计入）", got.Messages)
	}

	var sess sessionResponse
	getJSON("/api/session?id="+got.ID+"&from=0", &sess)
	if len(sess.Lines) != 2 {
		t.Fatalf("/api/session 返回 %d 行, want 2", len(sess.Lines))
	}
	if !sess.Lines[0].IsMeta() || sess.Lines[0].Text != "系统提示词" || len(sess.Lines[0].Tools) != 1 {
		t.Errorf("增量接口没有把 meta 行原样送出：%+v", sess.Lines[0])
	}
	if sess.Lines[1].IsMeta() {
		t.Errorf("第二条消息被当成 meta 行：%+v", sess.Lines[1])
	}

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	page, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`id="theme-toggle"`, "dsh.sessionview.theme", `data-theme="light"`} {
		if !strings.Contains(string(page), marker) {
			t.Errorf("实时页面缺少主题切换标识 %q", marker)
		}
	}
}

// TestProjectGroupMultiProjectLayout pins the sidebar grouping to the
// multi-project layout: <输出根>/<书名>/ is its own group (so several books
// in one output root do not collapse into one group), a legacy single-project
// root stays one group and is marked legacy, and a directory that is merely a
// workspace subdirectory (work/, source/, …) never becomes a group of its own.
func TestProjectGroupMultiProjectLayout(t *testing.T) {
	root := t.TempDir()

	// 新布局：每本书一个目录，docvision 在其中写 .docvision_project.json。
	bookA := jsonl(root, "latex_project", "概率论-测试", "work", "sessions", "convert_01.jsonl")
	writeFile(t, bookA, transcript(`{"t":"msg","role":"user","text":"转换"}`))
	writeFile(t, filepath.Join(root, "latex_project", "概率论-测试", ".docvision_project.json"), `{"name":"概率论-测试"}`)
	bookB := jsonl(root, "latex_project", "线性代数-测试", "source", "sessions", "vector_x.jsonl")
	writeFile(t, bookB, transcript(`{"t":"msg","role":"user","text":"画图"}`))
	writeFile(t, filepath.Join(root, "latex_project", "线性代数-测试", "progress.json"), `{}`)

	// 旧版单项目：输出根本身就是工作区（work/ 直接挂在它下面）。
	legacy := jsonl(root, "latex_project_0909", "work", "style_session.jsonl")
	writeFile(t, legacy, transcript(`{"t":"msg","role":"user","text":"样式"}`))

	// 档位2 旧版（finally_latex/progress_items 直接挂根下）。
	legacy2 := jsonl(root, "finally_latex", "sessions", "vector_y.jsonl")
	writeFile(t, legacy2, transcript(`{"t":"msg","role":"user","text":"矢量"}`))
	writeFile(t, filepath.Join(root, "finally_latex", "progress_items", ".keep"), "")

	// 书直接挂在扫描根下（实际部署的布局）：work/sessions 收转录，是现代
	// 布局，尽管书目录本身是工作区，也**不**是 legacy（T13：徽标误显）。
	bookC := jsonl(root, "概率论-根挂", "work", "sessions", "convert_02.jsonl")
	writeFile(t, bookC, transcript(`{"t":"msg","role":"user","text":"转换2"}`))
	writeFile(t, filepath.Join(root, "概率论-根挂", ".docvision_project.json"), `{"name":"概率论-根挂"}`)

	// 既不是工作区、也不是结构名的目录：分组退回第一层，不能凭空造组。
	plain := jsonl(root, "latex_project", "scratch", "notes.jsonl")
	writeFile(t, plain, transcript(`{"t":"msg","role":"user","text":"随手记"}`))

	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byID := map[string]SessionInfo{}
	for _, s := range sessions {
		byID[s.ID] = s
	}
	cases := []struct {
		id     string
		group  string
		legacy bool
	}{
		{"latex_project/概率论-测试/work/sessions/convert_01.jsonl", "latex_project/概率论-测试", false},
		{"latex_project/线性代数-测试/source/sessions/vector_x.jsonl", "latex_project/线性代数-测试", false},
		{"latex_project_0909/work/style_session.jsonl", "latex_project_0909", true},
		{"finally_latex/sessions/vector_y.jsonl", "finally_latex", true},
		// 容器本身不是工作区（书都在子目录里）：只退到容器名，不算 legacy。
		{"latex_project/scratch/notes.jsonl", "latex_project", false},
		// 书直接挂根下、转录在 work/sessions/ 里：现代布局，不挂徽标（T13）。
		{"概率论-根挂/work/sessions/convert_02.jsonl", "概率论-根挂", false},
	}
	for _, c := range cases {
		got, ok := byID[c.id]
		if !ok {
			t.Fatalf("没扫到 %s", c.id)
		}
		if got.Project != c.group {
			t.Errorf("%s 的 project = %q, want %q", c.id, got.Project, c.group)
		}
		if got.ProjectLegacy != c.legacy {
			t.Errorf("%s 的 legacy = %v, want %v", c.id, got.ProjectLegacy, c.legacy)
		}
	}

	// 组名必须逐个不同：三本书/工程各一组，而不是挤进 latex_project。
	seen := map[string]int{}
	for _, s := range sessions {
		seen[s.Project]++
	}
	for _, want := range []string{"latex_project/概率论-测试", "latex_project/线性代数-测试", "latex_project_0909", "finally_latex"} {
		if seen[want] != 1 {
			t.Errorf("组 %q 有 %d 个会话，want 1（分组：%v）", want, seen[want], seen)
		}
	}
}

// TestProjectGroupScannedInsideWorkspace covers scanning a project directory
// itself (docvision sessions --dir <proj>): the first segment is then a
// workspace internal (work/, source/), which must not become a group name.
func TestProjectGroupScannedInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, jsonl(root, "work", "sessions", "convert_01.jsonl"), transcript(`{"t":"msg","role":"user","text":"转换"}`))
	writeFile(t, jsonl(root, "source", "sessions", "vector_x.jsonl"), transcript(`{"t":"msg","role":"user","text":"矢量"}`))
	sessions, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, s := range sessions {
		if s.Project != RootProject {
			t.Errorf("%s 的 project = %q, want %q（扫描根就是工程时不该拿 work/source 当组名）", s.ID, s.Project, RootProject)
		}
	}
}

// jsFunc 取出 viewer 脚本里某个函数的源码（到下一个顶层函数定义为止），
// 用来断言"某个渲染路径里不许出现某个东西"这类结构性约束。
func jsFunc(t *testing.T, js, name string) string {
	t.Helper()
	start := strings.Index(js, "function "+name+"(")
	if start < 0 {
		t.Fatalf("找不到函数 %s", name)
	}
	rest := js[start+1:]
	if end := strings.Index(rest, "\n  function "); end >= 0 {
		return rest[:end]
	}
	return rest
}

// cssRule 取出样式表里某个选择器的规则体（到第一个 } 为止）。
func cssRule(t *testing.T, css, sel string) string {
	t.Helper()
	start := strings.Index(css, sel+" {")
	if start < 0 {
		t.Fatalf("找不到选择器 %s", sel)
	}
	rest := css[start:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("选择器 %s 的规则没有闭合", sel)
	}
	return rest[:end]
}

// TestViewerJSONHighlighting 钉住第五件：JSON 高亮用的是样式表里既有的
// k/s/n/b 类名（原来那套 key/str/bool/num 与 CSS 对不上，等于没高亮），
// 应用在工具输入、工具输出、工具 parameters、轨迹输入输出与代码块上，
// 复制按钮仍然只拿原始文本，超大内容退回纯文本。
func TestViewerJSONHighlighting(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")

	if !strings.Contains(js, "var cls = m[1] !== undefined ? 'k' : m[2] !== undefined ? 's' : m[3] !== undefined ? 'b' : 'n';") {
		t.Error("JSON 高亮没有产出 span.k / span.s / span.n / span.b")
	}
	for _, dead := range []string{"appendHighlighted", "'key' : m[2]", "'bool'", "'str' :", "'num';"} {
		if strings.Contains(js, dead) {
			t.Errorf("还留着与 CSS 对不上的旧高亮类名 %q", dead)
		}
	}
	// 只有"整段就是 JSON"才做 JSON 高亮（否则交给终端/diff 着色）。
	if !strings.Contains(js, "if (!s || (s.charAt(0) !== '{' && s.charAt(0) !== '[')) { return null; }") {
		t.Error("JSON 判定不是「整段必须是对象/数组」")
	}
	// 阈值的两个来源都要有：字符数 200KB、行数 4000。
	for _, marker := range []string{
		"var JSON_HL_MAX_CHARS = 200 * 1024;",
		"var JSON_HL_MAX_LINES = 4000;",
		"内容过大（' + fmtChars(pretty.length) + '），已按纯文本显示，不做 JSON 高亮",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("JSON 高亮的阈值处理缺少 %q", marker)
		}
	}
	// 应用位置：工具输入 / 工具输出 / parameters / 轨迹 / 提示词快照 / 代码块。
	for _, marker := range []string{
		"card.appendChild(ioSection('输入', prettyJSON(argsText) || '(无参数)', false, 'in.' + (call.id || ''), argsTokens));",
		"var out = ioSection('输出', text, status === 'error', 'result.' + line.n, estOf(line).text);",
		"out.classList.add('out-section');",
		"pbody.appendChild(machineBlock(String(tool.parameters), 'code'));",
		"scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'));",
		"if (k === 'input' || k === 'request' || k === 'output') {",
		"var res = highlightMachine(inner, code);",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("JSON 高亮没有应用到 %q", marker)
		}
	}
	// 复制按钮拿到的必须是原始文本（不是 pretty 过的、更不是带 span 的 HTML）。
	for _, marker := range []string{
		"actions.appendChild(copyButton(argsText));",
		"actions.appendChild(copyButton(String(tool.parameters)));",
		"head.appendChild(copyButton(code));",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("复制按钮没有拿到原始文本：缺少 %q", marker)
		}
	}
	// 键名/字符串/数字/字面量的配色要覆盖工具卡片的容器，不只是 .code。
	for _, sel := range []string{".io-text .k", ".io-text .s", ".io-text .n", ".io-text .b", ".md-code-body .k"} {
		if !strings.Contains(css, sel) {
			t.Errorf("样式表没有给 %s 上色", sel)
		}
	}
}

// TestViewerToolCardsScrollAndExpandLikeThinking 钉住第六件：工具展开区与思考块
// 用同一套限高与「展开全文」，按钮文案只有一处来源（foldLabel）。
func TestViewerToolCardsScrollAndExpandLikeThinking(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")

	if !strings.Contains(js, "toggle.textContent = foldLabel(expanded, lines.length, body.length, tokens);") {
		t.Error("工具输入输出的展开按钮没有走统一的 foldLabel")
	}
	if !strings.Contains(js, "return expanded ? '收起' : '展开全文（' + lines + ' 行 / ' + countText(chars, tokens) + '）';") {
		t.Error("foldLabel 的文案不是那一套（收起 / 展开全文（N 行 / 计数））")
	}
	// 三处（思考块所在的 collapsibleText、工具卡片 machineScroll、系统消息）共用同一文案。
	for _, fn := range []string{"collapsibleText", "machineScroll", "systemTurnSection"} {
		if !strings.Contains(jsFunc(t, js, fn), "foldLabel(") {
			t.Errorf("%s 没有用统一的折叠按钮文案", fn)
		}
	}
	// 限高来自同一个 token：思考块与工具卡片同值。
	if !strings.Contains(cssRule(t, css, ".io-scroll"), "max-height: var(--code-scroll-h)") {
		t.Error("工具卡片没有限高内滚（.io-scroll 未使用 --code-scroll-h）")
	}
	if !strings.Contains(cssRule(t, css, ".reasoning-scroll, .prompt-scroll"), "max-height: var(--code-scroll-h)") {
		t.Error("思考块与工具卡片不是同一个限高 token")
	}
	if !strings.Contains(css, ".io-scroll.open { max-height: none; }") {
		t.Error("「展开全文」没有解除高度限制")
	}
	if !strings.Contains(js, "scroll.classList.toggle('open', expanded);") {
		t.Error("展开状态没有落到 .io-scroll.open 上")
	}
}

// TestViewerConsoleAndDiffHighlighting 钉住第七件：JSON 之外，终端输出 / diff /
// 编译日志也要着色，并且这些高亮与 Markdown 开关无关。
func TestViewerConsoleAndDiffHighlighting(t *testing.T) {
	js := readAsset(t, "viewer.js")
	css := readAsset(t, "viewer.css")

	for _, marker := range []string{
		"function consoleLineClass(line)",
		"if (/^\\s*(\\$|#)\\s+\\S/.test(line)) { return 'cmd'; }",
		"if (/^\\+/.test(line)) { return 'add'; }",
		"if (/^-/.test(line)) { return 'del'; }",
		"if (/^@@/.test(line)) { return 'hunk'; }",
		"if (/^\\s*!/.test(line)) { return 'bad'; }",
		"if (/Overfull|Underfull/.test(line)) { return 'warn'; }",
		"(?:ERROR|Error|error|FAILED|FAIL|Failure|failed|FATAL|Fatal)",
		"(?:WARNING|Warning|warning|WARN|Warn|OVERFULL|Overfull|UNDERFULL|Underfull)",
		"(?:OK|PASS|PASSED|COMPILE OK|SUCCESS|Success|done)",
		"https?:\\/\\/[^\\s\"'<>]+",
		"function appendConsoleSpans(parent, text)",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("终端/diff/日志高亮缺少 %q", marker)
		}
	}
	for _, sel := range []string{".io-text .cmd", ".io-text .add", ".io-text .del", ".io-text .hunk", ".io-text .bad", ".io-text .warn", ".io-text .good", ".io-text .path"} {
		if !strings.Contains(css, sel) {
			t.Errorf("样式表没有给 %s 上色", sel)
		}
	}
	// 输入侧：命令行首行的强调（$ 提示符），且不比输出花。
	for _, marker := range []string{
		"function toolPromptLine(name, argsText)",
		"function cmdPreview(cmd)",
		"card.appendChild(cmdPreview(cmdLine));",
		"el('span', 'cmd-prompt', '$ ')",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("工具输入的命令行首行强调缺少 %q", marker)
		}
	}
	// 高亮与 Markdown 是两件事：机器文本的渲染路径不读 state.markdown。
	for _, fn := range []string{"machineScroll", "machineBlock", "highlightMachine", "ioSection"} {
		if strings.Contains(jsFunc(t, js, fn), "state.markdown") {
			t.Errorf("%s 里出现了 state.markdown（高亮不该跟着 Markdown 开关走）", fn)
		}
	}
}

// TestViewerCodeTypographyUsesOneToken 钉住第八件：等宽内容只有一套字号刻度
// （--code-font 11px / --code-line 19px），输入、输出、思考、JSON 高亮、
// 行内 code、代码块全部取自同一个 token，不许再有 11.5px/12.5px/13px 这类分歧。
func TestViewerCodeTypographyUsesOneToken(t *testing.T) {
	css := readAsset(t, "viewer.css")

	root := cssRule(t, css, ":root")
	for _, marker := range []string{"--code-font: 11px;", "--code-line: 19px;"} {
		if !strings.Contains(root, marker) {
			t.Errorf(":root 缺少统一刻度 %q", marker)
		}
	}
	for _, sel := range []string{".io-text", ".code", ".reasoning-text", ".prompt-text", ".md-code-body", ".md-inline-code"} {
		r := cssRule(t, css, sel)
		if !strings.Contains(r, "font-size: var(--code-font)") {
			t.Errorf("%s 的字号没有取自 --code-font（输入/输出/思考/JSON/行内 code 必须一致）", sel)
		}
		if !strings.Contains(r, "line-height: var(--code-line)") {
			t.Errorf("%s 的行高没有取自 --code-line", sel)
		}
		if regexp.MustCompile(`font-size:\s*[0-9.]+px`).MatchString(r) {
			t.Errorf("%s 还写着字面量字号（应当只有一套 token）", sel)
		}
	}
	// 上一次的分歧值不许再出现在"机器内容"这一族规则里（次级说明文字另有刻度）。
	for _, sel := range []string{".io-text", ".code", ".reasoning-text", ".prompt-text", ".md-code-body", ".md-inline-code", ".io-cmd"} {
		if strings.Contains(cssRule(t, css, sel), "11.5px") || strings.Contains(cssRule(t, css, sel), "12.5px") {
			t.Errorf("%s 还留着分叉的字号", sel)
		}
	}
}
