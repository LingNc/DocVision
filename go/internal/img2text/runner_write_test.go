package img2text

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteFinalMDMissingCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "out.md")
	written, err := writeFinalMD(p, []byte("hello"))
	if err != nil {
		t.Fatalf("writeFinalMD: %v", err)
	}
	if !written {
		t.Fatalf("missing file should be written")
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "hello" {
		t.Fatalf("file content = %q, err=%v", data, err)
	}
}

func TestWriteFinalMDSameContentSkips(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "out.md")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, before, before); err != nil {
		t.Fatal(err)
	}
	written, err := writeFinalMD(p, []byte("hello"))
	if err != nil {
		t.Fatalf("writeFinalMD: %v", err)
	}
	if written {
		t.Fatalf("identical content should not be rewritten")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(before) {
		t.Fatalf("mtime changed: %v (want %v)", fi.ModTime(), before)
	}
}

func TestWriteFinalMDDifferentContentWrites(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "out.md")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := writeFinalMD(p, []byte("new"))
	if err != nil {
		t.Fatalf("writeFinalMD: %v", err)
	}
	if !written {
		t.Fatalf("different content should be written")
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "new" {
		t.Fatalf("updated content = %q, err=%v", data, err)
	}
}
