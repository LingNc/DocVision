package img2text

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/pkg/util"
)

// ProgressItem is the on-disk record of one processed image. Field names
// match the Python dict ("key", "result", "start", "end", "img_path") so
// the JSON files are compatible with the existing tooling and analyzer.
//
// Start and End are the byte offsets of the image reference inside the
// source markdown (m.start() / m.end() in the Python regex), preserved
// verbatim so the final replacement pass can rebuild the file in place.
type ProgressItem struct {
	Key     string `json:"key"`
	Result  string `json:"result"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	ImgPath string `json:"img_path"`
}

// progressFileName converts an image path like "images/foo/bar.jpg" into
// a filesystem-safe basename. The Python reference uses
// `img_rel_path.replace("/", "_").replace("\\", "_")` followed by ".json".
func progressFileName(imgRelPath string) string {
	safe := strings.ReplaceAll(imgRelPath, "/", "_")
	safe = strings.ReplaceAll(safe, "\\", "_")
	return safe + ".json"
}

// LoadProgress scans progressRoot for per-item JSON files and returns
// the set of completed keys.
//
// Layout matches the Python reference:
//
//	<progressRoot>/<md_stem>/<safe_img_name>.json
//
// A missing progressRoot yields an empty map. Corrupt JSON files are
// silently skipped, matching the Python "except Exception: continue".
func LoadProgress(progressRoot string) map[string]bool {
	progress := map[string]bool{}
	if _, err := os.Stat(progressRoot); err != nil {
		return progress
	}

	entries, err := os.ReadDir(progressRoot)
	if err != nil {
		return progress
	}
	for _, mdDir := range entries {
		if !mdDir.IsDir() {
			continue
		}
		dirPath := filepath.Join(progressRoot, mdDir.Name())
		items, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, f := range items {
			if f.IsDir() {
				continue
			}
			if !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dirPath, f.Name()))
			if err != nil {
				continue
			}
			var doc struct {
				Key    string `json:"key"`
				Result string `json:"result"`
			}
			if err := json.Unmarshal(data, &doc); err != nil {
				continue
			}
			// Only count as "done" if result has [IMG_TYPE:] prefix,
			// indicating a successful conversion. Error sentinels like
			// [IMG_API_ERROR] should be reprocessed.
			if doc.Key != "" && strings.Contains(doc.Result, "[IMG_TYPE:") {
				progress[doc.Key] = true
			}
		}
	}
	return progress
}

// SaveProgressItem writes a single ProgressItem to
// <progressRoot>/<mdStem>/<safe_img_name>.json using the atomic
// temp+rename writer from pkg/util. The parent directory is created if
// it does not already exist.
func SaveProgressItem(progressRoot, mdStem, imgName string, item ProgressItem) error {
	subdir := filepath.Join(progressRoot, strings.TrimSuffix(mdStem, ".md"))
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		return fmt.Errorf("create progress subdir: %w", err)
	}
	target := filepath.Join(subdir, progressFileName(imgName))
	return util.AtomicWriteJSON(target, item)
}
