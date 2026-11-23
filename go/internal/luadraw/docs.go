package luadraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const docSchema = 4
const docIndexAlgorithm = "pdf-paragraph-v2"

type bbox [4]float64
type word struct {
	Text string  `json:"text"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
	X2   float64 `json:"x2"`
	Y2   float64 `json:"y2"`
}
type block struct {
	Text string `json:"text"`
	BBox bbox   `json:"bbox"`
}
type page struct {
	Number int     `json:"page"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Blocks []block `json:"blocks"`
}
type docEntry struct {
	Path  string `json:"path"`
	Kind  string `json:"kind"`
	Text  string `json:"text,omitempty"`
	Pages []page `json:"pages,omitempty"`
}
type fileMeta struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime_ns"`
}
type docIndex struct {
	Schema      int        `json:"schema"`
	Root        string     `json:"root"`
	Version     string     `json:"version"`
	Created     int64      `json:"created"`
	Key         string     `json:"key"`
	Fingerprint []fileMeta `json:"fingerprint"`
	Docs        []docEntry `json:"docs"`
}
type Docs struct {
	root, cache, path string
	mu                sync.Mutex
	freqMu            sync.Mutex
	index             *docIndex
}

// NewDocs creates a standalone Go documentation indexer. Poppler tools are required.
func NewDocs(root, cache string) (*Docs, error) {
	if root == "" {
		return nil, fmt.Errorf("LuaDraw documentation root is required")
	}
	root = abs(root)
	st, e := os.Stat(root)
	if e != nil || !st.IsDir() {
		return nil, fmt.Errorf("LuaDraw root not found: %s", root)
	}
	for _, n := range []string{"pdftotext", "pdfinfo", "pdftoppm"} {
		if lookup(n) == "" {
			return nil, fmt.Errorf("required Poppler executable not found: %s", n)
		}
	}
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "luadraw-doc-index")
	}
	cache = abs(cache)
	if e = os.MkdirAll(cache, 0700); e != nil {
		return nil, e
	}
	return &Docs{root: root, cache: cache, path: filepath.Join(cache, "luadraw-doc-index-v4.json")}, nil
}

