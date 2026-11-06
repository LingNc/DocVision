package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T290：会话看到的 project:source 只该有"书"本身——md 与 images/figures/tikz。
// 以前整棵 source 被软链进视图，作图会话转录、dedupe 产物、逐图进度全摊在
// 会话眼前（用户："那个project里面好像还有前面的会话什么的内容"）。
func TestProjectViewHidesSourceBookkeeping(t *testing.T) {
	proj := t.TempDir()
	src := filepath.Join(proj, "source")
	for _, d := range []string{"images", "figures", "tikz", "sessions", "dedupe", "progress_items"} {
		if err := os.MkdirAll(filepath.Join(src, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"book.md", "sessions/vector_a.jsonl", "dedupe/groups.json", "progress_items/x.json"} {
		if err := os.WriteFile(filepath.Join(src, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	view := ensureProjectView(proj)
	if view == "" {
		t.Fatal("视图没建起来")
	}
	vs := filepath.Join(view, "source")
	for _, want := range []string{"book.md", "images", "figures", "tikz"} {
		if _, err := os.Stat(filepath.Join(vs, want)); err != nil {
			t.Fatalf("视图里应当能看到 source/%s: %v", want, err)
		}
	}
	for _, hidden := range []string{"sessions", "dedupe", "progress_items"} {
		if _, err := os.Stat(filepath.Join(vs, hidden)); err == nil {
			t.Fatalf("视图里不该有 source/%s（流水账不进会话视野）", hidden)
		}
	}
	// 旧版本留下的整棵 source 软链要被清掉，否则下一次还会漏进来。
	_ = os.RemoveAll(filepath.Join(view, "source"))
	if err := os.Symlink(src, filepath.Join(view, "source")); err != nil {
		t.Fatal(err)
	}
	ensureProjectView(proj)
	if _, err := os.Stat(filepath.Join(view, "source", "sessions")); err == nil {
		t.Fatal("旧的整棵 source 软链应当被替换成分栏视图")
	}
}
