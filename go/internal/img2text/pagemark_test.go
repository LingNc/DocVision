package img2text

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writePart lays down one MinerU part directory with a content list.
func writePart(t *testing.T, root, dir string, items string) {
	t.Helper()
	p := filepath.Join(root, dir)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "uuid_content_list.json"), []byte(items), 0o644); err != nil {
		t.Fatal(err)
	}
}

// testAnchors builds a fake mineru_output for md "书.md":
//
//   - 书/       part 0: page 0 two text blocks, page 1 one text + one image
//     (page_idx as JSON STRING — the real-data shape)
//   - 书_part1/ part 1: page 0 one text, page 1 one text (page_idx as NUMBER)
//
// so global pages are 1..2 and 3..4 and both page_idx shapes are covered.
func testAnchors(t *testing.T) (string, []pageAnchor) {
	t.Helper()
	root := t.TempDir()
	writePart(t, root, "书", `[
		{"type":"text","text":"第 一 章 概 述","page_idx":"0"},
		{"type":"text","text":"本章介绍基本概念与背景。","page_idx":"0"},
		{"type":"text","text":"第二节 方法细节","page_idx":"1"},
		{"type":"image","img_path":"images/aaa111.jpg","page_idx":"1"}
	]`)
	writePart(t, root, "书_part1", `[
		{"type":"text","text":"第二章 实验结果","page_idx":0},
		{"type":"text","text":"实验表明方法有效。","page_idx":1}
	]`)
	want := []pageAnchor{
		{kind: "text", text: "第一章概述", page: 1},
		{kind: "text", text: "本章介绍基本概念与背景。", page: 1},
		{kind: "text", text: "第二节方法细节", page: 2},
		{kind: "image", img: "aaa111.jpg", page: 2},
		{kind: "text", text: "第二章实验结果", page: 3},
		{kind: "text", text: "实验表明方法有效。", page: 4},
	}
	return root, want
}

func TestPageAnchors(t *testing.T) {
	root, want := testAnchors(t)
	got := pageAnchors(root, "书.md")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("anchors mismatch:\n got %+v\nwant %+v", got, want)
	}
	// 部件目录按序号排序：part2 在 part1 之后。
	if got := pageAnchors(root, "不存在.md"); got != nil {
		t.Fatalf("unknown md must yield nil, got %+v", got)
	}
	if got := pageAnchors(filepath.Join(root, "nope"), "书.md"); got != nil {
		t.Fatalf("missing mineru_output must yield nil, got %+v", got)
	}
}

func TestAlignPages(t *testing.T) {
	root, _ := testAnchors(t)
	anchors := pageAnchors(root, "书.md")
	lines := []string{
		"# 第一章 概述",                       // text anchor 1 (contained)
		"",                                 // blank -> unknown
		"本章介绍基本概念与背景。",                    // exact hit
		"一些锚点之间没对上的自由文本",                   // unknown, inherits page 1
		"第二节 方法细节",                         // page 2
		"![图](images/书/aaa111.jpg)",        // image anchor, page 2
		"第二章 实验结果",                         // part1 page 0 -> global 3
		"```text",                          // fence line, no anchor
		"实验表明方法有效。",                        // page 4 (数字 page_idx 形态)
		"```",
	}
	pages := alignPages(lines, anchors)
	want := []int{1, 0, 1, 0, 2, 2, 3, 0, 4, 0}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("pages mismatch:\n got %v\nwant %v", pages, want)
	}
}

func TestPageMarkPositions(t *testing.T) {
	root, _ := testAnchors(t)
	anchors := pageAnchors(root, "书.md")
	lines := []string{
		"# 第一章 概述",
		"本章介绍基本概念与背景。",
		"自由文本",
		"第二节 方法细节",
		"![图](images/书/aaa111.jpg)",
		"第二章 实验结果",
		"实验表明方法有效。",
	}
	marks := pageMarkPositions(lines, alignPages(lines, anchors))
	// 页从 1 → 2（line 3）→ 3（line 5）→ 4（line 6）发生递增。
	want := []pageMark{{line: 3, page: 2}, {line: 5, page: 3}, {line: 6, page: 4}}
	if !reflect.DeepEqual(marks, want) {
		t.Fatalf("marks mismatch:\n got %+v\nwant %+v", marks, want)
	}
}

