package sessionview

import (
	"strings"
	"testing"
)

// P58：复盘会话（rescue_<章>…jsonl）落在自己的阶段键上，排在侧栏最后。
// P58 补起这只是**兜底**：能拿到章节身份的复盘（新老命名都算，见 Scan 的
// rescueLegacyChapter）归到章节转换组、在章节树里当复盘行；只有转录自己的 meta
// 也给不出章节的老转录才留在这个「错误处理」组。
func TestRescueTranscriptHasItsOwnStage(t *testing.T) {
	rel := "proj/work/sessions/rescue_chapter_004_20261022-101010.jsonl"
	label := LabelFor(rel)
	if !strings.HasPrefix(label, "rescue:") {
		t.Fatalf("LabelFor(%q) = %q，应以 rescue: 开头", rel, label)
	}
	if got := StageTitle("rescue"); got != "错误处理" {
		t.Fatalf("StageTitle(rescue) = %q，应为「错误处理」", got)
	}
	if StageRank("rescue") <= StageRank("img2text") {
		t.Fatalf("rescue 阶段应排在最后：rank(rescue)=%d rank(img2text)=%d", StageRank("rescue"), StageRank("img2text"))
	}
	s := SessionInfo{ID: rel, Label: label}
	if got := StageOf(s); got != "rescue" {
		t.Fatalf("StageOf = %q，应为 rescue", got)
	}
}
