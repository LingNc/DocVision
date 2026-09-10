package latex

// Printed-size measurement for ORIGINAL figure bitmaps.
//
// A view_image call returns pixels, so the model can see WHAT a figure
// looks like but never HOW BIG it is on the page. That is why redrawn
// figures came out page-sized: the source bitmap is 284x156px and the
// drawing prompt had no scale reference. The measurement below walks back
// from the bitmap to the MinerU parse (the part directory that holds
// content_list.json + layout.json), takes the block bbox of that image and
// the page size, and reports the figure's real printed size in millimetres
// plus its effective resolution — the numbers the model needs to keep the
// scale right.

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ImageMeasure describes one original bitmap's physical size on the page.
type ImageMeasure struct {
	WidthMM   float64 // printed width
	HeightMM  float64 // printed height
	PageWMM   float64 // page width (for the "x% of the page width" hint)
	PixelsW   int     // bitmap pixel width
	PixelsH   int     // bitmap pixel height
	DPI       int     // effective resolution: pixels / printed inch
	Found     bool    // false when the MinerU parse could not be located
	AspectStr string  // "1.82:1 (width:height)"
}

// String renders the standard display form (mm first — the unit the model
// must target — then pixels, effective dpi and the page ratio).
func (m ImageMeasure) String() string {
	if !m.Found {
		return ""
	}
	part := ""
	if m.PageWMM > 0 {
		part = fmt.Sprintf(", about %.0f%% of the page width", m.WidthMM/m.PageWMM*100)
	}
	return fmt.Sprintf("ORIGINAL FIGURE SIZE: %.1fmm x %.1fmm on the page%s; bitmap %dx%dpx, effective resolution %d dpi, aspect %s. Redraw it at that size — do NOT scale it up to the page.",
		m.WidthMM, m.HeightMM, part, m.PixelsW, m.PixelsH, m.DPI, m.AspectStr)
}

// AspectOnly is the fallback hint when the MinerU parse is unavailable
// (the ratio is still known from the bitmap itself).
func (m ImageMeasure) AspectOnly() string {
	if m.AspectStr == "" {
		return ""
	}
	return fmt.Sprintf("ORIGINAL BITMAP: %dx%dpx, aspect %s.",
		m.PixelsW, m.PixelsH, m.AspectStr)
}

type measureCacheEntry struct {
	m  ImageMeasure
	ok bool
}

var imageMeasureCache sync.Map // abs image path -> measureCacheEntry

// MeasureImage returns the printed size of the original bitmap at path.
// Results are cached per path; failures are cached too (the missing parse is
// not going to appear mid-run).
func MeasureImage(path string) ImageMeasure {
	if e, ok := imageMeasureCache.Load(path); ok {
		c := e.(measureCacheEntry)
		return c.m
	}
	m := measureImageUncached(path)
	imageMeasureCache.Store(path, measureCacheEntry{m: m, ok: true})
	return m
}

func measureImageUncached(path string) ImageMeasure {
	m := ImageMeasure{}
	if w, h := imageSize(path); w > 0 && h > 0 {
		m.PixelsW, m.PixelsH = w, h
		m.AspectStr = fmt.Sprintf("%.2f:1", float64(w)/float64(h))
	}
	// Locate the MinerU part directory: the nearest ancestor holding a
	// content_list.json (the folder such as <mineru>/<part>/images/<subject>/x.jpg).
	dir := filepath.Dir(path)
	var partDir string
	for i := 0; i < 5 && dir != "" && dir != "/"; i++ {
		if matches, _ := filepath.Glob(filepath.Join(dir, "*content_list.json")); len(matches) > 0 {
			partDir = dir
			break
		}
		dir = filepath.Dir(dir)
	}
	if partDir == "" {
		return m
	}
	base := strings.ToLower(filepath.Base(path))
	var bbox [4]float64
	found := false
	pageIdx := 0
	matches, _ := filepath.Glob(filepath.Join(partDir, "*content_list.json"))
	for _, cl := range matches {
		if found {
			break
		}
		raw, err := os.ReadFile(cl)
		if err != nil {
			continue
		}
		var entries []struct {
			PageIdx int        `json:"page_idx"`
			BBox    [4]float64 `json:"bbox"`
			ImgPath string     `json:"img_path"`
		}
		if json.Unmarshal(raw, &entries) != nil {
			continue
		}
		for _, e := range entries {
			if e.ImgPath == "" || len(e.BBox) != 4 {
				continue
			}
			if strings.ToLower(filepath.Base(e.ImgPath)) == base {
				bbox = e.BBox
				pageIdx = e.PageIdx
				found = true
				break
			}
		}
	}
	if !found {
		return m
	}
	// Page size from layout.json (points), then pt -> mm.
	var layout struct {
		PDFInfo []struct {
			PageSize [2]float64 `json:"page_size"`
		} `json:"pdf_info"`
	}
	pagePt := [2]float64{}
	if raw, err := os.ReadFile(filepath.Join(partDir, "layout.json")); err == nil {
		if json.Unmarshal(raw, &layout) == nil && len(layout.PDFInfo) > pageIdx {
			pagePt = layout.PDFInfo[pageIdx].PageSize
		}
	}
	const ptPerMM = 72.0 / 25.4
	// content_list bbox uses MinerU's layout coordinate space (layoutScale x
	// page points), the same factor doc_search prints crop hints with.
	wPt := (bbox[2] - bbox[0]) / layoutScale
	hPt := (bbox[3] - bbox[1]) / layoutScale
	if wPt <= 0 || hPt <= 0 {
		return m
	}
	m.WidthMM = wPt / ptPerMM
	m.HeightMM = hPt / ptPerMM
	// MinerU block bboxes are not tight around the artwork (they can include
	// a caption or padding), so trust the bbox for SCALE and the bitmap
	// itself for SHAPE: reporting width 36.9mm with a 2.7:1 aspect while
	// telling the model to keep 1.8:1 would contradict itself.
	if m.PixelsW > 0 && m.PixelsH > 0 {
		m.HeightMM = m.WidthMM * float64(m.PixelsH) / float64(m.PixelsW)
	}
	if pagePt[0] > 0 {
		m.PageWMM = pagePt[0] / ptPerMM
	}
	if m.PixelsW > 0 && m.WidthMM > 0 {
		m.DPI = int(float64(m.PixelsW) / (m.WidthMM / 25.4))
	}
	m.Found = true
	if m.AspectStr == "" {
		m.AspectStr = fmt.Sprintf("%.2f:1", wPt/hPt)
	}
	return m
}

// measureHint is the one-line hint used in prompts/tool results.
func measureHint(path string) string {
	if path == "" {
		return ""
	}
	m := MeasureImage(path)
	if m.Found {
		return m.String()
	}
	return m.AspectOnly()
}

var _ = image.DecodeConfig
