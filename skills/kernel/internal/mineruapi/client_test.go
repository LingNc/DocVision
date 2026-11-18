package mineruapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 假 MinerU 服务端：/file-urls/batch → 预签名 URL；PUT 上传收字节；
// /extract-results/batch/X → pending→done；/dl → zip（含 content_list+图）。
func TestAPIUploadPollDownload(t *testing.T) {
	// 造结果 zip
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for name, body := range map[string]string{
		"book_part1/auto/book_part1_content_list.json": `[{"type":"text","page_idx":0,"text":"正文"}]`,
		"book_part1/auto/layout.json":                  `{"pdf_info":[{"page_size":[595,842]}]}`,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()

	var uploaded []byte
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/api/v1/file-urls/batch", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"batch_id": "b1", "file_urls": []string{srv.URL + "/up"}}})
	})
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		uploaded = buf.Bytes()
		w.WriteHeader(200)
	})
	var polls int
	mux.HandleFunc("/api/v1/extract-results/batch/b1", func(w http.ResponseWriter, r *http.Request) {
		polls++
		state := "pending"
		if polls >= 2 {
			state = "done"
		}
		item := map[string]any{"state": state, "full_zip_url": srv.URL + "/dl"}
		if state == "done" {
			item["extract_progress"] = map[string]any{"extracted_pages": 1, "total_pages": 1}
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"extract_result": []any{item}}})
	})
	mux.HandleFunc("/dl", func(w http.ResponseWriter, r *http.Request) {
		w.Write(zbuf.Bytes())
	})

	c, err := NewClient(Config{APIBaseURL: srv.URL + "/api/v1", PollInterval: 1, PollTimeout: 30}, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	// 源文件
	src := filepath.Join(t.TempDir(), "book_part1.pdf")
	if err := os.WriteFile(src, []byte("%PDF-fake-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err := c.CreateUploadTask(src)
	if err != nil {
		t.Fatal(err)
	}
	if batch != "b1" {
		t.Fatalf("batch_id: %s", batch)
	}
	if string(uploaded) != "%PDF-fake-content" {
		t.Fatalf("上传内容应完整到达: %q", uploaded)
	}
	item, err := c.PollBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	if item.State != "done" {
		t.Fatalf("应 done: %s", item.State)
	}
	dst := t.TempDir()
	if err := c.DownloadAndExtract(item.FullZipURL, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "book_part1", "auto", "book_part1_content_list.json")); err != nil {
		t.Fatal("zip 应解压出 content_list:", err)
	}
}

func TestTokenRequired(t *testing.T) {
	os.Unsetenv("MINERU_TOKEN")
	if _, err := NewClient(DefaultConfig(), ""); err == nil {
		t.Fatal("无 token 应报错")
	}
}

func TestDataIDTruncation(t *testing.T) {
	long := string(make([]byte, 0))
	for i := 0; i < 200; i++ {
		long += "书"
	}
	id := dataIDForUpload(long)
	if len(id) > dataIDMaxLen {
		t.Fatalf("截断后仍超长: %d", len(id))
	}
	if dataIDForUpload("short") != "short" {
		t.Fatal("短名原样")
	}
	_ = time.Now
}