// Definition returns the OpenAI-compatible function schema for luadraw_doc.
func (d *Docs) Definition() map[string]any {
	props := map[string]any{
		"mode":  map[string]any{"type": "string", "enum": []string{"search", "read", "view", "list", "find", "grep", "frequent"}},
		"query": map[string]any{"type": "string"},
		"doc":   map[string]any{"type": "string", "description": "Path relative to the fixed LuaDraw root; PDF read accepts file.pdf#page=N"},
		"page":  map[string]any{"type": "integer", "description": "1-based PDF page for view/read; read may use #page=N in doc"},
		"left":  map[string]any{"type": "number"}, "top": map[string]any{"type": "number"}, "right": map[string]any{"type": "number"}, "bottom": map[string]any{"type": "number"},
		"zoom": map[string]any{"type": "integer"}, "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"},
		"start_line": map[string]any{"type": "integer"}, "end_line": map[string]any{"type": "integer"},
		"snippet_chars": map[string]any{"type": "integer", "description": "Search snippet size, default 1200, bounded 64-8000"},
		"max_chars":     map[string]any{"type": "integer", "description": "Result character budget, bounded 64-32000"},
		"line_chars":    map[string]any{"type": "integer", "description": "Frequent excerpt line budget"},
		"recursive":     map[string]any{"type": "boolean", "description": "List subdirectories recursively; default false"},
		"regex":         map[string]any{"type": "boolean", "description": "Grep query is a regular expression; default false (literal)"},
		"include_image": map[string]any{"type": "boolean", "default": false, "description": "Search only: explicitly attach a relevant PDF crop; view always renders"},
		"entry_id":      map[string]any{"type": "string", "description": "Select one frequent paragraph by stable entry id"},
	}
	return map[string]any{"type": "function", "function": map[string]any{"name": "luadraw_doc", "description": "Navigate/search/read LuaDraw source and manuals under the fixed root. PDF search/read locations are page-qualified; search is text-only by default and view explicitly renders a PDF page/crop.", "parameters": map[string]any{"type": "object", "properties": props, "required": []string{}}}}
}
func (d *Docs) sourceFiles() []string {
	var out []string
	for _, spec := range []struct {
		dir string
		ext []string
	}{{"doc", []string{".tex"}}, {"examples", []string{".tex", ".lua"}}, {"files", []string{".lua"}}} {
		base := filepath.Join(d.root, spec.dir)
		_ = filepath.WalkDir(base, func(p string, entry os.DirEntry, e error) error {
			if e != nil || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if !entry.IsDir() {
				for _, x := range spec.ext {
					if strings.EqualFold(filepath.Ext(p), x) {
						out = append(out, p)
						break
					}
				}
			}
			return nil
		})
	}
	doc := filepath.Join(d.root, "doc")
	entries, _ := os.ReadDir(doc)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 && !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			out = append(out, filepath.Join(doc, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}
func (d *Docs) fingerprint() []fileMeta {
	fs := d.sourceFiles()
	m := make([]fileMeta, 0, len(fs))
	for _, p := range fs {
		s, e := os.Stat(p)
		if e == nil {
			rel, _ := filepath.Rel(d.root, p)
			m = append(m, fileMeta{filepath.ToSlash(rel), s.Size(), s.ModTime().UnixNano()})
		}
	}
	return m
}
func versionDocs(root string) string {
	for _, p := range []string{"doc/src/luadraw-doc-en.tex", "doc/src/luadraw-doc-fr.tex"} {
		b, e := os.ReadFile(filepath.Join(root, p))
		if e == nil {
			m := regexp.MustCompile(`\\def\\version\{([^}]+)\}`).FindSubmatch(b)
			if len(m) > 1 {
				return string(m[1])
			}
		}
	}
	return drawVersion(root)
}
func (d *Docs) makeIndex(ctx context.Context, fp []fileMeta) (*docIndex, error) {
	h := sha256.New()
	_, _ = io.WriteString(h, docIndexAlgorithm+"\x00")
	idx := &docIndex{Schema: docSchema, Root: d.root, Version: versionDocs(d.root), Created: time.Now().Unix(), Fingerprint: fp}
	for _, m := range fp {
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", m.Path, m.Size, m.Mtime)
		p := filepath.Join(d.root, filepath.FromSlash(m.Path))
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		_, _ = h.Write(b)
		entry := docEntry{Path: m.Path}
		if strings.EqualFold(filepath.Ext(p), ".pdf") {
			entry.Kind = "pdf"
			pages, e := extractPDF(ctx, p)
			if e != nil {
				return nil, fmt.Errorf("index PDF %s: %w", m.Path, e)
			}
			entry.Pages = pages
		} else {
			entry.Kind = "source"
			entry.Text = strings.ToValidUTF8(string(b), "�")
		}
		idx.Docs = append(idx.Docs, entry)
	}
	idx.Key = hex.EncodeToString(h.Sum(nil))
	return idx, nil
}
func (d *Docs) get(ctx context.Context) (*docIndex, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fp := d.fingerprint()
	if d.index != nil && sameMeta(d.index.Fingerprint, fp) {
		return d.index, nil
	}
	if b, e := os.ReadFile(d.path); e == nil {
		var c docIndex
		if json.Unmarshal(b, &c) == nil && c.Schema == docSchema && c.Root == d.root && sameMeta(c.Fingerprint, fp) {
			d.index = &c
			return &c, nil
		}
	}
	idx, e := d.makeIndex(ctx, fp)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(idx)
	if e != nil {
		return nil, e
	}
	tmp, e := os.CreateTemp(d.cache, "luadraw-index-*.tmp")
	if e == nil {
		_, we := tmp.Write(b)
		ce := tmp.Close()
		if we == nil && ce == nil {
			e = os.Rename(tmp.Name(), d.path)
		} else {
			e = we
			if e == nil {
				e = ce
			}
		}
		if e != nil {
			_ = os.Remove(tmp.Name())
		}
	}
	if e != nil {
		return nil, e
	}
	d.index = idx
	return idx, nil
}
func sameMeta(a, b []fileMeta) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Poppler bbox XML uses word coordinates. Group adjacent words into visual lines and preserve page dimensions.
type bboxXML struct {
	Pages []bboxPage `xml:"page"`
}
type bboxPage struct {
	Width  float64    `xml:"width,attr"`
	Height float64    `xml:"height,attr"`
	Words  []bboxWord `xml:"word"`
}
type bboxWord struct {
	X1   float64 `xml:"xMin,attr"`
	Y1   float64 `xml:"yMin,attr"`
	X2   float64 `xml:"xMax,attr"`
	Y2   float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

func extractPDF(ctx context.Context, p string) ([]page, error) {
	cmd := exec.CommandContext(ctx, lookup("pdftotext"), "-bbox", p, "-")
	b, e := cmd.Output()
	if e != nil {
		if x, ok := e.(*exec.ExitError); ok {
			return nil, fmt.Errorf("pdftotext: %s: %w", tail(string(x.Stderr), 3000), e)
		}
		return nil, e
	}
	decoder := xml.NewDecoder(strings.NewReader(string(b)))
	var xmlPages []bboxPage
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "page" {
			var pg bboxPage
			if err := decoder.DecodeElement(&pg, &start); err != nil {
				return nil, err
			}
			xmlPages = append(xmlPages, pg)
		}
	}
	out := make([]page, 0, len(xmlPages))
	for pi, pg := range xmlPages {
		lines := [][]bboxWord{}
		centers := []float64{}
		for _, w := range pg.Words {
			y := (w.Y1 + w.Y2) / 2
			at := -1
			for i, c := range centers {
				if absFloat(c-y) <= 3 {
					at = i
					break
				}
			}
			if at < 0 {
				lines = append(lines, []bboxWord{w})
				centers = append(centers, y)
			} else {
				lines[at] = append(lines[at], w)
				centers[at] = (centers[at] + y) / 2
			}
		}
		order := make([]int, len(lines))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			if centers[order[i]] != centers[order[j]] {
				return centers[order[i]] < centers[order[j]]
			}
			leftI, leftJ := lines[order[i]][0].X1, lines[order[j]][0].X1
			return leftI < leftJ
		})
		blocks := []block{}
		for _, oi := range order {
			line := lines[oi]
			sort.SliceStable(line, func(i, j int) bool { return line[i].X1 < line[j].X1 })
			var text strings.Builder
			bb := bbox{line[0].X1, line[0].Y1, line[0].X2, line[0].Y2}
			for i, w := range line {
				if i > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(strings.TrimSpace(w.Text))
				if w.X1 < bb[0] {
					bb[0] = w.X1
				}
				if w.Y1 < bb[1] {
					bb[1] = w.Y1
				}
				if w.X2 > bb[2] {
					bb[2] = w.X2
				}
				if w.Y2 > bb[3] {
					bb[3] = w.Y2
				}
			}
			if strings.TrimSpace(text.String()) != "" {
				blocks = append(blocks, block{text.String(), bb})
			}
		}
		out = append(out, page{pi + 1, pg.Width, pg.Height, pdfParagraphs(blocks, pg.Width)})
	}
	return out, nil
}
func pdfParagraphs(lines []block, pageWidth float64) []block {
	if len(lines) == 0 {
		return nil
	}
	if pageWidth <= 0 {
		for _, line := range lines {
			if line.BBox[2] > pageWidth {
				pageWidth = line.BBox[2]
			}
		}
	}
	starts := make([]float64, len(lines))
	for i := range lines {
		starts[i] = lines[i].BBox[0]
	}
	sort.Float64s(starts)
	unique := starts[:0]
	for _, x := range starts {
		if len(unique) == 0 || x != unique[len(unique)-1] {
			unique = append(unique, x)
		}
	}
	var cuts []float64
	threshold := pageWidth * .12
	for i := 1; i < len(unique); i++ {
		if unique[i]-unique[i-1] > threshold {
			cuts = append(cuts, (unique[i]+unique[i-1])/2)
		}
	}
	column := func(x float64) int {
		col := 0
		for _, cut := range cuts {
			if x > cut {
				col++
			}
		}
		return col
	}
	ordered := append([]block(nil), lines...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ci, cj := column(ordered[i].BBox[0]), column(ordered[j].BBox[0])
		if ci != cj {
			return ci < cj
		}
		if ordered[i].BBox[1] != ordered[j].BBox[1] {
			return ordered[i].BBox[1] < ordered[j].BBox[1]
		}
		return ordered[i].BBox[0] < ordered[j].BBox[0]
	})
	out := make([]block, 0, len(ordered))
	current := ordered[0]
	lastLineHeight := maxF(1, current.BBox[3]-current.BBox[1])
	lineCount := 1
	for _, next := range ordered[1:] {
		gap := next.BBox[1] - current.BBox[3]
		if gap < 0 {
			gap = 0
		}
		nextLineHeight := maxF(1, next.BBox[3]-next.BBox[1])
		minWidth := minF(current.BBox[2]-current.BBox[0], next.BBox[2]-next.BBox[0])
		horizontal := overlap1(current.BBox[0], current.BBox[2], next.BBox[0], next.BBox[2])
		joins := gap <= maxF(1.6*maxF(lastLineHeight, nextLineHeight), 3) && horizontal >= maxF(1, minWidth*.18) && lineCount < 12 && len([]rune(current.Text))+len([]rune(next.Text))+1 <= 2048 && !pdfHeadingBoundary(next.Text)
		if !joins {
			out = append(out, current)
			current = next
			lastLineHeight = nextLineHeight
			lineCount = 1
			continue
		}
		current.Text += "\n" + next.Text
		if next.BBox[0] < current.BBox[0] {
			current.BBox[0] = next.BBox[0]
		}
		if next.BBox[1] < current.BBox[1] {
			current.BBox[1] = next.BBox[1]
		}
		if next.BBox[2] > current.BBox[2] {
			current.BBox[2] = next.BBox[2]
		}
		if next.BBox[3] > current.BBox[3] {
			current.BBox[3] = next.BBox[3]
		}
		lastLineHeight = nextLineHeight
		lineCount++
	}
	out = append(out, current)
	return out
}

