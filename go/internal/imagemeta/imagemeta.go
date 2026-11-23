package imagemeta

// T335：交付出去的图要能在外面的程序里算出和 compare_figure 一模一样的
// 印刷尺寸，就得把**原始量**（pt 的 bbox、页尺寸、页码）一起带上——mm、dpi、
// 宽高比都是这几个数的派生量，存原始量最省空间，也免得算法改了要重跑解析。
//
// 索引来自 MinerU 的 content_list.json（每张图的 bbox/page_idx）与
// layout.json（每页 page_size，pt）。文件名即 MinerU 的内容哈希名，和
// `images/<书>/<文件>` 里的一致。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Raw is the original measured quantity of one image, in points.
type Raw struct {
	Page  int        `json:"page"`
	BBox  [4]float64 `json:"bbox_pt"`
	PageW float64    `json:"page_w_pt"`
	PageH float64    `json:"page_h_pt"`
	Found bool       `json:"found"`
}

var (
	mu    sync.RWMutex
	root  string
	index map[string]Raw
)

// SetMineruRoot 指定 MinerU 结果根（paths.mineru_output）；重复调用安全。
func SetMineruRoot(dir string) {
	abs := ""
	if d := strings.TrimSpace(dir); d != "" {
		if a, err := filepath.Abs(d); err == nil {
			abs = a
		} else {
			abs = d
		}
	}
	mu.Lock()
	changed := root != abs
	root = abs
	if changed {
		index = nil
	}
	mu.Unlock()
}

// LookupImage 按文件名（含扩展名，如 `abc123.jpg`）返回原始量。
func LookupImage(file string) (Raw, bool) {
	mu.RLock()
	have := index
	base := root
	mu.RUnlock()
	if have == nil {
		mu.Lock()
		if index == nil {
			index = build(base)
		}
		have = index
		mu.Unlock()
	}
	r, ok := have[strings.TrimSpace(file)]
	return r, ok
}

// build 扫 <root>/*/_content_list.json 的图条目，页尺寸从同目录 layout.json 取。
func build(base string) map[string]Raw {
	out := map[string]Raw{}
	if base == "" {
		return out
	}
	parts, err := os.ReadDir(base)
	if err != nil {
		return out
	}
	for _, p := range parts {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(base, p.Name())
		pageSizes := layoutPageSizes(filepath.Join(dir, "layout.json"))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		// Treat directory names literally: Glob would interpret brackets in
		// book names. Merge every parse run; duplicate basenames keep the
		// first available metadata, as in the existing index.
		var lists []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_content_list.json") {
				lists = append(lists, filepath.Join(dir, entry.Name()))
			}
		}
		for _, name := range lists {
			raw, err := os.ReadFile(name)
			if err != nil {
				continue
			}
			var items []struct {
				Type      string    `json:"type"`
				ImgPath   string    `json:"img_path"`
				ImagePath string    `json:"image_path"`
				PageIdx   int       `json:"page_idx"`
				BBox      []float64 `json:"bbox"`
			}
			if err := json.Unmarshal(raw, &items); err != nil {
				continue
			}
			for _, it := range items {
				// Any item with a bitmap path and bbox has usable image metadata,
				// including chart/table items emitted by newer MinerU versions.
				ref := it.ImgPath
				if ref == "" {
					ref = it.ImagePath
				}
				if ref == "" || len(it.BBox) < 4 {
					continue
				}
				key := filepath.Base(ref)
				if _, dup := out[key]; dup {
					continue
				}
				r := Raw{Page: it.PageIdx + 1, BBox: [4]float64{it.BBox[0], it.BBox[1], it.BBox[2], it.BBox[3]}, Found: true}
				if sz, ok := pageSizes[it.PageIdx]; ok {
					r.PageW, r.PageH = sz[0], sz[1]
				}
				out[key] = r
			}
		}
	}
	return out
}

// layoutPageSizes 从 layout.json 取每页 [宽, 高]（pt），键是 page_idx。
func layoutPageSizes(path string) map[int][2]float64 {
	out := map[int][2]float64{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var lay struct {
		PDFInfo []struct {
			PageIdx  int       `json:"page_idx"`
			PageSize []float64 `json:"page_size"`
		} `json:"pdf_info"`
	}
	if err := json.Unmarshal(raw, &lay); err != nil {
		return out
	}
	for _, p := range lay.PDFInfo {
		if len(p.PageSize) >= 2 {
			out[p.PageIdx] = [2]float64{p.PageSize[0], p.PageSize[1]}
		}
	}
	return out
}
