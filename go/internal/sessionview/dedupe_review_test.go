package sessionview

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T154：dedupe 复查页要能从会话预览页点开——图片走 /media 链接而不是 base64，
// 靠的是给同一份 review.html 补一个 <base>。这个测试把整条链路走一遍：
// 索引里报出"有复查页"的项目、页面带 base、base 指向的缩略图与 ../ 原图都取得到。
func TestServeDedupeReviewPage(t *testing.T) {
	root := t.TempDir()
	proj := "bookA"
	dedupeDir := filepath.Join(root, proj, "source", "dedupe")
	if err := os.MkdirAll(filepath.Join(dedupeDir, "thumbs"), 0o755); err != nil {
		t.Fatal(err)
	}
	page := "<!doctype html>\n<html><head>\n<title>复查</title>\n</head>\n<body>" +
		"<img src=\"thumbs/aaaaaaaaaaaa.jpg\" data-orig=\"../images/b/a.jpg\">" +
		"<a href=\"../images/b/a.jpg\">x</a></body></html>"
	writeFile(t, filepath.Join(dedupeDir, "review.html"), page)
	writeFile(t, filepath.Join(dedupeDir, "thumbs", "aaaaaaaaaaaa.jpg"), "THUMB")
	os.MkdirAll(filepath.Join(root, proj, "source", "images", "b"), 0o755)
	writeFile(t, filepath.Join(root, proj, "source", "images", "b", "a.jpg"), "FULL")
	writeFile(t, filepath.Join(root, proj, "work", "sessions", "motif_dedupe_aabbcc.jsonl"),
		`{"t":"msg","role":"user","text":"hi"}`+"\n")

	srv := httptest.NewServer(newViewerServer(root, nil))
	defer srv.Close()

	// 索引：有复查页的项目要报出来（侧栏据此决定挂不挂按钮）。
	var idx struct {
		DedupeReviews []string `json:"dedupeReviews"`
	}
	getJSON(t, srv.URL+"/api/index", &idx)
	if len(idx.DedupeReviews) != 1 || idx.DedupeReviews[0] != proj {
		t.Fatalf("dedupeReviews = %v, want [%s]", idx.DedupeReviews, proj)
	}

	// 页面：同一份 review.html + 指向 /media 的 <base>。
	resp, err := http.Get(srv.URL + "/dedupe?project=" + proj)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码 = %d，body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	wantBase := `<base href="/media/` + proj + `/source/dedupe/">`
	if !strings.Contains(string(body), wantBase) {
		t.Fatalf("页面没补 base（want %s）：\n%s", wantBase, body)
	}
	if !strings.Contains(string(body), "thumbs/aaaaaaaaaaaa.jpg") { // 正文原样保留
		t.Fatalf("页面内容被改动了：\n%s", body)
	}

	// base 指向的两个地址都要真的能取到（缩略图与 ../ 原图）。
	for _, rel := range []string{"dedupe/thumbs/aaaaaaaaaaaa.jpg", "images/b/a.jpg"} {
		r2, err := http.Get(srv.URL + "/media/" + proj + "/source/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		b2, _ := io.ReadAll(r2.Body)
		r2.Body.Close()
		if r2.StatusCode != http.StatusOK || len(b2) == 0 {
			t.Fatalf("/media %s = %d, %q", rel, r2.StatusCode, b2)
		}
	}

	// 没跑过 dedupe 的项目：明确 404，不是空页面。
	resp3, err := http.Get(srv.URL + "/dedupe?project=nobody")
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("未知项目 = %d, want 404", resp3.StatusCode)
	}

	// 项目名来自查询串，越界一律拒绝。
	resp4, err := http.Get(srv.URL + "/dedupe?project=../etc")
	if err != nil {
		t.Fatal(err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusBadRequest {
		t.Fatalf("越界项目 = %d, want 400", resp4.StatusCode)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}
