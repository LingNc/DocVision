package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T292：这次实跑里 MinerU 的产物只有 markdown/images/layout.json，没有
// *_origin.pdf，于是 origin PDF 清单是空的 → source 挂载点关闭 → 样式会话
// `ls /source` 报"没有那个文件或目录"，一页原书都看不到。我们自己切的 part
// PDF 就在 split_files 里，必须拿它兜底。
func TestFindOriginPDFsFallsBackToSplitPDFs(t *testing.T) {
	root := t.TempDir()
	mineru := filepath.Join(root, "mineru_output")
	split := filepath.Join(root, "split_files")
	subj := "书 名 " // 真实数据里 md 基名带尾随空格，part 目录名是 "<subj>_part1"
	// part1：MinerU 没留 origin PDF（这次的真实情况）。
	if err := os.MkdirAll(filepath.Join(mineru, subj+"_part1", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	// part2：留了 origin PDF，应优先生效。
	p2 := filepath.Join(mineru, subj+"_part2")
	if err := os.MkdirAll(p2, 0o755); err != nil {
		t.Fatal(err)
	}
	origin2 := filepath.Join(p2, "uuid_origin.pdf")
	writeFile(t, origin2, "%PDF-1.4\n")
	// split_files：part1 的本体。
	if err := os.MkdirAll(split, 0o755); err != nil {
		t.Fatal(err)
	}
	own1 := filepath.Join(split, subj+"_part1.pdf")
	writeFile(t, own1, "%PDF-1.4\n")

	got := findOriginPDFs(mineru, split, subj)
	if len(got) != 2 {
		t.Fatalf("应当回退到 split_files 的 part1 + part2 的 origin，实得 %v", got)
	}
	if got[0].PDF != own1 {
		t.Fatalf("part1 应当用 split_files 里的本体，实得 %q", got[0].PDF)
	}
	if got[0].Part != subj+"_part1" {
		t.Fatalf("part1 的 part 名必须还是 MinerU 的目录名（FileByPart 靠它对齐），实得 %q", got[0].Part)
	}
	if got[1].PDF != origin2 || got[1].Part != subj+"_part2" {
		t.Fatalf("part2 应当用 *_origin.pdf 且 part 名正确，实得 %+v", got[1])
	}
}

// 没有 origin PDF、也没有 split 兜底目录时仍然是空（不虚构文件）。
func TestFindOriginPDFsWithoutFallbackDir(t *testing.T) {
	root := t.TempDir()
	mineru := filepath.Join(root, "mineru_output")
	if err := os.MkdirAll(filepath.Join(mineru, "书"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findOriginPDFs(mineru, "", "书"); len(got) != 0 {
		t.Fatalf("没有兜底目录时不该凭空返回 %v", got)
	}
	if got := findOriginPDFs(mineru, filepath.Join(root, "nope"), "书"); len(got) != 0 {
		t.Fatalf("兜底目录不存在时不该凭空返回 %v", got)
	}
}

// 有原书视图时 doc_search 才提示用 source: 打开原页；没有时不许指路。
func TestDocSearchHintMatchesViewAvailability(t *testing.T) {
	idx := &DocIndex{}
	idx.Entries = append(idx.Entries, DocEntry{Part: "书 名 _part1", Page: 3, Type: "text", Text: "第一章 基础篇"})
	noView := &DocSearchTool{Index: idx}
	res, err := noView.Execute(`{"query":"第一章"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(res.Text, "NOT available") || containsStr(res.Text, "list_source_pages {page: pN} prints") {
		t.Fatalf("没有视图时的回执不对: %q", res.Text)
	}
	view := &pdfView{Dir: "/tmp", total: 5, Files: []pdfViewFile{{Name: "book.pdf", Part: "书 名 _part1", First: 1, Count: 5}}}
	withView := &DocSearchTool{Index: idx, View: view, Mount: "source"}
	res, err = withView.Execute(`{"query":"第一章"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(res.Text, "view_pdf {path:\"source:book.pdf\"") {
		t.Fatalf("有视图时应当指向 source: 原页: %q", res.Text)
	}
}
