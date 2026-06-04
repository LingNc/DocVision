package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileExists reports whether path exists and is a regular file (or symlink to one).
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CopyFile copies src to dst, preserving file content. Mode bits are preserved.
// Existing destination files are overwritten. Parent directory of dst is created
// if it does not already exist.
func CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// CopyDir recursively copies the contents of src to dst. The destination
// directory is created if it does not exist. Symlinks are not followed.
func CopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		// Skip symlinks and other non-regular files to avoid surprising
		// behaviour. They are rare in our use case.
		if !entry.Type().IsRegular() {
			continue
		}

		if err := CopyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

// GlobSorted returns the glob matches for pattern, sorted lexicographically.
func GlobSorted(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// ListFiles returns regular files in dir whose name ends with ext (case-insensitive),
// sorted lexicographically. Pass "" to list all regular files. Non-recursive.
func ListFiles(dir string, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	extLower := strings.ToLower(ext)
	var out []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		if ext != "" && !strings.HasSuffix(strings.ToLower(name), extLower) {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}

// ListAllFiles returns regular files in dir whose extension (including the dot,
// case-insensitive) is in extensions. Sorted lexicographically. Non-recursive.
func ListAllFiles(dir string, extensions map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if extensions[ext] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Unzip extracts the zip archive at src into dst, creating dst if necessary.
// Path-traversal entries (those that escape dst via "..") are skipped.
func Unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	// Resolve and lock the destination to prevent traversal.
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	for _, f := range r.File {
		target, err := safeJoin(absDst, f.Name)
		if err != nil {
			return fmt.Errorf("unsafe zip entry %q: %w", f.Name, err)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode().Perm()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := unzipOne(f, target); err != nil {
			return err
		}
	}
	return nil
}

func unzipOne(f *zip.File, dst string) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// safeJoin joins absDst and rel, refusing any path that would escape absDst.
func safeJoin(absDst, rel string) (string, error) {
	joined := filepath.Join(absDst, rel)
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// Ensure absJoined is either absDst itself or starts with absDst + os.PathSeparator.
	if absJoined != absDst && !strings.HasPrefix(absJoined, absDst+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes destination")
	}
	return absJoined, nil
}

// EnsureDir creates the directory at path (and any parents) with mode 0o755 if
// it does not already exist. Returns nil if the directory already exists.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// BaseNameNoExt returns the final path element of path with its file extension
// stripped. It does not consult the filesystem; "a/b/c.txt" -> "c".
func BaseNameNoExt(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == "" {
		return base
	}
	return base[:len(base)-len(ext)]
}
