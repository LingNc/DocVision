package latex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// TestCompileFigureToolFromFile pins the process-session contract: the
// figure code lives in a workspace file, compile takes a PATH (never
// inline code) and submit references the same file.
func TestCompileFigureToolFromFile(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	dir := t.TempDir()
	body := "\\begin{tikzpicture}\n\\draw (0,0) rectangle (2,1);\n\\end{tikzpicture}\n"
	if err := os.WriteFile(filepath.Join(dir, "figure.tex"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &tikzState{workDir: dir}
	tool := &CompileFigureTool{
		Comp:  NewCompiler(config.LatexCompileConfig{Engine: "xelatex", RasterCommand: "pdftoppm", RasterDPI: 72}),
		State: state,
	}

	res, err := tool.Execute(`{}`) // default figure.tex
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "COMPILE OK.") || res.ImageBase64 == "" {
		t.Fatalf("compile from file = %q (image %d bytes)", truncateStr(res.Text, 300), len(res.ImageBase64))
	}
	if !state.compileOK || state.lastCode != body {
		t.Errorf("state not recorded: ok=%v code=%q", state.compileOK, state.lastCode)
	}
	if !fileExists(filepath.Join(dir, "standalone.pdf")) {
		t.Error("standalone.pdf missing")
	}

	sub := &SubmitFigureTool{State: state}
	res, err = sub.Execute(`{"path":"figure.tex"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "SUBMITTED") {
		t.Errorf("submit by path = %q", res.Text)
	}

	// A missing path must be reported, never silently compiled.
	missing := &CompileFigureTool{Comp: tool.Comp, State: &tikzState{workDir: dir}}
	res, err = missing.Execute(`{"path":"nope.tex"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "NOT FOUND") {
		t.Errorf("missing figure file = %q", res.Text)
	}
}

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
	if _, err := empty.resolve("preview.png"); err == nil || !strings.Contains(err.Error(), "compile") {
		t.Errorf("empty preview list must hint at compile, got %v", err)
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
