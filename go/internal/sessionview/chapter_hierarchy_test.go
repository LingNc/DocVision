package sessionview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChapterIdentityForFlatAndLegacyNestedTranscripts(t *testing.T) {
	cases := []struct {
		path, id, parent, role string
		depth, order           int
	}{
		{"book/work/sessions/convert_chapter_011_child_02_child_03.jsonl", "chapter_011_child_02_child_03", "chapter_011_child_02", "convert", 2, 11},
		{"book/work/temp/conv_chapter_011/work/_subdivision/child_02/node/sessions/convert.jsonl", "chapter_011_child_02", "chapter_011", "convert", 1, 11},
		{"book/work/temp/conv_chapter_011/work/_subdivision/child_02/node/work/_subdivision/child_03/node/sessions/checker.jsonl", "chapter_011_child_02_child_03", "chapter_011_child_02", "checker", 2, 11},
		// T252：残片/快照副本带 `YYYYMMDD-HHMMSS-` 前缀，剥掉后才认得出章节身份。
		{"book/work/rescue/cuts/chapter_011/20261027-111514-convert_chapter_011_child_02_child_04.jsonl", "chapter_011_child_02_child_04", "chapter_011_child_02", "convert", 2, 11},
		// T257：切回时"目标已存在、不覆盖"的改名产物 `parked-<名>` 也要认得出章节。
		{"book/work/sessions/parked-convert_chapter_013.try2.jsonl", "chapter_013", "", "convert", 0, 13},
	}
	for _, tc := range cases {
		id := chapterIdentity(tc.path)
		if id.ID != tc.id || id.Parent != tc.parent || id.Role != tc.role || id.Depth != tc.depth {
			t.Errorf("chapterIdentity(%q) = %+v", tc.path, id)
		}
		if got := chapterOrderOf(strings.Split(id.ID, "_child_")[0]); got != tc.order {
			t.Errorf("chapter order for %q = %d, want %d", id.ID, got, tc.order)
		}
		if got := LabelFor(tc.path); got != tc.role+":"+tc.id {
			t.Errorf("LabelFor(%q) = %q", tc.path, got)
		}
	}
}

func TestReadStatRetainsChapterActivity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "convert_chapter_002.jsonl")
	data := []byte("{\"t\":\"meta\",\"kind\":\"chapter_activity\",\"state\":\"waiting_children\"}\n")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	stat, err := readStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if stat.ChapterActivity != "waiting_children" {
		t.Fatalf("activity = %q", stat.ChapterActivity)
	}
}
