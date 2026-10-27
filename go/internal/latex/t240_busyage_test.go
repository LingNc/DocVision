package latex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T240：排队提示里"这一章最后一次输出在多久之前"，必须把递归子章节的转录算进去。
//
// 子章节的会话写在父章节的临时工作区 `<proj>/work/temp/conv_<父章>/work/
// _subdivision/…/node/sessions/` 下，不在 `<proj>/work/sessions/`。父节点停在
// 「等待子会话」时自己一行都不写，只看顶层目录就会把一个正在猛跑的子树报成
// "卡住了、只能重启"。
func TestChapterLastOutputAgeSeesChildSessions(t *testing.T) {
	proj := t.TempDir()
	sessDir := filepath.Join(proj, "work", "sessions")
	childDir := filepath.Join(proj, "work", "temp", "conv_chapter_013", "work",
		"_subdivision", "child_02", "node", "sessions")
	for _, d := range []string{sessDir, childDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 父章节自己的转录三小时前就不动了（它在等子会话）。
	parent := filepath.Join(sessDir, "convert_chapter_013.try2.jsonl")
	if err := os.WriteFile(parent, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(parent, old, old); err != nil {
		t.Fatal(err)
	}
	// 子章节的核对会话刚刚还在写。
	child := filepath.Join(childDir, "checker_chapter_013_child_02.try8.jsonl")
	if err := os.WriteFile(child, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Runner{}
	if age := r.chapterLastOutputAge(proj, "chapter_013"); age < 0 || age > time.Minute {
		t.Fatalf("父章节的输出年岁应该被子章节的转录刷新到 1 分钟内，实际 %v", age)
	}
	// 直接问子章节自己也要能看见（它的转录同样不在顶层目录里）。
	if age := r.chapterLastOutputAge(proj, "chapter_013_child_02"); age < 0 || age > time.Minute {
		t.Fatalf("子章节自己的输出年岁应该有值（1 分钟内），实际 %v", age)
	}
	// 子树里这条属于别的章节，不该被算成 chapter_013 的输出。
	if age := r.chapterLastOutputAge(proj, "chapter_014"); age > time.Minute {
		t.Fatalf("不相干的章节不该被这条转录刷新，实际 %v", age)
	}
}

// 完全没有痕迹时返回 -1（页面文案用得上"还没看到这一章的任何输出"）。
func TestChapterLastOutputAgeUnknownChapter(t *testing.T) {
	if age := (&Runner{}).chapterLastOutputAge(t.TempDir(), "chapter_099"); age != -1 {
		t.Fatalf("没有痕迹时应返回 -1，实际 %v", age)
	}
}

func TestTopChapterOf(t *testing.T) {
	cases := map[string]string{
		"chapter_013":                   "chapter_013",
		"chapter_004_child_02":          "chapter_004",
		"chapter_004_child_02_child_02": "chapter_004",
	}
	for in, want := range cases {
		if got := topChapterOf(in); got != want {
			t.Fatalf("topChapterOf(%q) = %q，想要 %q", in, got, want)
		}
	}
}
