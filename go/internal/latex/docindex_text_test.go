package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// text 是"这个图块对应什么内容"的入口：MinerU 没给图注时，用**我们**写在
// 图上的内容兜底（styled-text 的印刷原文 / raster 的解释 / vector 的图标签），
// 有厂商图注时保持原样。链接（Img）永远是原图。
func TestDocIndexTextFallsBackToOurOwnContent(t *testing.T) {
	root := t.TempDir()
	part := filepath.Join(root, "mineru_output", "book")
	if err := os.MkdirAll(part, 0o755); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		// 无图注的 raster：text 应变成我们生成的解释
		{"type": "image", "page_idx": 0, "bbox": []float64{10, 10, 200, 200}, "img_path": "images/raster1.png"},
		// 无图注的 styled-text：text 应变成图里的印刷原文
		{"type": "image", "page_idx": 0, "bbox": []float64{10, 210, 200, 400}, "img_path": "images/styled1.png"},
		// 无图注的 vector：text 应变成图标签（不是 LaTeX 本体）
		{"type": "image", "page_idx": 1, "bbox": []float64{10, 10, 200, 200}, "img_path": "images/vec1.png"},
		// 有厂商图注：text 保持厂商原文，我们的解释仍在 content 里
		{"type": "image", "page_idx": 1, "bbox": []float64{10, 210, 200, 400}, "img_path": "images/cap1.png",
			"image_caption": []string{"图 2.1 厂商给的原图注"}},
	}
	raw, _ := json.Marshal(items)
	if err := os.WriteFile(filepath.Join(part, "book_content_list.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	mdDir := filepath.Join(root, "source")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "<!-- DOCVISION-IMAGE: 实验装置 -->\n" +
		"DESCRIBE: 这是一台带电源的滑轮实验装置，左侧挂着砝码。\n" +
		"LINK: [image](images/book/raster1.png)\n\n" +
		"<!-- DOCVISION-STYLED-TEXT: 知识导图 -->\n" +
		"CONTENT: 第一章 随机变量 第二章 数字特征\n" +
		"LINK: [styled-text](images/book/styled1.png)\n\n" +
		"<!-- DOCVISION-VECTOR: mind-map diagram -->\n" +
		"LINK: [vector](images/book/vec1.png)\n" +
		"```latex\n\\begin{tikzpicture}\\draw (0,0)--(1,1);\\end{tikzpicture}\n```\n\n" +
		"<!-- DOCVISION-IMAGE: 有厂商图注的图 -->\n" +
		"DESCRIBE: 这里是我们生成的解释，厂商也有图注。\n" +
		"LINK: [image](images/book/cap1.png)\n"
	mdPath := filepath.Join(mdDir, "book.md")
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, _, err := buildDocIndex(filepath.Join(root, "mineru_output"), []string{mdPath}, mdDir,
		filepath.Join(root, "doc_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	byImg := map[string]DocEntry{}
	for _, e := range idx.Entries {
		if e.Img != "" {
			byImg[filepath.Base(e.Img)] = e
		}
	}

	raster := byImg["raster1.png"]
	if raster.Text != "这是一台带电源的滑轮实验装置，左侧挂着砝码。" {
		t.Errorf("raster 无图注时 text 应是我们的解释，got %q", raster.Text)
	}
	styled := byImg["styled1.png"]
	if styled.Text != "第一章 随机变量 第二章 数字特征" {
		t.Errorf("styled-text 无图注时 text 应是图里的印刷原文，got %q", styled.Text)
	}
	vec := byImg["vec1.png"]
	if vec.Text != "mind-map diagram" {
		t.Errorf("vector 无图注时 text 应是图标签（不是 LaTeX 本体），got %q", vec.Text)
	}
	if vec.Content == "" {
		t.Errorf("vector 的 LaTeX 本体仍应保留在 content 里")
	}
	capEntry := byImg["cap1.png"]
	if capEntry.Text != "图 2.1 厂商给的原图注" {
		t.Errorf("有厂商图注时不该被我们的解释顶掉，got %q", capEntry.Text)
	}
	if capEntry.Content == "" {
		t.Errorf("厂商图注存在时，我们的解释仍要在 content 里可检索")
	}

	// 链接一律是原图：三种类型都不指向 figures/*.svg 之类的生成物。
	for name, e := range byImg {
		if filepath.Dir(e.Img) != "images" && filepath.Base(filepath.Dir(e.Img)) != "" {
			t.Errorf("%s 的 Img 不是 images/ 下的原图: %q", name, e.Img)
		}
		if filepath.Ext(e.Img) == ".svg" {
			t.Errorf("%s 的 Img 指向了生成的 SVG（应为原图）: %q", name, e.Img)
		}
	}
	// 兜底进来的 text 照样可检索（这是用户要的效果）。
	for _, q := range []string{"滑轮实验装置", "数字特征", "mind-map"} {
		if hits := idx.Search(q, 5); len(hits) == 0 {
			t.Errorf("Search(%q) 应命中（text 兜底后仍要能按内容检索）", q)
		}
	}
}
