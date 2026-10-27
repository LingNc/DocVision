package latex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T260：通过过的产物不该被更早那条核对会话的旧 issues 打回。
func TestStaleCheckerFeedback(t *testing.T) {
	dir := t.TempDir()
	tex := filepath.Join(dir, "chapter_001.tex")
	parts := filepath.Join(dir, "chapter_001")
	tr := filepath.Join(dir, "checker_chapter_001.try4.jsonl")
	if err := os.MkdirAll(parts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tex, []byte("\\section{a}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parts, "p1.tex"), []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp := productFingerprint(tex, parts)
	if fp == "" {
		t.Fatal("指纹不该为空")
	}

	// 没记号：不是旧反馈，照常打回。
	if staleCheckerFeedback(tex, parts, tr) {
		t.Fatal("没通过过的产物不该被判成旧反馈")
	}

	// 记号比转录新：通过发生在这次核对之后 → 旧反馈。
	writePassMarker(tex, fp)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(tr, old, old); err != nil {
		t.Fatal(err)
	}
	if !staleCheckerFeedback(tex, parts, tr) {
		t.Fatal("产物未变且通过更晚，应判成旧反馈")
	}

	// 产物变了：不再是同一版，不能拿旧 via 结论压掉新反馈。
	if err := os.WriteFile(filepath.Join(parts, "p1.tex"), []byte("body changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if staleCheckerFeedback(tex, parts, tr) {
		t.Fatal("产物变了就不该判成旧反馈")
	}

	// 判 issues 会清记号。
	writePassMarker(tex, productFingerprint(tex, parts))
	dropPassMarker(tex)
	if _, err := os.Stat(passMarkerPath(tex)); !os.IsNotExist(err) {
		t.Fatal("dropPassMarker 应删掉记号")
	}
}
