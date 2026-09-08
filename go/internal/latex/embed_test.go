package latex

import (
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// testRunner builds a Runner with a quiet logger for embed tests.
func testRunner(t *testing.T) *Runner {
	t.Helper()
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	log.SetQuiet(true)
	t.Cleanup(func() { _ = log.Close() })
	return &Runner{cfg: &config.Config{}, log: log}
}

// TestEmbedBlockStyledTextMarkers pins the level-2 styled-text format:
// the extracted text is wrapped in BEGIN/END markers so the conversion
// agent can tell the image's own text from surrounding markdown, and no
// AI description is embedded.
func TestEmbedBlockStyledTextMarkers(t *testing.T) {
	r := testRunner(t)
	p := &imageProgress{
		Class: ClassText, Styled: true,
		StyleNote: "black characters in blue circular bubbles",
		Content:   "知识导图",
	}
	block := r.embedBlock(p, "book.md", t.TempDir())

	for _, want := range []string{
		"<!-- DOCVISION-STYLED-TEXT: ",
		"blue circular bubbles",
		"<!-- DOCVISION-STYLED-TEXT-BEGIN -->",
		"知识导图",
		"<!-- DOCVISION-STYLED-TEXT-END -->",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("styled block missing %q:\n%s", want, block)
		}
	}
	if strings.Index(block, "BEGIN") > strings.Index(block, "知识导图") ||
		strings.Index(block, "知识导图") > strings.Index(block, "END") {
		t.Errorf("text must sit between BEGIN/END:\n%s", block)
	}
}

// TestEmbedBlockPlainTextIsDirect: plain text images keep the old
// behaviour (the extracted text is embedded verbatim).
func TestEmbedBlockPlainTextIsDirect(t *testing.T) {
	r := testRunner(t)
	p := &imageProgress{Class: ClassText, Content: "随机试验的定义"}
	if got := r.embedBlock(p, "book.md", t.TempDir()); got != "随机试验的定义" {
		t.Errorf("plain text embed = %q", got)
	}
}

// TestEmbedBlockEmptyTextKeepsImage: when no text could be extracted the
// original image reference must survive (empty block = replacement
// skipped by rebuildPhase).
func TestEmbedBlockEmptyTextKeepsImage(t *testing.T) {
	r := testRunner(t)
	p := &imageProgress{Class: ClassText, Kept: true}
	if got := r.embedBlock(p, "book.md", t.TempDir()); got != "" {
		t.Errorf("empty text embed = %q, want empty (ref kept)", got)
	}
}
