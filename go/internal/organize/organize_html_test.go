package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// organize must also collect and rewrite HTML <img src='images/...'> refs,
// exactly as it does for markdown ![...](images/...).
func TestRewriteImagePathsHTML(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "sub.md")
	content := "before <img src='images/foo.jpg'/> after ![](images/bar.jpg) end"
	if err := os.WriteFile(md, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteImagePaths(md, content, "sub"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(md)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "<img src='images/sub/foo.jpg'/>") {
		t.Fatalf("HTML src not prefixed: %q", out)
	}
	if !strings.Contains(out, "![](images/sub/bar.jpg)") {
		t.Fatalf("markdown path not prefixed: %q", out)
	}
}
