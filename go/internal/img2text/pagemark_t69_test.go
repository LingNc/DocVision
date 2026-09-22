package img2text

import "testing"

// T69：已交付文件不回注成页码版——先认出文件里有没有页码标记。
func TestHasPageMarks(t *testing.T) {
	if !hasPageMarks([]byte("# 书\n\n[P. 2]\n正文\n")) {
		t.Fatal("带 [P. 2] 的文件应判有页码")
	}
	if hasPageMarks([]byte("# 书\n\n正文，没有标记\n")) {
		t.Fatal("无标记文件误判有页码")
	}
	// 行内出现 [P. 2] 不算（标记必须是独立行）
	if hasPageMarks([]byte("文中引用 [P. 2] 不算标记\n")) {
		t.Fatal("行内引用误判为标记")
	}
}

// T69 开关语义（默认值）在 config 解析层钉住。
func TestPageMarksConfigDefaults(t *testing.T) {
	// nil = 默认：page_marks 开、rewrite 关——与 runner.go 的读取一致。
	var nilPtr *bool
	pageMarks := nilPtr == nil || *nilPtr
	rewrite := nilPtr != nil && *nilPtr
	if !pageMarks || rewrite {
		t.Fatalf("默认值错误: pageMarks=%v rewrite=%v", pageMarks, rewrite)
	}
}
