package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeChapterIssuesInheritsAndResolves 钉住跨层合并语义：子章节的条目原样
// 继承、父章节自己新报的追加、写进 resolved 的划掉，且同一句话不会重复。
func TestMergeChapterIssuesInheritsAndResolves(t *testing.T) {
	child := []ChapterIssue{
		{ID: "chapter_004_child_01#1", Source: "chapter_004_child_01", Depth: 1, Text: "证明环境缺 \\qedhere", Status: issueOpen},
		{ID: "chapter_004_child_02#1", Source: "chapter_004_child_02", Depth: 1, Text: "长表格超出页宽", Status: issueOpen},
	}
	merged := mergeChapterIssues("chapter_004", 0,
		[]string{"图注字号与手册不一致", "证明环境缺 \\qedhere"},
		child, []string{"chapter_004_child_02#1"})

	byText := map[string]ChapterIssue{}
	for _, it := range merged {
		byText[it.Text] = it
	}
	if len(merged) != 3 {
		t.Fatalf("merged = %d 条, want 3（继承 2 + 新 1，重复的一句只留一条）: %+v", len(merged), merged)
	}
	if got := byText["长表格超出页宽"].Status; got != issueResolved {
		t.Errorf("resolved 的条目应被划掉: %q", got)
	}
	if got := byText["证明环境缺 \\qedhere"]; got.ID != "chapter_004_child_01#1" || got.Source != "chapter_004_child_01" {
		t.Errorf("继承条目必须保留原 ID/来源: %+v", got)
	}
	if got := byText["图注字号与手册不一致"]; got.ID != "chapter_004#1" || got.Depth != 0 {
		t.Errorf("自己的新条目用本节点 ID: %+v", got)
	}
	open := openIssues(merged)
	if len(open) != 2 {
		t.Fatalf("open = %d, want 2: %+v", len(open), open)
	}
}

// TestReportTextListAcceptsBothShapes：旧的一段字符串与新推荐的数组都要收，
// 数组里带 id 的对象也能读 text。
func TestReportTextListAcceptsBothShapes(t *testing.T) {
	if got := reportTextList("第一条\n- 第二条\n\n* 第三条"); len(got) != 3 || got[1] != "第二条" {
		t.Fatalf("string shape = %v", got)
	}
	arr := reportTextList([]any{"a", map[string]any{"text": "b"}})
	if len(arr) != 2 || arr[1] != "b" {
		t.Fatalf("array shape = %v", arr)
	}
	if got := reportTextList(nil); got != nil {
		t.Fatalf("nil = %v", got)
	}
}

// TestIssueStoreRoundTrip：store 是父节点继承的唯一来源，写读必须一致。
func TestIssueStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".issues", "issues.json")
	in := []ChapterIssue{{ID: "chapter_009#1", Source: "chapter_009", Text: "题号错位", Status: issueOpen}}
	if err := writeIssueStore(path, in); err != nil {
		t.Fatal(err)
	}
	got := readIssueStore(path)
	if len(got) != 1 || got[0].ID != "chapter_009#1" {
		t.Fatalf("round trip = %+v", got)
	}
	if err := writeIssueStore(filepath.Join(dir, "empty.json"), nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "empty.json")); err != nil || strings.TrimSpace(string(data)) != "[]" {
		t.Fatalf("空清单应写成 []: %q err=%v", string(data), err)
	}
}

// TestUnportableIncludes 钉住"提交产物必须自包含"的判据：指向临时树的
// \input/\includegraphics 一律抓出来，本目录/figures 的正常引用不动。
func TestUnportableIncludes(t *testing.T) {
	work := t.TempDir()
	base := "chapter_012"
	if err := os.MkdirAll(filepath.Join(work, base, "children"), 0o755); err != nil {
		t.Fatal(err)
	}
	main := "\\chapter{X}\n\\input{" + base + "/children/a.tex}\n\\includegraphics[width=\\linewidth]{figures/f.pdf}\n"
	if err := os.WriteFile(filepath.Join(work, base+".tex"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if bad := unportableIncludes(work, base); len(bad) != 0 {
		t.Fatalf("正常引用不该报: %v", bad)
	}
	part := "\\input{_subdivision/child_01/node/result/x.tex}\n"
	if err := os.WriteFile(filepath.Join(work, base, "children", "a.tex"), []byte(part), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := unportableIncludes(work, base)
	if len(bad) != 1 || !strings.Contains(bad[0], "_subdivision") || !strings.Contains(bad[0], "a.tex") {
		t.Fatalf("应指出文件与坏路径: %v", bad)
	}
}
