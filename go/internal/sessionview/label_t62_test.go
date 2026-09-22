package sessionview

import "testing"

// T62：装配期会话不再落「会话」桶——book_fix/final_review 各成阶段，
// stylefix_* 归样式修复。
func TestLabelForAssemblySessions(t *testing.T) {
	cases := []struct{ rel, label, title string }{
		{"work/sessions/book_fix.jsonl", "fix:book_fix", "全书修复"},
		{"work/sessions/final_review.jsonl", "final-review:final_review", "终审"},
		{"work/sessions/stylefix_integrate.jsonl", "style-fix:integrate", "样式修复 · integrate"},
		{"work/sessions/stylefix_style_r1.jsonl", "style-fix:style_r1", "样式修复 · style_r1"},
		{"work/sessions/stylefix_assess_r1.jsonl", "style-fix:assess_r1", "样式修复 · assess_r1"},
		{"work/sessions/stylefix-blk-r1-2.jsonl", "style-fix:blk-r1-2", "样式修复 · blk-r1-2"},
		{"work/sessions/convert_chapter_001.jsonl", "convert:chapter_001", "转换 · chapter_001"},
	}
	for _, c := range cases {
		if got := LabelFor(c.rel); got != c.label {
			t.Errorf("LabelFor(%q) = %q, want %q", c.rel, got, c.label)
		}
		if got := TitleFor(c.rel); got != c.title {
			t.Errorf("TitleFor(%q) = %q, want %q", c.rel, got, c.title)
		}
	}
}
