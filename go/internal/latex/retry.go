package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/retryq"
)

// P53：手动重试（网页上的「重试这一章」/命令行的 --only）。
//
// 两条角色各有一条路，语义都是"这一章再开一个尝试"，但重跑的东西不同：
//
//	convert  重转这一章：归档旧产物（work/attempts/<章>/try<N>/）、清掉提交的
//	         .tex，新会话写 convert_<章>.try<N+1>.jsonl。
//	checker  只重新核对已经提交的产物：产物一个字不动，旧结论在
//	         <章>.tex.checker 里，挪成 .try<N> 留档，新会话写
//	         checker_<章>.try<N+1>.jsonl。"核对说错了"和"转换做错了"是两件事，
//	         不该逼用户把转换也一起重跑。
//
// 请求来源有两处，走同一条路：
//   - 预览页的按钮 → `work/retry/*.json`（retryq 包），进程在转换阶段的
//     安全点消费；没在跑就留到下次启动。
//   - 命令行 `--only chapter_002` → 直接当作"这些章要重转"，不进队列。

// setRetryNote 把"为什么重试"记进新尝试的转录，事后对得上。
func (r *Runner) setRetryNote(base, reason string) {
	if r.retryNotes == nil {
		r.retryNotes = map[string]string{}
	}
	r.retryNotes[base] = reason
}

// retryAttemptOf 返回这一章被要求的转换尝试号（0 = 没被要求，按磁盘推导）。
func (r *Runner) retryAttemptOf(base string) int {
	if r.retryAttempt == nil {
		return 0
	}
	return r.retryAttempt[base]
}

// forceChapterRetry 归档这一章现有的转换尝试并分配新的尝试号，返回新号。
//
// 归档只做一次：`--only` 与队列命中都会调用它，但每一章只会被处理一次
// （调用方先去重）。
func (r *Runner) forceChapterRetry(proj, base, reason string) int {
	baseTranscript := filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
	cur, _ := attemptNumberOnDisk(baseTranscript)
	// 本次运行里上一次请求（比如连点两下）还没真正写出转录文件，磁盘上看不到
	// 那个号——用内存里的号兜住，否则第二次会撞回同一个 try 目录。
	if prev := r.retryAttemptOf(base); prev > cur {
		cur = prev
	}
	node := chapterNodeConfig{
		Base:           base,
		NodeRoot:       filepath.Join(proj, "work", "temp", "conv_"+base),
		TranscriptPath: baseTranscript,
		SubmitRoot:     filepath.Join(proj, "work", "chapters"),
		ReportPath:     filepath.Join(proj, "work", "reports", base+".md"),
		AttemptNum:     cur,
	}
	texPath := filepath.Join(node.SubmitRoot, base+".tex")
	if cur >= 1 || fileExists(texPath) || fileExists(filepath.Join(node.NodeRoot, "work")) {
		keep := cur
		if keep < 1 {
			keep = 1
		}
		node.AttemptNum = keep
		r.archiveChapterAttempt(proj, base, node, texPath, "")
	}
	// 产物必须消失：转换阶段对"已有 <章>.tex"的章节直接跳过。
	_ = os.Remove(texPath)
	_ = os.RemoveAll(filepath.Join(node.SubmitRoot, base))
	_ = os.Remove(texPath + ".checker")
	next := cur + 1
	if next < 2 {
		next = 2
	}
	if r.retryAttempt == nil {
		r.retryAttempt = map[string]int{}
	}
	r.retryAttempt[base] = next
	if reason == "" {
		reason = "手动重试"
	}
	r.setRetryNote(base, reason)
	r.log.Log(0, "[convert]", base, "手动重试：旧尝试留档到 work/attempts/"+base+"/try"+strconv.Itoa(cur)+
		"，新尝试写 convert_"+base+".try"+strconv.Itoa(next)+".jsonl")
	return next
}

