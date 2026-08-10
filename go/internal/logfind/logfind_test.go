package logfind

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if age > 0 {
		past := time.Now().Add(-age)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	return path
}

func TestIsErrorLog(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"img2text_error_20250101_120000.log", true},
		{"img2text_20250101_120000.log", false},
		{"img2text_error_.log", false}, // empty timestamp
		{"img2text_error_20250101_120000.txt", false},
		{"prefix_img2text_error_20250101_120000.log", false},
		{"other_error_20250101.log", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsErrorLog(c.name); got != c.want {
			t.Errorf("IsErrorLog(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFindLatest_IgnoresErrorByMtime(t *testing.T) {
	dir := t.TempDir()
	old := write(t, dir, "img2text_20250101_120000.log", 2*time.Hour)
	latest := write(t, dir, "img2text_20250105_080000.log", time.Hour)
	// error log is newest by mtime but should be ignored.
	write(t, dir, "img2text_error_20250106_090000.log", time.Minute)

	got, err := FindLatest(dir)
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	if got != latest {
		t.Fatalf("FindLatest = %q, want %q (older primary %q)", got, latest, old)
	}
}

func TestFindAll_StableSortedAndNoError(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "img2text_20250101_000000.log", time.Hour)
	b := write(t, dir, "img2text_20250102_000000.log", 2*time.Hour)
	c := write(t, dir, "img2text_20250103_000000.log", 3*time.Hour)
	write(t, dir, "img2text_error_20250110_000000.log", time.Minute)

	files, err := FindAll(dir)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if !sort.StringsAreSorted(files) {
		t.Fatalf("FindAll result not sorted: %v", files)
	}
	if strings.Contains(strings.Join(files, ","), "img2text_error_") {
		t.Fatalf("FindAll returned error logs: %v", files)
	}
	if len(files) != 3 {
		t.Fatalf("len = %d, want 3 (%v)", len(files), files)
	}
	if files[0] != a || files[1] != b || files[2] != c {
		t.Fatalf("unexpected order: %v", files)
	}
}

func TestFindAll_IgnoresSubdirsAndOtherFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "img2text_20250101_000000.log", time.Hour)
	if err := os.Mkdir(filepath.Join(dir, "img2text_subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img2text_subdir", "img2text_20250102_000000.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := FindAll(dir)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len = %d, want 1 (%v)", len(files), files)
	}
}

func TestFindLatest_NoMatches(t *testing.T) {
	dir := t.TempDir()
	if _, err := FindLatest(dir); err == nil {
		t.Fatalf("expected error for empty dir")
	}
}

func TestFindLatest_AllErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "img2text_error_20250101_000000.log", time.Hour)
	if _, err := FindLatest(dir); err == nil {
		t.Fatalf("expected error when only error logs present")
	}
}

func TestFindLatest_MissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := FindLatest(missing)
	if err == nil || !strings.Contains(err.Error(), "no primary img2text") {
		t.Fatalf("want missing-dir error, got %v", err)
	}
}

func TestFindAll_MissingDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, err := FindAll(missing)
	if err == nil {
		t.Fatalf("expected error for missing dir")
	}
}
