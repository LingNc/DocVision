package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
