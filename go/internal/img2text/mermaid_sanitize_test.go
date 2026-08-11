// Tests for sanitizeMermaidError and the buildMermaidRepairMessage
// stack-sanitisation contract. The mmdc / puppeteer error strings
// contain hundreds of useless stack frames; we need to confirm only the
// "Error: ... Expecting ... got ..." summary survives in the repair
// prompt while the puppeteer internals are stripped.
//
// Coverage:
//
//  1. Realistic mmdc output with Parser.parseError stack → keep key
//     lines, drop stack / node_modules / preamble.
//  2. ValidateMermaid "block N: " wrapper → wrapper stripped, key
//     error preserved.
//  3. Fallback stack marker "\n    at " is used when Parser.parseError
//     is absent.
//  4. Pure text without "Error:" → returned verbatim (within the cap).
//  5. Oversize input → truncated, ends with "...", respects cap.
//  6. Empty input → empty output.
//  7. buildMermaidRepairMessage output: "Validator output:" section
//     must not contain stack frames; key error must be quoted.
package img2text

import (
	"strings"
	"testing"
)

// realisticMMDCError mimics what ValidateMermaid forwards when mmdc
// fails: a "Generating single mermaid chart" preamble, the
// "block N: " wrapper, the parse error block with caret underline and
// "Expecting ... got ..." line, followed by a puppeteer-style stack.
const realisticMMDCError = `Generating single mermaid chart

block 1: Error: Parse error on line 17:
... end    Cup1 -->|P(A) = 3/5| Cup2
---------------------^
Expecting 'SQE', 'DOUBLECIRCLEEND', ... got 'PS'
Parser.parseError (https://mermaid-cli-intercept.invalid/Users/x/node_modules/@mermaid-js/mermaid/src/parser/index.js:1234:17)
    at #evaluate (node:internal/process/task_queues:94:5)
    at processTicksAndRejections (node:internal/process/task_queues:60:5)
    at async Parser.parse (file:///Users/x/node_modules/@mermaid-js/mermaid/src/parser/index.js:1200:10)
    at async parse (file:///Users/x/node_modules/@mermaid-js/mermaid/dist/mermaid.js:4321:11)
    at async render (file:///Users/x/node_modules/@mermaid-js/mermaid/dist/mermaid.js:6500:11)
    at async draw (file:///Users/x/node_modules/mermaid-cli/src/index.js:88:9)
    at async <anonymous> (file:///Users/x/node_modules/mermaid-cli/src/index.js:42:7)`

// TestSanitizeMermaidError_StripsStackAndPreamble covers scenario (1).
// Parser.parseError marks the stack boundary; the preamble and the
// "block N: " wrapper must both be stripped, and key error tokens
// must survive.
func TestSanitizeMermaidError_StripsStackAndPreamble(t *testing.T) {
	got := sanitizeMermaidError(realisticMMDCError)

	mustContain := []string{
		"Parse error on line",
		"Expecting",
		"got 'PS'",
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Fatalf("sanitizeMermaidError must contain %q; got %q", s, got)
		}
	}

	mustNotContain := []string{
		"Parser.parseError",
		"node_modules",
		"    at ",
		"Generating single mermaid chart",
		"block 1:",
	}
	for _, s := range mustNotContain {
		if strings.Contains(got, s) {
			t.Fatalf("sanitizeMermaidError must NOT contain %q; got %q", s, got)
		}
	}
}

// TestSanitizeMermaidError_BlockPrefixStripped covers scenario (2):
// even when the stack marker is missing, the "block N: " wrapper is
// dropped by anchoring at the first "Error:".
func TestSanitizeMermaidError_BlockPrefixStripped(t *testing.T) {
	in := "block 1: Error: Syntax error at line 4: missing arrow head"
	got := sanitizeMermaidError(in)
	if strings.Contains(got, "block 1:") {
		t.Fatalf("sanitizeMermaidError must strip 'block 1:' prefix; got %q", got)
	}
	if !strings.Contains(got, "Error:") || !strings.Contains(got, "missing arrow head") {
		t.Fatalf("sanitizeMermaidError must keep the key error; got %q", got)
	}
}