// forceCheckerRetry 为这一章分配一个新的核对尝试号，返回新号。
//
// 与转换重试的区别：**产物不动**——重跑的是"再看一遍"，不是"重做一遍"。
// 旧结论挪到 `<章>.tex.checker.try<N>`（新会话会覆盖 `.checker`），转录按尝试
// 号新开，预览页「历史」里两条都在。
func (r *Runner) forceCheckerRetry(proj, base string) int {
	baseTranscript := filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	cur, _ := attemptNumberOnDisk(baseTranscript)
	if prev := r.retryCheckerAttempt[base]; prev > cur {
		cur = prev
	}
	next := cur + 1
	if next < 2 {
		next = 2
	}
	if r.retryCheckerAttempt == nil {
		r.retryCheckerAttempt = map[string]int{}
	}
	r.retryCheckerAttempt[base] = next
	verdict := filepath.Join(proj, "work", "chapters", base+".tex") + ".checker"
	if fileExists(verdict) {
		keep := cur
		if keep < 1 {
			keep = 1
		}
		_ = copyFile(verdict, verdict+".try"+strconv.Itoa(keep))
	}
	r.setRetryNote(base, "网页重试请求（重新核对）")
	r.log.Log(0, "[checker]", base, "手动重新核对：产物不动，新尝试写 checker_"+base+".try"+
		strconv.Itoa(next)+".jsonl（旧结论留档 .checker.try"+strconv.Itoa(cur)+"）")
	return next
}

// consumeRetryRequests 取走队列里指定角色的待办请求，返回去重后的章节基名。
// 其它角色原样留在队列里，等各自那一轮处理。
func (r *Runner) consumeRetryRequests(proj, role string) []string {
	files := retryq.Pending(proj)
	if len(files) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var bases []string
	for _, f := range files {
		if retryRoleOf(f.Request) != role {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(f.Chapter), filepath.Ext(f.Chapter))
		if base == "" || strings.Contains(base, "..") {
			continue
		}
		if err := retryq.Consume(f.Path); err != nil {
			r.log.LogWarning(0, "[convert]", base, "重试请求标记完成失败（下次启动会再处理一次）:", err)
		}
		if seen[base] {
			continue
		}
		seen[base] = true
		bases = append(bases, base)
	}
	return bases
}

// retryRoleOf 归一化请求角色（空 = convert）。
func retryRoleOf(req retryq.Request) string {
	if req.Role == "" {
		return "convert"
	}
	return req.Role
}

// prepareRetries 消费队列里的转换请求并完成归档，返回真正要重转的章节。
// 章节 md 不在的请求只警告（书被重新划分过时会出现）。
func (r *Runner) prepareRetries(proj string) []string {
	bases := r.consumeRetryRequests(proj, "convert")
	if len(bases) == 0 {
		return nil
	}
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		if !fileExists(filepath.Join(proj, "chapters", base+".md")) {
			r.log.LogWarning(0, "[convert]", base, "重试请求指向的章节不存在（书被重新划分过？），忽略")
			continue
		}
		out = append(out, base)
	}
	if len(out) == 0 {
		return nil
	}
	r.log.Log(0, "[convert] 手动重试队列：", strings.Join(out, ", "))
	for _, base := range out {
		r.forceChapterRetry(proj, base, "网页重试请求")
	}
	return out
}

// prepareCheckerRetries 消费队列里的"重新核对"请求：产物已经在的才接，缺产物
// 的只警告（那一章还没转完，重核没有对象）。
func (r *Runner) prepareCheckerRetries(proj string) []string {
	bases := r.consumeRetryRequests(proj, "checker")
	if len(bases) == 0 {
		return nil
	}
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		if !fileExists(filepath.Join(proj, "chapters", base+".md")) {
			r.log.LogWarning(0, "[checker]", base, "重新核对请求指向的章节不存在，忽略")
			continue
		}
		if !fileExists(filepath.Join(proj, "work", "chapters", base+".tex")) {
			r.log.LogWarning(0, "[checker]", base, "还没有提交产物，重新核对没有对象（先重试转换），忽略")
			continue
		}
		out = append(out, base)
	}
	return out
}

// hasPendingRetry 报告有没有待处理的重试请求（用来决定 convert 阶段即使
// 标了 done 也必须再跑一遍）。
func (r *Runner) hasPendingRetry(proj string) bool {
	return len(retryq.Chapters(proj)) > 0
}

