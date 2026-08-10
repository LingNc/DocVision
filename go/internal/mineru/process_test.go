package mineru

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHasCoreOutput verifies the completion check used to decide whether
// to skip a previously "done" task. The rule is: a folder counts as
// fully extracted only when the canonical full.md exists and is
// non-empty. Partial or empty directories must not be misread as done.
func TestHasCoreOutput(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(dir string) error
		wantOut bool
	}{
		{
			name:    "missing_dir",
			setup:   func(dir string) error { return os.MkdirAll(filepath.Dir(dir), 0o755) },
			wantOut: false,
		},
		{
			name: "empty_dir",
			setup: func(dir string) error {
				return os.MkdirAll(dir, 0o755)
			},
			wantOut: false,
		},
		{
			name: "missing_full_md",
			setup: func(dir string) error {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "images"), []byte("x"), 0o644)
			},
			wantOut: false,
		},
		{
			name: "empty_full_md",
			setup: func(dir string) error {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				f, err := os.Create(filepath.Join(dir, "full.md"))
				if err != nil {
					return err
				}
				return f.Close()
			},
			wantOut: false,
		},
		{
			name: "valid_full_md",
			setup: func(dir string) error {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "full.md"), []byte("# done\n"), 0o644)
			},
			wantOut: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "stem")
			if err := tc.setup(dir); err != nil {
				t.Fatal(err)
			}
			got := hasCoreOutput(dir)
			if got != tc.wantOut {
				t.Fatalf("hasCoreOutput(%q) = %v, want %v", dir, got, tc.wantOut)
			}
		})
	}
}

// TestSkipRequiresCoreOutput documents the resume-fast-path contract:
// even when the on-disk task state says "done", a file is only skipped
// once full.md exists and is non-empty. A folder that contains only an
// images/ subdirectory (the typical leftover from a failed extraction)
// must not be treated as complete, so the caller re-downloads.
func TestSkipRequiresCoreOutput(t *testing.T) {
	root := t.TempDir()
	outputDir := filepath.Join(root, "out")
	stem := "demo"
	folder := filepath.Join(outputDir, stem)
	if err := os.MkdirAll(filepath.Join(folder, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	// On-disk state claims done, but the canonical file is missing.
	if utilDirExists(folder) && hasCoreOutput(folder) {
		t.Fatalf("folder with only images/ should not be considered complete")
	}
}

// utilDirExists is a local re-export so this test compiles without
// importing the util package solely for one call. The package already
// depends on util in non-test code, so this keeps the test self-contained.
func utilDirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
