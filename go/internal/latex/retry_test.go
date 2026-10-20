package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/retryq"
)

// queueRetryForTest 往项目的队列里放一条请求（与网页按钮同一条路）。
func queueRetryForTest(t *testing.T, proj, base string) error {
	t.Helper()
	_, err := retryq.Queue(proj, retryq.Request{Chapter: base, Role: "convert", Reason: "测试"})
	return err
}

// P53：重试 = 这一章开一个新的尝试——旧尝试整棵留档进
// work/attempts/<章>/try<N>/，提交的 .tex 被挪走（转换阶段才不会"已有产物，
// 跳过"），新尝试号比磁盘上的最大号大 1，转录文件名跟着变成 .try<N+1>。
func TestForceChapterRetryArchivesAndBumpsAttempt(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_002"
	for _, d := range []string{
		filepath.Join(proj, "work", "sessions"),
		filepath.Join(proj, "work", "chapters", base),
		filepath.Join(proj, "work", "reports"),
		filepath.Join(proj, "work", "temp", "conv_"+base, "work"),
		filepath.Join(proj, "chapters"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, text string) {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(proj, "chapters", base+".md"), "# 章\n\n正文\n")
	write(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"), `{"t":"msg"}`+"\n")
	write(filepath.Join(proj, "work", "chapters", base+".tex"), "\\chapter{x}\n")
	write(filepath.Join(proj, "work", "chapters", base, "p1.tex"), "p1\n")
	write(filepath.Join(proj, "work", "reports", base+".md"), "# report\n")
	write(filepath.Join(proj, "work", "temp", "conv_"+base, "work", "draft.tex"), "draft\n")

	r := testRunner(t, true)
	if n := r.forceChapterRetry(proj, base, "测试重试"); n != 2 {
		t.Fatalf("新尝试号 = %d, want 2", n)
	}
	if got := r.retryAttemptOf(base); got != 2 {
		t.Fatalf("retryAttemptOf = %d, want 2", got)
	}
	// 产物必须消失，否则转换阶段会跳过这一章。
	for _, p := range []string{
		filepath.Join(proj, "work", "chapters", base+".tex"),
		filepath.Join(proj, "work", "chapters", base),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s 应当已被挪走: %v", p, err)
		}
	}
	// 旧尝试整棵留档。
	tryDir := filepath.Join(proj, "work", "attempts", base, "try1")
	for _, p := range []string{
		filepath.Join(tryDir, base+".tex"),
		filepath.Join(tryDir, base, "p1.tex"),
		filepath.Join(tryDir, base+".report.md"),
		filepath.Join(tryDir, "work"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("留档缺失 %s: %v", p, err)
		}
	}
	// 转录留下：历史里两条都要在，新尝试用 .try2 名字。
	if _, err := os.Stat(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")); err != nil {
		t.Fatalf("旧转录不该被删: %v", err)
	}
	if got := attemptTranscriptPath(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"), 2); filepath.Base(got) != "convert_chapter_002.try2.jsonl" {
		t.Fatalf("新转录名 = %s", filepath.Base(got))
	}
	// 再来一次（例如用户连点两下）：留档进 try2，尝试号继续往上。
	if n := r.forceChapterRetry(proj, base, "又点了一次"); n != 3 {
		t.Fatalf("第二次新尝试号 = %d, want 3", n)
	}
	if _, err := os.Stat(filepath.Join(proj, "work", "attempts", base, "try2")); err != nil {
		t.Fatalf("第二次留档缺失: %v", err)
	}
}

// 队列里的请求被消费成"要重转的章节"，并把请求文件改名留档。
func TestPrepareRetriesConsumesQueue(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_003"
	for _, d := range []string{
		filepath.Join(proj, "work", "sessions"),
		filepath.Join(proj, "work", "chapters"),
		filepath.Join(proj, "chapters"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", base+".md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	if err := queueRetryForTest(t, proj, base); err != nil {
		t.Fatal(err)
	}
	if !r.hasPendingRetry(proj) {
		t.Fatal("队列里应当有请求")
	}
	bases := r.prepareRetries(proj)
	if len(bases) != 1 || bases[0] != base {
		t.Fatalf("prepareRetries = %v, want [%s]", bases, base)
	}
	if r.hasPendingRetry(proj) {
		t.Fatal("消费后队列应当空了")
	}
	if r.retryAttemptOf(base) != 2 {
		t.Fatalf("重试尝试号 = %d, want 2", r.retryAttemptOf(base))
	}
	// 章节 md 不在的请求只警告、不返回。
	if err := queueRetryForTest(t, proj, "chapter_999"); err != nil {
		t.Fatal(err)
	}
	if got := r.prepareRetries(proj); len(got) != 0 {
		t.Fatalf("不存在的章节应当被忽略, got %v", got)
	}
}

// selectOnlyChapters 保持原顺序，只留名单里的章节。
func TestSelectOnlyChapters(t *testing.T) {
	chapters := []string{"/p/chapters/chapter_001.md", "/p/chapters/chapter_002.md", "/p/chapters/chapter_003.md"}
	got := selectOnlyChapters(chapters, map[string]bool{"chapter_003": true, "chapter_001": true})
	if len(got) != 2 || filepath.Base(got[0]) != "chapter_001.md" || filepath.Base(got[1]) != "chapter_003.md" {
		t.Fatalf("selectOnlyChapters = %v", got)
	}
	if got := selectOnlyChapters(chapters, nil); len(got) != 3 {
		t.Fatalf("空名单应当原样返回, got %v", got)
	}
	if got := selectOnlyChapters(chapters, map[string]bool{"chapter_999": true}); len(got) != 0 {
		t.Fatalf("名单不匹配应当返回空, got %v", got)
	}
}