// TestSanitizeMermaidError_FallbackStackMarker covers scenario (3):
// when "Parser.parseError" is absent we fall back to the first
// "\n    at " stack frame.
func TestSanitizeMermaidError_FallbackStackMarker(t *testing.T) {
	in := `block 1: Error: Unexpected token at line 2
    at parseThing (/tmp/foo.js:1:1)
    at run (/tmp/bar.js:2:2)`
	got := sanitizeMermaidError(in)
	if !strings.Contains(got, "Unexpected token at line 2") {
		t.Fatalf("sanitizeMermaidError must keep the error line; got %q", got)
	}
	if strings.Contains(got, "    at ") || strings.Contains(got, "/tmp/foo.js") {
		t.Fatalf("sanitizeMermaidError must strip the fallback stack marker; got %q", got)
	}
}

// TestSanitizeMermaidError_PlainTextPreserved covers scenario (4):
// when the input lacks "Error:" we return the trimmed input verbatim
// (within the cap). This matches the documented fallback so simple
// validator messages ("validator timed out") still reach the model.
func TestSanitizeMermaidError_PlainTextPreserved(t *testing.T) {
	in := "  validator timed out after 30s\n"
	got := sanitizeMermaidError(in)
	want := "validator timed out after 30s"
	if got != want {
		t.Fatalf("sanitizeMermaidError(%q) = %q, want %q", in, got, want)
	}
}

// TestSanitizeMermaidError_OversizeTruncated covers scenario (5):
// inputs longer than mermaidErrorStackCap bytes are truncated with a
// trailing "..." so the repair prompt stays bounded even when no
// stack marker is present.
func TestSanitizeMermaidError_OversizeTruncated(t *testing.T) {
	pad := strings.Repeat("x", mermaidErrorStackCap+500)
	in := "Error: " + pad

	got := sanitizeMermaidError(in)
	if len(got) > mermaidErrorStackCap+3 { // +3 for the "..." suffix
		t.Fatalf("len(sanitizeMermaidError) = %d, want <= %d", len(got), mermaidErrorStackCap+3)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("oversize output must end with \"...\"; got suffix %q", got[len(got)-3:])
	}
	if !strings.HasPrefix(got, "Error: ") {
		t.Fatalf("truncated output must start with the error anchor; got prefix %q", got[:10])
	}
}

// TestSanitizeMermaidError_Empty covers scenario (6): empty / whitespace
// inputs collapse to empty so the repair prompt's "Validator output: "
// suffix does not become "Validator output:  " with dangling spaces.
func TestSanitizeMermaidError_Empty(t *testing.T) {
	cases := []string{"", "   ", "\n\n\t  \n"}
	for _, in := range cases {
		if got := sanitizeMermaidError(in); got != "" {
			t.Fatalf("sanitizeMermaidError(%q) = %q, want empty", in, got)
		}
	}
}

// TestBuildMermaidRepairMessage_ValidatorSectionClean covers scenario
// (7): the "Validator output: ..." line quoted in the repair prompt
// must not contain the puppeteer stack; only the sanitised summary.
func TestBuildMermaidRepairMessage_ValidatorSectionClean(t *testing.T) {
	msg := buildMermaidRepairMessage("prev", realisticMMDCError)

	// Find the "Validator output:" line and grab everything up to the
	// next blank line so we can assert on the quoted portion without
	// pulling in the special-character rules further down.
	const marker = "Validator output: "
	idx := strings.Index(msg, marker)
	if idx < 0 {
		t.Fatalf("buildMermaidRepairMessage missing %q; got:\n%s", marker, msg)
	}
	tail := msg[idx+len(marker):]
	if end := strings.Index(tail, "\n\n"); end >= 0 {
		tail = tail[:end]
	}

	mustContain := []string{"Parse error on line", "Expecting", "got 'PS'"}
	for _, s := range mustContain {
		if !strings.Contains(tail, s) {
			t.Fatalf("validator section must contain %q; got:\n%s", s, tail)
		}
	}
	mustNotContain := []string{
		"Parser.parseError",
		"node_modules",
		"    at ",
		"Generating single mermaid chart",
		"block 1:",
	}
	for _, s := range mustNotContain {
		if strings.Contains(tail, s) {
			t.Fatalf("validator section must NOT contain %q; got:\n%s", s, tail)
		}
	}
}
