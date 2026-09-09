package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteWorkFileToolPrefixes pins the convert-session submission
// rules: only the chapter's own main file and its asset folder are
// writable, and chapter files must be \input fragments.
func TestWriteWorkFileToolPrefixes(t *testing.T) {
	root := t.TempDir()
	tool := &WriteWorkFileTool{
		Root:                root,
		Prefixes:            []string{"chapters/ch1.tex", "chapters/ch1/"},
		RejectDocumentclass: true,
	}
	if res, err := tool.Execute(`{"path":"chapters/ch1.tex","content":"\\section{One}"}`); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(res.Text, "WROTE") {
		t.Fatalf("main file write = %q", res.Text)
	}
	if res, err := tool.Execute(`{"path":"chapters/ch1/tables/t1.tex","content":"x"}`); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(res.Text, "WROTE") {
		t.Fatalf("asset write = %q", res.Text)
	}
	if res, err := tool.Execute(`{"path":"chapters/ch2.tex","content":"x"}`); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(res.Text, "outside your writable paths") {
		t.Fatalf("foreign chapter write = %q", res.Text)
	}
	if res, err := tool.Execute(`{"path":"chapters/ch1.tex","content":"\\documentclass{book}"}`); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(res.Text, "input fragment") {
		t.Fatalf("documentclass must be rejected: %q", res.Text)
	}
	if res, err := tool.Execute(`{"path":"project:chapters/ch1.tex","content":"x"}`); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(res.Text, "read-only") {
		t.Fatalf("foreign mount write = %q", res.Text)
	}
	if res, err := tool.Execute(`{"path":"work:chapters/ch1.tex","content":"\\section{One}"}`); err != nil {
		t.Fatal(err)
	} else if !strings.HasPrefix(res.Text, "WROTE") {
		t.Fatalf("work: prefix = %q", res.Text)
	}
}

// TestViewImageResolveFigureSession pins the figure-session contract:
// Root is the images root, Subject is the current document's folder, and
// the model may pass the markdown ref or just the file name — both are a
// plain join, never a search.
func TestViewImageResolveFigureSession(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(sub, "fig.jpg")
	if err := os.WriteFile(img, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "测试-概率论"}

	for _, rel := range []string{
		"images/测试-概率论/fig.jpg",
		"测试-概率论/fig.jpg",
		"fig.jpg",
		"./images/测试-概率论/fig.jpg",
	} {
		got, err := tool.resolve(rel)
		if err != nil {
			t.Fatalf("resolve(%q): %v", rel, err)
		}
		if got != img {
			t.Errorf("resolve(%q) = %q, want %q", rel, got, img)
		}
	}

	for _, rel := range []string{"images/missing.jpg", "missing.jpg", "../outside.jpg", "/etc/passwd", ""} {
		if _, err := tool.resolve(rel); err == nil {
			t.Errorf("resolve(%q) unexpectedly succeeded", rel)
		}
	}
}

// TestViewImageResolveNoSearchAcrossSubjects: a wrong name must fail
// instead of silently finding the same file in another document's
// folder.
func TestViewImageResolveNoSearchAcrossSubjects(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "其他")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "fig.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "测试-概率论"}
	if _, err := tool.resolve("fig.jpg"); err == nil {
		t.Fatal("view_image must not search other documents' folders")
	}
}

// TestViewImageResolveDocumentRoot keeps the style-analyst case working:
// Root is the document directory and Subject is its images/ folder, so
// both "images/subject/fig.png" and "subject/fig.png" resolve.
func TestViewImageResolveDocumentRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "images", "subject")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(sub, "fig.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "images"}
	for _, rel := range []string{"images/subject/fig.png", "subject/fig.png"} {
		got, err := tool.resolve(rel)
		if err != nil {
			t.Fatalf("resolve(%q): %v", rel, err)
		}
		if got != img {
			t.Errorf("resolve(%q) = %q, want %q", rel, got, img)
		}
	}
}
