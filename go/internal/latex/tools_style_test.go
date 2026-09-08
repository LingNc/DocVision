package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// TestViewImageResolveAcceptsMarkdownRefs pins the fix for the "文件不存在"
// first-call failure: figure sessions ask for the markdown ref
// "images/<subject>/x.jpg" while the tool root is the images directory.
func TestViewImageResolveAcceptsMarkdownRefs(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(sub, "fig.jpg")
	if err := os.WriteFile(img, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root}

	for _, rel := range []string{
		"images/测试-概率论/fig.jpg",
		"测试-概率论/fig.jpg",
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

// TestViewImageResolveBareNameWithSubject covers the "just give me the
// file name" contract: the session knows the document's image folder.
func TestViewImageResolveBareNameWithSubject(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "测试-概率论")
	other := filepath.Join(root, "其他")
	for _, d := range []string{sub, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	img := filepath.Join(sub, "fig.jpg")
	if err := os.WriteFile(img, []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same file name in another document's folder: the Subject decides.
	if err := os.WriteFile(filepath.Join(other, "fig.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "测试-概率论"}
	got, err := tool.resolve("fig.jpg")
	if err != nil {
		t.Fatalf("resolve bare name: %v", err)
	}
	if got != img {
		t.Errorf("resolve = %q, want %q", got, img)
	}

	// Without a subject the name is ambiguous and must be reported.
	plain := &ViewImageTool{Root: root}
	if _, err := plain.resolve("fig.jpg"); err == nil {
		t.Error("ambiguous bare name without subject must fail")
	}
}

// TestViewImageResolveDocumentRoot keeps the style-analyst case working:
// there the root IS the document directory, so "images/..." must resolve
// directly.
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
	tool := &ViewImageTool{Root: root}
	got, err := tool.resolve("images/subject/fig.png")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != img {
		t.Errorf("resolve = %q, want %q", got, img)
	}
}
