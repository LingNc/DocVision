package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T294：图片路径里带空格（项目名/分片目录带空格时就是 `images/<项目名> /x.jpg`）
// 以前被正则按空白截断，整批图片一张也没铺进试转工作区，会话只能放占位框。
func TestStagePilotImagesWithSpacesInPath(t *testing.T) {
	proj := t.TempDir()
	work := t.TempDir()
	rel := "images/书 名 /a b.jpg"
	src := filepath.Join(proj, "source", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, src, "jpeg-bytes")

	sample := filepath.Join(proj, "sample.md")
	writeFile(t, sample, "# t\n\nLINK: [image]("+rel+")\n\nLINK: [image]("+rel+")\n")

	missing := stagePilotImages(proj, sample, work)
	if len(missing) != 0 {
		t.Fatalf("不该有缺失: %v", missing)
	}
	dst := filepath.Join(work, filepath.FromSlash(rel))
	if data, err := os.ReadFile(dst); err != nil || string(data) != "jpeg-bytes" {
		t.Fatalf("图片没铺进工作区: %v", err)
	}
}

// 取不到的图要如实报出来，不能又静默跳过。
func TestStagePilotImagesReportsMissing(t *testing.T) {
	proj := t.TempDir()
	work := t.TempDir()
	sample := filepath.Join(proj, "sample.md")
	writeFile(t, sample, "LINK: [image](images/书 名 /nope.jpg)\n")

	missing := stagePilotImages(proj, sample, work)
	if len(missing) != 1 || missing[0] != "images/书 名 /nope.jpg" {
		t.Fatalf("缺失清单不对: %v", missing)
	}
}

// 没有空格的老路径照旧复制（不能因为改正则而退化）。
func TestStagePilotImagesPlainPath(t *testing.T) {
	proj := t.TempDir()
	work := t.TempDir()
	rel := "images/book/a.jpg"
	src := filepath.Join(proj, "source", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, src, "x")
	sample := filepath.Join(proj, "sample.md")
	writeFile(t, sample, "LINK: [image]("+rel+")\n")
	if missing := stagePilotImages(proj, sample, work); len(missing) != 0 {
		t.Fatalf("普通路径不该缺失: %v", missing)
	}
	if !fileExists(filepath.Join(work, filepath.FromSlash(rel))) {
		t.Fatal("普通路径没复制")
	}
}
