package sessionview

import (
	"strings"
	"testing"
)

// T77：img2text 编号按图片走——同图的分析与升级修复成员共用一个 #N，
// 不同图片不会撞号。
func TestNumberImg2TextPerImage(t *testing.T) {
	s := &scanner{}
	// 项目 P：图片 a.jpg（分析+修复）、b.jpg（仅分析）
	s.img2Text = []img2TextEntry{
		{id: "img2text:sessions/m.md/a.jpg.jsonl", key: "a.jpg"},
		{id: "img2text:mermaid_fix/m.md/a.jpg/session-stage0.jsonl", key: "a.jpg"},
		{id: "img2text:sessions/m.md/b.jpg.jsonl", key: "b.jpg"},
	}
	out := []SessionInfo{
		{ID: "img2text:sessions/m.md/a.jpg.jsonl", Project: "img2text · m", Stage: "img2text"},
		{ID: "img2text:mermaid_fix/m.md/a.jpg/session-stage0.jsonl", Project: "img2text · m", Stage: "mermaid-fix"},
		{ID: "img2text:sessions/m.md/b.jpg.jsonl", Project: "img2text · m", Stage: "img2text"},
	}
	s.numberImg2Text(out)
	if out[0].ChapterOrder != 1 || out[1].ChapterOrder != 1 {
		t.Fatalf("同图两成员应同号: %d %d", out[0].ChapterOrder, out[1].ChapterOrder)
	}
	if out[2].ChapterOrder != 2 {
		t.Fatalf("b.jpg 应为 #2: %d", out[2].ChapterOrder)
	}
	if out[1].Title != "#1 · 升级修复 · a.jpg" || out[0].Title != "#1 · 逐图 · a.jpg" {
		t.Fatalf("标题错: %q %q", out[0].Title, out[1].Title)
	}
	if !strings.HasPrefix(out[2].Title, "#2 · 逐图 · ") {
		t.Fatalf("b 标题错: %q", out[2].Title)
	}
}
