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
		"# 第一章 概述",     // text anchor 1 (contained)
		"",             // blank -> unknown
		"本章介绍基本概念与背景。", // exact hit
		"一些锚点之间没对上的自由文本",            // unknown, inherits page 1
		"第二节 方法细节",                  // page 2
		"![图](images/书/aaa111.jpg)", // image anchor, page 2
		"第二章 实验结果",                  // part1 page 0 -> global 3
		"```text",                   // fence line, no anchor
		"实验表明方法有效。",                 // page 4 (数字 page_idx 形态)
		"```",
	}
	pages := alignPages(lines, anchors)
	want := []int{1, 0, 1, 0, 2, 2, 3, 0, 4, 0}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("pages mismatch:\n got %v\nwant %v", pages, want)
	}
}

func TestPageMarkCoverage(t *testing.T) {
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
	cov := pageMarkCoverage(alignPages(lines, anchors))
	// 第 1 页在文首就说，之后 2/3/4 各在自己的第一行之前；未知行不给。
	want := [][]int{{1}, nil, nil, {2}, nil, {3}, {4}}
	if !reflect.DeepEqual(cov, want) {
		t.Fatalf("coverage mismatch:\n got %v\nwant %v", cov, want)
	}
	// T330 回归：中间隔着的那些页（空白页/纯图页对不上任何文本锚）必须
	// 补齐——页 1 之后直接跳到页 4，2 和 3 一个都不能少。
	if got := pageMarkCoverage([]int{1, 0, 0, 4}); !reflect.DeepEqual(got, [][]int{{1}, nil, nil, {2, 3, 4}}) {
		t.Fatalf("gap pages must be filled, got %v", got)
	}
	// 页码不递增（对齐抖动）时不再重复/回退标记。
	if got := pageMarkCoverage([]int{3, 2, 3}); !reflect.DeepEqual(got, [][]int{{1, 2, 3}, nil, nil}) {
		t.Fatalf("non increasing pages must not re-announce, got %v", got)
	}
}

// newTestEntry 按 runner 的口径造一条 mdEntry（lines/starts/content 同步）。
func newTestEntry(lines []string) mdEntry {
	content := strings.Join(lines, "\n")
	starts := make([]int, len(lines)+1)
	for i, l := range lines {
		starts[i+1] = starts[i] + len(l) + 1
	}
	return mdEntry{name: "书.md", content: content, lines: lines, starts: starts}
}

// TestRewriteFinalFenceDeferral 钉：页码标记落在代码围栏内时顺延到围栏结束后
// 的第一行之前（T330：单遍改写，不再有字节偏移平移）。
func TestRewriteFinalFenceDeferral(t *testing.T) {
	root, _ := testAnchors(t)
	lines := []string{
		"# 第一章 概述",                  // page 1
		"```python",                 // fence opens
		"x = \"第二节 方法细节\"",          // fence body 误中 text 锚（page 2）→ 顺延
		"```",                       // fence closes
		"![图](images/书/aaa111.jpg)", // page 2（image 锚，不递增）
		"第二章 实验结果",                  // page 3
	}
	e := newTestEntry(lines)
	plan := planPageMarks(root, "书.md", lines)
	got, marks := rewriteFinal(e, nil, plan)
	want := "[P. 1]\n# 第一章 概述\n```python\nx = \"第二节 方法细节\"\n```\n[P. 2]\n![图](images/书/aaa111.jpg)\n[P. 3]\n第二章 实验结果"
	if got != want {
		t.Fatalf("content mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if len(marks) != 3 {
		t.Fatalf("expected 3 markers, got %d", len(marks))
	}
	if strings.Contains(got, "```python\n[P. ") {
		t.Fatalf("marker fell inside the fence:\n%s", got)
	}
}

// TestRewriteFinalEndToEnd 走一遍 Final pass 的纯函数部分：图片锚带页码、
// 锚上方的 [url] 链接行、跨页处的标记，且**锚的页码与它上方的标记同源**。
func TestRewriteFinalEndToEnd(t *testing.T) {
	root, _ := testAnchors(t)
	lines := []string{
		"# 第一章 概述",
		"本章介绍基本概念与背景。",
		"第二节 方法细节",
		"![图](images/书/aaa111.jpg)",
		"自由文本",
		"第二章 实验结果",
	}
	e := newTestEntry(lines)
	plan := planPageMarks(root, "书.md", lines)
	if plan == nil {
		t.Fatal("plan must not be nil with mineru output present")
	}
	refStart := e.starts[3]
	reps := []finalRep{{
		line:  3,
		start: 0,
		end:   len(lines[3]),
		ref:   e.content[refStart : refStart+len(lines[3])],
		res:   "[IMG_TYPE: screenshot]\n一张图。",
	}}
	got, marks := rewriteFinal(e, reps, plan)
	if len(marks) != 3 {
		t.Fatalf("expected 3 markers (pages 1,2,3), got %d:\n%s", len(marks), got)
	}
	// T329：原图链接行在描述锚之上。
	if !strings.Contains(got, "[url](images/书/aaa111.jpg)\n[Image]( 一张图。 · P.2 )") {
		t.Fatalf("embed must carry the link line above the anchor:\n%s", got)
	}
	if !strings.Contains(got, "[P. 1]\n# 第一章 概述") {
		t.Fatalf("page-1 marker missing at the top:\n%s", got)
	}
	if !strings.Contains(got, "\n[P. 2]\n第二节 方法细节") {
		t.Fatalf("page-2 marker not before its first line:\n%s", got)
	}
	if !strings.Contains(got, "\n[P. 3]\n第二章 实验结果") {
		t.Fatalf("page-3 marker not before its first line:\n%s", got)
	}
}

// TestRewriteFinalCoverageFillsBlankPages 钉 T330 的核心回归：老算法只在
// "页码上升"时打标记，整页空白/整页图那种对不上任何文本锚的页会被整个跳过
// （实跑抓到缺 P.4）。新算法把中间跳过的页全部补齐。
func TestRewriteFinalCoverageFillsBlankPages(t *testing.T) {
	lines := []string{"第一页正文", "第四页正文"}
	e := newTestEntry(lines)
	plan := &pageMarkPlan{linePages: []int{1, 4}}
	got, marks := rewriteFinal(e, nil, plan)
	want := "[P. 1]\n第一页正文\n[P. 2]\n[P. 3]\n[P. 4]\n第四页正文"
	if got != want {
		t.Fatalf("gap pages must be announced:\ngot:\n%s\nwant:\n%s", got, want)
	}
	a := plan.audit(marks)
	if a.Missing != 0 || a.MaxPage != 4 || a.Backward != 0 {
		t.Fatalf("audit after filling gaps = %+v, want no missing page", a)
	}
}

// TestPlanPageMarksWithoutMinerU 钉：没有 mineru 产物时 plan 为 nil、
// rewriteFinal 原样返回——老数据静默跳过，绝不报错、绝不改内容。
func TestPlanPageMarksWithoutMinerU(t *testing.T) {
	if plan := planPageMarks(t.TempDir(), "书.md", []string{"a"}); plan != nil {
		t.Fatalf("empty dir must give nil plan, got %+v", plan)
	}
	// nil plan 的 rewriteFinal 必须是安全 no-op。
	lines := []string{"原文", "第二行"}
	e := newTestEntry(lines)
	out, marks := rewriteFinal(e, nil, nil)
	if len(marks) != 0 || out != e.content {
		t.Fatalf("nil plan must be a no-op, got %q, %d marks", out, len(marks))
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