// onlyChapterList 返回 --only 名单（排序后），只用于报错信息。
func (r *Runner) onlyChapterList() []string {
	out := make([]string, 0, len(r.onlyChapters))
	for b := range r.onlyChapters {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

// basesToSet 把章节基名列表变成过滤集合。
func basesToSet(bases []string) map[string]bool {
	out := make(map[string]bool, len(bases))
	for _, b := range bases {
		out[b] = true
	}
	return out
}

// selectOnlyChapters 按 --only 名单过滤章节文件，并保持原来的顺序。
func selectOnlyChapters(chapters []string, only map[string]bool) []string {
	if len(only) == 0 {
		return chapters
	}
	out := make([]string, 0, len(chapters))
	for _, p := range chapters {
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		if only[base] {
			out = append(out, p)
		}
	}
	return out
}

// chapterCompiledPDF 找这一章上一次编译出的 PDF（转换工作区里 .gate/compile.json
// 记的那个）：重新核对时把它挂进只读视图，核对才有"产物页"可看。
func chapterCompiledPDF(proj, base string) string {
	work := filepath.Join(proj, "work", "temp", "conv_"+base, "work")
	data, err := os.ReadFile(filepath.Join(work, ".gate", "compile.json"))
	if err != nil {
		return ""
	}
	var cs struct {
		PDF string `json:"pdf"`
	}
	if json.Unmarshal(data, &cs) != nil || cs.PDF == "" {
		return ""
	}
	p := filepath.Join(work, cs.PDF)
	if fileExists(p) {
		return p
	}
	return ""
}

// recheckChapter 用全新会话重新核对这一章**已提交的产物**（P53 的 checker 角色）。
//
// 与转换重试的区别：这里不碰 `.tex`、分片与工作区，只换核对会话与只读视图。
// 结论照既有语义落盘：通过就删 `<章>.tex.checker`，有问题就写进去（旧结论已在
// forceCheckerRetry 里留档）。API 失败不改结论——那是环境问题，不是"这一章错"。
func (r *Runner) recheckChapter(proj, base string, tid int) (bool, string) {
	chapPath := filepath.Join(proj, "chapters", base+".md")
	texPath := filepath.Join(proj, "work", "chapters", base+".tex")
	partsPath := filepath.Join(proj, "work", "chapters", base)
	if !fileExists(texPath) {
		return false, "这一章还没有提交产物，无法重新核对（先重试转换）"
	}
	hints := checkHints{}
	// 上一次的 submit.notes 还在只读视图里（转换会话留给核对的说法），继续用。
	if data, err := os.ReadFile(filepath.Join(proj, "work", "views", "checker_"+base, "notes.md")); err == nil {
		hints.Notes = string(data)
	}
	hints.PDF = chapterCompiledPDF(proj, base)
	if rp := filepath.Join(proj, "work", "reports", base+".md"); fileExists(rp) {
		hints.Report = rp
	}
	n := r.retryCheckerAttempt[base]
	if n < 2 {
		n = 2
	}
	trPath := attemptTranscriptPath(filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl"), n)
	view := filepath.Join(proj, "work", "views", fmt.Sprintf("checker_%s_try%d", base, n))
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints, tid, trPath, view, false)
	if isSessionAPIError(issues) {
		return false, issues
	}
	if ok {
		_ = os.Remove(texPath + ".checker")
		r.log.Log(tid, "[checker]", base, "重新核对通过")
		return true, ""
	}
	_ = os.WriteFile(texPath+".checker", []byte(issues), 0o644)
	r.log.LogWarning(tid, "[checker]", base, "重新核对仍有问题:", issues)
	return false, issues
}

// SetOnlyChapters 限定并强制重跑这些章节（--only）。
func (r *Runner) SetOnlyChapters(bases []string) {
	r.onlyChapters = map[string]bool{}
	for _, b := range bases {
		b = strings.TrimSpace(b)
		b = strings.TrimSuffix(filepath.Base(b), filepath.Ext(b))
		if b != "" {
			r.onlyChapters[b] = true
		}
	}
}

// ---------- P2：运行期派发（不等阶段边界、不用重启） ----------

// 章节锁：主流程的并发批与运行期的重试派发共用，同一章永远不会被跑两遍。
//
//	holdChapter  阻塞拿锁（主流程的并发批用：轮到这一章就必须等前一轮收尾）
//	lockChapter  非阻塞（派发端用：忙就**不消费**请求，留在队列里等下一次 tick，
//	             比"排上了但被丢掉"好）
func (r *Runner) chapterMutex(base string) *sync.Mutex {
	r.busyMu.Lock()
	defer r.busyMu.Unlock()
	if r.chapterBusy == nil {
		r.chapterBusy = map[string]*sync.Mutex{}
	}
	m, ok := r.chapterBusy[base]
	if !ok {
		m = &sync.Mutex{}
		r.chapterBusy[base] = m
	}
	return m
}

func (r *Runner) holdChapter(base string) func() {
	m := r.chapterMutex(base)
	m.Lock()
	return m.Unlock
}

func (r *Runner) lockChapter(base string) bool { return r.chapterMutex(base).TryLock() }

func (r *Runner) unlockChapter(base string) { r.chapterMutex(base).Unlock() }

// continueTextOf 取这一章的"继续跑"提示（没有则空）。
func (r *Runner) continueTextOf(base string) string {
	if r.continueText == nil {
		return ""
	}
	return r.continueText[base]
}

// setContinueText 记下"这一章要接着跑"的提示；跑完就清掉，免得下一次普通
// 运行也被当成手动继续。
func (r *Runner) setContinueText(base, text string) {
	if r.continueText == nil {
		r.continueText = map[string]string{}
	}
	if text == "" {
		delete(r.continueText, base)
		return
	}
	r.continueText[base] = text
}

// startRetryWatcher 起一条 goroutine，每 2 秒看一遍重试队列（P2）。
//
// 之前重试只在转换阶段的两个安全点消费：转换阶段跑完之后再点按钮就没人管了，
// 得等下一次启动。现在它跑在整个 RunBook 生命周期里——样式、划分、转换、组装
// 任何阶段都能收，真正进并发槽位（转换/核对各自的池子），同一章忙就留队。
func (r *Runner) startRetryWatcher(proj string) {
	if proj == "" || r.retryWatcherStop != nil {
		return
	}
	stop := make(chan struct{})
	r.retryWatcherStop = stop
	r.retryWatcherWG.Add(1)
	go func() {
		defer r.retryWatcherWG.Done()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				r.dispatchRetries(proj)
			}
		}
	}()
}

