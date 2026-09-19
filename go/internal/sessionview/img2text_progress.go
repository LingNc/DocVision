// ScanImg2TextProgress 在下方。P18: img2text progress board — per-book per-image status for the web UI.
//
// Data sources under <finally>/progress_items/ (no logs involved):
//   - <md>.md/original.md     book markdown copy (dir keeps the .md suffix)
//                             → total image refs (order = book order)
//   - <md>/images_<img>.json  per-image result (older runs: images_<md>_<img>;
//                             result contains [IMG_TYPE:) → done
//   - mermaid_fix/<md>_<img>/ upgrade-fix workspace exists → escalated
//
// An image with neither a result nor a fix workspace is "pending" (never
// processed, failed-and-skipped, or in flight — the pipeline does not
// distinguish these on disk, so the board reports them honestly as 待处理).
package sessionview

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Img2TextItem is one image of one book on the progress board.
type Img2TextItem struct {
	// Name is the image file base name (as referenced from the md).
	Name string `json:"name"`
	// Status: done | fixed | escalated | pending。
	// fixed = 有结果且留过升级修复工作区（修好了）；escalated = 只有修复
	// 工作区没有结果（升级也未修好）。
	Status string `json:"status"`
}

// Img2TextBook is one md's progress summary + per-image list.
type Img2TextBook struct {
	// Book is the md base name without extension (the sidebar's img2text
	// project is "img2text · <Book>").
	Book      string         `json:"book"`
	Total     int            `json:"total"`
	Done      int            `json:"done"`
	Fixed     int            `json:"fixed"`
	Escalated int            `json:"escalated"`
	Pending   int            `json:"pending"`
	Items     []Img2TextItem `json:"items"`
}

// mdImageRe matches markdown image refs pointing at the images tree.
var mdImageRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]*)\)`)

// ScanImg2TextProgress builds the board for every book found under
// progressRoot (the img2text extra scan root = progress_items).
// Missing/unreadable pieces degrade to empty, never error out the page.
func ScanImg2TextProgress(progressRoot string) []Img2TextBook {
	st, err := os.Stat(progressRoot)
	if err != nil || !st.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(progressRoot)
	if err != nil {
		return nil
	}
	var books []Img2TextBook
	for _, e := range entries {
		name := e.Name()
		// 每本书一对目录：<md>/（逐图 json）+ <md>.md/original.md（书的
		// markdown 副本）。以 original.md 存在为准识别书目录。
		if !e.IsDir() || strings.HasSuffix(name, ".md") || name == "mermaid_fix" || name == "sessions" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(progressRoot, name+".md", "original.md"))
		if err != nil {
			continue
		}
		book := name
		b := Img2TextBook{Book: book}
		doneSet := map[string]bool{}
		escSet := map[string]bool{}

		// done: <md>/<imgfile>.json whose result carries an IMG_TYPE marker.
		itemDir := filepath.Join(progressRoot, book)
		if its, derr := os.ReadDir(itemDir); derr == nil {
			for _, it := range its {
				if it.IsDir() || !strings.HasSuffix(it.Name(), ".json") {
					continue
				}
				// images_<hash>.jpg.json / images_<md>_<hash>.jpg.json → <hash>.jpg
				imgName := strings.TrimSuffix(it.Name(), ".json")
				imgName = strings.TrimPrefix(imgName, "images_")
				imgName = strings.TrimPrefix(imgName, book+"_")
				small, _ := os.ReadFile(filepath.Join(itemDir, it.Name()))
				if strings.Contains(string(small), "[IMG_TYPE:") {
					doneSet[imgName] = true
				}
			}
		}
		// escalated: mermaid_fix/<md>_<imgfile>/ workspace.
		fixDir := filepath.Join(progressRoot, "mermaid_fix")
		if fixes, ferr := os.ReadDir(fixDir); ferr == nil {
			for _, f := range fixes {
				if !f.IsDir() || !strings.HasPrefix(f.Name(), book+"_") {
					continue
				}
				// mermaid_fix/<md>_<hash>.<ext>/ → <hash>.<ext>
				escSet[strings.TrimPrefix(f.Name(), book+"_")] = true
			}
		}

		// Order from the md's image refs (book order); unknown extras appended.
		seen := map[string]bool{}
		for _, m := range mdImageRe.FindAllStringSubmatch(string(data), -1) {
			img := baseName(m[1])
			if img == "" || seen[img] {
				continue
			}
			seen[img] = true
			b.Items = append(b.Items, Img2TextItem{Name: img, Status: itemStatus(doneSet, escSet, img)})
		}
		// Result/fix entries whose ref is not in the md (stale renames) still
		// show up, appended at the end.
		for img := range doneSet {
			if !seen[img] {
				b.Items = append(b.Items, Img2TextItem{Name: img, Status: itemStatus(doneSet, escSet, img)})
			}
		}
		for img := range escSet {
			if !seen[img] && !doneSet[img] {
				b.Items = append(b.Items, Img2TextItem{Name: img, Status: "escalated"})
			}
		}
		for _, it := range b.Items {
			b.Total++
			switch it.Status {
			case "done":
				b.Done++
			case "fixed":
				b.Fixed++
			case "escalated":
				b.Escalated++
			default:
				b.Pending++
			}
		}
		books = append(books, b)
	}
	sort.Slice(books, func(i, j int) bool { return books[i].Book < books[j].Book })
	return books
}

// itemStatus resolves one image's board status. done+fix workspace = fixed
// (修复成功，值得在板块上区分出来——它构成"修复记录"视角）。
func itemStatus(done, esc map[string]bool, img string) string {
	switch {
	case done[img] && esc[img]:
		return "fixed"
	case done[img]:
		return "done"
	case esc[img]:
		return "escalated"
	}
	return "pending"
}

// baseName strips directories; img2text progress files name images as
// images_<md>_<hash>.<ext> while the md refs images/<md>/<hash>.<ext>.
func baseName(p string) string {
	return filepath.Base(strings.ReplaceAll(p, "\\", "/"))
}
