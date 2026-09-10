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
	if !strings.Contains(page, "#0f1115") {
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
