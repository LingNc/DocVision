package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T319：纸张尺寸别让样式会话猜。书自己声明的开本（版权页「开本 787×1092 1/16」
// 这类）优先于 PDF 页框——扫描常带白边或被缩放，PDF 尺寸未必等于成品开版。
func TestTrimSizeSnippets(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string // 期望命中的片段（空 = 不该命中）
	}{
		{"版权页开本", "责任编辑 张三\n开本 787×1092 1/16\n印张 12.5 字数 300千字", "开本"},
		{"成品尺寸 mm", "成品尺寸：185mm×260mm\n版次 2019年8月第1版", "185mm×260mm"},
		{"裸的三位数乘", "封面\n787×1092\n定价 39.80 元", "787×1092"},
		{"正文里没有尺寸", "第一章 函数与极限\n函数是数学分析的研究对象。", ""},
	} {
		got := strings.Join(trimSizeSnippets(tc.text), " | ")
		if tc.want == "" {
			if got != "" {
				t.Fatalf("%s：不该命中，实际 %q", tc.name, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%s：应当命中 %q，实际 %q", tc.name, tc.want, got)
		}
	}
	// 版权页整行里本来就带着 850×1168，别把同一条信息拆成两行塞进去。
	if got := trimSizeSnippets("前 言\n开本 850×1168 1/32 印张 12.75 字数 300 000\nISBN 7-04-004077-8"); len(got) > 1 && strings.Contains(got[0], "850×1168") && strings.Contains(got[1], "850×1168") {
		t.Fatalf("同一行规格被拆成两条：%q", got)
	}
	// 只给两条：版权页往往连着好几行规格，全塞进提示词反而淹了重点。
	if n := len(trimSizeSnippets("开本 787×1092 1/16\n尺寸 185mm×260mm\n印张 12.5")); n > 2 {
		t.Fatalf("最多两条，实际 %d", n)
	}
	if got := trimSizeSnippets(""); got != nil {
		t.Fatalf("空文本应当返回 nil，实际 %v", got)
	}
}

// 量不到时必须安静降级：样式阶段不能因为读不到 PDF 尺寸就失败。
func TestBookPageBoxesDegradesQuietly(t *testing.T) {
	r := &Runner{}
	if got := r.bookPageBoxes(); got != nil {
		t.Fatalf("没有 pdfView 时应当返回 nil，实际 %v", got)
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "not-a.pdf")
	if err := os.WriteFile(fake, []byte("not a pdf"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.pdfView = &pdfView{Files: []pdfViewFile{{Name: "book_part1.pdf", PDF: fake, Count: 10}}}
	if got := r.bookPageBoxes(); len(got) != 0 {
		t.Fatalf("量不出来时应当返回空，实际 %v", got)
	}
}
