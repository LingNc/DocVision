package sessionview

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T37: img2text extra-root sessions group per book, get numbered, and
// carry short hash names — upgrade-fix workspaces
// (progress_items/mermaid_fix/<书>_<图>/session-stageN.jsonl) and debug
// per-image transcripts (progress_items/sessions/<md>/<图>.jsonl).
func TestScanWith_Img2TextExtras(t *testing.T) {
	root := t.TempDir()
	progress := t.TempDir()

	write := func(rel, content string) {
		p := filepath.Join(progress, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Two books, two fix workspaces each; one debug per-image transcript.
	fixMeta := `{"t":"meta","kind":"mermaid-fix","label":"x","model":"m","system":""}` + "\n"
	write("mermaid_fix/书A_aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb.jpg/session-stage0.jsonl", fixMeta)
	write("mermaid_fix/书A_zzzz9999yyyy8888xxxx7777wwww6666vvvv5555uuuu4444tttt3333ssss.jpg/session-stage0.jsonl", fixMeta)
	write("mermaid_fix/书B_abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234.jpg/session-stage0.jsonl", fixMeta)
	write("sessions/书A.md/aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb.jpg.jsonl", fixMeta)

	sessions, err := ScanWith(root, []ScanExtra{{Dir: progress, Kind: "img2text"}})
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if len(sessions) != 4 {
		t.Fatalf("sessions = %d, want 4", len(sessions))
	}

	byID := map[string]SessionInfo{}
	for _, s := range sessions {
		byID[s.ID] = s
	}

	// 书A 的升级修复组：两张图各得一个稳定序号，标题带 "#N ·"。
	var fixA []SessionInfo
	for _, s := range sessions {
		if s.Project == "img2text · 书A" && s.Stage == "mermaid-fix" {
			fixA = append(fixA, s)
		}
	}
	if len(fixA) != 2 {
		t.Fatalf("书A fix sessions = %d, want 2", len(fixA))
	}
	orders := map[int]bool{}
	for _, s := range fixA {
		if s.ChapterOrder < 1 || s.ChapterOrder > 2 {
			t.Errorf("ChapterOrder = %d, want 1 or 2 (%s)", s.ChapterOrder, s.ID)
		}
		orders[s.ChapterOrder] = true
		if !strings.HasPrefix(s.Title, "#") || !strings.Contains(s.Title, "升级修复 · ") {
			t.Errorf("Title = %q, want '#N · 升级修复 · …'", s.Title)
		}
	}
	if !orders[1] || !orders[2] {
		t.Errorf("orders = %v, want both 1 and 2", orders)
	}

	// 哈希短名：64 位哈希折叠成 8 位 + 扩展名。
	s := byID["img2text:mermaid_fix/书A_aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb.jpg/session-stage0.jsonl"]
	if !strings.Contains(s.Title, "aaaa1111.jpg") {
		t.Errorf("short hash name missing in Title %q", s.Title)
	}
	if s.StageTitle != "升级修复" {
		t.Errorf("StageTitle = %q, want 升级修复", s.StageTitle)
	}

	// debug 逐图转录：按 md 分组、阶段 逐图分析。
	s = byID["img2text:sessions/书A.md/aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb.jpg.jsonl"]
	if s.Project != "img2text · 书A" || s.Stage != "img2text" {
		t.Errorf("debug session grouped as %q / %q", s.Project, s.Stage)
	}

	// 主根不受影响：无 extras 时一个也不出现。
	plain, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, s := range plain {
		if strings.HasPrefix(s.ID, "img2text:") {
			t.Errorf("img2text session leaked into plain scan: %s", s.ID)
		}
	}
}

// splitFixDirName: the LAST underscore separates book from image file,
// and non-conforming names are rejected.
func TestSplitFixDirName(t *testing.T) {
	book, img := splitFixDirName("书_名字_abcdef.jpg")
	if book != "书_名字" || img != "abcdef.jpg" {
		t.Fatalf("got %q / %q", book, img)
	}
	if b, i := splitFixDirName("nounderscore"); b != "" || i != "" {
		t.Fatalf("non-conforming name accepted: %q / %q", b, i)
	}
	if b, i := splitFixDirName("_x"); b != "" || i != "" {
		t.Fatalf("leading underscore accepted: %q / %q", b, i)
	}
}

// T37: media of an extra-root transcript is served from the extra root
// (it lives outside the main scan root).
func TestServeFile_ExtraRootMedia(t *testing.T) {
	root := t.TempDir()
	extra := t.TempDir()
	// <extra>/mermaid_fix/书_a.jpg/media/pic.png
	mediaDir := filepath.Join(extra, "mermaid_fix", "书_a.jpg", "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "pic.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newViewerServer(root, []ScanExtra{{Dir: extra, Kind: "img2text"}}))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/media/mermaid_fix/%E4%B9%A6_a.jpg/media/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("media from extra root: status = %d, want 200", resp.StatusCode)
	}
}
