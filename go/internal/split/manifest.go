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
//
// Source paths are compared separator-insensitively (see
// normalizeSourcePath): the same file reaches the code as
// "files\a.pdf" on Windows and "files/a.pdf" on POSIX, and a raw
// string compare made the cache miss every time the same project was
// driven from the other platform — each side then re-ran the whole
// pdfcpu split of the other's library.
//
// DOCX manifests use the same schema with Kind="docx" and
// Mode="split" (parts exist as DOCX part files) or
// Mode="passthrough" (the source is comfortably under the page
// limit so no conversion/splitting is needed; Parts is empty).
// Older manifests written before the Kind/Mode fields existed
// default to pdf/split on load so PDF caches stay valid.
package split

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mineru-tools/pkg/util"
)

// ManifestSchemaVersion is bumped whenever an incompatible change is
// made to the manifest JSON layout. Any persisted manifest with a
// different value is rejected as a miss.
const ManifestSchemaVersion = 1

// StatusComplete marks a manifest whose corresponding split finished
// without error. Partial / failed splits must not be written.
const StatusComplete = "complete"

// Manifest kinds. "kind" identifies which pipeline produced the
// manifest so a PDF manifest cannot accidentally satisfy a DOCX
// lookup and vice-versa.
const (
	KindPDF  = "pdf"
	KindDOCX = "docx"
)

// Manifest modes. "mode=split" means the source was split into
// multiple part files (Parts is non-empty). "mode=passthrough"
// means the source was under the page limit and was not split
// (Parts is empty). It is used by DOCX to remember the no-op
// decision so the next run skips the LibreOffice conversion too.
const (
	ModeSplit       = "split"
	ModePassthrough = "passthrough"
)

// Manifest is the on-disk schema written by SplitPDF / SplitDOCX
// after a successful run.
type Manifest struct {
	SchemaVersion int     `json:"schema_version"`
	Kind          string  `json:"kind,omitempty"`
	Mode          string  `json:"mode,omitempty"`
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
// part files. It deliberately does not match the "_part*.pdf" /
// "_part*.docx" glob used for parts, so a stray glob call never
// sweeps the manifest in.
//
// The leading kind segment prevents collisions between a PDF and a
// DOCX that share a basename (e.g. "doc.pdf" and "doc.docx" each get
// their own manifest in the same outputDir).
const ManifestFilename = "_split_manifest.json"

// ManifestPath returns the manifest path for a given source and
// output directory. baseName should be the source's BaseNameNoExt
// and kind should be KindPDF or KindDOCX. Existing call sites that
// always wrote PDF manifests keep working because kind is only
// included when non-empty; this preserves on-disk name stability
// for PDF caches.
func ManifestPath(outputDir, baseName string) string {
	return filepath.Join(outputDir, baseName+ManifestFilename)
}

// LoadManifest reads and parses the manifest at path. Any error
// (missing file, malformed JSON, missing required fields) is
// surfaced so the caller can treat it as a cache miss. Missing
// Kind/Mode fields are filled in with the legacy defaults
// (pdf/split) so manifests written before the generalization
// remain valid.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("manifest json: %w", err)
	}
	if m.Kind == "" {
		m.Kind = KindPDF
	}
	if m.Mode == "" {
		m.Mode = ModeSplit
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
	if m.Kind != KindPDF && m.Kind != KindDOCX {
		return fmt.Errorf("manifest kind=%q, want %q or %q", m.Kind, KindPDF, KindDOCX)
	}
	if m.Mode != ModeSplit && m.Mode != ModePassthrough {
		return fmt.Errorf("manifest mode=%q, want %q or %q", m.Mode, ModeSplit, ModePassthrough)
	}
	switch m.Mode {
	case ModePassthrough:
		// Passthrough manifests intentionally have no parts — they
		// only mark the source as already known to be under the
		// limit.
		if len(m.Parts) != 0 {
			return fmt.Errorf("manifest mode=passthrough has %d parts, want 0", len(m.Parts))
		}
	case ModeSplit:
		if len(m.Parts) == 0 {
			return fmt.Errorf("manifest mode=split has no parts")
		}
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

// normalizeSourcePath makes a recorded source path comparable across
// platforms and spellings. Callers hand us the path they were given
// ("files\book.pdf" on Windows, "files/book.pdf" on POSIX, possibly
// "./files/book.pdf"), so the manifest key must not depend on the
// separator or on redundant prefixes. Both backslash and slash are
// folded to "/" on every platform (filepath.ToSlash is a no-op on
// POSIX, which would leave a Windows-written key unmatched when the
// same tree is read from Linux), then path.Clean collapses "." / "//".
func normalizeSourcePath(p string) string {
	return path.Clean(strings.ReplaceAll(p, `\`, "/"))
}

// Matches reports whether m describes a split with the same source
// identity and split parameters as p. Pure (no filesystem access).
func (m *Manifest) Matches(p MatchParams) bool {
	if m == nil {
		return false
	}
	if normalizeSourcePath(m.SourcePath) != normalizeSourcePath(p.SourcePath) {
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
// that has just been deleted. No-op if no matches are found. The
// base name is matched against both ManifestPath variants — the
// default PDF/draft layout and the kind-aware layout — so a
// caller can request removal without having to know which
// flavour is on disk.
func CleanupStaleManifests(outputDir string, baseNames ...string) {
	for _, b := range baseNames {
		if b == "" {
			continue
		}
		_ = os.Remove(ManifestPath(outputDir, b))
		_ = os.Remove(KindedManifestPath(outputDir, b, KindPDF))
		_ = os.Remove(KindedManifestPath(outputDir, b, KindDOCX))
	}
}

// KindedManifestPath returns a kind-aware manifest path. Use this
// to keep a DOCX manifest separate from a PDF manifest that
// shares the same basename (e.g. doc.pdf and doc.docx in the
// same outputDir). PDF callers can keep using ManifestPath — the
// unkinded layout is preserved for backwards compatibility.
func KindedManifestPath(outputDir, baseName, kind string) string {
	switch kind {
	case KindDOCX:
		return filepath.Join(outputDir, baseName+"_"+KindDOCX+"_"+ModeSplit+"_"+manifestStem())
	case KindPDF:
		return filepath.Join(outputDir, baseName+"_"+KindPDF+"_"+ModeSplit+"_"+manifestStem())
	default:
		return ManifestPath(outputDir, baseName)
	}
}

// manifestStem returns the trailing filename portion shared by
// every kind-aware manifest. Exposed only via KindedManifestPath.
func manifestStem() string {
	return "manifest.json"
}
