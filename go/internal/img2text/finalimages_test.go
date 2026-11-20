package img2text

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
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
