package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestViewImageResolvePreviewNames pins the compile-preview contract:
// preview.png is the newest compile, preview-<n>.png the n-th, and
// unknown/out-of-range names are errors (never a search).
func TestViewImageResolvePreviewNames(t *testing.T) {
	dir := t.TempDir()
	var previews []previewEntry
	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, "preview-"+string(rune('0'+i))+".png")
		if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
		previews = append(previews, previewEntry{png: p})
	}
	tool := &ViewImageTool{Root: dir, Previews: func() []previewEntry { return previews }}

	if got, err := tool.resolve("preview.png"); err != nil || got != previews[2].png {
		t.Errorf("preview.png = %q, %v; want newest %q", got, err, previews[2].png)
	}
	if got, err := tool.resolve("preview-1.png"); err != nil || got != previews[0].png {
		t.Errorf("preview-1.png = %q, %v; want %q", got, err, previews[0].png)
	}
	if _, err := tool.resolve("preview-9.png"); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("out-of-range preview must fail with a hint, got %v", err)
	}
	if _, err := tool.resolve("preview-abc.png"); err == nil || !strings.Contains(err.Error(), "未知的预览名") {
		t.Errorf("unknown preview name must fail with a hint, got %v", err)
	}

	// No previews yet: a clear "compile first" error instead of a
	// confusing file-not-found.
	empty := &ViewImageTool{Root: dir, Previews: func() []previewEntry { return nil }}
	if _, err := empty.resolve("preview.png"); err == nil || !strings.Contains(err.Error(), "compile_preview") {
		t.Errorf("empty preview list must hint at compile_preview, got %v", err)
	}
}

// TestViewImagePreviewDoesNotShadowDocumentImages: without a preview
// source, preview.png is just an ordinary (missing) file name.
func TestViewImagePreviewDoesNotShadowDocumentImages(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "preview.png")
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: dir}
	got, err := tool.resolve("preview.png")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != img {
		t.Errorf("resolve = %q, want %q", got, img)
	}
}