// stopRetryWatcher 停掉 watcher 并等在跑的重试收尾（会等，别让文件写到一半）。
func (r *Runner) stopRetryWatcher() {
	if r.retryWatcherStop == nil {
		return
	}
	close(r.retryWatcherStop)
	r.retryWatcherStop = nil
	r.retryWatcherWG.Wait()
}

// retryPrecondition 说明"现在能不能跑这条请求"。返回非空 = 还不能跑（请求留在
// 队列里，下一个 tick 再看）。样式未就绪、产物还没有都属于这一类。
func (r *Runner) retryPrecondition(proj, role, base string) string {
	if !fileExists(filepath.Join(proj, "chapters", base+".md")) {
		return "章节 markdown 还不存在"
	}
	// 继续跑的对象是已有的那条会话：没有转录就没什么可接着的。
	if role == "continue" {
		tr := filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
		hasTr := false
		for n := 1; n <= 99; n++ {
			if fileExists(attemptTranscriptPath(tr, n)) {
				hasTr = true
				break
			}
		}
		if !hasTr {
			return "这一章还没有转换转录"
		}
	}
	if role == "checker" && !fileExists(filepath.Join(proj, "work", "chapters", base+".tex")) {
		return "这一章还没有提交产物"
	}
	if role == "continue" && !fileExists(filepath.Join(proj, "work", "temp", "conv_"+base, "work")) {
		return "这一章的工作区已经不在了"
	}
	if classNameOfFile(filepath.Join(proj, "style")) == "" {
		return "样式还没就绪"
	}
	return ""
}

