package latex

import (
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
)

// newT244Runner 是这一批测试用的最小 Runner：只要 log 与 cfg 就够。
func newT244Runner(t *testing.T, proj string) *Runner {
	t.Helper()
	log, err := logger.NewLogger(filepath.Join(proj, "run.log"), filepath.Join(proj, "error.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return &Runner{cfg: &config.Config{Latex: config.LatexConfig{ConvertConcurrency: 1}}, log: log}
}

func queueRestore(t *testing.T, proj, chapter string, attempt int) {
	t.Helper()
	if _, err := retryq.Queue(proj, retryq.Request{
		Chapter: chapter, Role: "restore", Attempt: attempt, Source: "web",
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPendingRestoreOnlyCountsReachableRequests：拿"有切回在等"当转换停止条件，前提
// 是这条请求**真能切**——否则这一章永远不跑、请求也永远派不下去，两边互相等死
// （T244）。所以只认 restore 角色、折回顶层章节名、留档确实在、而且指的不是现在
// 这一次。
func TestPendingRestoreOnlyCountsReachableRequests(t *testing.T) {
	proj, base := restoreFixture(t) // 现役第 3 次，第 1 次整棵留档
	r := newT244Runner(t, proj)

	if n, ok := r.pendingRestore(proj, base); ok {
		t.Fatalf("队列空的时候不该报告切回在等（拿到第 %d 次）", n)
	}
	// 别的角色不算。
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: base, Role: "continue"}); err != nil {
		t.Fatal(err)
	}
	if n, ok := r.pendingRestore(proj, base); ok {
		t.Fatalf("续跑请求不该被当成切回（拿到第 %d 次）", n)
	}
	// 没有留档的那一次切不回来，不能拿它拦着转换。
	queueRestore(t, proj, base, 7)
	if n, ok := r.pendingRestore(proj, base); ok {
		t.Fatalf("没有留档的切回不该拦住转换（拿到第 %d 次）", n)
	}
	// "切回现在这一次"永远切不成（前置条件会拒），同样不拦。
	queueRestore(t, proj, base, 3)
	if n, ok := r.pendingRestore(proj, base); ok {
		t.Fatalf("切回现役这一次不该拦住转换（拿到第 %d 次）", n)
	}
	// 真能切的：子章节名要折回顶层章节，子章节的尝试归父章节。
	queueRestore(t, proj, base+"_child_02_child_01", 1)
	n, ok := r.pendingRestore(proj, base)
	if !ok || n != 1 {
		t.Fatalf("父章节应当看到第 1 次的切回请求（拿到第 %d 次，%v）", n, ok)
	}
	if got, want := rootChapterOf(base+"_child_02_child_01"), base; got != want {
		t.Fatalf("子章节名应当折回 %q，实际 %q", want, got)
	}
	if _, ok := r.pendingRestore(proj, base+"_child_02"); !ok {
		t.Fatal("子树里的名字也应当看到父章节排着的切回")
	}
}

// TestConvertChapterNodeStopsWhenRestorePending：有切回在等时，这一章一个字节都不
// 碰就退出去——不建会话、不写转录、不写终态，只回一个"先停"的标记（T244）。这是
// 细分父章节能在一两分钟内把锁松开、让切回落地的前提。
func TestConvertChapterNodeStopsWhenRestorePending(t *testing.T) {
	proj, base := restoreFixture(t)
	r := newT244Runner(t, proj)
	queueRestore(t, proj, base, 1)

	chap := filepath.Join(proj, "chapters", base+".md")
	writeFile(t, chap, "# 一章\n\n正文\n")
	nodeRoot := filepath.Join(proj, "work", "temp", "conv_"+base)
	node := chapterNodeConfig{
		Base: base, Depth: 0,
		InputMount:      "work",
		InputRef:        "chapter.md",
		NodeRoot:        nodeRoot,
		TranscriptPath:  filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"),
		SubmitRoot:      filepath.Join(nodeRoot, "result"),
		FinalSubmitRoot: filepath.Join(proj, "work", "chapters"),
		ReportPath:      filepath.Join(nodeRoot, "reports", "conversion.md"),
		Seq:             1,
	}
	err := r.convertChapterNode(proj, "", "", chap, filepath.Join(nodeRoot, "work"), 1, 0, false, &node)
	if err == nil {
		t.Fatal("有切回在等时这一章应当先停，不该真的去转换")
	}
	if n, waiting := isRestoreWaiting(err); !waiting || n != 1 {
		t.Fatalf("应当返回「等切回第 1 次」的标记，实际：%v", err)
	}
	if strings.Contains(err.Error(), "会话失败") || strings.Contains(err.Error(), "转换失败") {
		t.Fatalf("先停不是失败，措辞里不该出现失败：%s", err.Error())
	}
	// 没有新转录、没有产物：整章什么都没动。
	if fileExists(filepath.Join(nodeRoot, "sessions", "convert_"+base+".try4.jsonl")) {
		t.Fatal("先停的那一次不该新建转录")
	}
	if pathExistsAny(filepath.Join(nodeRoot, "result")) {
		t.Fatal("先停的那一次不该产出工件")
	}
	// 子章节名走同一条路（它的尝试归父章节）。
	child := chapterNodeConfig{Base: base + "_child_01", Depth: 1, InputMount: "input", InputRef: "chapter.md",
		NodeRoot:       filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node"),
		TranscriptPath: filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node", "sessions", "convert_"+base+"_child_01.jsonl"),
		SubmitRoot:     filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node", "result"),
		ReportPath:     filepath.Join(nodeRoot, "work", "_subdivision", "child_01", "node", "reports", "conversion.md"),
		Seq:            2,
	}
	if _, waiting := isRestoreWaiting(r.convertChapterNode(proj, "", "", chap, filepath.Join(nodeRoot, "work"), 2, 1, false, &child)); !waiting {
		t.Fatal("子章节也应当因为父章节的切回请求先停")
	}
}

// TestConvertBatchSkipsChapterWaitingForRestore：批次派发前就跳过——不是失败、不是
// 暂停，单独记一份"这次没跑"的清单，别让上面报"全部转换完成"（T244）。
func TestConvertBatchSkipsChapterWaitingForRestore(t *testing.T) {
	proj, base := restoreFixture(t)
	r := newT244Runner(t, proj)
	queueRestore(t, proj, base, 1)
	chap := filepath.Join(proj, "chapters", base+".md")
	writeFile(t, chap, "# 一章\n\n正文\n")

	fails, pauses, skipped, fatal := r.convertChapterBatch(proj, "", "", filepath.Join(proj, "work"), []string{chap}, "[convert]")
	if fatal != nil {
		t.Fatalf("没有接口级致命错误: %v", fatal)
	}
	if len(fails) != 0 || len(pauses) != 0 {
		t.Fatalf("等切回不是失败也不是暂停：fails=%v pauses=%v", fails, pauses)
	}
	if len(skipped) != 1 || skipped[0] != base {
		t.Fatalf("应当记下这一章这次没跑，实际 %v", skipped)
	}
	// 请求还在队列里，等这一章停止写盘后由派发端真正执行。
	if files := retryq.Pending(proj); len(files) != 1 {
		t.Fatalf("切回请求应当还留着，实际 %d 条", len(files))
	}
}

// TestDispatchHoldsOtherRolesBehindRestore：这一章排着切回时，续跑/转换/核对先别
// 动手——它们一开跑就占住锁，切回又得从头等（T244）。
func TestDispatchHoldsOtherRolesBehindRestore(t *testing.T) {
	proj, base := restoreFixture(t)
	r := newT244Runner(t, proj)
	queueRestore(t, proj, base, 1)
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: base, Role: "continue", Source: "web"}); err != nil {
		t.Fatal(err)
	}
	// 占住这一章模拟"会话还在跑"：连切回自己也只能排队。
	release := r.holdChapter(base)
	n := r.dispatchRetries(proj)
	release()
	r.retryWatcherWG.Wait()
	if n != 0 {
		t.Fatalf("切回在等、章节又忙时不该派发任何请求，实际派发 %d 条", n)
	}
	if files := retryq.Pending(proj); len(files) != 2 {
		t.Fatalf("两条请求都应当留在队列里，实际 %d 条", len(files))
	}
}

// TestRestoreRequestAlreadySatisfiedIsConsumed：同一个按钮点两下，第二条切回的目标
// 就是第一轮搬完后的现役那一次——它永远排不上。以前这种请求会一直躺在队列里，
// 每 5 分钟重排一次、永远不消失；现在直接作废，只留一行说明（T244）。
func TestRestoreRequestAlreadySatisfiedIsConsumed(t *testing.T) {
	proj, base := restoreFixture(t) // 现役第 3 次
	r := newT244Runner(t, proj)
	queueRestore(t, proj, base, 3)

	if n := r.dispatchRetries(proj); n != 0 {
		t.Fatalf("目标已是现役那一次，不该派发，实际派发 %d 条", n)
	}
	r.retryWatcherWG.Wait()
	if files := retryq.Pending(proj); len(files) != 0 {
		t.Fatalf("作废的请求应当被收掉，实际还剩 %d 条", len(files))
	}
	// 现役产物一个字节都没动。
	if got := readFileAt(t, filepath.Join(proj, "work", "chapters", base+".tex")); got != "live-tex" {
		t.Fatalf("产物不该被动过: %q", got)
	}
}
