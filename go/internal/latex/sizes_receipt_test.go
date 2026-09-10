package latex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// 用户实测的两个缺口：① 裁剪之后回执只说 "width 1280px"，看不出"现在看的是
// 多大一块"；② 看 PDF（原书页 / 自己编的成品）完全不给物理尺寸，无法判断
// 写出来的 latex 尺寸是否与原书一致。两个回执现在都带 mm。
func TestViewImageReceiptReportsCropSize(t *testing.T) {
	part := t.TempDir()
	imgDir := filepath.Join(part, "images", "doc")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(imgDir, "fig.png")
	writePNG(t, img, 284, 156)
	cl := `[{"page_idx":0,"bbox":[346,468,555,545],"img_path":"images/doc/fig.png"}]`
	if err := os.WriteFile(filepath.Join(part, "x_content_list.json"), []byte(cl), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "layout.json"), []byte(`{"pdf_info":[{"page_size":[493,720]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: imgDir, Subject: "doc"}

	res, err := tool.Execute(`{"path":"fig.png","left":50,"top":0,"right":100,"bottom":50}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "ORIGINAL FIGURE SIZE") {
		t.Errorf("整图尺寸缺失: %q", res.Text)
	}
	// 右下半块：宽高各一半（宽度 36.9mm 的一半 ≈ 18.4mm）
	if !strings.Contains(res.Text, "this crop is 18.4mm x 10.1mm on the page") {
		t.Errorf("裁剪尺寸缺失或不正确: %q", res.Text)
	}

	// 不裁剪时不该出现"这块多大"的冗余行
	res, err = tool.Execute(`{"path":"fig.png"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "this crop is") {
		t.Errorf("未裁剪时不该报裁剪尺寸: %q", res.Text)
	}
}

// 看 PDF 要报页面尺寸与裁剪区域尺寸（mm），原书页与自编 PDF 同一口径。
func TestViewPDFReceiptReportsMillimetres(t *testing.T) {
	for _, bin := range []string{"xelatex", "pdfinfo", "pdftoppm"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	dir := t.TempDir()
	tex := "\\documentclass{article}\n\\usepackage[paperwidth=100mm,paperheight=150mm,margin=10mm]{geometry}\n\\begin{document}\nsize probe\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(dir, "page.tex"), []byte(tex), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", "-halt-on-error", "page.tex")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("xelatex 不可用: %v: %s", err, truncateStr(string(out), 200))
	}
	tool := &ViewPDFTool{
		Root: dir,
		Comp: NewCompiler(config.LatexCompileConfig{RasterCommand: "pdftoppm", RasterDPI: 72}),
	}

	res, err := tool.Execute(`{"path":"page.pdf","left":0,"top":0,"right":50,"bottom":50}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "page 100.0mm x 150.0mm") {
		t.Errorf("页面尺寸缺失: %q", res.Text)
	}
	if !strings.Contains(res.Text, "this crop 50.0mm x 75.0mm") {
		t.Errorf("裁剪区域尺寸缺失: %q", res.Text)
	}
	if !strings.Contains(res.Text, "page 1/1") {
		t.Errorf("页码缺失: %q", res.Text)
	}

	// 整页查看只报页面尺寸，不重复裁剪尺寸
	res, err = tool.Execute(`{"path":"page.pdf"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "page 100.0mm x 150.0mm") || strings.Contains(res.Text, "this crop") {
		t.Errorf("整页回执 = %q", res.Text)
	}
}

// pdfPageSizeMM 是"每页真实尺寸"的唯一来源：多页 PDF 里各页可以不同。
func TestPDFPageSizeMMPerPage(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo not installed")
	}
	dir := t.TempDir()
	// 两页真内容（只 \newpage 不写字的话 xelatex 只产出 1 页）
	tex := "\\documentclass{article}\n\\usepackage[paperwidth=100mm,paperheight=150mm,margin=10mm]{geometry}\n" +
		"\\begin{document}\nfirst page\n\\newpage\nsecond page\n\\end{document}\n"
	if err := os.WriteFile(filepath.Join(dir, "two.tex"), []byte(tex), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", "-halt-on-error", "two.tex")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("xelatex 不可用: %v: %s", err, truncateStr(string(out), 200))
	}
	pdf := filepath.Join(dir, "two.pdf")
	w, h, ok := pdfPageSizeMM(pdf, 2)
	if !ok {
		t.Fatalf("第 2 页尺寸未取到")
	}
	if w < 99 || w > 101 || h < 149 || h > 151 {
		t.Errorf("第 2 页 = %.1f x %.1f mm，期望 ~100x150", w, h)
	}
	if _, _, ok := pdfPageSizeMM(filepath.Join(dir, "nope.pdf"), 1); ok {
		t.Error("不存在的 PDF 不该给出尺寸")
	}
}