var pdfNumberedHeading = regexp.MustCompile(`^\d+(?:\.\d+)*[.)]?\s+\S`)

func pdfHeadingBoundary(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	if pdfNumberedHeading.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	for _, marker := range []string{"new section", "section ", "chapter ", "part "} {
		if strings.HasPrefix(lower, marker) {
			return true
		}
	}
	return false
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func num(v any, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return def
}
func clamp(v, def float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	if v == 0 && def != 0 {
		return def
	}
	return v
}
func splitPDFPageRef(name string) (string, int, error) {
	marker := "#page="
	at := strings.LastIndex(name, marker)
	if at < 0 {
		return name, 0, nil
	}
	pageNumber, err := strconv.Atoi(name[at+len(marker):])
	if err != nil || pageNumber < 1 {
		return "", 0, fmt.Errorf("invalid PDF page selector in document path %q", name)
	}
	return name[:at], pageNumber, nil
}

func (d *Docs) safeDoc(idx *docIndex, name, kind string) (*docEntry, error) {
	base, _, err := splitPDFPageRef(name)
	if err != nil {
		return nil, err
	}
	_, rel, err := d.rootPath(base)
	if err != nil {
		return nil, err
	}
	for i := range idx.Docs {
		if idx.Docs[i].Path == rel && (kind == "" || idx.Docs[i].Kind == kind) {
			return &idx.Docs[i], nil
		}
	}
	return nil, fmt.Errorf("document not indexed under root: %s", name)
}
func (d *Docs) render(ctx context.Context, pdf string, pg page, args map[string]any, out string, bb *bbox) (string, error) {
	left, top, right, bottom := clamp(num(args["left"], 0), 0), clamp(num(args["top"], 0), 0), clamp(num(args["right"], 100), 100), clamp(num(args["bottom"], 100), 100)
	hasCrop := false
	for _, k := range []string{"left", "top", "right", "bottom"} {
		if _, ok := args[k]; ok {
			hasCrop = true
		}
	}
	if bb != nil && !hasCrop {
		w := pg.Width
		h := pg.Height
		cw := minF(w, maxF(bb[2]-bb[0]+w*.08, w*.82))
		ch := minF(h, maxF(bb[3]-bb[1]+h*.04, h*.42))
		cx := (bb[0] + bb[2]) / 2
		cy := (bb[1] + bb[3]) / 2
		left = maxF(0, (cx-cw/2)/w*100)
		right = minF(100, (cx+cw/2)/w*100)
		top = maxF(0, (cy-ch/2)/h*100)
		bottom = minF(100, (cy+ch/2)/h*100)
	}
	if right <= left || bottom <= top {
		return "", fmt.Errorf("crop bounds must have positive area")
	}
	zoom := int(num(args["zoom"], 1200))
	if zoom == 0 {
		zoom = 1200
	}
	if zoom < 200 {
		zoom = 200
	}
	if zoom > 5000 {
		zoom = 5000
	}
	_ = os.MkdirAll(out, 0700)
	whole := int(float64(zoom)/((right-left)/100) + 2)
	if whole < 400 {
		whole = 400
	}
	if whole > 12000 {
		whole = 12000
	}
	base, e := os.CreateTemp(out, "luadraw-page-*.png")
	if e != nil {
		return "", e
	}
	prefix := strings.TrimSuffix(base.Name(), filepath.Ext(base.Name()))
	base.Close()
	_ = os.Remove(base.Name())
	cmd := exec.CommandContext(ctx, lookup("pdftoppm"), "-f", strconv.Itoa(pg.Number), "-l", strconv.Itoa(pg.Number), "-scale-to-x", strconv.Itoa(whole), "-scale-to-y", "-1", "-png", pdf, prefix)
	if b, e := cmd.CombinedOutput(); e != nil {
		return "", fmt.Errorf("pdftoppm: %s: %w", tail(string(b), 3000), e)
	}
	rendered := prefix + "-" + strconv.Itoa(pg.Number) + ".png"
	if !exists(rendered) {
		matches, _ := filepath.Glob(prefix + "-*.png")
		if len(matches) > 0 {
			rendered = matches[0]
		}
	}
	defer os.Remove(rendered)
	f, e := os.Open(rendered)
	if e != nil {
		return "", e
	}
	im, e := png.Decode(f)
	f.Close()
	if e != nil {
		return "", e
	}
	bounds := im.Bounds()
	x1 := int(float64(bounds.Dx()) * left / 100)
	x2 := int(float64(bounds.Dx()) * right / 100)
	y1 := int(float64(bounds.Dy()) * top / 100)
	y2 := int(float64(bounds.Dy()) * bottom / 100)
	rect := image.Rect(x1, y1, x2, y2).Intersect(bounds)
	if rect.Empty() {
		return "", fmt.Errorf("crop is empty")
	}
	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), im, rect.Min, draw.Src)
	height := maxInt(1, int(float64(cropped.Bounds().Dy())*float64(zoom)/float64(cropped.Bounds().Dx())))
	dst := image.NewRGBA(image.Rect(0, 0, zoom, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Over, nil)
	target := filepath.Join(out, fmt.Sprintf("luadraw-p%d-%d.png", pg.Number, time.Now().UnixNano()))
	f, e = os.Create(target)
	if e != nil {
		return "", e
	}
	e = png.Encode(f, dst)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	if ce != nil {
		return "", ce
	}
	return target, nil
}
func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func terms(s string) []string {
	re := regexp.MustCompile(`[^\W\d_]+|[\w]+`)
	m := re.FindAllString(strings.ToLower(s), -1)
	var r []string
	for _, x := range m {
		if len([]rune(x)) > 1 {
			r = append(r, x)
		}
	}
	if len(r) == 0 && s != "" {
		r = []string{strings.ToLower(s)}
	}
	return r
}

