package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 原图印刷尺寸测量：从位图回溯到 MinerU 解析目录（content_list.json +
// layout.json），算出 mm/有效 dpi；测不出时只给宽高比兜底。
func TestMeasureImagePrintedSize(t *testing.T) {
	part := t.TempDir()
	imgDir := filepath.Join(part, "images", "doc")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(imgDir, "fig.png")
	writePNG(t, img, 284, 156)
	// bbox 是 MinerU 版面坐标（2x pt）：493x720pt 的页上约 103.6x55.1mm 的图
	cl := `[{"page_idx":0,"bbox":[346,468,555,545],"img_path":"images/doc/fig.png"}]`
	if err := os.WriteFile(filepath.Join(part, "x_content_list.json"), []byte(cl), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := `{"pdf_info":[{"page_size":[493,720]}]}`
	if err := os.WriteFile(filepath.Join(part, "layout.json"), []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}

	m := MeasureImage(img)
	if !m.Found {
		t.Fatalf("未测到印刷尺寸")
	}
	// 209x77 版面单位 = 104.5x38.5pt = 36.9x13.6mm；页宽 493pt = 174mm → 占 21%
	if m.WidthMM < 35 || m.WidthMM > 39 {
		t.Errorf("宽 = %.1fmm, 期望 ~36.9mm", m.WidthMM)
	}
	// 高按位图比例（156/284）从宽度推得：36.9 * 0.549 = 20.3mm
	if m.HeightMM < 19 || m.HeightMM > 21.5 {
		t.Errorf("高 = %.1fmm, 期望 ~20.3mm（位图比例）", m.HeightMM)
	}
	if m.DPI < 180 || m.DPI > 210 {
		t.Errorf("有效 dpi = %d, 期望 ~195", m.DPI)
	}
	if m.PageWMM < 170 || m.PageWMM > 178 {
		t.Errorf("页宽 = %.1fmm, 期望 ~174mm", m.PageWMM)
	}
	s := m.String()
	for _, want := range []string{"ORIGINAL FIGURE SIZE", "mm", "dpi", "of the page width"} {
		if !strings.Contains(s, want) {
			t.Errorf("显示串缺少 %q: %s", want, s)
		}
	}

	// 找不到解析目录时退化到"只有宽高比"
	orphan := filepath.Join(t.TempDir(), "solo.png")
	writePNG(t, orphan, 100, 50)
	om := MeasureImage(orphan)
	if om.Found {
		t.Errorf("孤立图片不应测出印刷尺寸")
	}
	if !strings.Contains(om.AspectOnly(), "2.00:1") {
		t.Errorf("兜底宽高比不对: %s", om.AspectOnly())
	}
}

// 用户实测：生产里会话看到的是 images 阶段**拷进项目**的位图
// (<proj>/source/images/<主题>/<sha>.jpg)，向上走永远找不到 MinerU 解析，
// 于是每次 view_image 都只剩 "ORIGINAL BITMAP: 320x178px"——mm 全丢了。
// 现在按文件名到 paths.mineru_output 的索引里找。
func TestMeasureImageCopiedIntoProject(t *testing.T) {
	root := t.TempDir() // paths.mineru_output
	part := filepath.Join(root, "1000题_part1")
	if err := os.MkdirAll(filepath.Join(part, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(part, "images", "abc123.png"), 284, 156)
	cl := `[{"page_idx":0,"bbox":[346,468,555,545],"img_path":"images/abc123.png"}]`
	if err := os.WriteFile(filepath.Join(part, "x_content_list.json"), []byte(cl), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := `{"pdf_info":[{"page_size":[493,720]}]}`
	if err := os.WriteFile(filepath.Join(part, "layout.json"), []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}
	// 项目里的是拷贝（同名：MinerU 的图片名就是内容哈希，拷贝保留文件名）
	proj := t.TempDir()
	copied := filepath.Join(proj, "source", "images", "测试书", "abc123.png")
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, copied, 284, 156)

	if m := MeasureImage(copied); m.Found {
		t.Fatalf("未告知解析根时不该测出 mm（避免测试假阳性）")
	}
	SetImageParseRoot(root)
	defer SetImageParseRoot("")

	m := MeasureImage(copied)
	if !m.Found {
		t.Fatalf("拷贝进项目的位图应能按文件名找到解析并算出 mm")
	}
	if m.WidthMM < 35 || m.WidthMM > 39 || m.PageWMM < 170 || m.PageWMM > 178 {
		t.Errorf("宽 %.1fmm / 页宽 %.1fmm，期望 ~36.9 / ~174", m.WidthMM, m.PageWMM)
	}
	if s := m.String(); !strings.Contains(s, "ORIGINAL FIGURE SIZE") || !strings.Contains(s, "mm") {
		t.Errorf("显示串应给 mm: %s", s)
	}

	// 裁剪后的当前尺寸：取右半边（50%..100%）→ 宽高各减半
	c := m.Crop(50, 0, 100, 50)
	if !c.Found {
		t.Fatalf("裁剪区域应能算出尺寸")
	}
	if c.PixelsW != 142 || c.PixelsH != 78 {
		t.Errorf("裁剪位图 %dx%d，期望 142x78", c.PixelsW, c.PixelsH)
	}
	if c.WidthMM < 17 || c.WidthMM > 20 {
		t.Errorf("裁剪宽 %.1fmm，期望约为整图一半", c.WidthMM)
	}
	if !strings.Contains(c.CropHint(), "this crop is") {
		t.Errorf("裁剪提示串 = %q", c.CropHint())
	}
	// 全图（0-100%）裁出来的尺寸应与整图一致
	if f := m.Crop(0, 0, 100, 100); f.WidthMM < 35 || f.WidthMM > 39 {
		t.Errorf("全图裁剪 %.1fmm，期望与整图相同", f.WidthMM)
	}
	// 测不出尺寸时裁剪也不该编一个出来
	empty := ImageMeasure{}
	if empty.Crop(0, 0, 50, 50).Found || empty.CropHint() != "" {
		t.Error("未测到尺寸时裁剪应保持为空")
	}
}
