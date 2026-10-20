package latex

import (
	"strings"
	"testing"
)

// TestChapterSizeNote 钉住"首条消息就说清章节多大"：小章节只报数字，超大章节
// 直接要求先拆分并给出每段行数区间，最深一层则说明只能在这里尽力而为。
func TestChapterSizeNote(t *testing.T) {
	if got := chapterSizeNote(0, 3000, 0); got != "" {
		t.Fatalf("没有行数时不该说任何话，得到 %q", got)
	}
	small := chapterSizeNote(600, 3000, 0)
	if !strings.Contains(small, "600") || strings.Contains(small, "request_chapter_split") {
		t.Fatalf("正常章节只报规模、不该催拆分: %q", small)
	}
	big := chapterSizeNote(9000, 3000, 0)
	for _, want := range []string{"9000", "request_chapter_split", "1000-1500"} {
		if !strings.Contains(big, want) {
			t.Errorf("超大章节的提示缺少 %q: %q", want, big)
		}
	}
	deep := chapterSizeNote(9000, 3000, maxChapterSplitDepth)
	if strings.Contains(deep, "request_chapter_split") || !strings.Contains(deep, "deepest") {
		t.Fatalf("到最深一层不该再催拆分: %q", deep)
	}
	if off := chapterSizeNote(9000, 0, 0); strings.Contains(off, "request_chapter_split") {
		t.Fatalf("上限为 0（关闭）时只报规模: %q", off)
	}
}

// TestChapterSizeReject 钉住这条硬规则的边界：超限且没拆分过才打回；
// 已经拆过（正在整合子章节）、到最深一层、或规则关闭时都放行。
func TestChapterSizeReject(t *testing.T) {
	if msg := chapterSizeReject(600, 3000, 0, false); msg != "" {
		t.Fatalf("没超限不该打回: %q", msg)
	}
	msg := chapterSizeReject(9000, 3000, 0, false)
	for _, want := range []string{"9000", "3000", "request_chapter_split"} {
		if !strings.Contains(msg, want) {
			t.Errorf("打回文案缺少 %q: %q", want, msg)
		}
	}
	if msg := chapterSizeReject(9000, 3000, 0, true); msg != "" {
		t.Fatalf("已拆分的节点（整合子章节）不该被打回: %q", msg)
	}
	if msg := chapterSizeReject(9000, 3000, maxChapterSplitDepth, false); msg != "" {
		t.Fatalf("最深一层拆不动，不该打回: %q", msg)
	}
	if msg := chapterSizeReject(9000, 0, 0, false); msg != "" {
		t.Fatalf("上限为 0 时规则关闭: %q", msg)
	}
}
