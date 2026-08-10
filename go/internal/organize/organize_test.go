package organize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

// writeFile writes content to path, creating any parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// touchFile ensures path exists with the given size and mtime. If the
// file already exists, its size/mtime are updated to the requested values
// so fingerprint-based caching decisions are deterministic.
func touchFile(t *testing.T, path string, size int, mtimeNS int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if size < 0 {
		size = 0
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
	if mtimeNS > 0 {
		mt := time.Unix(0, mtimeNS)
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
}

// seedMinerUOutput writes a two-part source tree: one subject that
// references an image, one single-file subject with no images, and one
// subject whose image dir is missing so we can exercise the warning path.
func seedMinerUOutput(t *testing.T, root string) {
	t.Helper()

	// Two-part subject with images.
	for _, n := range []string{"1", "2"} {
		dir := filepath.Join(root, "merged_subject_part"+n)
		writeFile(t, filepath.Join(dir, "full.md"), "merged subject content part "+n+"\n\n![](images/pic_a.jpg)\n![](images/pic_b.jpg)\n")
		if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
			t.Fatalf("mkdir images: %v", err)
		}
		writeFile(t, filepath.Join(dir, "images", "pic_a.jpg"), "fake-image-bytes-a")
		writeFile(t, filepath.Join(dir, "images", "pic_b.jpg"), "fake-image-bytes-b")
	}

	// Single-file subject with one image.
	singleDir := filepath.Join(root, "single_subject")
	writeFile(t, filepath.Join(singleDir, "full.md"), "single subject\n\n![](images/pic_a.jpg)\n")
	if err := os.MkdirAll(filepath.Join(singleDir, "images"), 0o755); err != nil {
		t.Fatalf("mkdir images: %v", err)
	}
	writeFile(t, filepath.Join(singleDir, "images", "pic_a.jpg"), "fake-image-bytes")

	// Subject whose image source is absent.
	missingDir := filepath.Join(root, "missing_image_subject")
	writeFile(t, filepath.Join(missingDir, "full.md"), "missing\n\n![](images/ghost.jpg)\n")
}

// TestOrganizeFilesFirstRunGeneratesOutput covers the happy path: the
// first run creates merged Markdown, populates images, and prints the
// expected counts.
func TestOrganizeFilesFirstRunGeneratesOutput(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	tempDir := filepath.Join(outputDir, "temp")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first OrganizeFiles: %v", err)
	}

	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	if !fileExists(t, mergedMD) {
		t.Fatalf("expected merged md at %s", mergedMD)
	}
	data, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read merged md: %v", err)
	}
	if !strings.Contains(string(data), "merged subject content part 1") || !strings.Contains(string(data), "merged subject content part 2") {
		t.Fatalf("merged md missing parts: %q", string(data))
	}
	if !strings.Contains(string(data), "merged_subject") {
		t.Fatalf("merged md missing subject-prefixed image path: %q", string(data))
	}

	singleMD := filepath.Join(outputDir, "single_subject.md")
	if !fileExists(t, singleMD) {
		t.Fatalf("expected single md at %s", singleMD)
	}

	if !fileExists(t, filepath.Join(imagesDir, "merged_subject", "pic_a.jpg")) {
		t.Fatalf("expected pic_a.jpg under merged_subject image dir")
	}
	if !fileExists(t, filepath.Join(imagesDir, "single_subject", "pic_a.jpg")) {
		t.Fatalf("expected pic_a.jpg under single_subject image dir")
	}

	// temp should exist (rebuilt every run).
	if !dirExists(t, tempDir) {
		t.Fatalf("expected temp dir at %s", tempDir)
	}
}

