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
