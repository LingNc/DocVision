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
