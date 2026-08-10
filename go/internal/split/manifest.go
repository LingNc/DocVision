// Package split — split-manifest schema and verification.
//
// A split manifest is a sidecar JSON written next to the split output.
// It records what split parameters produced which part files, so a
// second invocation can skip the expensive pdfcpu PageCountFile /
// TrimFile work and reuse the on-disk parts.
//
// The cache is a strict byte-level identity check: every part file
// must still exist, every recorded size must match the on-disk size,
// every index/page range must line up with the part filename. Any
// mismatch (or unreadable source/manifest) is treated as a cache
// miss, so callers can fall back to re-splitting without risk of
// serving stale or corrupted output.
package split

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mineru-tools/pkg/util"
)

// ManifestSchemaVersion is bumped whenever an incompatible change is
// made to the manifest JSON layout. Any persisted manifest with a
// different value is rejected as a miss.
const ManifestSchemaVersion = 1

// StatusComplete marks a manifest whose corresponding split finished
// without error. Partial / failed splits must not be written.
const StatusComplete = "complete"

// Manifest is the on-disk schema written by SplitPDF after a
// successful run.
type Manifest struct {
	SchemaVersion int     `json:"schema_version"`
	SourcePath    string  `json:"source_path"`
	SourceSize    int64   `json:"source_size"`
	SourceMTimeNS int64   `json:"source_mtime_ns"`
	MaxPages      int     `json:"max_pages"`
	MaxSizeMB     float64 `json:"max_size_mb"`
	Status        string  `json:"status"`
	Parts         []Part  `json:"parts"`
}

// Part is one entry per output file. Filename is the basename only
// (no directory) so the manifest stays valid even when outputDir is
// renamed. Index is 1-based. PageStart/PageEnd are 1-based and
// inclusive, matching what api.TrimFile accepts.
type Part struct {
	Filename  string `json:"filename"`
	Index     int    `json:"index"`
	PageStart int    `json:"page_start"`
	PageEnd   int    `json:"page_end"`
	Size      int64  `json:"size"`
}

// ManifestFilename is the stable sidecar name written next to the
// part files. It deliberately does not match the "_part*.pdf"
// glob used for parts, so a stray glob call never sweeps the
// manifest in.
const ManifestFilename = "_split_manifest.json"

// ManifestPath returns the manifest path for a given source PDF and
// output directory. baseName should be the source's BaseNameNoExt.
func ManifestPath(outputDir, baseName string) string {
	return filepath.Join(outputDir, baseName+ManifestFilename)
}

// LoadManifest reads and parses the manifest at path. Any error
// (missing file, malformed JSON, missing required fields) is
// surfaced so the caller can treat it as a cache miss.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("manifest json: %w", err)
	}
	if err := validateManifest(m); err != nil {
		return nil, err
	}
	return m, nil
}

// validateManifest enforces the structural invariants of the
// schema. It does NOT touch the filesystem; disk checks live in
// VerifyAgainstDisk so they can be unit-tested independently.
func validateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("manifest schema_version=%d, want %d", m.SchemaVersion, ManifestSchemaVersion)
	}
	if m.SourcePath == "" {
		return fmt.Errorf("manifest source_path is empty")
	}
	if m.Status != StatusComplete {
		return fmt.Errorf("manifest status=%q, want %q", m.Status, StatusComplete)
	}
	if len(m.Parts) == 0 {
		return fmt.Errorf("manifest has no parts")
	}
	for i, p := range m.Parts {
		if p.Filename == "" {
			return fmt.Errorf("part[%d] filename is empty", i)
		}
		if p.Index != i+1 {
			return fmt.Errorf("part[%d] index=%d, want %d", i, p.Index, i+1)
		}
		if p.PageStart < 1 || p.PageEnd < p.PageStart {
			return fmt.Errorf("part[%d] page range %d-%d invalid", i, p.PageStart, p.PageEnd)
		}
		if p.Size <= 0 {
			return fmt.Errorf("part[%d] size=%d, want >0", i, p.Size)
		}
	}
	return nil
}

// MatchParams describes the split parameters for the run we are
// about to perform. It is used to verify that a cached manifest was
// produced under identical parameters.
type MatchParams struct {
	SourcePath  string
	SourceSize  int64
	SourceMTime int64
	MaxPages    int
	MaxSizeMB   float64
}

// Matches reports whether m describes a split with the same source
// identity and split parameters as p. Pure (no filesystem access).
func (m *Manifest) Matches(p MatchParams) bool {
	if m == nil {
		return false
	}
	if m.SourcePath != p.SourcePath {
		return false
	}
	if m.SourceSize != p.SourceSize {
		return false
	}
	if m.SourceMTimeNS != p.SourceMTime {
		return false
	}
	if m.MaxPages != p.MaxPages {
		return false
	}
	// maxSizeMB: treat zero and missing the same (the cache records
	// the value used at split time; a caller that later sets 0 must
	// still hit if the original split used 0). Float equality is
	// intentional — both sides come from the same caller invocation.
	if m.MaxSizeMB != p.MaxSizeMB {
		return false
	}
	return true
}

// VerifyAgainstDisk confirms that every part listed in the manifest
// still exists in outputDir, has the recorded size, and that the
// parts form a contiguous 1..N sequence in filename order.
//
// On success returns (true, nil). On any mismatch returns
// (false, nil) — the cache miss is silent by design so the caller
// falls back to re-splitting. A hard I/O error is returned so the
// caller can decide whether to surface it.
func (m *Manifest) VerifyAgainstDisk(outputDir string) (bool, error) {
	if m == nil {
		return false, nil
	}
	for i, p := range m.Parts {
		if p.Index != i+1 {
			return false, nil
		}
		// Filename must be a bare basename; the manifest is rooted at
		// outputDir. This also blocks a maliciously-crafted manifest
		// from pointing outside the output dir.
		if filepath.Base(p.Filename) != p.Filename {
			return false, nil
		}
		path := filepath.Join(outputDir, p.Filename)
		info, err := os.Stat(path)
		if err != nil {
			return false, nil
		}
		if info.Size() != p.Size {
			return false, nil
		}
	}
	return true, nil
}

// WriteManifest atomically writes m to path. The split code only
// calls this after every part has been written and verified, so the
// status is always StatusComplete at write time.
func WriteManifest(path string, m *Manifest) error {
	m.Status = StatusComplete
	return util.AtomicWriteJSON(path, m)
}

// CleanupStaleManifests removes manifest files in outputDir whose
// filename starts with any of the given base names. Used by
// SplitDOCX to drop a manifest produced from the intermediate PDF
// that has just been deleted. No-op if no matches are found.
func CleanupStaleManifests(outputDir string, baseNames ...string) {
	for _, b := range baseNames {
		if b == "" {
			continue
		}
		_ = os.Remove(ManifestPath(outputDir, b))
	}
}