// dispatchRetries 扫一遍队列并派发（主流程与 watcher 共用一次实现）。
// 返回本次派发的条数。
func (r *Runner) dispatchRetries(proj string) int {
	files := retryq.Pending(proj)
	if len(files) == 0 {
		return 0
	}
	started := 0
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.Chapter), filepath.Ext(f.Chapter))
		if base == "" || strings.Contains(base, "..") {
			_ = retryq.Consume(f.Path)
			continue
		}
		role := retryRoleOf(f.Request)
		if role != "convert" && role != "checker" && role != "continue" {
			r.log.LogWarning(0, "[retry]", base, "未知的请求角色，忽略:", role)
			_ = retryq.Consume(f.Path)
			continue
		}
		if !r.lockChapter(base) {
			continue // 正在跑：留在队列里
		}
		if why := r.retryPrecondition(proj, role, base); why != "" {
			r.unlockChapter(base)
			// 前置条件没到：留队。只在有进展时才会重复扫到它，日志按章去重。
			if !r.retryWarnOnce(f.Path, base+" "+why) {
				r.log.LogInfo(0, "[retry]", base, "请求排队中："+why)
			}
			continue
		}
		if err := retryq.Consume(f.Path); err != nil {
			r.unlockChapter(base)
			r.log.LogWarning(0, "[retry]", base, "标记请求完成失败（下次会再处理一次）:", err)
			continue
		}
		started++
		req := f.Request
		r.retryWatcherWG.Add(1)
		go func(base string, req retryq.Request) {
			defer r.retryWatcherWG.Done()
			defer r.unlockChapter(base)
			r.runRetryTask(proj, base, req)
		}(base, req)
	}
	return started
}

// retryWarnOnce 让"排队中"这类提示每个请求文件只打一次。
func (r *Runner) retryWarnOnce(key, msg string) bool {
	r.busyMu.Lock()
	defer r.busyMu.Unlock()
	if r.retryWarned == nil {
		r.retryWarned = map[string]bool{}
	}
	if r.retryWarned[key] {
		return true
	}
	r.retryWarned[key] = true
	return false
}

// runRetryTask 执行一条重试请求（已在章节锁里）。
func (r *Runner) runRetryTask(proj, base string, req retryq.Request) {
	role := retryRoleOf(req)
	reason := strings.TrimSpace(req.Reason)
	switch role {
	case "convert":
		r.forceChapterRetry(proj, base, reason)
		if err := r.convertChapterNow(proj, base, ""); err != nil {
			r.log.LogWarning(0, "[convert]", base, "手动重试失败:", err)
		}
	case "checker":
		r.forceCheckerRetry(proj, base)
		if _, issues := r.recheckChapter(proj, base, 0); issues != "" {
			r.log.LogInfo(0, "[checker]", base, "重新核对的结论:", issues)
		}
	case "continue":
		if req.From > 0 {
			if err := r.continueFromChapter(proj, base, req.Path, req.From); err != nil {
				r.log.LogWarning(0, "[convert]", base, "从记录继续失败:", err)
			}
			return
		}
		if err := r.continueChapter(proj, base, reason); err != nil {
			r.log.LogWarning(0, "[convert]", base, "继续跑失败:", err)
		}
	}
}

// convertChapterNow 在运行期把一章转换跑掉（主流程之外的那条路）。
func (r *Runner) convertChapterNow(proj, base, continueText string) error {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	if clsName == "" {
		return fmt.Errorf("样式还没就绪（style 阶段未完成）")
	}
	chapPath := filepath.Join(proj, "chapters", base+".md")
	if !fileExists(chapPath) {
		return fmt.Errorf("章节不存在: %s", base)
	}
	if continueText != "" {
		r.setContinueText(base, continueText)
		defer r.setContinueText(base, "")
	}
	return r.convertOneChapter(proj, clsName, filepath.Join(proj, "style", "manual.md"), chapPath,
		filepath.Join(proj, "work"), 1, 0, true)
}

// continueChapter 让这一章**接着上一次尝试干**（P2）：不新建尝试、不归档、
// 不动已提交的产物，只是把一句"接着干"递进那条会话，让它自己把活收尾。
//
// 现场是这样的：会话因为厂商报错/上下文跑偏/被手动改过文件而停住，重启整个
// 流程要重烧一整本的 token。这里给的是"接着这一条继续"。
func (r *Runner) continueChapter(proj, base, reason string) error {
	if reason == "" {
		reason = "手动继续"
	}
	text := "The user asked you to continue this chapter manually: " + reason + "\n\n" + continueChapterPrompt
	r.log.Log(0, "[convert]", base, "手动继续：接着最近一次尝试跑（不新建尝试）")
	return r.convertChapterNow(proj, base, text)
}