// TestInsertPageMarksFenceDeferral 钉：插入点落在代码围栏内时顺延到围栏结束后
// 的第一行之前，且同一位置已有标记不重复插。
func TestInsertPageMarksFenceDeferral(t *testing.T) {
	root, _ := testAnchors(t)
	anchors := pageAnchors(root, "书.md")
	lines := []string{
		"# 第一章 概述",                // page 1
		"```python",                // fence opens
		"x = \"第二节 方法细节\"",        // fence body 误中 text 锚（page 2）→ 顺延
		"```",                      // fence closes
		"![图](images/书/aaa111.jpg)", // page 2（image 锚，不递增）
		"第二章 实验结果",                // page 3
	}
	pages := alignPages(lines, anchors)
	marks := pageMarkPositions(lines, pages)
	content := strings.Join(lines, "\n") + "\n"
	starts := make([]int, len(lines)+1)
	for i, l := range lines {
		starts[i+1] = starts[i] + len(l) + 1
	}
	got, n := insertPageMarks(content, marks, starts, nil)
	if n != 2 {
		t.Fatalf("expected 2 markers inserted, got %d:\n%s", n, got)
	}
	// 围栏内的 line 2 顺延到围栏结束后（line 4 之前），不落在 ```python 与 ```
	// 之间；[P. 3] 紧随其后。
	if strings.Contains(got, "```python\n\n[P. ") || strings.Contains(got, "x = \"第二节 方法细节\"\n\n[P. ") {
		t.Fatalf("marker fell inside the fence:\n%s", got)
	}
	want := "# 第一章 概述\n```python\nx = \"第二节 方法细节\"\n```\n\n[P. 2]\n![图](images/书/aaa111.jpg)\n\n[P. 3]\n第二章 实验结果\n"
	if got != want {
		t.Fatalf("content mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	// 同一位置已有标记不重复插：插入点前一行已是同页标记时跳过。
	dl := []string{"前文", "[P. 2]", "目标行"}
	dc := strings.Join(dl, "\n") + "\n"
	dst := make([]int, len(dl)+1)
	for i, l := range dl {
		dst[i+1] = dst[i] + len(l) + 1
	}
	dup, n3 := insertPageMarks(dc, []pageMark{{line: 2, page: 2}}, dst, nil)
	if n3 != 0 || dup != dc {
		t.Fatalf("duplicate marker must be skipped, inserted %d:\n%s", n3, dup)
	}
	dup, n4 := insertPageMarks(dc, []pageMark{{line: 2, page: 3}}, dst, nil)
	if n4 != 1 || !strings.Contains(dup, "[P. 2]\n[P. 3]\n目标行") {
		t.Fatalf("different-page marker at same spot must insert:\n%s", dup)
	}
}

// TestPageMarkPlanEndToEnd 走一遍"embed 改写 + 标记插入"的组合（Final pass 的
// 纯函数部分）：图片 embed 锚带页码、跨页处插标记、offset 平移不串位。
func TestPageMarkPlanEndToEnd(t *testing.T) {
	root, _ := testAnchors(t)
	lines := []string{
		"# 第一章 概述",
		"本章介绍基本概念与背景。",
		"第二节 方法细节",
		"![图](images/书/aaa111.jpg)",
		"自由文本",
		"第二章 实验结果",
	}
	content := strings.Join(lines, "\n") + "\n"
	starts := make([]int, len(lines)+1)
	for i, l := range lines {
		starts[i+1] = starts[i] + len(l) + 1
	}
	plan := planPageMarks(root, "书.md", lines)
	if plan == nil {
		t.Fatal("plan must not be nil with mineru output present")
	}
	// 图片引用行在 page 2；embed 锚应带 · P.2。
	refStart := starts[3]
	refEnd := refStart + len(lines[3])
	page := plan.pageAt(starts, refStart)
	if page != 2 {
		t.Fatalf("image ref line page = %d, want 2", page)
	}
	emb := embedBlockForRef("[IMG_TYPE: screenshot]\n一张图。", content[refStart:refEnd], page)
	shifts := []embedShift{{start: refStart, delta: len(emb) - (refEnd - refStart)}}
	nc := content[:refStart] + emb + content[refEnd:]
	nc, marks := plan.applyTo(nc, lines, starts, shifts)
	n := len(marks)
	if n != 2 {
		t.Fatalf("expected 2 markers (page 2 at line 2, page 3 at line 5), got %d:\n%s", n, nc)
	}
	if !strings.Contains(nc, "[Image]( 一张图。 · P.2 )") {
		t.Fatalf("image anchor missing page suffix:\n%s", nc)
	}
	if !strings.Contains(nc, "[P. 2]\n第二节 方法细节") {
		t.Fatalf("page-2 marker not before its first line:\n%s", nc)
	}
	if !strings.Contains(nc, "\n[P. 3]\n第二章 实验结果") {
		t.Fatalf("page-3 marker not before its first line:\n%s", nc)
	}
}

// TestPlanPageMarksWithoutMinerU 钉：没有 mineru 产物时 plan 为 nil、
// applyTo 原样返回——老数据静默跳过，绝不报错、绝不改内容。
func TestPlanPageMarksWithoutMinerU(t *testing.T) {
	if plan := planPageMarks(t.TempDir(), "书.md", []string{"a"}); plan != nil {
		t.Fatalf("empty dir must give nil plan, got %+v", plan)
	}
	// nil plan 的 applyTo 必须是安全 no-op。
	var plan *pageMarkPlan
	out, marks := plan.applyTo("原文", []string{"原文"}, []int{0, 3}, nil)
	n := len(marks)
	if n != 0 || out != "原文" {
		t.Fatalf("nil plan applyTo must be a no-op, got %q, %d marks", out, n)
	}
}

// 回归（2026-09-20 真实数据）：书名带 [a4]/[正度纸] 时 Glob 把方括号当
// 字符类，content_list 永远找不到 → 整本书 0 标记。ReadDir 版本必须认。
func TestPageContentListPathBracketName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "[a4] 某书_part1")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "abc_content_list.json"), []byte("[]"), 0o644)
	os.WriteFile(filepath.Join(dir, "abc_content_list_v2.json"), []byte("[]"), 0o644)
	got := pageContentListPath(dir)
	if got == "" || strings.Contains(got, "_v2") {
		t.Fatalf("got %q", got)
	}
}

// 回归（2026-09-20 真实数据）：目录行的"第三章 认识论……029"曾一路匹配到
// body 里的同名章节标题锚点，指针狂奔全书（P.2 后直接 P.243）。前向搜索
// 窗口必须把这种远距离误配挡掉——目录行继承未知页，不产生标记。
func TestAlignPagesWindowBlocksTOCRunaway(t *testing.T) {
	anchors := []pageAnchor{{kind: "text", text: "封面", page: 1}}
	// 200 个远端锚（模拟 body 章节标题），页码 2..201
	for i := 2; i <= 201; i++ {
		anchors = append(anchors, pageAnchor{kind: "text",
			text: pageNormalize(fmt.Sprintf("第%d章 认识论", i)), page: i})
	}
	lines := []string{"# 目录", "第三章 认识论……029", "正文开始"}
	pg := alignPages(lines, anchors)
	if pg[1] != 0 {
		t.Fatalf("目录行不该匹配到远端锚点: %v", pg)
	}
}