type hitSegment struct {
	doc   string
	page  int
	start int
	end   int
	text  string
	kind  string
}

type hit struct {
	score           int
	doc             string
	page, line, end int
	kind, text      string
	box             *bbox
	info            page
	segments        []hitSegment
}

func searchSnippet(s string, max int) string {
	if max <= 0 {
		max = 1200
	}
	if max < 64 {
		max = 64
	}
	if max > 8000 {
		max = 8000
	}
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len([]rune(s)) <= max {
		return s
	}
	cut := []rune(s)[:max]
	end := max
	for i := max - 1; i >= max/2; i-- {
		if cut[i] == ' ' || cut[i] == '.' || cut[i] == ';' || cut[i] == ',' {
			end = i
			break
		}
	}
	if end < 64 {
		end = max
	}
	return strings.TrimSpace(string(cut[:end])) + "…"
}
func snippetForHit(_ *docIndex, h hit, chars int) (string, int) {
	return searchSnippet(h.text, chars), h.end
}

func normalizeHitText(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func overlap1(a1, a2, b1, b2 float64) float64 {
	x := minF(a2, b2) - maxF(a1, b1)
	if x < 0 {
		return 0
	}
	return x
}
func mergePDFHits(hits []hit) []hit {
	used := make([]bool, len(hits))
	out := make([]hit, 0, len(hits))
	for i, h := range hits {
		if used[i] || h.kind != "pdf" || h.box == nil {
			if !used[i] {
				out = append(out, h)
				used[i] = true
			}
			continue
		}
		used[i] = true
		merged := h
		if len(merged.segments) == 0 {
			merged.segments = []hitSegment{{doc: h.doc, page: h.page, start: h.line, end: maxInt(h.line, h.end), text: h.text, kind: h.kind}}
		}
		box := *h.box
		texts := []string{h.text}
		primary := h.text
		primaryScore := h.score
		changed := true
		for changed {
			changed = false
			for j, c := range hits {
				if used[j] || c.kind != "pdf" || c.doc != h.doc || c.page != h.page || c.box == nil {
					continue
				}
				gap := 0.0
				if c.box[1] > box[3] {
					gap = c.box[1] - box[3]
				} else if box[1] > c.box[3] {
					gap = box[1] - c.box[3]
				}
				lineH := maxF(box[3]-box[1], c.box[3]-c.box[1])
				horiz := overlap1(box[0], box[2], c.box[0], c.box[2])
				minW := minF(box[2]-box[0], c.box[2]-c.box[0])
				if gap <= maxF(2*lineH, 12) && horiz >= maxF(1, minW*.15) {
					used[j] = true
					if c.box[0] < box[0] {
						box[0] = c.box[0]
					}
					if c.box[1] < box[1] {
						box[1] = c.box[1]
					}
					if c.box[2] > box[2] {
						box[2] = c.box[2]
					}
					if c.box[3] > box[3] {
						box[3] = c.box[3]
					}
					if len(c.segments) == 0 {
						c.segments = []hitSegment{{doc: c.doc, page: c.page, start: c.line, end: maxInt(c.line, c.end), text: c.text, kind: c.kind}}
					}
					merged.segments = append(merged.segments, c.segments...)
					if merged.line == 0 || c.line > 0 && c.line < merged.line {
						merged.line = c.line
					}
					if c.end > merged.end {
						merged.end = c.end
					}
					texts = append(texts, c.text)
					if c.score > primaryScore {
						primary = c.text
						primaryScore = c.score
					}
					if c.score > merged.score {
						merged.score = c.score
					}
					changed = true
				}
			}
		}
		merged.box = &box
		if len(texts) > 0 && texts[0] != primary {
			rest := make([]string, 0, len(texts))
			rest = append(rest, primary)
			for _, t := range texts {
				if t != primary {
					rest = append(rest, t)
				}
			}
			texts = rest
		}
		merged.text = strings.Join(texts, " ")
		out = append(out, merged)
	}
	return out
}
func dedupHits(hits []hit) []hit {
	seen := map[string]bool{}
	out := make([]hit, 0, len(hits))
	for _, h := range hits {
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d", h.doc, h.kind, h.page, h.line, maxInt(h.line, h.end))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out
}
func (d *Docs) search(idx *docIndex, q string, limit, offset int) []hit {

	ts := terms(q)
	exact := strings.ToLower(strings.TrimSpace(q))
	var hits []hit
	for _, doc := range idx.Docs {
		if doc.Kind == "source" {
			for _, paragraph := range sourceParagraphs(doc.Text) {
				flat := strings.Join(strings.Fields(paragraph.Text), " ")
				low := strings.ToLower(flat)
				score := 0
				for _, term := range ts {
					if strings.Contains(low, term) {
						score++
					}
				}
				if exact != "" && strings.Contains(low, exact) {
					score += 1000
				}
				if score > 0 {
					hits = append(hits, hit{score: score, doc: doc.Path, kind: "source", line: paragraph.Start, end: paragraph.End, text: paragraph.Text, segments: []hitSegment{{doc: doc.Path, start: paragraph.Start, end: paragraph.End, text: paragraph.Text, kind: "source"}}})
				}
			}
		} else {
			for _, pg := range doc.Pages {
				for bi, b := range pg.Blocks {
					score := 0
					low := strings.ToLower(b.Text)
					for _, t := range ts {
						score += strings.Count(low, t)
					}
					if exact != "" && strings.Contains(low, exact) {
						score = 10000
						if strings.Contains(low, exact+"(") && !strings.Contains(low, exact+"()") {
							score += 1000
						}
					}
					if score > 0 {
						rank := score
						if pg.Number > 10 {
							rank += 2
						} else {
							rank -= 3
						}
						if strings.Contains(b.Text, "\n") {
							rank += 2
						}
						bb := b.BBox
						blockNum := bi + 1
						segment := hitSegment{doc: doc.Path, page: pg.Number, start: blockNum, end: blockNum, text: b.Text, kind: "pdf"}
						hits = append(hits, hit{score: rank, doc: doc.Path, kind: "pdf", page: pg.Number, line: blockNum, end: blockNum, text: b.Text, box: &bb, info: pg, segments: []hitSegment{segment}})
					}
				}
			}
		}
	}
	hits = mergePDFHits(hits)
	hits = dedupHits(hits)
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.doc != b.doc {
			return a.doc < b.doc
		}
		if a.page != b.page {
			return a.page < b.page
		}
		return a.line < b.line
	})
	if offset >= len(hits) {
		return nil
	}
	end := offset + limit
	if end > len(hits) {
		end = len(hits)
	}
	return hits[offset:end]
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func intArg(m map[string]any, k string, d int) int { return int(num(m[k], float64(d))) }

func boolArg(m map[string]any, k string, d bool) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return d
}

