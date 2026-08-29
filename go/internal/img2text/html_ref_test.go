package img2text

import (
	"os"
	"path/filepath"
	"testing"
)

// imageRefRe must recognise both markdown ![...](images/...) and self-contained
// HTML <img src='images/...'> elements (MinerU emits these inside table cells),
// capturing the path in group 1.
func TestImageRefReBothDialects(t *testing.T) {
	cases := []struct{ in, want string }{
		{"![](images/abc.jpg)", "images/abc.jpg"},
		{"![alt](images/def.jpeg)", "images/def.jpeg"},
		{"before ![](images/ghi.png) after", "images/ghi.png"},
		{"<img src='images/00138580021bbb45516586596c4ff9215146968448a9fd6175dfcf7f9cc29bce.jpg'/>", "images/00138580021bbb45516586596c4ff9215146968448a9fd6175dfcf7f9cc29bce.jpg"},
		{"<img width='10' src='images/xyz.jpg'>", "images/xyz.jpg"},
		{"<img src='images/foo.jpg' alt='bar'>", "images/foo.jpg"},
		{"<img SRC='images/upper.JPG' />", "images/upper.JPG"},
		{"<span>not an image</span>", ""},
		{"plain text", ""},
	}
	for _, c := range cases {
		m := imageRefRe.FindStringSubmatch(c.in)
		got := ""
		if m != nil {
			got = m[1]
		}
		if got != c.want {
			t.Errorf("in %q: got %q want %q", c.in, got, c.want)
		}
	}
}

// validateOffsets must accept an HTML <img> offset in addition to markdown.
func TestValidateOffsetsAcceptsHTML(t *testing.T) {
	content := "<td><img src='images/abc.jpg'/></td>"
	start := 4
	end := start + len("<img src='images/abc.jpg'/>")
	out := validateOffsets(content, []OffsetPair{{Start: start, End: end}})
	if len(out) != 1 {
		t.Fatalf("want 1 valid offset, got %d", len(out))
	}
}

// The subject fallback in resolveImageFile must resolve bare paths cited via HTML.
func TestResolveImageFileHTML(t *testing.T) {
	tmp := t.TempDir()
	imagesDir := filepath.Join(tmp, "output", "images")
	subject := "subject"
	name := "abc.jpg"
	dir := filepath.Join(imagesDir, subject)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveImageFile(imagesDir, "images/"+name, subject)
	if err != nil {
		t.Fatalf("want resolve via subject fallback, got %v", err)
	}
	if got != filepath.Join(dir, name) {
		t.Fatalf("got %q want %q", got, filepath.Join(dir, name))
	}
}
