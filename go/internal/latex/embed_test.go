package latex

import (
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
	return &Runner{cfg: &config.Config{}, log: log, inline: inline}
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
	}
	block := r.embedBlock(p, "book.md", t.TempDir())

	for _, want := range []string{
		"<!-- DOCVISION-STYLED-TEXT: black characters in blue circular bubbles",
		"CONTENT: 知识导图",
		" -->",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("level-1 styled block missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "BEGIN") || strings.Contains(block, "END") {
		t.Errorf("level-1 styled block must not use BEGIN/END markers:\n%s", block)
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
