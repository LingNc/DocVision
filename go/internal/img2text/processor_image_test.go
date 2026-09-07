package img2text

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveImageFileSubjectFallback guards the regression where the
// merged markdown keeps a bare reference (images/foo.jpg) while the
// collected images live under images/<subject>/foo.jpg. resolveImageFile
// must fall back to the per-subject directory instead of reporting
// IMG_MISSING.
func TestResolveImageFileSubjectFallback(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "output", "images")
	subject := "高数 上册 (数学)" // subject with spaces/ASCII + CJK
	name := "deadbeef.jpg"
	subjectDir := filepath.Join(imagesDir, subject)
	if err := os.MkdirAll(subjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subjectDir, name), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Bare reference resolves via the subject fallback.
	got, err := resolveImageFile(imagesDir, "images/"+name, subject)
	if err != nil {
		t.Fatalf("bare reference should resolve via subject fallback, got error: %v", err)
	}
	if got != filepath.Join(subjectDir, name) {
		t.Fatalf("resolved %q, want %q", got, filepath.Join(subjectDir, name))
	}

	// Prefixed reference resolves directly.
	got2, err := resolveImageFile(imagesDir, "images/"+subject+"/"+name, subject)
	if err != nil {
		t.Fatalf("prefixed reference should resolve, got error: %v", err)
	}
	if got2 != filepath.Join(subjectDir, name) {
		t.Fatalf("resolved %q, want %q", got2, filepath.Join(subjectDir, name))
	}

	// Missing image still errors.
	if _, err := resolveImageFile(imagesDir, "images/ghost.jpg", subject); err == nil {
		t.Fatalf("missing image should return an error")
	}
}
