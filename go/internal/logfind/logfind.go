// Package logfind locates primary img2text log files.
package logfind

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IsErrorLog reports whether name is an error log filename.
func IsErrorLog(name string) bool {
	const prefix = "img2text_error_"
	return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".log") && len(name) > len(prefix)+len(".log")
}

func find(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "img2text_*.log"))
	if err != nil {
		return nil, fmt.Errorf("find logs in %s: %w", dir, err)
	}
	files := make([]string, 0, len(matches))
	for _, path := range matches {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() || IsErrorLog(filepath.Base(path)) {
			continue
		}
		files = append(files, path)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no primary img2text_*.log found in %s", dir)
	}
	return files, nil
}

// FindLatest returns the filename-sorted latest primary log.
func FindLatest(dir string) (string, error) {
	files, err := find(dir)
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	return files[len(files)-1], nil
}

// FindAll returns all primary logs in ascending filename order.
func FindAll(dir string) ([]string, error) {
	files, err := find(dir)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// FindLatestWithFallback returns the latest primary log found in primaryDir.
// If primaryDir has no primary logs, it falls back to legacyDir. Returns an
// error only when neither directory yields a primary log. Results from the
// two directories are never merged, so the returned path always belongs to a
// single, unambiguous source.
func FindLatestWithFallback(primaryDir, legacyDir string) (string, error) {
	files, err := find(primaryDir)
	if err == nil {
		sort.Strings(files)
		return files[len(files)-1], nil
	}
	if legacyDir == "" || legacyDir == primaryDir {
		return "", err
	}
	files, err = find(legacyDir)
	if err != nil {
		return "", fmt.Errorf("no primary img2text_*.log in %s and fallback %s", primaryDir, legacyDir)
	}
	sort.Strings(files)
	return files[len(files)-1], nil
}

// FindAllWithFallback returns all primary logs from primaryDir. If primaryDir
// has no primary logs, it falls back to legacyDir. Results from the two
// directories are never merged or duplicated.
func FindAllWithFallback(primaryDir, legacyDir string) ([]string, error) {
	files, err := find(primaryDir)
	if err == nil {
		sort.Strings(files)
		return files, nil
	}
	if legacyDir == "" || legacyDir == primaryDir {
		return nil, err
	}
	files, err = find(legacyDir)
	if err != nil {
		return nil, fmt.Errorf("no primary img2text_*.log in %s and fallback %s", primaryDir, legacyDir)
	}
	sort.Strings(files)
	return files, nil
}