func readPDFText(d *Docs, idx *docIndex, args map[string]any) (string, error) {
	name := strOpt(args, "doc", strOpt(args, "query", ""))
	base, pageFromRef, err := splitPDFPageRef(name)
	if err != nil {
		return "", err
	}
	doc, err := d.safeDoc(idx, base, "pdf")
	if err != nil {
		return "", err
	}
	pageNumber := pageFromRef
	if requested, ok := args["page"]; ok {
		argPage := int(num(requested, 1))
		if pageFromRef > 0 && argPage != pageFromRef {
			return "", fmt.Errorf("doc page selector %d conflicts with page argument %d", pageFromRef, argPage)
		}
		pageNumber = argPage
	}
	if pageNumber == 0 {
		pageNumber = 1
	}
	if pageNumber < 1 || pageNumber > len(doc.Pages) {
		return "", fmt.Errorf("page out of range: %d; document has %d pages", pageNumber, len(doc.Pages))
	}
	pg := doc.Pages[pageNumber-1]
	start := intArg(args, "start_line", 1)
	if start < 1 {
		start = 1
	}
	end := intArg(args, "end_line", minInt(len(pg.Blocks), start+399))
	if end < start {
		end = start
	}
	if end > start+399 {
		end = start + 399
	}
	if end > len(pg.Blocks) {
		end = len(pg.Blocks)
	}
	if start > len(pg.Blocks) {
		return fmt.Sprintf("%s#page=%d blocks %d-%d/%d", doc.Path, pageNumber, start, start, len(pg.Blocks)), nil
	}
	var b strings.Builder
	pageDoc := fmt.Sprintf("%s#page=%d", doc.Path, pageNumber)
	for i := start; i <= end; i++ {
		blockText := pg.Blocks[i-1].Text
		fmt.Fprintf(&b, "%d: %s\n", i, blockText)
		d.recordFrequency(idx.Key, pageDoc, i, i, blockText, "read", 1)
	}
	text := strings.TrimSuffix(b.String(), "\n")
	return fmt.Sprintf("%s#page=%d blocks %d-%d/%d\n%s", doc.Path, pageNumber, start, end, len(pg.Blocks), text), nil
}