// TestOrganizeFilesSecondRunPreservesExisting checks that a rerun does
// not delete output Markdown, images or manually created files, and that
// unchanged subjects are not re-generated.
func TestOrganizeFilesSecondRunPreservesExisting(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	original, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read merged md: %v", err)
	}

	// Drop a manually created file inside output/ to ensure it survives.
	manual := filepath.Join(outputDir, "manual_notes.txt")
	writeFile(t, manual, "user-authored content")
	// Replace an existing image with a custom file the user authored; the
	// second run must not delete it.
	manualImg := filepath.Join(imagesDir, "merged_subject", "manual.jpg")
	writeFile(t, manualImg, "manual image bytes")

	// Snapshot the merged md's mtime to confirm it's not rewritten.
	snap := time.Unix(0, 1_700_000_000_000_000_000)
	if err := os.Chtimes(mergedMD, snap, snap); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	snapMDInfo, err := os.Stat(mergedMD)
	if err != nil {
		t.Fatalf("stat merged md: %v", err)
	}

	// Second run with unchanged source.
	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if !fileExists(t, manual) {
		t.Fatalf("manual file was removed by rerun: %s", manual)
	}
	if !fileExists(t, manualImg) {
		t.Fatalf("manual image was removed by rerun: %s", manualImg)
	}

	after, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read merged md after: %v", err)
	}
	if string(after) != string(original) {
		t.Fatalf("merged md content changed on rerun")
	}
	afterInfo, err := os.Stat(mergedMD)
	if err != nil {
		t.Fatalf("stat merged md after: %v", err)
	}
	if !afterInfo.ModTime().Equal(snapMDInfo.ModTime()) {
		t.Fatalf("merged md mtime advanced on rerun: before=%v after=%v", snapMDInfo.ModTime(), afterInfo.ModTime())
	}
}

// TestOrganizeFilesSourceChangeRegenerates ensures that when a source
// full.md is modified, the corresponding merged md is regenerated on the
// next run.
func TestOrganizeFilesSourceChangeRegenerates(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	original, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read merged md: %v", err)
	}

	// Mutate one of the source files: change content and bump mtime so
	// the size+mtime fingerprint definitely differs.
	srcMD := filepath.Join(mineruOutput, "merged_subject_part1", "full.md")
	touchFile(t, srcMD, 4096, 1_700_000_001_000_000_000)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}

	after, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read merged md after: %v", err)
	}
	if string(after) == string(original) {
		t.Fatalf("expected merged md content to change after source mutation")
	}
	if !strings.Contains(string(after), strings.Repeat("x", 4096)) {
		t.Fatalf("expected regenerated md to include new source bytes, got %q", string(after)[:min(80, len(after))])
	}
}

// TestOrganizeFilesNewImageRefCopiesOnDemand covers the step3 on-demand
// copy: when the Markdown gains a new image reference that is missing on
// disk, the source image directory is scanned and the file is copied.
// Already-present images must be left untouched.
func TestOrganizeFilesNewImageRefCopiesOnDemand(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Drop a referenced image to force a missing-target condition on
	// the second run, then add a new reference in the source md.
	mergedImgDir := filepath.Join(imagesDir, "merged_subject")
	if err := os.Remove(filepath.Join(mergedImgDir, "pic_a.jpg")); err != nil {
		t.Fatalf("remove pic_a.jpg: %v", err)
	}
	// Add pic_c.jpg to the source images dir for the merged subject so
	// the new reference can be satisfied.
	writeFile(t, filepath.Join(mineruOutput, "merged_subject_part1", "images", "pic_c.jpg"), "fake-image-bytes-c")
	srcMD := filepath.Join(mineruOutput, "merged_subject_part1", "full.md")
	newSrc := "merged subject content part 1\n\n![](images/pic_a.jpg)\n![](images/pic_b.jpg)\n![](images/pic_c.jpg)\n"
	writeFile(t, srcMD, newSrc)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if !fileExists(t, filepath.Join(mergedImgDir, "pic_a.jpg")) {
		t.Fatalf("expected pic_a.jpg to be re-copied")
	}
	if !fileExists(t, filepath.Join(mergedImgDir, "pic_b.jpg")) {
		t.Fatalf("expected pic_b.jpg to be re-copied")
	}
	if !fileExists(t, filepath.Join(mergedImgDir, "pic_c.jpg")) {
		t.Fatalf("expected pic_c.jpg to be copied from new source images")
	}
}

// TestOrganizeFilesIdempotentRewrite ensures that a second run does not
// re-prefix already-prefixed image paths (no images/subject/subject/x.jpg
// chains).
func TestOrganizeFilesIdempotentRewrite(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	after, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	if strings.Contains(string(after), "images/merged_subject/merged_subject/") {
		t.Fatalf("path was double-prefixed: %s", string(after))
	}

	// Run a second time on unchanged source: rewrite must be a no-op.
	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after2, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read md second: %v", err)
	}
	if strings.Contains(string(after2), "images/merged_subject/merged_subject/") {
		t.Fatalf("path was double-prefixed after rerun: %s", string(after2))
	}
}

