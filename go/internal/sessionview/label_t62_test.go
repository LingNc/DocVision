package sessionview

import "testing"

// T62：装配期会话不再落「会话」桶——book_fix/final_review 各成阶段，
// stylefix_* 归样式修复。
func TestLabelForAssemblySessions(t *testing.T) {
	cases := []struct{ rel, label, title string }{
		{"work/sessions/book_fix.jsonl", "fix:book_fix", "全书修复"},
		{"work/sessions/final_review.jsonl", "final-review:final_review", "终审"},
		{"work/sessions/stylefix_integrate.jsonl", "style-fix:integrate", "问题整合"},
		{"work/sessions/stylefix_style_r1.jsonl", "style-fix:style_r1", "样式修复 · 第1轮"},
		{"work/sessions/stylefix_assess_r1.jsonl", "style-fix:assess_r1", "评估分派 · 第1轮"},
		{"work/sessions/stylefix-blk-r1-2.jsonl", "style-fix:blk-r1-2", "分块修复 · 第1轮 · 块2"},
		{"work/sessions/convert_chapter_001.jsonl", "convert:chapter_001", "转换 · chapter_001"},
		// T110：试点三会话归样式阶段。
		{"work/sessions/style_pilot_select.jsonl", "style:pilot_select", "样式 · 试点选材"},
		{"work/sessions/style_pilot_trial.jsonl", "style:pilot_trial", "样式 · 试点试转"},
		{"work/sessions/style_pilot_revise.jsonl", "style:pilot_revise", "样式 · 试点修订"},
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
