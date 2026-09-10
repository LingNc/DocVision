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
	// compile is a PURE compile tool: no preview image is attached, the
	// model inspects the PDF with view_pdf instead.
	if !strings.HasPrefix(res.Text, "COMPILE OK.") {
		t.Fatalf("compile from file = %q", truncateStr(res.Text, 300))
	}
	if res.ImageBase64 != "" {
		t.Fatalf("compile must not attach a preview image any more (%d bytes)", len(res.ImageBase64))
	}
	if !strings.Contains(res.Text, "view_pdf") {
		t.Errorf("compile result must point at view_pdf: %q", res.Text)
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