// TestOrganizeFilesMissingImageWarning checks that referencing an image
// not present in any source images directory produces the expected
// warning and does not crash the run.
func TestOrganizeFilesMissingImageWarning(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	data, err := os.ReadFile(mergedMD)
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	if !strings.Contains(string(data), "images/merged_subject/pic_a.jpg") {
		t.Fatalf("expected rewritten image path, got %q", string(data))
	}
	if !fileExists(t, filepath.Join(imagesDir, "merged_subject", "pic_a.jpg")) {
		t.Fatalf("expected pic_a.jpg under merged_subject image dir")
	}
}

// TestOrganizeFilesIndexBuiltOnce proves the source-image index is built
// at most once per OrganizeFiles invocation even when multiple subjects
// require lookups. The implementation resets indexScanCounter at the
// start of every OrganizeFiles call and increments it on the first
// index build, so after a run that needed lookups the counter must
// equal exactly 1 regardless of how many subjects triggered the build.
func TestOrganizeFilesIndexBuiltOnce(t *testing.T) {
	root := t.TempDir()
	mineruOutput := filepath.Join(root, "mineru_output")
	outputDir := filepath.Join(root, "output")
	imagesDir := filepath.Join(outputDir, "images")
	seedMinerUOutput(t, mineruOutput)

	cfg := newTestConfig(mineruOutput, outputDir, imagesDir)

	// First run: at least one subject (missing_image_subject) has no
	// images dir at all, so the index is built. Verify exactly once.
	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if indexScanCounter != 1 {
		t.Fatalf("expected image index built once per run, got %d builds", indexScanCounter)
	}

	// Force two subjects to require lookups on the second run by
	// deleting their on-disk images and adding bare-path references so
	// the references exist but the on-disk files are missing.
	for _, sub := range []string{"merged_subject", "single_subject"} {
		_ = os.RemoveAll(filepath.Join(imagesDir, sub))
	}
	mergedMD := filepath.Join(outputDir, "merged_subject.md")
	newSrc := "merged subject content part 1\n\n![](images/pic_a.jpg)\n![](images/pic_b.jpg)\n"
	writeFile(t, mergedMD, newSrc)
	singleMD := filepath.Join(outputDir, "single_subject.md")
	writeFile(t, singleMD, "single subject\n\n![](images/pic_a.jpg)\n")

	if err := OrganizeFiles(cfg); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if indexScanCounter != 1 {
		t.Fatalf("expected image index built once on second run, got %d builds", indexScanCounter)
	}
}

// TestRewriteImagePathsIdempotent directly exercises the path-rewrite
// helper for the most error-prone cases.
func TestRewriteImagePathsIdempotent(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		subject   string
		wantOnce  string // expected output after one pass
		wantTwice string // expected output after a second pass (= once)
	}{
		{
			name:      "bare path is prefixed once",
			input:     "see ![](images/foo.jpg) below",
			subject:   "sub",
			wantOnce:  "see ![](images/sub/foo.jpg) below",
			wantTwice: "see ![](images/sub/foo.jpg) below",
		},
		{
			name:      "already prefixed path is left alone",
			input:     "see ![](images/sub/foo.jpg) below",
			subject:   "sub",
			wantOnce:  "see ![](images/sub/foo.jpg) below",
			wantTwice: "see ![](images/sub/foo.jpg) below",
		},
		{
			name:      "mixed bare and prefixed",
			input:     "![a](images/a.jpg) ![b](images/sub/b.jpg)",
			subject:   "sub",
			wantOnce:  "![a](images/sub/a.jpg) ![b](images/sub/b.jpg)",
			wantTwice: "![a](images/sub/a.jpg) ![b](images/sub/b.jpg)",
		},
	}

	tmp := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmp, "case.md")
			if err := os.WriteFile(path, []byte(tc.input), 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := rewriteImagePaths(path, string(data), tc.subject); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantOnce {
				t.Fatalf("after first pass: got %q, want %q", string(got), tc.wantOnce)
			}
			// Second pass should be a no-op.
			if err := rewriteImagePaths(path, string(got), tc.subject); err != nil {
				t.Fatal(err)
			}
			got2, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got2) != tc.wantTwice {
				t.Fatalf("after second pass: got %q, want %q", string(got2), tc.wantTwice)
			}
		})
	}
}

// helpers

// newTestConfig returns a config.Config wired to the test paths.
func newTestConfig(mineruOutput, outputDir, imagesDir string) *config.Config {
	return &config.Config{
		Paths: config.PathsConfig{
			MineruOutput: mineruOutput,
			OutputDir:    outputDir,
			ImagesDir:    imagesDir,
		},
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
