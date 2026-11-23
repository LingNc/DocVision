package luadraw

import (
	"os"
	"path/filepath"
)

// CacheDirectory shares documentation indexes and learned excerpts between
// native tools and runner sessions without writing into the pinned library.
func CacheDirectory(resourceRoot string) string {
	if configured := os.Getenv("COMPARE_FIGURE_CACHE"); configured != "" {
		return configured
	}
	if resourceRoot != "" {
		return filepath.Join(resourceRoot, "build", "doc-cache")
	}
	return filepath.Join(os.TempDir(), "compare-figure-go-cache", "doc-index")
}
