package latex

import (
	"os"
	"path/filepath"
	"strings"
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

// checker 角色：只换核对尝试号，产物一个字不动；旧结论留档 .checker.try<N>。
func TestForceCheckerRetryKeepsProduct(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_005"
	for _, d := range []string{
		filepath.Join(proj, "work", "sessions"),
		filepath.Join(proj, "work", "chapters"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	texPath := filepath.Join(proj, "work", "chapters", base+".tex")
	for _, p := range []string{
		texPath,
		filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl"),
	} {
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(texPath+".checker", []byte("第 3 页缺一段\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	if n := r.forceCheckerRetry(proj, base); n != 2 {
		t.Fatalf("核对尝试号 = %d, want 2", n)
	}
	// 产物不能被动：重跑的是"再看一遍"，不是"重做一遍"。
	if _, err := os.Stat(texPath); err != nil {
		t.Fatalf("重新核对不该动产物: %v", err)
	}
	// 旧结论留档，否则新会话覆盖 `.checker` 就没了。
	data, err := os.ReadFile(texPath + ".checker.try1")
	if err != nil || string(data) != "第 3 页缺一段\n" {
		t.Fatalf("旧结论留档 = %q (%v)", string(data), err)
	}
	// 连点两下：第二次继续往上，不撞回 try2。
	if n := r.forceCheckerRetry(proj, base); n != 3 {
		t.Fatalf("第二次核对尝试号 = %d, want 3", n)
	}
}

// 队列按角色分流：转换请求不会被"重新核对"那一轮吃掉，反之亦然。
func TestRetryQueueSplitsByRole(t *testing.T) {
	proj := t.TempDir()
	for _, d := range []string{filepath.Join(proj, "work", "sessions"), filepath.Join(proj, "work", "chapters"), filepath.Join(proj, "chapters")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_001.md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: "chapter_001", Role: "convert"}); err != nil {
		t.Fatal(err)
	}
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: "chapter_002", Role: "checker"}); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	// checker 轮：只有 checker 请求被拿走；chapter_002 没产物 → 忽略但已消费。
	if got := r.prepareCheckerRetries(proj); len(got) != 0 {
		t.Fatalf("没产物的重新核对应被忽略, got %v", got)
	}
	if got := r.prepareRetries(proj); len(got) != 1 || got[0] != "chapter_001" {
		t.Fatalf("convert 轮 = %v, want [chapter_001]", got)
	}
	if r.hasPendingRetry(proj) {
		t.Fatal("两条请求都应已消费")
	}
}

// 没有产物时重新核对直接说清楚，而不是空跑一个会话。
func TestRecheckWithoutProduct(t *testing.T) {
	proj := t.TempDir()
	r := testRunner(t, true)
	ok, issues := r.recheckChapter(proj, "chapter_009", 1)
	if ok || !strings.Contains(issues, "还没有提交产物") {
		t.Fatalf("recheckChapter = (%v, %q)", ok, issues)
	}
}

// P53 阶段二：章节锁——忙的章节不消费请求（留在队列里等下一次 tick），拿锁是非阻塞的，
// 主流程那边是阻塞拿锁，所以同一章永远不会被跑两遍。
func TestChapterLockAndBusyDispatch(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_002.md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := retryq.Queue(proj, retryq.Request{Chapter: "chapter_002", Role: "convert"}); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	release := r.holdChapter("chapter_002")
	if r.lockChapter("chapter_002") {
		t.Fatal("已持锁时 TryLock 应失败")
	}
	if n := r.dispatchRetries(proj); n != 0 {
		t.Fatalf("忙的章节不应派发, started = %d", n)
	}
	if pending := retryq.Pending(proj); len(pending) != 1 {
		t.Fatalf("请求应留在队列里, pending = %d", len(pending))
	}
	release()
	if !r.lockChapter("chapter_002") {
		t.Fatal("释放后应能拿到锁")
	}
	r.unlockChapter("chapter_002")
}

// 前置条件：样式没就绪、没有产物、没有转录分别给出说明（请求留队而不是被吃掉）。
func TestRetryPrecondition(t *testing.T) {
	proj := t.TempDir()
	for _, d := range []string{
		filepath.Join(proj, "chapters"), filepath.Join(proj, "style"),
		filepath.Join(proj, "work", "sessions"), filepath.Join(proj, "work", "chapters"),
		filepath.Join(proj, "work", "temp", "conv_chapter_002", "work"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", "chapter_002.md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	if why := r.retryPrecondition(proj, "convert", "chapter_002"); !strings.Contains(why, "样式") {
		t.Fatalf("没有 cls 时应说样式未就绪, got %q", why)
	}
	// 补一个 cls（内容不重要，只要能被 classNameOfFile 认出来）
	if err := os.WriteFile(filepath.Join(proj, "style", "book.cls"), []byte("\\ProvidesClass{book}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if why := r.retryPrecondition(proj, "checker", "chapter_002"); !strings.Contains(why, "产物") {
		t.Fatalf("没有 .tex 时重新核对应说没产物, got %q", why)
	}
	if why := r.retryPrecondition(proj, "continue", "chapter_002"); !strings.Contains(why, "转录") {
		t.Fatalf("没有转录时继续跑应说没记录, got %q", why)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "sessions", "convert_chapter_002.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if why := r.retryPrecondition(proj, "continue", "chapter_002"); why != "" {
		t.Fatalf("有转录与工作区时应放行, got %q", why)
	}
	if err := os.WriteFile(filepath.Join(proj, "work", "chapters", "chapter_002.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if why := r.retryPrecondition(proj, "checker", "chapter_002"); why != "" {
		t.Fatalf("有产物时重新核对应放行, got %q", why)
	}
}

// 继续跑的提示只对下一次转换有效，跑完必须清掉，免得普通运行也当成手动继续。
func TestContinueTextScoped(t *testing.T) {
	r := testRunner(t, true)
	r.setContinueFlag("chapter_002")
	if !r.takeContinueFlag("chapter_002") {
		t.Fatal("手动继续的标记应被取到")
	}
	if r.takeContinueFlag("chapter_002") {
		t.Fatal("标记取了就应清掉")
	}
}

// 从某条记录之后继续（P53 阶段二）：前 from 行抄成新尝试，后面的轮次丢掉，源转录不动。
func TestTruncateChapterTranscript(t *testing.T) {
	proj := t.TempDir()
	dir := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "convert_chapter_002.jsonl")
	body := `{"ts":"1","t":"meta","kind":"system"}
{"ts":"2","t":"msg","role":"user","content":"先做这件事"}
{"ts":"3","t":"msg","role":"assistant","content":"好"}
{"ts":"4","t":"usage","kind":"request"}
{"ts":"5","t":"msg","role":"user","content":"从这里开始跑飞了"}
{"ts":"6","t":"msg","role":"assistant","content":"乱改一通"}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	dst, next, err := r.truncateChapterTranscript(proj, "chapter_002", "convert_chapter_002.jsonl", 3)
	if err != nil {
		t.Fatal(err)
	}
	if next != 2 || filepath.Base(dst) != "convert_chapter_002.try2.jsonl" {
		t.Fatalf("新尝试 = %s (号 %d)", filepath.Base(dst), next)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("新转录应只留 3 行, got %d: %q", len(lines), string(data))
	}
	// 源转录一个字不动（原记录还留在「历史」里）。
	keep, _ := os.ReadFile(src)
	if string(keep) != body {
		t.Fatal("源转录被改动了")
	}
	// 再切一次：继续往上递增，不覆盖上一次。
	if _, next2, err := r.truncateChapterTranscript(proj, "chapter_002", "convert_chapter_002.jsonl", 2); err != nil || next2 != 3 {
		t.Fatalf("第二次截断 = %d (%v), want 3", next2, err)
	}
	// 切在用量行 / 超出范围 / 非法文件名都要拒绝。
	if _, _, err := r.truncateChapterTranscript(proj, "chapter_002", "convert_chapter_002.jsonl", 4); err == nil {
		t.Fatal("切在用量行应拒绝")
	}
	if _, _, err := r.truncateChapterTranscript(proj, "chapter_002", "convert_chapter_002.jsonl", 99); err == nil {
		t.Fatal("超出范围应拒绝")
	}
	if _, _, err := r.truncateChapterTranscript(proj, "chapter_002", "../x.jsonl", 1); err != nil {
		// "../x.jsonl" 会被 Base 成 x.jsonl，读不到 → 报错，这是期望行为
		if !strings.Contains(err.Error(), "读不到") {
			t.Fatalf("应报读不到, got %v", err)
		}
	}
}

// 重画一张图（P53 矢量图侧）：旧转录留档成 .tryN、进度记录清成品但保留类别，
// 类别/分组不动；副本图拒绝（重画要落在代表图上）。
func TestPrepareVectorRedraw(t *testing.T) {
	proj := t.TempDir()
	progDir := filepath.Join(proj, "source", "progress_items")
	sessDir := filepath.Join(proj, "source", "sessions")
	if err := os.MkdirAll(progDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := &imageProgress{
		Key: "m.md::images/b/pic001.jpg", MDName: "m.md", ImgPath: "images/b/pic001.jpg",
		Class: ClassVector, Status: "done", Label: "系统结构图",
		TikzCode: "\\draw", FigPDF: "figures/x.pdf", FigPNG: "figures/x.png", FigSVG: "figures/x.svg",
	}
	texBase := vectorTexBase(rec.MDName, rec.ImgPath, rec.Label)
	if texBase == "" || !strings.Contains(texBase, "pic001") {
		t.Fatalf("成品名 = %q", texBase)
	}
	saveProgress(progDir, rec)
	trName := "vector_" + texBase + ".jsonl"
	if err := os.WriteFile(filepath.Join(sessDir, trName), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	key, got, archived, err := prepareVectorRedraw(proj, trName, 0)
	if err != nil {
		t.Fatal(err)
	}
	if key != rec.Key || got == nil {
		t.Fatalf("key = %q, rec = %v", key, got)
	}
	if archived != "vector_"+texBase+".try1.jsonl" {
		t.Fatalf("留档名 = %q", archived)
	}
	if fileExists(filepath.Join(sessDir, trName)) {
		t.Fatal("旧转录应已改名留档")
	}
	if !fileExists(filepath.Join(sessDir, archived)) {
		t.Fatal("留档转录应存在")
	}
	// 成品清空、类别与标签保留（重画不重分类、不改分组）。
	if got.Status == "done" || got.TikzCode != "" || got.FigPDF != "" || got.FigSVG != "" {
		t.Fatalf("成品没清干净: %+v", got)
	}
	if got.Class != ClassVector || got.Label != "系统结构图" {
		t.Fatalf("类别/标签被动了: %+v", got)
	}
	// 进度落盘了（重新加载还能看到清空后的状态）。
	reload := map[string]*imageProgress{}
	loadProgress(progDir, reload)
	if reload[rec.Key] == nil || reload[rec.Key].Status == "done" {
		t.Fatalf("进度没落盘: %+v", reload[rec.Key])
	}
	// 副本图（dedupe 跟随）不给重画：它复用的是代表图的成品。
	follower := &imageProgress{
		Key: "m.md::images/b/pic002.jpg", MDName: "m.md", ImgPath: "images/b/pic002.jpg",
		Class: ClassVector, Status: "done", Label: "系统结构图",
		EquivalentTo: rec.Key,
	}
	saveProgress(progDir, follower)
	ftr := "vector_" + vectorTexBase(follower.MDName, follower.ImgPath, follower.Label) + ".jsonl"
	if err := os.WriteFile(filepath.Join(sessDir, ftr), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := prepareVectorRedraw(proj, ftr, 0); err == nil {
		t.Fatal("副本图应拒绝重画")
	}
	// 名字不对（不是 vector_*.jsonl）也要拒绝。
	if _, _, _, err := prepareVectorRedraw(proj, "convert_chapter_002.jsonl", 0); err == nil {
		t.Fatal("非作图转录应拒绝")
	}
}

// 矢量图"从某条记录之后继续"：前缀留在原地当续跑起点、完整旧转录留档 .tryN，
// 第二次再切递增不覆盖；非消息行与越界都拒。
func TestTruncateVectorTranscript(t *testing.T) {
	proj := t.TempDir()
	sessDir := filepath.Join(proj, "source", "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	texBase := "book__pic001__fig"
	tr := filepath.Join(sessDir, "vector_"+texBase+".jsonl")
	body := strings.Join([]string{
		`{"t":"meta","kind":"system"}`,
		`{"t":"msg","role":"user","content":"draw it"}`,
		`{"t":"msg","role":"assistant","content":"step 1"}`,
		`{"t":"msg","role":"assistant","content":"step 2 (wrong)"}`,
		`{"t":"usage","model":"m"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(tr, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	archived, err := truncateVectorTranscript(proj, texBase, 3)
	if err != nil {
		t.Fatal(err)
	}
	if archived != "vector_"+texBase+".try1.jsonl" {
		t.Fatalf("留档名 = %q", archived)
	}
	// 留档是**完整**那份（回滚不会丢对话）。
	full, err := os.ReadFile(filepath.Join(sessDir, archived))
	if err != nil || !strings.Contains(string(full), "step 2 (wrong)") {
		t.Fatalf("留档不完整: %v", err)
	}
	// 原地只剩前三行，且以消息行收尾。
	head, err := os.ReadFile(tr)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(head), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("前缀行数 = %d（期望 3）", len(lines))
	}
	if !strings.Contains(lines[2], "step 1") || strings.Contains(string(head), "wrong") {
		t.Fatalf("前缀内容不对: %q", string(head))
	}
	// 再切一次：号递增，不覆盖 .try1。
	archived2, err := truncateVectorTranscript(proj, texBase, 2)
	if err != nil {
		t.Fatal(err)
	}
	if archived2 != "vector_"+texBase+".try2.jsonl" {
		t.Fatalf("第二次留档名 = %q", archived2)
	}
	// 越界与非消息行都拒（usage 行不进消息流，切在那儿等于没切）。
	if _, err := truncateVectorTranscript(proj, texBase, 99); err == nil {
		t.Fatal("越界应报错")
	}
	if _, err := truncateVectorTranscript(proj, texBase, 0); err == nil {
		t.Fatal("from=0 应报错")
	}
}

// T184：核对转录也能从某条记录之后继续——前 from 行抄成一条**新的核对尝试**
// （checker_<章>.try<N+1>.jsonl），源转录一个字不动，产物更不动。
func TestTruncateCheckerTranscript(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_002"
	sessDir := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"t":"meta","kind":"system"}`,
		`{"t":"msg","role":"user","content":"review it"}`,
		`{"t":"msg","role":"assistant","content":"step 1"}`,
		`{"t":"usage","model":"m"}`,
		`{"t":"msg","role":"assistant","content":"step 2 (wrong)"}`,
	}, "\n") + "\n"
	src := filepath.Join(sessDir, "checker_"+base+".try2.jsonl")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}

	dst, n, err := r.truncateCheckerTranscript(proj, base, "checker_"+base+".try2.jsonl", 3)
	if err != nil {
		t.Fatal(err)
	}
	// try1 是转换阶段自带的那次核对；try2 已存在，所以新号从 3 起。
	if n != 3 || filepath.Base(dst) != "checker_"+base+".try3.jsonl" {
		t.Fatalf("新尝试 = %d / %q", n, filepath.Base(dst))
	}
	head, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(head), "\n"); got != 3 {
		t.Fatalf("前缀行数 = %d（期望 3）", got)
	}
	if strings.Contains(string(head), "wrong") {
		t.Fatalf("前缀里不该有被切掉的内容: %q", string(head))
	}
	// 源转录不动。
	orig, err := os.ReadFile(src)
	if err != nil || !strings.Contains(string(orig), "wrong") {
		t.Fatalf("源转录被改了: %v", err)
	}
	// 再切一次：号继续递增（内存里记的号也要算进去，连点两下不撞车）。
	r.retryCheckerAttempt = map[string]int{base: 3}
	dst2, n2, err := r.truncateCheckerTranscript(proj, base, "checker_"+base+".try2.jsonl", 2)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 4 || filepath.Base(dst2) != "checker_"+base+".try4.jsonl" {
		t.Fatalf("第二次新尝试 = %d / %q", n2, filepath.Base(dst2))
	}
	// 越界、from=0、切在用量行上都拒。
	if _, _, err := r.truncateCheckerTranscript(proj, base, "checker_"+base+".try2.jsonl", 99); err == nil {
		t.Fatal("越界应报错")
	}
	if _, _, err := r.truncateCheckerTranscript(proj, base, "checker_"+base+".try2.jsonl", 0); err == nil {
		t.Fatal("from=0 应报错")
	}
	if _, _, err := r.truncateCheckerTranscript(proj, base, "checker_"+base+".try2.jsonl", 4); err == nil {
		t.Fatal("切在用量行应报错")
	}
}

// TestChapterMarkdownForChild：递归子章节的 md 在父节点的细分树里，重新核对
// 找不到它就会拿一个空视图去核（P56⑤ 放开子章节重核之后必须能定位到）。
func TestChapterMarkdownForChild(t *testing.T) {
	proj := t.TempDir()
	top := filepath.Join(proj, "chapters", "chapter_002.md")
	if err := os.MkdirAll(filepath.Dir(top), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(top, []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := chapterMarkdownFor(proj, "chapter_002"); got != top {
		t.Fatalf("顶层章节 md = %q, want %q", got, top)
	}
	child := filepath.Join(proj, "work", "temp", "conv_chapter_003", "work", "_subdivision", "chapter_003_child_01", "source", "chapter_003_child_01.md")
	if err := os.MkdirAll(filepath.Dir(child), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("# 子章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := chapterMarkdownFor(proj, "chapter_003_child_01"); got != child {
		t.Fatalf("子章节 md = %q, want %q", got, child)
	}
}
