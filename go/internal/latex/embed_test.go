package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// testRunner builds a Runner with a quiet logger for embed tests.
// inline=true mimics level 1 (full-book LaTeX), inline=false level 2.
func testRunner(t *testing.T, inline bool) *Runner {
	t.Helper()
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	log.SetQuiet(true)
	t.Cleanup(func() { _ = log.Close() })
	// 提供真实 images 目录，让 copyOriginalImage 能复制原图并生成 LINK 行。
	imgs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(imgs, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgs, "book", "img1.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Runner{cfg: &config.Config{Paths: config.PathsConfig{ImagesDir: imgs}},
		log: log, inline: inline}
}

// TestEmbedBlockLevel2StyledTextIsDirect: level 2 replaces a text image
// with its extracted text and nothing else — no styling preservation,
// no markers, no original image link.
func TestEmbedBlockLevel2StyledTextIsDirect(t *testing.T) {
	r := testRunner(t, false)
	p := &imageProgress{
		Class: ClassText, Styled: true,
		StyleNote: "black characters in blue circular bubbles",
		Content:   "知识导图",
	}
	if got := r.embedBlock(p, "book.md", t.TempDir()); got != "知识导图" {
		t.Errorf("level-2 styled text embed = %q, want the plain text", got)
	}
}

// TestEmbedBlockLevel1StyledTextComment pins the level-1 format: one
// comment carrying the style note, the image's own text (CONTENT) and
// the original image link (LINK).
func TestEmbedBlockLevel1StyledTextComment(t *testing.T) {
	r := testRunner(t, true)
	p := &imageProgress{
		Class: ClassText, Styled: true,
		StyleNote: "black characters in blue circular bubbles",
		Content:   "知识导图",
		ImgPath:   "images/book/img1.png",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())

	for _, want := range []string{
		"<!-- DOCVISION-STYLED-TEXT: black characters in blue circular bubbles -->",
		"CONTENT: 知识导图",
		"LINK: [styled-text](",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("level-1 styled block missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "BEGIN") || strings.Contains(block, "END") {
		t.Errorf("level-1 styled block must not use BEGIN/END markers:\n%s", block)
	}
}

// TestEmbedBlockLevel1VectorComment pins the level-1 vector format:
// a first-line-closed comment (DOCVISION-VECTOR), the LINK outside the
// comment above the latex fence, in [vector](path) link form.
func TestEmbedBlockLevel1VectorComment(t *testing.T) {
	r := testRunner(t, true)
	p := &imageProgress{
		Class: ClassVector, Label: "Venn diagram",
		TikzCode: "\\begin{tikzpicture}\n%...\n\\end{tikzpicture}",
		ImgPath:  "images/book/img1.png",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())
	for _, want := range []string{
		"<!-- DOCVISION-VECTOR: Venn diagram -->",
		"LINK: [vector](images/",
		"```latex",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("level-1 vector block missing %q:\n%s", want, block)
		}
	}
	// 注释首行即闭合，LINK 是注释后的独立行：--> 直接跟换行再 LINK。
	if !strings.Contains(block, " -->\nLINK:") {
		t.Errorf("comment must close on the first line with LINK outside:\n%s", block)
	}
}

// TestEmbedBlockRasterDescription pins the level-1 raster format:
// DOCVISION-IMAGE comment (first-line closed), DESCRIBE block with the
// AI explanation and LINK in [image](path) link form — same skeleton
// as STYLED/VECTOR.
func TestEmbedBlockRasterDescription(t *testing.T) {
	r := testRunner(t, true)
	r.cfg.Latex.InsertImageDescription = true
	p := &imageProgress{
		Class: ClassRaster, Label: "experiment setup",
		Content: "a drawing of the experimental setup",
		ImgPath: "images/book/img1.png",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())
	for _, want := range []string{
		"<!-- DOCVISION-IMAGE: experiment setup -->",
		"DESCRIBE: a drawing of the experimental setup",
		"LINK: [image](images/",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("level-1 raster block missing %q:\n%s", want, block)
		}
	}
}

// TestEmbedBlockLevel2RasterDescription: level-2 keeps the historical
// "[Image]( content )" embed when insert_image_description is on.
func TestEmbedBlockLevel2RasterDescription(t *testing.T) {
	r := testRunner(t, false)
	r.cfg.Latex.InsertImageDescription = true
	p := &imageProgress{
		Class: ClassRaster, Content: "a drawing of the experimental setup",
		ImgPath: "images/book/img1.png",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())
	if want := "[Image]( a drawing of the experimental setup )"; !strings.Contains(block, want) {
		t.Errorf("level-2 raster description missing %q:\n%s", want, block)
	}
}

// TestEmbedBlockLevel2RasterNoDescription: level-2 without the flag keeps
// the plain image link.
func TestEmbedBlockLevel2RasterNoDescription(t *testing.T) {
	r := testRunner(t, false)
	p := &imageProgress{Class: ClassRaster, ImgPath: "images/book/img1.png"}
	block := r.embedBlock(p, "book.md", t.TempDir())
	if want := "![image](images/"; !strings.Contains(block, want) {
		t.Errorf("level-2 raster missing %q:\n%s", want, block)
	}
}

// TestEmbedBlockLevel1RasterNoDescription: without an AI description the
// level-1 raster still emits the LINK line so the block shape stays
// uniform (the converter includes the image via LINK).
func TestEmbedBlockLevel1RasterNoDescription(t *testing.T) {
	r := testRunner(t, true) // insert_image_description 默认 false
	p := &imageProgress{
		Class: ClassRaster, ImgPath: "images/book/img1.png",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())
	if want := "LINK: [image](images/"; !strings.Contains(block, want) {
		t.Errorf("level-1 raster without description missing %q:\n%s", want, block)
	}
}

// TestEmbedBlockLevel1PlainTextIsDirect: only styled images get the
// comment; plain text is embedded verbatim in both levels.
func TestEmbedBlockLevel1PlainTextIsDirect(t *testing.T) {
	r := testRunner(t, true)
	p := &imageProgress{Class: ClassText, Content: "随机试验的定义"}
	if got := r.embedBlock(p, "book.md", t.TempDir()); got != "随机试验的定义" {
		t.Errorf("plain text embed = %q", got)
	}
}

// TestEmbedBlockEmptyTextKeepsImage: when no text could be extracted the
// original image reference must survive (empty block = replacement
// skipped by rebuildPhase).
func TestEmbedBlockEmptyTextKeepsImage(t *testing.T) {
	r := testRunner(t, false)
	p := &imageProgress{Class: ClassText, Kept: true}
	if got := r.embedBlock(p, "book.md", t.TempDir()); got != "" {
		t.Errorf("empty text embed = %q, want empty (ref kept)", got)
	}
}

// TestMDCommentBodyKeepsLinesAndNeutralisesTerminator: CONTENT may be a
// multi-line list, but "--" would close the HTML comment early.
func TestMDCommentBody(t *testing.T) {
	got := mdCommentBody("知\r\n识--导图\n- 知\n- 识")
	if strings.Contains(got, "--") {
		t.Errorf("comment body must not contain '--': %q", got)
	}
	if !strings.Contains(got, "- 知\n- 识") {
		t.Errorf("line structure lost: %q", got)
	}
}
