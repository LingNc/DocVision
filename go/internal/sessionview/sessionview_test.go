package sessionview

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if !strings.Contains(rootBlock, "--bg: #ffffff") || !strings.Contains(rootBlock, "--text: #18181b") {
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

	// 思考过程：默认折叠 + 字符数摘要 + 滚动容器。
	if !strings.Contains(js, "details.open = !state.forceCollapse && remembered") {
		t.Error("思考块不再默认折叠")
	}
	if !strings.Contains(js, "'· ' + line.reasoning.length + ' 字符'") {
		t.Error("思考摘要行没有写清字符数")
	}
	if !strings.Contains(js, "reasoning-scroll") {
		t.Error("思考正文没有放进带最大高度的滚动容器")
	}

	// meta 卡片：标题、最新一条、工具二级折叠。
	for _, marker := range []string{
		"系统提示词（本次运行快照，不参与回放）",
		"共 ' + m.count + ' 条，显示最新",
		"tool-schema",
		"schema-params",
		"refs.timeline.insertBefore(card",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("脚本缺少 meta 卡片关键逻辑 %q", marker)
		}
	}

	// 侧栏按项目分组，折叠状态在内存里保持。
	for _, marker := range []string{"proj-group", "state.collapsed", "wrap.open = g.matched ? true : !state.collapsed[g.name]"} {
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
