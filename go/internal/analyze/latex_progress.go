package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// latexProgressEntry mirrors latex.imageProgress (kept local to avoid an
// import cycle: latex imports nothing from analyze, but the JSON shape is
// all we need).
type latexProgressEntry struct {
	Status string `json:"status"`
	Class  string `json:"class,omitempty"`
	Kept   bool   `json:"original_kept,omitempty"`
	SVGFl  bool   `json:"svg_failed,omitempty"`
}

// PrintLatexProgressFooter summarises the progress_items of a LaTeX
// output directory (level-1: <latex_project>/source, level-2:
// latex.output_dir). Buckets:
//   - 已完成: text / raster-by-design / vector with a real figure
//   - 回退:   vector items degraded to the original image or a PNG/PDF
//     link (status "fallback" or svg_failed) — retried next run
//   - 未完成: classified-but-unprocessed or errored entries
func PrintLatexProgressFooter(outDir string) {
	progDir := filepath.Join(outDir, "progress_items")
	total, done, fallback, pending := 0, 0, 0, 0
	matches, _ := filepath.Glob(filepath.Join(progDir, "*", "*.json"))
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var p latexProgressEntry
		if json.Unmarshal(data, &p) != nil || p.Status == "" {
			continue
		}
		total++
		switch {
		case p.Status == "done" && !p.Kept && !p.SVGFl:
			done++
		case p.Status == "fallback" || p.SVGFl || (p.Status == "done" && p.Kept && p.Class == "vector"):
			fallback++
		default:
			pending++
		}
	}
	sep := strings.Repeat("=", 70)
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("【LaTeX 图片进度】(%s) 总计 %d | 已完成 %d | 回退 %d | 未完成 %d\n",
		filepath.Base(outDir), total, done, fallback, pending)
	if total > 0 {
		fmt.Printf("  完成率: %.2f%%\n", float64(done)/float64(total)*100)
	}
}
