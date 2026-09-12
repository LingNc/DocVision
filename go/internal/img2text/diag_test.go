// Tests for the diagnostics helpers:
//
//  1. extractValidationError must surface the REAL error lines and drop
//     the engine's version banner ("This is XeTeX, Version …" /
//     "entering extended mode" / "Document Class: standalone") instead of
//     the first 400 bytes, which are always banner.
//  2. snippet must hand back the model's own text (truncated, rune-safe,
//     with the total length) so "invalid response" is diagnosable.
//  3. leftoverDrawingBlock must recognise the LaTeX/TikZ drawing blocks
//     img2text cannot validate or embed.
package img2text

import (
	"strings"
	"testing"
)

// realXeTeXOutput is a trimmed copy of the output a real failing TikZ
// compile produced during the 2026-09-12 run: the useful part sits far
// past the banner, which is exactly why "first 400 bytes" was useless.
const realXeTeXOutput = `This is XeTeX, Version 3.141592653-2.6-0.999998 (TeX Live 2026) (preloaded format=xelatex)
 restricted \write18 enabled.
entering extended mode
(./check.tex
LaTeX2e <2025-11-01>
(/usr/share/texlive/texmf-dist/tex/latex/standalone/standalone.cls
Document Class: standalone 2025/07/22 v1.3a
)
! Undefined control sequence.
l.14 \drawarr
             ow (0,0) -- (1,1);
! Emergency stop.
<*> check.tex

No pages of output.
Transcript written on check.log.
`

func TestExtractValidationError_PicksRealErrorNotBanner(t *testing.T) {
	got := extractValidationError(realXeTeXOutput)

	// The real failure must be present.
	for _, want := range []string{"! Undefined control sequence.", "l.14", "! Emergency stop."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in extracted error:\n%s", want, got)
		}
	}
	// The banner must not be.
	for _, bad := range []string{"This is XeTeX", "entering extended mode", "Document Class: standalone", "preloaded format="} {
		if strings.Contains(got, bad) {
			t.Errorf("version banner leaked into extracted error (%q):\n%s", bad, got)
		}
	}
	// And it must be far shorter than the raw dump.
	if len(got) >= len(realXeTeXOutput) {
		t.Errorf("extracted error not shorter than raw output: %d >= %d", len(got), len(realXeTeXOutput))
	}
}

// With nothing that looks like an error, the fallback must be the LAST
// lines of the output, never the first N bytes.
func TestExtractValidationError_FallsBackToTail(t *testing.T) {
	var b strings.Builder
	b.WriteString("line-1-filler\n")
	for i := 2; i <= 30; i++ {
		b.WriteString("line-")
		b.WriteString(strings.Repeat("x", 40))
		b.WriteString("\n")
	}
	b.WriteString("THE-LAST-LINE\n")

	got := extractValidationError(b.String())
	if !strings.Contains(got, "THE-LAST-LINE") {
		t.Errorf("tail fallback lost the last line: %q", got)
	}
	if strings.Contains(got, "line-1-filler") {
		t.Errorf("tail fallback returned the head instead of the tail: %q", got)
	}
}

// A mermaid/CLI failure that carries no TeX-style error line still gets
// a non-empty, useful summary.
func TestExtractValidationError_NonTeXOutput(t *testing.T) {
	got := extractValidationError("Generating single mermaid chart\nblock 1: Error: Parse error on line 17:\nExpecting 'SQE' got 'PS'\n")
	if got == "" {
		t.Fatal("empty summary for mermaid-style output")
	}
	if !strings.Contains(got, "Parse error") {
		t.Errorf("mermaid parse error lost: %q", got)
	}
	if strings.HasPrefix(got, "Generating single mermaid chart") {
		t.Errorf("kept only the CLI preamble: %q", got)
	}
}

func TestSnippet_TruncatesRuneSafeWithLength(t *testing.T) {
	short := "  一段说明  "
	got := snippet(short)
	if !strings.Contains(got, "一段说明") || !strings.Contains(got, "共 4 字符") {
		t.Errorf("short snippet = %q", got)
	}

	long := strings.Repeat("图", snippetMaxChars+25)
	got = snippet(long)
	if !strings.Contains(got, "截断") {
		t.Errorf("long snippet not marked as truncated: %q", got[:40])
	}
	if !strings.Contains(got, "共 825 字符") {
		t.Errorf("long snippet misses total length: %q", got[len(got)-30:])
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("snippet broke a multibyte character: %q", got)
	}
}

func TestLeftoverDrawingBlock(t *testing.T) {
	cases := []struct {
		name, text string
		want       bool
	}{
		{"tikz fence", "[IMG_TYPE: tikz]\n```tikz\n\\begin{tikzpicture}\\end{tikzpicture}\n```", true},
		{"latex fence", "[IMG_TYPE: vector]\n```latex\n\\begin{tikzpicture}\\end{tikzpicture}\n```", true},
		{"pgfplots fence", "```pgfplots\n\\begin{axis}\\end{axis}\n```", true},
		{"mermaid is fine", "[IMG_TYPE: flowchart]\n```mermaid\ngraph TD; A-->B\n```", false},
		{"plain text is fine", "[IMG_TYPE: text]\n这是一段说明。", false},
		{"latex math is fine", "[IMG_TYPE: latex]\n$$E=mc^2$$", false},
		{"python code screenshot is fine", "[IMG_TYPE: code]\n```python\nprint(1)\n```", false},
	}
	for _, tc := range cases {
		if got := leftoverDrawingBlock(tc.text); got != tc.want {
			t.Errorf("%s: leftoverDrawingBlock=%v want %v", tc.name, got, tc.want)
		}
	}
}

// The one-line expectation hint must name the accepted shape and the
// rejected one, so a log reader sees the deviation immediately.
func TestExpectedFormatHint_MentionsBothSides(t *testing.T) {
	if !strings.Contains(expectedFormatHint, "[IMG_TYPE:") {
		t.Errorf("hint does not describe the required prefix: %q", expectedFormatHint)
	}
	if !strings.Contains(expectedFormatHint, "mermaid") {
		t.Errorf("hint does not name mermaid as the diagram format: %q", expectedFormatHint)
	}
	if !strings.Contains(expectedFormatHint, "tikz") {
		t.Errorf("hint does not forbid tikz: %q", expectedFormatHint)
	}
}
