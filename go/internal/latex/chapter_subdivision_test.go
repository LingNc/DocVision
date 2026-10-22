package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestChapterSlotsShareConfiguredLimit(t *testing.T) {
	r := keepTestRunner(t)
	r.cfg.Latex.Concurrency = 2
	var active, maximum atomic.Int32
	ready := make(chan struct{}, 8)
	releaseAll := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := r.acquireChapterSlot()
			n := active.Add(1)
			for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
			}
			ready <- struct{}{}
			<-releaseAll
			active.Add(-1)
			release()
		}()
	}
	<-ready
	<-ready
	close(releaseAll)
	wg.Wait()
	if maximum.Load() > 2 {
		t.Fatalf("maximum concurrent chapter sessions = %d, want <= 2", maximum.Load())
	}
}

func TestChapterSubrangePreservesExactLines(t *testing.T) {
	lines := []string{"# A", "text", "", "# B", "tail", ""}
	got := chapterSubrange(lines, ChapterSubdivision{StartLine: 2, EndLine: 4})
	if got != "text\n\n# B" {
		t.Fatalf("chapterSubrange = %q", got)
	}
}

func TestWriteChapterChildrenManifestListsOrderedArtifacts(t *testing.T) {
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "_subdivision"), 0o755); err != nil {
		t.Fatal(err)
	}
	children := []ChapterSubdivision{{Title: "first", StartLine: 1, EndLine: 2}, {Title: "second", StartLine: 3, EndLine: 4}}
	if err := writeChapterChildrenManifest(work, "chapter_01", children); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(work, "_subdivision", "children-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	first, second := strings.Index(text, "first"), strings.Index(text, "second")
	if first < 0 || second <= first || !strings.Contains(text, "chapter_01/children/chapter_01_child_01.tex") || !strings.Contains(text, "_subdivision/child_02/node/sessions/checker.jsonl") {
		t.Fatalf("manifest missing ordered artifact paths: %s", text)
	}
}

func TestValidateChapterSubdivisionsRequiresExactCoverage(t *testing.T) {
	valid := []ChapterSubdivision{
		{Title: "opening", StartLine: 1, EndLine: 3},
		{Title: "main", StartLine: 4, EndLine: 7},
	}
	if err := validateChapterSubdivisions(valid, 7, 7); err != nil {
		t.Fatalf("valid partition rejected: %v", err)
	}
	cases := []struct {
		name string
		list []ChapterSubdivision
	}{
		{"gap", []ChapterSubdivision{{Title: "a", StartLine: 1, EndLine: 2}, {Title: "b", StartLine: 4, EndLine: 7}}},
		{"overlap", []ChapterSubdivision{{Title: "a", StartLine: 1, EndLine: 4}, {Title: "b", StartLine: 4, EndLine: 7}}},
		{"tail omitted", []ChapterSubdivision{{Title: "a", StartLine: 1, EndLine: 3}, {Title: "b", StartLine: 4, EndLine: 6}}},
		{"empty title", []ChapterSubdivision{{Title: " ", StartLine: 1, EndLine: 3}, {Title: "b", StartLine: 4, EndLine: 7}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateChapterSubdivisions(tc.list, 7, 7); err == nil {
				t.Fatal("invalid partition accepted")
			}
		})
	}
}

// T212：文件末尾的换行会让 len(Split(content,"\n")) 多出一行空行（1671 行的
// 章节数出 1672 行）。模型按自己看到的正文切到 1671，旧判据"必须盖到 1672"就
// 把它打回——实测 173 次调用里 21 次第一次是这样撞回来的。正文盖满即可。
func TestValidateChapterSubdivisionsAllowsTrailingBlankLine(t *testing.T) {
	// 8 行 = 7 行正文 + 末尾空行：切到 7 算盖满。
	cover := []ChapterSubdivision{
		{Title: "a", StartLine: 1, EndLine: 3},
		{Title: "b", StartLine: 4, EndLine: 7},
	}
	if err := validateChapterSubdivisions(cover, 7, 8); err != nil {
		t.Fatalf("正文盖满却被拒: %v", err)
	}
	// 显式盖到含空行的最后一行也接受。
	withBlank := []ChapterSubdivision{
		{Title: "a", StartLine: 1, EndLine: 3},
		{Title: "b", StartLine: 4, EndLine: 8},
	}
	if err := validateChapterSubdivisions(withBlank, 7, 8); err != nil {
		t.Fatalf("末尾空行并入最后一段却被拒: %v", err)
	}
	// 真漏了一行正文（正文止于 8，只切到 7）仍然拒。
	short := []ChapterSubdivision{
		{Title: "a", StartLine: 1, EndLine: 3},
		{Title: "b", StartLine: 4, EndLine: 7},
	}
	if err := validateChapterSubdivisions(short, 8, 8); err == nil {
		t.Fatal("漏了一行正文却放行")
	}
}

func TestLastContentLine(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"a\nb\n", 2},
		{"a\nb", 2},
		{"a\nb\n\n\n", 2},
		{"\n\n", 1},
		{"a", 1},
	}
	for _, c := range cases {
		if got := lastContentLine(strings.Split(c.text, "\n")); got != c.want {
			t.Errorf("lastContentLine(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

func TestRequestChapterSplitPersistsStructuredPlan(t *testing.T) {
	root := t.TempDir()
	tool := &RequestChapterSplitTool{Root: root, TotalLine: 4, LastContent: 4, Depth: 1}
	result, err := tool.Execute(`{"children":[{"title":"a","start_line":1,"end_line":2},{"title":"b","start_line":3,"end_line":4}]}`)
	if err != nil || !tool.Requested || !result.Terminal {
		t.Fatalf("Execute = (%+v, %v), requested=%v", result, err, tool.Requested)
	}
	var got struct {
		Depth    int                  `json:"depth"`
		Children []ChapterSubdivision `json:"children"`
	}
	data, err := os.ReadFile(filepath.Join(root, "subdivision.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Depth != 2 || len(got.Children) != 2 || got.Children[1].StartLine != 3 {
		t.Fatalf("persisted plan = %+v", got)
	}
}

func TestRequestChapterSplitRestoresPersistedPlan(t *testing.T) {
	root := t.TempDir()
	first := &RequestChapterSplitTool{Root: root, TotalLine: 4, LastContent: 4, Depth: 0}
	if _, err := first.Execute(`{"children":[{"title":"a","start_line":1,"end_line":2},{"title":"b","start_line":3,"end_line":4}]}`); err != nil {
		t.Fatal(err)
	}
	resumed := &RequestChapterSplitTool{Root: root, TotalLine: 4, LastContent: 4, Depth: 0}
	if err := resumed.Restore(); err != nil {
		t.Fatal(err)
	}
	if !resumed.Requested || len(resumed.Children) != 2 || resumed.Children[1].EndLine != 4 {
		t.Fatalf("restored plan = %+v", resumed)
	}
}

func TestRequestChapterSplitRejectsAtDepthLimit(t *testing.T) {
	tool := &RequestChapterSplitTool{Root: t.TempDir(), TotalLine: 2, LastContent: 2, Depth: maxChapterSplitDepth}
	result, err := tool.Execute(`{"children":[{"title":"a","start_line":1,"end_line":1},{"title":"b","start_line":2,"end_line":2}]}`)
	if err != nil || result.Terminal || tool.Requested {
		t.Fatalf("depth-limit result = %+v, err=%v, requested=%v", result, err, tool.Requested)
	}
}