func readSourceText(d *Docs, idx *docIndex, args map[string]any) (string, error) {
	name := strOpt(args, "doc", strOpt(args, "query", ""))
	_, rel, data, err := d.sourcePath(name)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start := intArg(args, "start_line", 1)
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return fmt.Sprintf("%s lines %d-%d/%d", rel, start, start, len(lines)), nil
	}
	end := intArg(args, "end_line", minInt(len(lines), start+399))
	if end < start {
		end = start
	}
	if end > start+399 {
		end = start + 399
	}
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for n := start; n <= end; n++ {
		fmt.Fprintf(&b, "%d: %s\n", n, lines[n-1])
	}
	for _, paragraph := range sourceParagraphs(string(data)) {
		if paragraph.End >= start && paragraph.Start <= end {
			d.recordFrequency(idx.Key, rel, paragraph.Start, paragraph.End, paragraph.Text, "read", 1)
		}
	}
	return fmt.Sprintf("%s lines %d-%d/%d\n%s", rel, start, end, len(lines), strings.TrimSuffix(b.String(), "\n")), nil
}

func fitRunes(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= budget {
		return string(r)
	}
	if budget == 1 {
		return "…"
	}
	return string(r[:budget-1]) + "…"
}

func displayDoc(h hit) string {
	if h.kind == "pdf" {
		return fmt.Sprintf("%s#page=%d", h.doc, h.page)
	}
	return h.doc
}

