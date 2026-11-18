package troubleshoot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSubmitViaRelay(t *testing.T) {
	root := t.TempDir()
	if _, err := Start(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(Dir(root), "issue.md"), []byte("# [bug] 测试\n\n正文"), 0o644)

	var gotAuthor, gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Title  string `json:"title"`
			Body   string `json:"body"`
			Author string `json:"author"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		gotTitle, gotAuthor = p.Title, p.Author
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "issue_url": "https://github.com/x/y/issues/9"})
	}))
	defer srv.Close()

	url, err := SubmitViaRelay(root, srv.URL, "agent-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if url == "" || gotAuthor != "agent-e2e" || gotTitle != "[bug] 测试" {
		t.Fatalf("relay 字段错: url=%s author=%s title=%s", url, gotAuthor, gotTitle)
	}
}
