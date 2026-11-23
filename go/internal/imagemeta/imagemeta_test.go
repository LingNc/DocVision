package imagemeta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexLiteralDirectoriesMultipleRunsAndBitmapTypes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "[edition] sample_part1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a_content_list.json": `[{"type":"text","text":"Earlier parse has no images"}]`,
		"b_content_list.json": `[{"type":"image","img_path":"images/a.jpg","bbox":[10,20,60,50],"page_idx":0},{"type":"chart","img_path":"images/b.jpg","bbox":[30,40,100,80],"page_idx":0},{"type":"table","img_path":"images/c.jpg","bbox":[50,60,120,100],"page_idx":0}]`,
		"layout.json":         `{"pdf_info":[{"page_idx":0,"page_size":[600,800]}]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	SetMineruRoot(root)
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		r, ok := LookupImage(name)
		if !ok || r.PageW != 600 || r.PageH != 800 || r.Page != 1 {
			t.Fatalf("%s: %+v found=%v", name, r, ok)
		}
	}
}
