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

// parseRoot is the MinerU output root (paths.mineru_output). The sessions
// look at bitmaps that images 阶段 COPIED into the project
// (<proj>/source/images/<主题>/<sha>.jpg), so walking up from the bitmap
// never reaches the parse and every measurement silently degraded to the
// px-only fallback ("ORIGINAL BITMAP: 320x178px"): the mm the model needs
// to keep the printed scale was missing in every real run. With the root
// known, the bitmap is matched to its parse entry by FILE NAME (MinerU
// image names are content hashes and the copies keep them).
var (
	parseRootMu sync.RWMutex
	parseRoot   string
)

// SetImageParseRoot tells the measurement where MinerU results live. Called
// once per runner (paths.mineru_output); safe to call repeatedly.
func SetImageParseRoot(dir string) {
	if strings.TrimSpace(dir) == "" {
		// 空串 = 清空（测试隔离；正常运行总是给 paths.mineru_output）。
		parseRootMu.Lock()
		parseRoot, parseIndex, parseIndexOnce = "", nil, sync.Once{}
		parseRootMu.Unlock()
		resetImageMeasureCache()
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	parseRootMu.Lock()
	changed := parseRoot != abs
	if changed {
		parseRoot = abs
		parseIndexOnce = sync.Once{}
		parseIndex = nil
	}
	parseRootMu.Unlock()
	if changed {
		// 之前那些"找不解析→只有 px"的失败结论已经过时，必须丢掉。
		resetImageMeasureCache()
	}
}

// resetImageMeasureCache drops cached measurements (they depend on the parse
// root: a miss recorded before the root was known would otherwise stick).
func resetImageMeasureCache() {
	imageMeasureCache.Range(func(k, _ any) bool {
		imageMeasureCache.Delete(k)
		return true
	})
}

// parseEntry is one image block of a MinerU parse: where it is printed on
// which page, and how big that page is.
type parseEntry struct {
	WidthMM  float64
	HeightMM float64
	PageWMM  float64
	DPI      int
	Aspect   string
}

var (
	parseIndexOnce sync.Once
	parseIndex     map[string]parseEntry // lower-cased image file name -> entry
)

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
	// Symlinks are resolved first: a project view may link into the parse.
	lookup := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		lookup = resolved
	}
	dir := filepath.Dir(lookup)
	var partDir string
	for i := 0; i < 5 && dir != "" && dir != "/"; i++ {
		if matches, _ := filepath.Glob(filepath.Join(dir, "*content_list.json")); len(matches) > 0 {
			partDir = dir
			break
		}
		dir = filepath.Dir(dir)
	}
	if partDir == "" {
		// The bitmap is a copy inside the project: match it by file name
		// against the parse index built from paths.mineru_output.
		if e, ok := lookupParseEntry(filepath.Base(path)); ok {
			m.WidthMM, m.HeightMM, m.PageWMM = e.WidthMM, e.HeightMM, e.PageWMM
			m.DPI, m.Found = e.DPI, true
			if e.Aspect != "" {
				m.AspectStr = e.Aspect
			}
		}
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

// lookupParseEntry finds one bitmap in the MinerU parse index (built once
// per root from every <root>/<part>/content_list.json + layout.json).
func lookupParseEntry(name string) (parseEntry, bool) {
	parseRootMu.RLock()
	root := parseRoot
	parseRootMu.RUnlock()
	if root == "" {
		return parseEntry{}, false
	}
	parseIndexOnce.Do(func() { parseIndex = buildParseIndex(root) })
	if parseIndex == nil {
		return parseEntry{}, false
	}
	e, ok := parseIndex[strings.ToLower(name)]
	return e, ok
}

// buildParseIndex maps every parsed image file name to its printed size.
// content_list.json holds one bbox per image block, layout.json the page
// size of each page; both are needed to turn a bbox into millimetres.
func buildParseIndex(root string) map[string]parseEntry {
	idx := map[string]parseEntry{}
	parts, _ := filepath.Glob(filepath.Join(root, "*"))
	const ptPerMM = 72.0 / 25.4
	for _, part := range parts {
		matches, _ := filepath.Glob(filepath.Join(part, "*content_list.json"))
		if len(matches) == 0 {
			continue
		}
		// Page sizes of this part, indexed by page.
		pageMM := map[int][2]float64{}
		var layout struct {
			PDFInfo []struct {
				PageSize [2]float64 `json:"page_size"`
			} `json:"pdf_info"`
		}
		if raw, err := os.ReadFile(filepath.Join(part, "layout.json")); err == nil {
			if json.Unmarshal(raw, &layout) == nil {
				for i, p := range layout.PDFInfo {
					if p.PageSize[0] > 0 && p.PageSize[1] > 0 {
						pageMM[i] = [2]float64{p.PageSize[0] / ptPerMM, p.PageSize[1] / ptPerMM}
					}
				}
			}
		}
		for _, cl := range matches {
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
				wMM := (e.BBox[2] - e.BBox[0]) / layoutScale / ptPerMM
				hMM := (e.BBox[3] - e.BBox[1]) / layoutScale / ptPerMM
				if wMM <= 0 || hMM <= 0 {
					continue
				}
				key := strings.ToLower(filepath.Base(e.ImgPath))
				if _, dup := idx[key]; dup {
					continue
				}
				ent := parseEntry{WidthMM: wMM, HeightMM: hMM, Aspect: fmt.Sprintf("%.2f:1", wMM/hMM)}
				if pg, ok := pageMM[e.PageIdx]; ok {
					ent.PageWMM = pg[0]
				}
				idx[key] = ent
			}
		}
	}
	return idx
}

// Crop returns the measurement of a sub-rectangle of this bitmap, given crop
// percentages. view_image crops in bitmap pixels; the printed size of the
// crop follows from the same mm-per-pixel scale, which is what lets the
// model size a redrawn detail (or check one) against the original.
func (m ImageMeasure) Crop(left, top, right, bottom float64) ImageMeasure {
	if !m.Found || m.PixelsW <= 0 || m.PixelsH <= 0 {
		return ImageMeasure{}
	}
	fw := (right - left) / 100
	fh := (bottom - top) / 100
	if fw <= 0 || fh <= 0 {
		return ImageMeasure{}
	}
	out := m
	out.PixelsW = int(float64(m.PixelsW) * fw)
	out.PixelsH = int(float64(m.PixelsH) * fh)
	out.WidthMM = m.WidthMM * fw
	// 高度按位图比例换算（与 MeasureImage 同一口径：宽度信 bbox，形状信位图）。
	if m.WidthMM > 0 && m.PixelsW > 0 && m.PixelsH > 0 {
		out.HeightMM = out.WidthMM * float64(out.PixelsH) / float64(out.PixelsW)
	} else {
		out.HeightMM = m.HeightMM * fh
	}
	return out
}

// CropHint renders a crop's printed size in the same mm-first form as the
// full figure (empty when the measurement is unavailable).
func (m ImageMeasure) CropHint() string {
	if !m.Found {
		return ""
	}
	return fmt.Sprintf("this crop is %.1fmm x %.1fmm on the page", m.WidthMM, m.HeightMM)
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
