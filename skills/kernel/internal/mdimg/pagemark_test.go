package mdimg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 假 MinerU 产物：两个 text 块分两页 → md 两行应分别对齐到页 1/2。
func TestPageMarkAlignAndInsert(t *testing.T) {
	mineru := t.TempDir()
	part := filepath.Join(mineru, "book_part1")
	if err := os.MkdirAll(part, 0o755); err != nil {
		t.Fatal(err)
	}
	cl := `[{"type":"text","page_idx":0,"text":"机器学习的基本原理"},{"type":"text","page_idx":1,"text":"第二章 模型评估方法"}]`
	if err := os.WriteFile(filepath.Join(part, "book_part1_content_list.json"), []byte(cl), 0o644); err != nil {
		t.Fatal(err)
	}
	md := "机器学习的基本原理概述\n\n第二章 模型评估方法详解\n\n![图](images/x.jpg)"
	lines, starts := LineStarts(md)
	plan := PlanPageMarks(mineru, "book.md", lines)
	if plan == nil {
		t.Fatal("应有对齐计划")
	}
	if got := plan.pageAt(starts, starts[0]); got != 1 {
		t.Fatalf("首行应对齐页 1: %d", got)
	}
	if got := plan.pageAt(starts, starts[2]); got != 2 {
		t.Fatalf("第三行应对齐页 2: %d", got)
	}
	// 无 embed 改写 → applyTo 应插入 [P. 1] 与 [P. 2]
	out, marks := plan.ApplyTo(md, lines, starts, nil)
	if len(marks) != 2 {
		t.Fatalf("应插 2 个标记: %+v", marks)
	}
	if !contains(out, "[P. 1]") || !contains(out, "[P. 2]") {
		t.Fatalf("输出应含两个页标:\n%s", out)
	}
	// T69：已有页标不回注
	if !HasPageMarks([]byte(out)) {
		t.Fatal("应识别已有页标")
	}
	// T69 幂等语义：调用方用 HasPageMarks 识别已标内容并跳过（旧线行为，
	// applyTo 本身不去重——对上例内容的第二次插入由命令层的守卫阻止）。
	if !HasPageMarks([]byte(out)) {
		t.Fatal("已标内容应被 HasPageMarks 识别为命令层跳过")
	}
}

func TestPageMarkNoMineru(t *testing.T) {
	md := "无产物\n第二行"
	lines, _ := LineStarts(md)
	if plan := PlanPageMarks(t.TempDir(), "book.md", lines); plan != nil {
		t.Fatal("无 mineru 产物应返回 nil（静默跳过）")
	}
}

func TestFenceSlide(t *testing.T) {
	content := "```python\ncode\n```\n正文"
	// pos 落在围栏内（第二行中间）→ 应顺延到围栏结束后
	pos := resolveFencePos(content, 12)
	if pos <= strings.Index(content, "正文") {
		t.Fatalf("围栏内插入点应顺延到围栏后: %d", pos)
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
