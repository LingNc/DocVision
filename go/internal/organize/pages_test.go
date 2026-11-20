package organize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// layoutFixture writes one MinerU part directory with a layout.json whose
// pages carry the given first-block texts.
func layoutFixture(t *testing.T, root, dir string, pages []string) {
	t.Helper()
	p := filepath.Join(root, dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	var pdfInfo []map[string]any
	for i, text := range pages {
		block := map[string]any{"type": "text"}
		if i == 0 {
			block = map[string]any{"type": "title", "level": 2}
		}
		block["lines"] = []any{map[string]any{
			"spans": []any{map[string]any{"type": "text", "content": text}},
		}}
		pdfInfo = append(pdfInfo, map[string]any{"para_blocks": []any{block}})
	}
	raw, err := json.Marshal(map[string]any{"pdf_info": pdfInfo})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "layout.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWritePageIndex pins T331：页界索引按 layout.json 的逐页首块落到合并后
// md 的行号上，跨部件累加全局页号，行数写进 sidecar 供消费侧判过期。
func TestWritePageIndex(t *testing.T) {
	root := t.TempDir()
	layoutFixture(t, root, "书", []string{"第一章 概述", "正文第一段"})
	layoutFixture(t, root, "书_part1", []string{"第二章 方法"})

	part0 := "## 第一章 概述\n\n正文第一段\n\n## 第一节 引言\n"
	part1 := "## 第二章 方法\n\n正文第二段\n"
	parts := []partText{{dir: "书", text: part0}, {dir: "书_part1", text: part1}}

	// 合并后的行数直接按真实拼接算（part 之间还夹着 mdSeparator 的行）。
	merged := strings.Split(part0+mdSeparator+part1, "\n")
	total := len(merged)
	if err := writePageIndex(root, root, "书", parts, mdSeparator, total); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(pageIndexPath(root, "书"))
	if err != nil {
		t.Fatal(err)
	}
	var idx pageIndexJSON
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.Lines != total || len(idx.Pages) != 3 {
		t.Fatalf("index = %+v (total %d)", idx, total)
	}
	wantPages := []int{1, 2, 3}
	for i, p := range idx.Pages {
		if p.Page != wantPages[i] {
			t.Fatalf("page numbers = %+v", idx.Pages)
		}
	}
	// 页 1 在 0 行，页 2 在 part0 的第 2 行，页 3 在 part1 起始行之后。
	if idx.Pages[0].Line != 0 {
		t.Fatalf("page 1 must start at line 0, got %d", idx.Pages[0].Line)
	}
	if idx.Pages[1].Line != 2 {
		t.Fatalf("page 2 line = %d, want 2", idx.Pages[1].Line)
	}
	part1Start := 0
	for i, l := range merged {
		if l == "## 第二章 方法" {
			part1Start = i
			break
		}
	}
	if idx.Pages[2].Line != part1Start {
		t.Fatalf("page 3 line = %d, want %d", idx.Pages[2].Line, part1Start)
	}

	// 没有 layout.json 的部件不贡献页号，但 sidecar 仍然照写（存在即代表
	// "这版 md 的页界已经算过"，下次命中指纹不会反复重算）。
	empty := t.TempDir()
	if err := writePageIndex(empty, empty, "书2", []partText{{dir: "缺", text: "x\n"}}, mdSeparator, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pageIndexPath(empty, "书2")); err != nil {
		t.Fatalf("sidecar must exist even without layout: %v", err)
	}
}
