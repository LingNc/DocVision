package latex

// Tests for the DOCVISION-note side of the original-document index:
// parseMdMarkers (processed md -> notes, keyed by image file name), the
// join onto the MinerU blocks in buildDocIndex, and the presentation in
// doc_search / list_source_pages. An image block whose content_list entry
// has no caption is only a filename without them: the note is what makes
// "find that mind map" work.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------

// writeMarkerImages adds the pictures the tests reference to the runner's
// images directory (testRunner already provides <images>/book/).
func writeMarkerImages(t *testing.T, r *Runner) {
	t.Helper()
	for _, name := range []string{"aaa111.png", "bbb222.png", "ccc333.png"} {
		if err := os.WriteFile(filepath.Join(r.cfg.Paths.ImagesDir, "book", name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// buildMarkerTestIndex builds a throw-away MinerU part plus a processed
// markdown carrying three DOCVISION notes, and returns the compiled index
// (and the index file path). The notes are keyed on the image FILE name
// while the index stores "images/<sha>.png" and the note
// "images/book/<sha>.png" — the cross-directory join is the point.
func buildMarkerTestIndex(t *testing.T) (*DocIndex, string) {
	t.Helper()
	root := t.TempDir()
	part := filepath.Join(root, "mineru_output", "book")
	if err := os.MkdirAll(part, 0o755); err != nil {
		t.Fatal(err)
	}
	items := []map[string]any{
		{"type": "text", "page_idx": 0, "bbox": []float64{10, 20, 300, 60}, "text": "第一章 随机变量"},
		{"type": "image", "page_idx": 0, "bbox": []float64{60, 100, 400, 300}, "img_path": "images/aaa111.png"},
		{"type": "image", "page_idx": 1, "bbox": []float64{60, 100, 400, 300}, "img_path": "images/bbb222.png"},
		{"type": "image", "page_idx": 1, "bbox": []float64{60, 400, 400, 600}, "img_path": "images/ccc333.png",
			"image_caption": []string{"图 1.1 实验装置"}},
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "book_content_list.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	mdDir := filepath.Join(root, "source")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 第三条注释挂在 zzz999.png 上：索引里没有这个块，必须被静默丢弃
	// （不报错），第四张图 ccc333.png 有 OCR caption、没有注释。
	md := "<!-- DOCVISION-STYLED-TEXT: 知识导图 -->\n" +
		"CONTENT: 第一章 随机变量\n" +
		"LINK: [styled-text](images/book/aaa111.png)\n\n" +
		"<!-- DOCVISION-VECTOR: mind-map diagram -->\n" +
		"LINK: [vector](images/book/bbb222.png)\n" +
		"```latex\n\\begin{tikzpicture}\\end{tikzpicture}\n```\n\n" +
		"<!-- DOCVISION-IMAGE: experiment setup -->\n" +
		"DESCRIBE: 实验装置的示意图\n" +
		"LINK: [image](images/book/zzz999.png)\n"
	mdPath := filepath.Join(mdDir, "book.md")
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(root, "doc_index", "doc_index.json")
	idx, _, err := buildDocIndex(filepath.Join(root, "mineru_output"), []string{mdPath}, mdDir, out)
	if err != nil {
		t.Fatalf("buildDocIndex 失败: %v", err)
	}
	return idx, out
}

// ---------------------------------------------------------------------
// parseMdMarkers
// ---------------------------------------------------------------------

// TestParseMdMarkers pins the parser against the REAL writer: the three
// note kinds are produced by embedBlock (level 1), so the format tested
// is the format the pipeline emits.
func TestParseMdMarkers(t *testing.T) {
	r := testRunner(t, true)
	writeMarkerImages(t, r)
	dir := t.TempDir()

	blocks := []string{
		r.embedBlock(&imageProgress{Class: ClassText, Styled: true, StyleNote: "知识导图",
			Content: "第一章 随机变量\n第二章 分布函数", ImgPath: "images/book/aaa111.png"}, "book.md", dir),
		r.embedBlock(&imageProgress{Class: ClassVector, Label: "mind-map diagram",
			TikzCode: "\\begin{tikzpicture}\\end{tikzpicture}", ImgPath: "images/book/bbb222.png"}, "book.md", dir),
		r.embedBlock(&imageProgress{Class: ClassRaster, Label: "experiment setup",
			Content: "实验装置的示意图", ImgPath: "images/book/ccc333.png"}, "book.md", dir),
	}
	for i, b := range blocks {
		if !strings.Contains(b, "DOCVISION-") {
			t.Fatalf("embedBlock #%d 没有写出注释（测试前提不成立）:\n%s", i, b)
		}
	}
	// 末尾一条"无注释的普通图片引用"：解析器不得为它编造条目。
	md := strings.Join(blocks, "\n\n") + "\n\nLINK: [image](images/book/plain999.png)\n"
	if err := os.WriteFile(filepath.Join(dir, "book.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	got := parseMdMarkers(dir, []string{"book.md"})
	if len(got) != 3 {
		t.Fatalf("parseMdMarkers 得到 %d 条，期望 3 条: %#v", len(got), got)
	}
	styled := got["aaa111.png"]
	if styled.Marker != markerStyledText || styled.Label != "知识导图" ||
		!strings.Contains(styled.Content, "第一章 随机变量") || !strings.Contains(styled.Content, "第二章 分布函数") {
		t.Errorf("STYLED-TEXT 解析错误: %#v", styled)
	}
	vec := got["bbb222.png"]
	if vec.Marker != markerVector || vec.Label != "mind-map diagram" || vec.Content != "" {
		t.Errorf("VECTOR 解析错误（内容应来自注释、tikz 不进 Content）: %#v", vec)
	}
	ras := got["ccc333.png"]
	if ras.Marker != markerImage || ras.Label != "experiment setup" || ras.Content != "实验装置的示意图" {
		t.Errorf("RASTER 解析错误: %#v", ras)
	}
	if _, ok := got["plain999.png"]; ok {
		t.Errorf("无注释的普通图片引用不应产生条目: %#v", got["plain999.png"])
	}

	// 容错：CRLF + 旧式多行注释（首行未闭合）+ 多行 DESCRIBE；缺失的
	// 文件与空 mdNames 都必须安全返回。
	crlf := "<!-- DOCVISION-IMAGE: 实验装置\r\n" +
		"DESCRIBE: 第一行说明\r\n" +
		"第二行说明\r\n" +
		"LINK: [image](images/book/old888.png)\r\n" +
		" -->\r\n"
	if err := os.WriteFile(filepath.Join(dir, "crlf.md"), []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	old := parseMdMarkers(dir, []string{"crlf.md", "does-not-exist.md"})
	note, ok := old["old888.png"]
	if !ok {
		t.Fatalf("CRLF/多行注释未解析出来: %#v", old)
	}
	if note.Marker != markerImage || note.Label != "实验装置" ||
		!strings.Contains(note.Content, "第一行说明") || !strings.Contains(note.Content, "第二行说明") {
		t.Errorf("CRLF/多行注释解析错误: %#v", note)
	}
	if m := parseMdMarkers(dir, nil); len(m) != 0 {
		t.Errorf("空 mdNames 应返回空 map，得到 %#v", m)
	}
	if m := parseMdMarkers("", []string{"book.md"}); len(m) != 0 {
		t.Errorf("空 mdDir 应返回空 map，得到 %#v", m)
	}

	// 长 CONTENT：LINK 离注释头几十行也必须匹配上（样式化文本图可能
	// 携带整页文字），不能因为"离得太远"而丢弃。
	var long []string
	for i := 1; i <= 30; i++ {
		long = append(long, fmt.Sprintf("第 %d 行文字", i))
	}
	longMD := "<!-- DOCVISION-STYLED-TEXT: 整页文字 -->\nCONTENT: " +
		strings.Join(long, "\n") + "\nLINK: [styled-text](images/book/long777.png)\n"
	if err := os.WriteFile(filepath.Join(dir, "long.md"), []byte(longMD), 0o644); err != nil {
		t.Fatal(err)
	}
	longMarkers := parseMdMarkers(dir, []string{"long.md"})
	lm, ok := longMarkers["long777.png"]
	if !ok {
		t.Fatalf("长 CONTENT 的注释未解析: %#v", longMarkers)
	}
	if lm.Label != "整页文字" || strings.Count(lm.Content, "行文字") != 30 {
		t.Errorf("长 CONTENT 内容不完整: label=%q hits=%d", lm.Label, strings.Count(lm.Content, "行文字"))
	}
}

// ---------------------------------------------------------------------
// buildDocIndex backfill
// ---------------------------------------------------------------------

// TestBuildDocIndexBackfillsDocMarkers: the notes are joined onto the
// image blocks by file name, an unmatched note is dropped, blocks without
// a note keep their old fields, and an index written before this feature
// still loads.
func TestBuildDocIndexBackfillsDocMarkers(t *testing.T) {
	idx, out := buildMarkerTestIndex(t)
	if len(idx.Entries) != 4 {
		t.Fatalf("期望 4 个块，得到 %d", len(idx.Entries))
	}
	byImg := map[string]DocEntry{}
	for _, e := range idx.Entries {
		if e.Img != "" {
			byImg[filepath.Base(e.Img)] = e
		}
	}

	styled := byImg["aaa111.png"]
	if styled.Marker != markerStyledText || styled.Label != "知识导图" ||
		!strings.Contains(styled.Content, "第一章 随机变量") {
		t.Errorf("aaa111.png 未回填: %#v", styled)
	}
	if styled.Global != 1 || styled.Page != 0 || styled.Type != "image" || styled.Seq == 0 {
		t.Errorf("回填不应破坏既有字段: %#v", styled)
	}
	vec := byImg["bbb222.png"]
	if vec.Marker != markerVector || vec.Label != "mind-map diagram" {
		t.Errorf("bbb222.png 未回填: %#v", vec)
	}
	cap := byImg["ccc333.png"]
	if cap.Marker != "" || cap.Label != "" || cap.Content != "" {
		t.Errorf("无注释的块必须留空: %#v", cap)
	}
	if !strings.Contains(cap.Text, "图 1.1 实验装置") {
		t.Errorf("有 caption 的块文本丢失: %#v", cap)
	}

	// 落盘再读回：新字段必须保留（doc_index.json 是会话读的唯一来源）。
	reloaded, err := loadDocIndex(out)
	if err != nil {
		t.Fatalf("loadDocIndex: %v", err)
	}
	for _, e := range reloaded.Entries {
		if filepath.Base(e.Img) == "aaa111.png" && e.Marker != markerStyledText {
			t.Errorf("重新加载后 marker 丢失: %#v", e)
		}
	}

	// 旧版索引（没有这三个字段）仍然可以反序列化。
	legacyPath := filepath.Join(filepath.Dir(out), "legacy.json")
	legacy := `{"parts":["book"],"page_sizes":{"book":[493,720]},"entries":[` +
		`{"seq":1,"part":"book","page":0,"global":1,"type":"image","bbox":[0,0,10,10],"text":"","img":"images/aaa111.png"}]}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	lidx, err := loadDocIndex(legacyPath)
	if err != nil {
		t.Fatalf("旧版索引无法反序列化: %v", err)
	}
	if len(lidx.Entries) != 1 || lidx.Entries[0].Img != "images/aaa111.png" {
		t.Fatalf("旧版索引字段丢失: %#v", lidx.Entries)
	}
	if lidx.Entries[0].Marker != "" || lidx.Entries[0].Content != "" {
		t.Errorf("旧版索引的新字段应为空: %#v", lidx.Entries[0])
	}
}

// TestBuildDocIndexWithoutMarkerDir: 没有注释目录（例如只跑部分阶段）
// 时不报错、不回填，旧行为照常可用。
func TestBuildDocIndexWithoutMarkerDir(t *testing.T) {
	idx, out := buildMarkerTestIndex(t)
	root := filepath.Dir(filepath.Dir(out)) // <root>/doc_index -> <root>
	out2 := filepath.Join(root, "doc_index", "no_notes.json")
	idx2, _, err := buildDocIndex(filepath.Join(root, "mineru_output"),
		[]string{filepath.Join(root, "source", "book.md")}, "", out2)
	if err != nil {
		t.Fatalf("无注释目录时 buildDocIndex 失败: %v", err)
	}
	if len(idx2.Entries) != len(idx.Entries) {
		t.Fatalf("块数变化: %d vs %d", len(idx2.Entries), len(idx.Entries))
	}
	for _, e := range idx2.Entries {
		if e.Marker != "" || e.Label != "" || e.Content != "" {
			t.Errorf("没有注释目录时不应回填: %#v", e)
		}
	}
}

// ---------------------------------------------------------------------
// production wiring: buildDocIndexQuiet
// ---------------------------------------------------------------------

// TestBuildDocIndexQuietReadsProcessedSource pins the wiring end to end:
// the index built by the book pipeline takes the notes from <proj>/source
// (the images-phase output) and NOT from the raw input md, falls back to
// the input md directory when <proj>/source is missing, and writes the
// result to <proj>/doc_index/doc_index.json.
func TestBuildDocIndexQuietReadsProcessedSource(t *testing.T) {
	root := t.TempDir()
	mineru := filepath.Join(root, "mineru_output")
	if err := os.MkdirAll(filepath.Join(mineru, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	items := `[{"type":"image","page_idx":0,"bbox":[10,10,100,100],"img_path":"images/aaa111.png"}]`
	if err := os.WriteFile(filepath.Join(mineru, "book", "book_content_list.json"), []byte(items), 0o644); err != nil {
		t.Fatal(err)
	}
	// 原始输入 md：只有图片引用，没有任何注释。
	input := filepath.Join(root, "files")
	if err := os.MkdirAll(input, 0o755); err != nil {
		t.Fatal(err)
	}
	inputMD := filepath.Join(input, "book.md")
	if err := os.WriteFile(inputMD, []byte("![](images/aaa111.png)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 加工过的 md（images 阶段产物）：带注释。
	proj := filepath.Join(root, "latex_project")
	if err := os.MkdirAll(filepath.Join(proj, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	annotated := "<!-- DOCVISION-STYLED-TEXT: 知识导图 -->\nCONTENT: 第一章 随机变量\nLINK: [styled-text](images/book/aaa111.png)\n"
	if err := os.WriteFile(filepath.Join(proj, "source", "book.md"), []byte(annotated), 0o644); err != nil {
		t.Fatal(err)
	}

	r := testRunner(t, true)
	r.cfg.Paths.MineruOutput = mineru
	r.buildDocIndexQuiet(proj, input, []string{inputMD})
	if r.docIndex == nil || len(r.docIndex.Entries) != 1 {
		t.Fatalf("索引未建立: %#v", r.docIndex)
	}
	e := r.docIndex.Entries[0]
	if e.Marker != markerStyledText || e.Label != "知识导图" || e.Content != "第一章 随机变量" {
		t.Errorf("未从 <proj>/source 回填注释: %#v", e)
	}
	if _, err := os.Stat(filepath.Join(proj, "doc_index", "doc_index.json")); err != nil {
		t.Errorf("索引文件未写入: %v", err)
	}

	// 回退：没有 <proj>/source（例如只跑部分阶段）时用输入 md 目录。
	proj2 := filepath.Join(root, "proj2")
	if err := os.MkdirAll(proj2, 0o755); err != nil {
		t.Fatal(err)
	}
	input2 := filepath.Join(root, "files2")
	if err := os.MkdirAll(input2, 0o755); err != nil {
		t.Fatal(err)
	}
	inputMD2 := filepath.Join(input2, "book.md")
	if err := os.WriteFile(inputMD2, []byte(annotated), 0o644); err != nil {
		t.Fatal(err)
	}
	r2 := testRunner(t, true)
	r2.cfg.Paths.MineruOutput = mineru
	r2.buildDocIndexQuiet(proj2, input2, []string{inputMD2})
	if r2.docIndex == nil || r2.docIndex.Entries[0].Marker != markerStyledText {
		t.Errorf("无 <proj>/source 时未回退到输入 md 目录: %#v", r2.docIndex)
	}
}

// ---------------------------------------------------------------------
// presentation: doc_search / list_source_pages
// ---------------------------------------------------------------------

// TestSearchFindsImagesByDocMarker: 图片条目可以按注释里的描述/原文命中，
// 不再只能靠文件名。
func TestSearchFindsImagesByDocMarker(t *testing.T) {
	idx, _ := buildMarkerTestIndex(t)

	for _, tc := range []struct{ query, want string }{
		{"知识导图", "aaa111.png"}, // STYLED-TEXT 的描述
		{"第一章", "aaa111.png"},  // CONTENT 原文
		{"mind-map", "bbb222.png"},
		{"vector", "bbb222.png"},
		{"aaa111.png", "aaa111.png"}, // 文件名照旧可检索
	} {
		hits := idx.Search(tc.query, 5)
		if len(hits) == 0 {
			t.Errorf("Search(%q) 无命中", tc.query)
			continue
		}
		found := false
		for _, h := range hits {
			if filepath.Base(h.Img) == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("Search(%q) 未命中 %s: %#v", tc.query, tc.want, hits)
		}
	}
	if hits := idx.Search("完全不存在的词", 5); len(hits) != 0 {
		t.Errorf("无关词不应命中: %#v", hits)
	}

	// 命中输出里要看得见标记与内容（否则模型只知道"有张图"）。
	text := ""
	for _, e := range idx.Search("知识导图", 5) {
		text += FormatEntry(e)
	}
	for _, want := range []string{"[styled-text] 知识导图", "content: 第一章 随机变量"} {
		if !strings.Contains(text, want) {
			t.Errorf("FormatEntry 缺少 %q:\n%s", want, text)
		}
	}
	vtext := ""
	for _, e := range idx.Search("mind-map", 5) {
		vtext += FormatEntry(e)
	}
	if !strings.Contains(vtext, "[vector] mind-map diagram") {
		t.Errorf("FormatEntry 缺少矢量图标记:\n%s", vtext)
	}
}

// TestListSourcePagesShowsDocMarkers: 页详情里的图片清单同样要带上
// 标记/描述/内容摘要。
func TestListSourcePagesShowsDocMarkers(t *testing.T) {
	idx, _ := buildMarkerTestIndex(t)
	view := &pdfView{total: 2,
		Files: []pdfViewFile{{Name: "book.pdf", PDF: "/nonexistent/book.pdf", Part: "book", First: 1, Count: 2}}}
	tool := &ListSourcePagesTool{View: view, Index: idx, Mount: "source"}

	res, err := tool.Execute(`{"page":1}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"aaa111.png", "[styled-text] 知识导图"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("页详情缺少 %q:\n%s", want, res.Text)
		}
	}

	res2, err := tool.Execute(`{"page":2}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bbb222.png", "[vector] mind-map diagram", "ccc333.png", "图 1.1 实验装置"} {
		if !strings.Contains(res2.Text, want) {
			t.Errorf("页详情缺少 %q:\n%s", want, res2.Text)
		}
	}
}
