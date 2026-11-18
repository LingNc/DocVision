package docindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakePart 造一个 MinerU part 目录：content_list + layout.json。
func fakePart(t *testing.T, mineru, dir string, items []map[string]any) {
	t.Helper()
	p := filepath.Join(mineru, dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	if err := os.WriteFile(filepath.Join(p, dir+"_content_list.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	layout := map[string]any{"pdf_info": []any{map[string]any{"page_size": []any{595.0, 842.0}}}}
	lb, _ := json.Marshal(layout)
	if err := os.WriteFile(filepath.Join(p, "layout.json"), lb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAndSearch(t *testing.T) {
	mineru := t.TempDir()
	fakePart(t, mineru, "book", []map[string]any{
		{"type": "text", "page_idx": 0, "bbox": []any{10.0, 10.0, 100.0, 30.0}, "text": "机器学习的基础理论包括监督学习"},
		{"type": "image", "page_idx": 0, "bbox": []any{50.0, 200.0, 300.0, 400.0}, "img_path": "images/abc.jpg", "image_caption": []any{"图 1-1 决策树"}},
		{"type": "text", "page_idx": 1, "bbox": []any{10.0, 10.0, 100.0, 30.0}, "text": "第二章 概率图模型"},
	})
	idx, pages, err := Build(mineru, "", []string{"book.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Total() != 2 || len(idx.Entries) != 3 {
		t.Fatalf("应为 2 页 3 条目: total=%d entries=%d", idx.Total(), len(idx.Entries))
	}
	if pages.Total != 0 {
		t.Fatalf("无 OriginPDFsFunc 时页表应为空: %+v", pages)
	}
	// 检索：命中 + 排序（前缀命中排最前）
	hits := idx.Search("监督学习", 5)
	if len(hits) == 0 || hits[0].Text != "机器学习的基础理论包括监督学习" {
		t.Fatalf("监督学习 应命中第一条: %+v", hits)
	}
	// text 字段过滤：图块的 caption 进 Text
	hits = idx.Search("text:决策树", 5)
	if len(hits) != 1 || hits[0].Img != "images/abc.jpg" {
		t.Fatalf("text:决策树 应命中图块: %+v", hits)
	}
	// 多词 AND：第二个词不在任何条目 → 无命中
	if hits := idx.Search("监督学习 不存在的词xyz", 5); len(hits) != 0 {
		t.Fatalf("AND 语义下应无命中: %+v", hits)
	}
}

func TestEntryOnPageAndCropHint(t *testing.T) {
	mineru := t.TempDir()
	fakePart(t, mineru, "book", []map[string]any{
		{"type": "text", "page_idx": 0, "bbox": []any{10.0, 10.0, 100.0, 30.0}, "text": "见图 3-2 所示"},
		{"type": "image", "page_idx": 0, "bbox": []any{50.0, 200.0, 300.0, 400.0}, "img_path": "images/fig32.jpg"},
	})
	idx, _, err := Build(mineru, "", []string{"book.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.EntryOnPage(1); len(got) != 2 {
		t.Fatalf("第 1 页应有 2 条目: %d", len(got))
	}
	hint, ok := idx.CropHint(1, "见图 3-2")
	if !ok || hint != [4]float64{50, 200, 300, 400} {
		t.Fatalf("CropHint 应落到图块 bbox: %v %v", hint, ok)
	}
	if _, ok := idx.CropHint(99, "见图"); ok {
		t.Fatal("不存在的页应无 hint")
	}
}

func TestMarkersBackfill(t *testing.T) {
	mineru := t.TempDir()
	fakePart(t, mineru, "book", []map[string]any{
		{"type": "image", "page_idx": 0, "bbox": []any{0.0, 0.0, 10.0, 10.0}, "img_path": "images/sha1.jpg"},
	})
	mdDir := t.TempDir()
	note := `<!-- DOCVISION-STYLED-TEXT: 知识导图 -->
CONTENT: 第一行印刷文字
第二行续行
LINK: [styled-text](images/主题/sha1.jpg)
`
	if err := os.WriteFile(filepath.Join(mdDir, "book.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, _, err := Build(mineru, "", []string{"book.md"}, mdDir, "")
	if err != nil {
		t.Fatal(err)
	}
	e := idx.Entries[0]
	if e.Marker != "styled-text" || e.Label != "知识导图" {
		t.Fatalf("marker 回填失败: %+v", e)
	}
	if e.Text != "第一行印刷文字 第二行续行" {
		t.Fatalf("无厂商图注时 Text 应用 CONTENT 兜底: %q", e.Text)
	}
}

func TestMarkersVectorFence(t *testing.T) {
	mineru := t.TempDir()
	fakePart(t, mineru, "book", []map[string]any{
		{"type": "image", "page_idx": 0, "bbox": []any{0.0, 0.0, 10.0, 10.0}, "img_path": "images/v1.jpg"},
	})
	mdDir := t.TempDir()
	note := `<!-- DOCVISION-VECTOR: 流程图 -->
LINK: [vector](images/主题/v1.jpg)
` + "```latex\n\\begin{tikzpicture}\n\\draw (0,0)--(1,1);\n\\end{tikzpicture}\n```\n"
	if err := os.WriteFile(filepath.Join(mdDir, "book.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, _, err := Build(mineru, "", []string{"book.md"}, mdDir, "")
	if err != nil {
		t.Fatal(err)
	}
	e := idx.Entries[0]
	if e.Marker != "vector" || !contains(e.Content, "tikzpicture") {
		t.Fatalf("矢量代码块应进 content: %+v", e)
	}
	// vector 的 Text 兜底用 Label 而非整段代码
	if e.Text != "流程图" {
		t.Fatalf("vector Text 应用标签: %q", e.Text)
	}
}

func TestLoadRoundtrip(t *testing.T) {
	mineru := t.TempDir()
	fakePart(t, mineru, "book", []map[string]any{
		{"type": "text", "page_idx": 0, "bbox": []any{0.0, 0.0, 1.0, 1.0}, "text": "回环测试"},
	})
	out := filepath.Join(t.TempDir(), "doc_index.json")
	if _, _, err := Build(mineru, "", []string{"book.md"}, "", out); err != nil {
		t.Fatal(err)
	}
	idx, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if hits := idx.Search("回环", 3); len(hits) != 1 {
		t.Fatalf("落盘重载后检索应一致: %+v", hits)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
