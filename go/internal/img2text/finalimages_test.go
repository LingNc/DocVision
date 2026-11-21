package img2text

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/imagemeta"
)

// TestCopyDeliveredImages 钉 T334：交付 md 里 `[url](images/<书>/<文件>)` 指到
// 的原图要复制到 finally 下的同一相对路径；没有 url 行的直接嵌入不搬；解析
// 不到源的引用跳过而不是报错。
func TestCopyDeliveredImages(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.Paths.ImagesDir = filepath.Join(root, "output", "images")
	cfg.Paths.OutputDir = filepath.Join(root, "output")
	cfg.Paths.FinallyDir = filepath.Join(root, "finally")

	src := filepath.Join(cfg.Paths.ImagesDir, "某书", "abc123.jpg")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	content := "[P. 1]\n\n[url](images/某书/abc123.jpg)\n[Image]( mermaid · P.1 )\n" +
		"```mermaid\ngraph TD\n```\n\n[url](images/某书/缺失.jpg)\n[Image]( 图 · P.2 )\n" +
		"[url](../外面/不许.jpg)\n"

	n, err := copyDeliveredImages(cfg, content)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应只复制 1 张，got %d", n)
	}
	dst := filepath.Join(cfg.Paths.FinallyDir, "images", "某书", "abc123.jpg")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("原图没复制到 finally: %v", err)
	}
	if string(got) != "jpeg-bytes" {
		t.Fatalf("内容不对: %q", got)
	}
	// 再跑一次不应重复复制（同大小跳过）。
	if n, err = copyDeliveredImages(cfg, content); err != nil || n != 0 {
		t.Fatalf("重复调用应跳过，got n=%d err=%v", n, err)
	}
	// 越界路径不许写到 finally 之外。
	if _, err := os.Stat(filepath.Join(root, "外面", "不许.jpg")); err == nil {
		t.Fatal("`..` 引用必须被忽略")
	}
}

// TestDeliveredImageTargets 钉住解析：只认 url 行、按首次出现去重。
func TestDeliveredImageTargets(t *testing.T) {
	c := "[url](images/a/x.png)\n[url](<images/a/y.png>)\n[url](images/a/x.png)\n![](images/a/z.png)\n"
	got := deliveredImageTargets(c)
	if len(got) != 2 || got[0] != "images/a/x.png" || got[1] != "images/a/y.png" {
		t.Fatalf("targets = %v", got)
	}
}

// TestCopyDeliveredImagesWritesSizes 钉 T336：sizes.csv 只给"像素 → pt"的
// 换算（每册一个比例，写在注释行），宽高同除一个比例所以位图比例原样保留。
func TestCopyDeliveredImagesWritesSizes(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.Paths.ImagesDir = filepath.Join(root, "output", "images")
	cfg.Paths.OutputDir = filepath.Join(root, "output")
	cfg.Paths.FinallyDir = filepath.Join(root, "finally")
	cfg.Paths.MineruOutput = filepath.Join(root, "mineru")

	// 位图 100x50 px；bbox 宽 100pt → 比例 1.00 px/pt，换算后 100x50 pt。
	src := filepath.Join(cfg.Paths.ImagesDir, "某书", "abc.png")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	part := filepath.Join(cfg.Paths.MineruOutput, "某书_part1")
	if err := os.MkdirAll(part, 0o755); err != nil {
		t.Fatal(err)
	}
	cl := `[{"type":"image","img_path":"images/abc.png","page_idx":11,"bbox":[10,20,110,90]}]`
	lay := `{"pdf_info":[{"page_idx":11,"page_size":[595.28,841.89]}]}`
	if err := os.WriteFile(filepath.Join(part, "u_content_list.json"), []byte(cl), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "layout.json"), []byte(lay), 0o644); err != nil {
		t.Fatal(err)
	}
	imagemeta.SetMineruRoot(cfg.Paths.MineruOutput)

	if _, err := copyDeliveredImages(cfg, "[url](images/某书/abc.png)\n[url](images/某书/没有.png)\n"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.Paths.FinallyDir, "images", "某书")
	cs, err := os.ReadFile(filepath.Join(dir, "sizes.csv"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(cs)
	if !strings.Contains(got, "# scale_px_per_pt=1.0000") {
		t.Fatalf("缺比例注释：\n%s", got)
	}
	if !strings.Contains(got, "file,width_mm,height_mm,page_w_mm") || !strings.Contains(got, "abc.png,35.28,17.64,210.00") {
		t.Fatalf("映射不对（100×50px @1px/pt = 35.28×17.64mm，页宽 595.28pt = 210.00mm）：\n%s", got)
	}
	if strings.Contains(got, "没有.png") {
		t.Fatalf("没有原始量的图不该出现：\n%s", got)
	}
}
