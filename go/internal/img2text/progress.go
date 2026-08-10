package img2text

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mineru-tools/pkg/util"
)

// OffsetPair records the [start, end) byte offsets of a single image
// reference inside its source markdown file. A ProgressItem may carry
// multiple pairs when the same image is referenced more than once in the
// same markdown; we always replace every occurrence with the same AI
// result so the markdown stays consistent.
type OffsetPair struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ProgressItem is the on-disk record of one processed image. Field names
// match the Python dict ("key", "result", "start", "end", "img_path") so
// the JSON files are compatible with the existing tooling and analyzer.
//
// Start and End preserve the first image reference's offsets for legacy
// readers (the analyze package, third-party tooling). New callers should
// prefer Offsets, which is a complete list of every occurrence in the
// source markdown.
type ProgressItem struct {
	Key     string       `json:"key"`
	Result  string       `json:"result"`
	Start   int          `json:"start"`
	End     int          `json:"end"`
	ImgPath string       `json:"img_path"`
	Offsets []OffsetPair `json:"offsets,omitempty"`
}

// EffectiveOffsets returns every offset pair this item represents. When
// the on-disk record pre-dates the multi-offset schema, EffectiveOffsets
// falls back to the legacy Start/End pair so callers always see at least
// one entry.
func (p ProgressItem) EffectiveOffsets() []OffsetPair {
	if len(p.Offsets) > 0 {
		out := make([]OffsetPair, len(p.Offsets))
		copy(out, p.Offsets)
		return out
	}
	return []OffsetPair{{Start: p.Start, End: p.End}}
}

// SetOffsets replaces Offsets and refreshes the legacy Start/End fields
// with the first offset so legacy readers still see a sensible value.
func (p *ProgressItem) SetOffsets(offsets []OffsetPair) {
	p.Offsets = append([]OffsetPair(nil), offsets...)
	if len(offsets) > 0 {
		p.Start = offsets[0].Start
		p.End = offsets[0].End
	}
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

// LoadProgressItems returns the full ProgressItem for every successful
// on-disk record. Corrupt JSON, missing keys, error sentinels and items
// whose md subdirectory does not match their key are silently skipped,
// mirroring the legacy LoadProgress resilience contract.
//
// Use this instead of LoadProgress when the caller needs offsets or
// results, e.g. for the final markdown rebuild pass.
func LoadProgressItems(progressRoot string) []ProgressItem {
	var items []ProgressItem
	if _, err := os.Stat(progressRoot); err != nil {
		return items
	}

	entries, err := os.ReadDir(progressRoot)
	if err != nil {
		return items
	}
	for _, mdDir := range entries {
		if !mdDir.IsDir() {
			continue
		}
		dirName := mdDir.Name()
		dirPath := filepath.Join(progressRoot, dirName)
		files, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, f := range files {
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
			var item ProgressItem
			if err := json.Unmarshal(data, &item); err != nil {
				continue
			}
			// Reject empty or sentinel results - they should be retried.
			if item.Key == "" || !strings.Contains(item.Result, "[IMG_TYPE:") {
				continue
			}
			// Reject items whose directory does not match their key.
			// SaveProgressItem strips ".md" from the key prefix when
			// naming the directory, so we strip it here too before the
			// comparison.
			expected := strings.SplitN(item.Key, "::", 2)
			if len(expected) != 2 {
				continue
			}
			if strings.TrimSuffix(expected[0], ".md") != dirName {
				continue
			}
			items = append(items, item)
		}
	}
	return items
}

// SaveProgressItem writes a single ProgressItem to
// <progressRoot>/<mdStem>/<safe_img_name>.json using the atomic
// temp+rename writer from pkg/util. The parent directory is created if
// it does not already exist.
//
// mdStem is trimmed of a trailing ".md" for backward compatibility with
// the existing on-disk layout.
func SaveProgressItem(progressRoot, mdStem, imgName string, item ProgressItem) error {
	subdir := filepath.Join(progressRoot, strings.TrimSuffix(mdStem, ".md"))
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		return fmt.Errorf("create progress subdir: %w", err)
	}
	target := filepath.Join(subdir, progressFileName(imgName))
	return util.AtomicWriteJSON(target, item)
}

// mergeProgressItems folds two slices of ProgressItem together, keeping
// the entry with the latest non-empty offsets for each key. Results in
// the second slice win on conflict; identical keys are deduplicated so
// the caller can apply the merged result without overwriting earlier
// offsets.
func mergeProgressItems(a, b []ProgressItem) []ProgressItem {
	byKey := make(map[string]ProgressItem, len(a)+len(b))
	for _, it := range a {
		byKey[it.Key] = it
	}
	for _, it := range b {
		if existing, ok := byKey[it.Key]; ok {
			merged := mergeOffsets(existing, it)
			byKey[it.Key] = merged
			continue
		}
		byKey[it.Key] = it
	}
	out := make([]ProgressItem, 0, len(byKey))
	for _, it := range byKey {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// mergeOffsets combines two ProgressItem records that share a key. The
// returned item keeps the result from "winner" but unions offsets from
// both records (deduplicating identical pairs).
func mergeOffsets(keep, winner ProgressItem) ProgressItem {
	seen := map[OffsetPair]bool{}
	var merged []OffsetPair
	for _, o := range keep.EffectiveOffsets() {
		if !seen[o] {
			seen[o] = true
			merged = append(merged, o)
		}
	}
	for _, o := range winner.EffectiveOffsets() {
		if !seen[o] {
			seen[o] = true
			merged = append(merged, o)
		}
	}
	keep.SetOffsets(merged)
	keep.Result = winner.Result
	if winner.ImgPath != "" {
		keep.ImgPath = winner.ImgPath
	}
	return keep
}
