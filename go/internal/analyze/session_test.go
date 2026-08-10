package analyze

import "testing"

// TestClassifyError_MermaidInvalid pins the [IMG_MERMAID_INVALID] sentinel
// to the "mermaid_invalid" category instead of the catch-all "unknown".
// The Go-side error classifier mirrors the Python ERROR_PATTERNS list; if
// a new sentinel is added in img2text (processor.go) it must be wired up
// here too, otherwise the report's 错误分类统计 falls back to "unknown".
func TestClassifyError_MermaidInvalid(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"[IMG_MERMAID_INVALID] some detail", "mermaid_invalid"},
		{"[img_mermaid_invalid] lowercase", "mermaid_invalid"},
		{"prefix [IMG_MERMAID_INVALID] suffix", "mermaid_invalid"},
	}
	for _, tc := range cases {
		got := ClassifyError(tc.in)
		if got != tc.want {
			t.Fatalf("ClassifyError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestClassifyError_ExistingCategories guards the existing mapping from
// regressing when the new mermaid pattern is added. Order matters because
// the first match wins; these cases already matched before but are pinned
// here so the test suite catches accidental reorderings.
func TestClassifyError_ExistingCategories(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"[IMG_RATE_LIMIT_EXCEEDED] waiting", "rate_limit"},
		{"[IMG_CONNECTION_TIMEOUT] retry exhausted", "connection_timeout"},
		{"[IMG_MISSING: foo.png]", "img_missing"},
		{"[IMG_ERROR: foo.png - boom]", "img_error"},
		{"[IMG_API_ERROR: bad gateway]", "api_error"},
		{"[IMG_WORKER_FATAL: md missing]", "worker_fatal"},
		{"[IMG_EMPTY_RESPONSE]", "empty_response"},
		{"[IMG_INVALID_FORMAT]", "invalid_format"},
		{"totally unrelated string", "unknown"},
	}
	for _, tc := range cases {
		got := ClassifyError(tc.in)
		if got != tc.want {
			t.Fatalf("ClassifyError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