// continueChapterPrompt 是"继续跑"递给会话的原话。
const continueChapterPrompt = "Re-read your workspace as it is NOW (read_file), then finish the job: fix whatever is still wrong, compile until clean, and submit the two paths again. Do not restart from scratch and do not ignore work you already did."

// continueFromChapter 从转录的第 from 条记录之后继续（P2 的"从某个错误的地方
// 开始"）：把前 from 行抄成**一条新尝试**，后面的轮次丢掉，然后让它接着干。
//
// 源转录一个字不动——原记录还在「历史」里，看得见、也删得掉。这样"回滚到某一
// 步"不会毁掉现场，只是多出一条从那儿分叉出去的尝试。
//
// 一个诚实的限制：只回滚**对话**，工作区还是最后一次的样子（没有按轮次快照
// 文件树）。所以它适合"模型从那一步开始跑飞、后面几轮全在乱改"的情形；若
// 要连文件一起回滚，那是另一次重转（重试）该做的事。
func (r *Runner) continueFromChapter(proj, base, srcName string, from int) error {
	dst, next, err := r.truncateChapterTranscript(proj, base, srcName, from)
	if err != nil {
		return err
	}
	if r.retryAttempt == nil {
		r.retryAttempt = map[string]int{}
	}
	r.retryAttempt[base] = next
	text := fmt.Sprintf("The user rolled this chapter back to record #%d of %s and asked you to continue from there. "+
		"Everything after that record is GONE from your history — do not assume work you may have done later still applies: "+
		"%s", from, filepath.Base(dst), continueChapterPrompt)
	r.setContinueText(base, text)
	defer r.setContinueText(base, "")
	r.log.Log(0, "[convert]", base, "从第", strconv.Itoa(from), "条记录继续：新尝试写",
		filepath.Base(dst), "（源转录保留）")
	return r.convertChapterNow(proj, base, "") // continueText 已经塞好
}

// truncateChapterTranscript 是 continueFromChapter 的纯文件部分：把前 from 行
// 抄成一条新尝试的转录，返回新文件与它的尝试号。源转录一个字不动。
func (r *Runner) truncateChapterTranscript(proj, base, srcName string, from int) (string, int, error) {
	if from < 1 {
		return "", 0, fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	name := filepath.Base(strings.TrimSpace(srcName))
	if name == "" || !strings.HasSuffix(name, ".jsonl") {
		return "", 0, fmt.Errorf("源转录不合法: %q", srcName)
	}
	dir := filepath.Join(proj, "work", "sessions")
	src := filepath.Join(dir, name)
	data, err := os.ReadFile(src)
	if err != nil {
		return "", 0, fmt.Errorf("读不到源转录 %s: %w", name, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if from > len(lines) {
		return "", 0, fmt.Errorf("截断点 %d 超出转录长度 %d", from, len(lines))
	}
	// 只能切在消息行上：meta/usage 行不进消息流，切在那儿等于没切。
	var probe struct {
		T string `json:"t"`
	}
	if err := json.Unmarshal([]byte(lines[from-1]), &probe); err != nil || probe.T != "msg" {
		return "", 0, fmt.Errorf("第 %d 行不是一条消息记录（选了用量/元信息行？）", from)
	}
	baseTr := filepath.Join(dir, "convert_"+base+".jsonl")
	next := 1
	for n := 1; n <= 999; n++ {
		if !fileExists(attemptTranscriptPath(baseTr, n)) {
			next = n
			break
		}
	}
	if prev := r.retryAttemptOf(base); prev >= next {
		next = prev + 1
	}
	dst := attemptTranscriptPath(baseTr, next)
	head := strings.Join(lines[:from], "\n") + "\n"
	if err := os.WriteFile(dst, []byte(head), 0o644); err != nil {
		return "", 0, fmt.Errorf("写新尝试的转录失败: %w", err)
	}
	return dst, next, nil
}
