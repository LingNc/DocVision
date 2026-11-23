package luadraw

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Bundled from LuaDraw 3.7 and compare-figure 0.2.1; see NOTICE.md.
//
//go:embed resources.tar.gz
var resources []byte
var resourceMu sync.Mutex

// Resources materializes the versioned runtime and documentation once in a
// writable cache without depending on a developer checkout or network.
func Resources() (string, error) {
	resourceMu.Lock()
	defer resourceMu.Unlock()
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(resources)
	parent := filepath.Join(cache, "docvision", "luadraw")
	root := filepath.Join(parent, fmt.Sprintf("3.7-%x", sum[:8]))
	if _, err := os.Stat(filepath.Join(root, ".complete")); err == nil {
		return root, nil
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(parent, "extract-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	gz, err := gzip.NewReader(bytes.NewReader(resources))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invalid resource path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg {
			return "", fmt.Errorf("unsupported resource entry %q", h.Name)
		}
		target := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, ".complete"), []byte(fmt.Sprintf("%x", sum)), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, root); err != nil {
		if _, e := os.Stat(filepath.Join(root, ".complete")); e != nil {
			return "", err
		}
	}
	return root, nil
}
