package util

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AtomicWriteJSON writes JSON data to a file atomically using temp+rename pattern.
// Data is written to a temp file in the same directory, fsync'd, then renamed
// over the destination. On any failure, the temp file is cleaned up.
func AtomicWriteJSON(path string, data interface{}) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // cleanup on any error path

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
