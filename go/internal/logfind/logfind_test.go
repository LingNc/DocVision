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
	if err == nil || !strings.Contains(err.Error(), "no primary pipeline log") {
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

func TestFindLatestWithFallback_PrimaryWins(t *testing.T) {
	primary := t.TempDir()
	legacy := t.TempDir()
	primaryHit := write(t, primary, "img2text_20250110_000000.log", time.Hour)
	legacyHit := write(t, legacy, "img2text_20250120_000000.log", time.Hour)

	got, err := FindLatestWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindLatestWithFallback: %v", err)
	}
	if got != primaryHit {
		t.Fatalf("FindLatestWithFallback = %q, want primary %q (legacy was %q)", got, primaryHit, legacyHit)
	}
}

func TestFindLatestWithFallback_LegacyOnly(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "nope")
	legacy := t.TempDir()
	legacyHit := write(t, legacy, "img2text_20250111_000000.log", time.Hour)

	got, err := FindLatestWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindLatestWithFallback: %v", err)
	}
	if got != legacyHit {
		t.Fatalf("FindLatestWithFallback = %q, want legacy %q", got, legacyHit)
	}
}

func TestFindLatestWithFallback_BothEmpty(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "nope")
	legacy := filepath.Join(t.TempDir(), "nope2")
	if _, err := FindLatestWithFallback(primary, legacy); err == nil {
		t.Fatalf("expected error when both directories missing")
	}
}

func TestFindLatestWithFallback_SkipsLegacyWhenPrimaryHasErrorsOnly(t *testing.T) {
	primary := t.TempDir()
	legacy := t.TempDir()
	write(t, primary, "img2text_error_20250101_000000.log", time.Hour)
	legacyHit := write(t, legacy, "img2text_20250102_000000.log", time.Hour)

	got, err := FindLatestWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindLatestWithFallback: %v", err)
	}
	if got != legacyHit {
		t.Fatalf("FindLatestWithFallback = %q, want legacy %q", got, legacyHit)
	}
}

func TestFindLatestWithFallback_NoMergeWhenBothHaveLogs(t *testing.T) {
	primary := t.TempDir()
	legacy := t.TempDir()
	primaryHit := write(t, primary, "img2text_20250103_000000.log", time.Hour)
	write(t, legacy, "img2text_20250110_000000.log", time.Hour)

	got, err := FindLatestWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindLatestWithFallback: %v", err)
	}
	if got != primaryHit {
		t.Fatalf("FindLatestWithFallback = %q, want primary %q (must not merge with legacy)", got, primaryHit)
	}
}

func TestFindAllWithFallback_PrimaryWins(t *testing.T) {
	primary := t.TempDir()
	legacy := t.TempDir()
	p1 := write(t, primary, "img2text_20250101_000000.log", time.Hour)
	p2 := write(t, primary, "img2text_20250102_000000.log", time.Hour)
	write(t, legacy, "img2text_20250103_000000.log", time.Hour)
	write(t, legacy, "img2text_20250104_000000.log", time.Hour)

	files, err := FindAllWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindAllWithFallback: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2 (%v)", len(files), files)
	}
	if files[0] != p1 || files[1] != p2 {
		t.Fatalf("unexpected order: %v", files)
	}
}

func TestFindAllWithFallback_LegacyOnly(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "nope")
	legacy := t.TempDir()
	l1 := write(t, legacy, "img2text_20250101_000000.log", time.Hour)
	l2 := write(t, legacy, "img2text_20250102_000000.log", time.Hour)

	files, err := FindAllWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindAllWithFallback: %v", err)
	}
	if len(files) != 2 || files[0] != l1 || files[1] != l2 {
		t.Fatalf("unexpected files: %v", files)
	}
}

func TestFindAllWithFallback_BothEmpty(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "nope")
	legacy := filepath.Join(t.TempDir(), "nope2")
	if _, err := FindAllWithFallback(primary, legacy); err == nil {
		t.Fatalf("expected error when both directories missing")
	}
}

func TestFindAllWithFallback_NoDuplicateMerge(t *testing.T) {
	primary := t.TempDir()
	legacy := t.TempDir()
	p1 := write(t, primary, "img2text_20250101_000000.log", time.Hour)
	write(t, legacy, "img2text_20250110_000000.log", time.Hour)

	files, err := FindAllWithFallback(primary, legacy)
	if err != nil {
		t.Fatalf("FindAllWithFallback: %v", err)
	}
	if len(files) != 1 || files[0] != p1 {
		t.Fatalf("expected primary-only result, got %v", files)
	}
}

// T55：两种前缀混合时按内嵌时间排序，不按前缀字母（原先
// "img2text_" < "latex_" 让 analyze -r 0 永远拿到 latex 最新日志）。
func TestSortByTimeMixedPrefixes(t *testing.T) {
	files := []string{
		"a/latex_20260910_100000.log",
		"a/img2text_20260919_130000.log", // 最新
		"a/latex_20260916_184221.log",
		"a/img2text_20260915_090000.log",
	}
	SortByTime(files)
	want := []string{
		"a/latex_20260910_100000.log",
		"a/img2text_20260915_090000.log",
		"a/latex_20260916_184221.log",
		"a/img2text_20260919_130000.log",
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", files, want)
		}
	}
}
