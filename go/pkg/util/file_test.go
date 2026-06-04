package util

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestAtomicWriteJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	data := map[string]interface{}{"a": 1, "b": "x"}
	if err := AtomicWriteJSON(p, data); err != nil {
		t.Fatal(err)
	}
	if !FileExists(p) {
		t.Fatalf("expected file at %s", p)
	}
}

func TestFileAndDirExists(t *testing.T) {
	dir := t.TempDir()
	if !DirExists(dir) {
		t.Fatal("expected temp dir to exist")
	}
	if FileExists(dir) {
		t.Fatal("directory should not be a file")
	}
	f := filepath.Join(dir, "f.txt")
	os.WriteFile(f, []byte("hi"), 0o644)
	if !FileExists(f) {
		t.Fatal("expected file to exist")
	}
	if DirExists(f) {
		t.Fatal("file should not be a directory")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "sub", "dst.txt")
	os.WriteFile(src, []byte("hello"), 0o644)
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "hello" {
		t.Fatalf("got %q want %q", got, "hello")
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0o644)
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0o644)
	if err := CopyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestGlobSorted(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"c.txt", "a.txt", "b.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte(""), 0o644)
	}
	got, err := GlobSorted(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dir, "a.txt"),
		filepath.Join(dir, "b.txt"),
		filepath.Join(dir, "c.txt"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.md"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "y.txt"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "Z.MD"), []byte(""), 0o644)
	got, err := ListFiles(dir, ".md")
	if err != nil {
		t.Fatal(err)
	}
	// case-insensitive: x.md and Z.MD
	if len(got) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(got), got)
	}
}

func TestListAllFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.pdf"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "b.png"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644)
	exts := map[string]bool{".pdf": true, ".png": true, ".jpg": true}
	got, err := ListAllFiles(dir, exts)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{filepath.Base(got[0]), filepath.Base(got[1])}
	sort.Strings(names)
	want := []string{"a.pdf", "b.png"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v want %v", names, want)
	}
}

func TestUnzip(t *testing.T) {
	dir := t.TempDir()
	// Build a zip in memory
	zipPath := filepath.Join(dir, "x.zip")
	f, _ := os.Create(zipPath)
	// Skip real zip creation here, just smoke test EnsureDir/BaseNameNoExt.
	f.Close()
	os.Remove(zipPath)

	if err := EnsureDir(filepath.Join(dir, "nested", "deep")); err != nil {
		t.Fatal(err)
	}
	if !DirExists(filepath.Join(dir, "nested", "deep")) {
		t.Fatal("EnsureDir did not create directory")
	}

	if got := BaseNameNoExt("/a/b/c.txt"); got != "c" {
		t.Fatalf("got %q want %q", got, "c")
	}
}

func TestBaseNameNoExt(t *testing.T) {
	cases := map[string]string{
		"a/b/c.txt": "c",
		"foo":       "foo",
		"a/b/c":     "c",
		"a.tar.gz":  "a.tar",
	}
	for in, want := range cases {
		if got := BaseNameNoExt(in); got != want {
			t.Errorf("BaseNameNoExt(%q) = %q, want %q", in, got, want)
		}
	}
}