func recordToolExcerpt(d *Docs, idx *docIndex, doc, source string, start, end int, text string) {
	text = strings.TrimSpace(text)
	if text == "" || text == "No matches." || strings.HasPrefix(text, "No matches before") {
		return
	}
	d.recordFrequency(idx.Key, doc, start, end, fitRunes(text, 2048), source, 1)
}

func (d *Docs) Execute(ctx context.Context, args map[string]any, outdir string) (string, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	idx, e := d.get(ctx)
	if e != nil {
		return "", "", e
	}
	mode := strOpt(args, "mode", "search")
	maxMtime := int64(0)
	for _, m := range idx.Fingerprint {
		if m.Mtime > maxMtime {
			maxMtime = m.Mtime
		}
	}
	meta := fmt.Sprintf("LuaDraw %s | index_sha256 %s | schema %d | source_mtime_ns %d | indexed %s", idx.Version, idx.Key, idx.Schema, maxMtime, time.Unix(idx.Created, 0).Format("2006-01-02 15:04:05"))
	switch mode {
	case "list":
		scope := strOpt(args, "doc", ".")
		files, e := d.listFiles(scope, boolArg(args, "recursive", false))
		if e != nil {
			return "", "", e
		}
		result := navigationRows(files, intArg(args, "limit", 100), intArg(args, "offset", 0), intArg(args, "max_chars", 6000))
		recordToolExcerpt(d, idx, "@navigation/list:"+scope, "list", intArg(args, "offset", 0), intArg(args, "offset", 0), result)
		return meta + "\n" + result, "", nil
	case "find":
		scope := strOpt(args, "doc", ".")
		files, e := d.findFiles(scope, strings.TrimSpace(fmt.Sprint(args["query"])))
		if e != nil {
			return "", "", e
		}
		result := navigationRows(files, intArg(args, "limit", 100), intArg(args, "offset", 0), intArg(args, "max_chars", 6000))
		recordToolExcerpt(d, idx, "@navigation/find:"+scope, "find", intArg(args, "offset", 0), intArg(args, "offset", 0), result)
		return meta + "\n" + result, "", nil
	case "grep":
		scope := strOpt(args, "doc", ".")
		text, e := d.grepFiles(scope, fmt.Sprint(args["query"]), boolArg(args, "regex", false), intArg(args, "limit", 50), intArg(args, "offset", 0), intArg(args, "max_chars", 6000))
		if e != nil {
			return "", "", e
		}
		recordToolExcerpt(d, idx, "@navigation/grep:"+scope, "grep", intArg(args, "offset", 0), intArg(args, "offset", 0), text)
		return meta + "\n" + text, "", nil
	case "frequent":
		text, e := d.frequent(ctx, idx, args)
		if e != nil {
			return "", "", e
		}
		return meta + "\n" + text, "", nil

	case "search":
		q := strings.TrimSpace(fmt.Sprint(args["query"]))
		if q == "" || q == "<nil>" {
			return "", "", fmt.Errorf("search requires query")
		}
		limit := intArg(args, "limit", 5)
		if limit < 1 {
			limit = 1
		}
		if limit > 20 {
			limit = 20
		}
		off := intArg(args, "offset", 0)
		if off < 0 {
			off = 0
		}
		hits := d.search(idx, q, limit, off)
		var rows []string
		imagePath := ""
		snippetChars := intArg(args, "snippet_chars", 1200)
		if snippetChars < 64 {
			snippetChars = 64
		}
		if snippetChars > 8000 {
			snippetChars = 8000
		}
		maxChars := intArg(args, "max_chars", 6000)
		if maxChars < 64 {
			maxChars = 64
		}
		if maxChars > 32000 {
			maxChars = 32000
		}
		usedChars := 0
		for i, h := range hits {
			snippet, endLine := snippetForHit(idx, h, snippetChars)
			suffix := fmt.Sprintf(" lines %d-%d", h.line, endLine)
			if h.kind == "pdf" {
				pct := [4]float64{h.box[0] / h.info.Width * 100, h.box[1] / h.info.Height * 100, h.box[2] / h.info.Width * 100, h.box[3] / h.info.Height * 100}
				suffix = fmt.Sprintf(" blocks %d-%d p.%d page_size_pts=%.1fx%.1f bbox_pts=%.1f,%.1f,%.1f,%.1f bbox_pct=%.2f,%.2f,%.2f,%.2f", h.line, maxInt(h.line, h.end), h.page, h.info.Width, h.info.Height, h.box[0], h.box[1], h.box[2], h.box[3], pct[0], pct[1], pct[2], pct[3])
			}
			header := fmt.Sprintf("%d. %s%s\n   ", off+i+1, displayDoc(h), suffix)
			budget := maxChars - usedChars - len([]rune(header))
			if len(rows) > 0 {
				budget--
			}
			if budget <= 0 {
				break
			}
			snippet = fitRunes(snippet, budget)
			if snippet == "" {
				break
			}
			row := header + snippet
			rowChars := len([]rune(row))
			if len(rows) > 0 {
				rowChars++
			}
			if usedChars+rowChars > maxChars {
				break
			}
			rows = append(rows, row)
			usedChars += rowChars
			if len(h.segments) == 0 {
				h.segments = []hitSegment{{doc: h.doc, page: h.page, start: h.line, end: maxInt(h.line, h.end), text: h.text, kind: h.kind}}
			}
			for _, segment := range h.segments {
				frequencyDoc := segment.doc
				if segment.kind == "pdf" {
					frequencyDoc = fmt.Sprintf("%s#page=%d", segment.doc, segment.page)
				}
				d.recordFrequency(idx.Key, frequencyDoc, segment.start, maxInt(segment.start, segment.end), segment.text, "search", 1)
			}
			include, ok := args["include_image"].(bool)
			if !ok {
				include = false
			}
			if imagePath == "" && h.kind == "pdf" && include {
				imagePath, e = d.render(ctx, filepath.Join(d.root, filepath.FromSlash(h.doc)), h.info, args, outdir, h.box)
				if e != nil {
					return "", "", e
				}
			}
		}
		body := "No matches."
		if len(rows) > 0 {
			body = strings.Join(rows, "\n")
		}
		return meta + "\n" + body, imagePath, nil
	case "read":
		name := strOpt(args, "doc", strOpt(args, "query", ""))
		base, selectedPage, splitErr := splitPDFPageRef(name)
		if splitErr != nil {
			return "", "", splitErr
		}
		if selectedPage > 0 {
			if _, ok := args["page"]; !ok {
				args["page"] = selectedPage
			}
		}
		var text string
		if strings.EqualFold(filepath.Ext(base), ".pdf") {
			text, e = readPDFText(d, idx, args)
		} else {
			text, e = readSourceText(d, idx, args)
		}
		if e != nil {
			return "", "", e
		}
		return meta + "\n" + text, "", nil
	case "view":
		name := strOpt(args, "doc", "")
		_, selectedPage, splitErr := splitPDFPageRef(name)
		if splitErr != nil {
			return "", "", splitErr
		}
		doc, e := d.safeDoc(idx, name, "pdf")
		if e != nil {
			return "", "", e
		}
		n := intArg(args, "page", selectedPage)
		if n == 0 {
			n = 1
		}
		if n < 1 || n > len(doc.Pages) {
			return "", "", fmt.Errorf("page out of range: %d; document has %d pages", n, len(doc.Pages))
		}
		im, e := d.render(ctx, filepath.Join(d.root, filepath.FromSlash(doc.Path)), doc.Pages[n-1], args, outdir, nil)
		if e != nil {
			return "", "", e
		}
		pg := doc.Pages[n-1]
		left, top, right, bottom := clamp(num(args["left"], 0), 0), clamp(num(args["top"], 0), 0), clamp(num(args["right"], 100), 100), clamp(num(args["bottom"], 100), 100)
		for i, paragraph := range pg.Blocks {
			box := paragraph.BBox
			if box[2] < left*pg.Width/100 || box[0] > right*pg.Width/100 || box[3] < top*pg.Height/100 || box[1] > bottom*pg.Height/100 {
				continue
			}
			pageDoc := fmt.Sprintf("%s#page=%d", doc.Path, n)
			recordToolExcerpt(d, idx, pageDoc, "view", i+1, i+1, paragraph.Text)
		}
		return fmt.Sprintf("%s\nPDF %s page %d/%d attached.", meta, doc.Path, n, len(doc.Pages)), im, nil
	default:
		return "", "", fmt.Errorf("mode must be search, read, or view")
	}
}

var _ = strconv.IntSize
