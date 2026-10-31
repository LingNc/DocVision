package latex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	next := r.raiseCheckerAttemptNo(base, cur)
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

// checkerAttemptNo 读"这一章这次要写第几号核对转录"（T232）。
//
// retryCheckerAttempt 会被派发端（写）与递归节点的转换 goroutine（读）同时碰到，
// 所以读写都走这两个小函数。以前直接读写这个 map，靠的是"只在批次之间写"，递归
// 节点让这个前提不再成立。
func (r *Runner) checkerAttemptNo(base string) int {
	r.busyMu.Lock()
	defer r.busyMu.Unlock()
	return r.retryCheckerAttempt[base]
}

// raiseCheckerAttemptNo 把这一章的核对尝试号抬到 cur+1（不小于 2），返回新号。
func (r *Runner) raiseCheckerAttemptNo(base string, cur int) int {
	r.busyMu.Lock()
	defer r.busyMu.Unlock()
	if r.retryCheckerAttempt == nil {
		r.retryCheckerAttempt = map[string]int{}
	}
	if prev := r.retryCheckerAttempt[base]; prev > cur {
		cur = prev
	}
	next := cur + 1
	if next < 2 {
		next = 2
	}
	r.retryCheckerAttempt[base] = next
	return next
}

// bumpCheckerAttemptNo 把这一章的核对尝试号抬到 n（只增不减），返回是否真的变了。
func (r *Runner) bumpCheckerAttemptNo(base string, n int) bool {
	r.busyMu.Lock()
	defer r.busyMu.Unlock()
	if r.retryCheckerAttempt == nil {
		r.retryCheckerAttempt = map[string]int{}
	}
	if n <= r.retryCheckerAttempt[base] {
		return false
	}
	r.retryCheckerAttempt[base] = n
	return true
}

// rotateSpentChildCheckers 在手动重跑一个父章节之前，把这一支里"提醒已经打满"
// 的子章节核对转录换成下一个尝试号（T232）。
//
// 为什么必须在重跑之前换：递归节点的核对转录按转换尝试号命名，一条转录攒满
// 「没提交」的提醒之后，节点这条路就进不去它了（checker 的入口守卫），于是父章节
// 每次「继续跑」都当场撞在"子章节转换或 checker 失败；父章节禁止提交"上——请求
// 被消费掉、页面上什么都没变，看起来就是点了没反应。手动重跑本来就想让这一支
// 再试一次，所以先给那些打满的转录换新号：产物一个字不动，与页面上的「重新核对」
// 同一条通路。
func (r *Runner) rotateSpentChildCheckers(proj, base string) {
	for _, tr := range r.childCheckerTranscripts(proj, base) {
		n := countCheckerSubmitReminders(loadTranscriptMessages(tr))
		if n < checkerNoVerdictAttempts {
			continue
		}
		child, cur := childBaseOfCheckerTranscript(filepath.Base(tr))
		if child == "" || child == base {
			continue
		}
		next := cur + 1
		if next < 2 {
			next = 2
		}
		if r.bumpCheckerAttemptNo(child, next) {
			r.log.LogInfo(0, "[checker]", child, "这条核对转录已有", strconv.Itoa(n),
				"次没提交的提醒，手动重跑换新尝试 try"+strconv.Itoa(next))
		}
	}
}

// childCheckerTranscripts 列出这一支下面所有子章节的核对转录：顶层的（重新核对
// 写的那些）与细分树里每个节点的（递归节点写的那些）。
func (r *Runner) childCheckerTranscripts(proj, base string) []string {
	var out []string
	sessions := filepath.Join(proj, "work", "sessions")
	if ents, err := os.ReadDir(sessions); err == nil {
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".jsonl") {
				continue
			}
			if !strings.HasPrefix(n, "checker_"+base+"_child_") {
				continue
			}
			out = append(out, filepath.Join(sessions, n))
		}
	}
	roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+base+"*", "work", "_subdivision"))
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			n := d.Name()
			if strings.HasPrefix(n, "checker_") && strings.HasSuffix(n, ".jsonl") {
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}

// childBaseOfCheckerTranscript 从核对转录文件名反推章节名与尝试号。
func childBaseOfCheckerTranscript(name string) (string, int) {
	s := strings.TrimSuffix(strings.TrimPrefix(name, "checker_"), ".jsonl")
	s = strings.TrimSuffix(s, ".partial")
	attempt := 0
	if i := strings.Index(s, ".try"); i > 0 {
		if n, err := strconv.Atoi(s[i+4:]); err == nil {
			attempt = n
		}
		s = s[:i]
	}
	return s, attempt
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
	if !dirExists(work) {
		// 递归子章节没有自己的 temp 工作区，它在父节点的细分树里干活：那个节点的
		// 转录目录是 <节点>/sessions/，工作区就是同级的 <节点>/work。
		if nodeWork := childNodeWork(proj, base); nodeWork != "" {
			work = nodeWork
		}
	}
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
// continueCheckerFrom 从核对转录的第 from 条记录之后继续（T184）：前缀抄成
// 一条**新的核对尝试**（checker_<章>.try<N>.jsonl），后面的轮次丢掉，然后把
// 这条会话推下去。产物一个字不动——核对本来就是只读判断，回滚对话只是让它
// 别带着"后面那些已经被推翻的结论"继续想。
func (r *Runner) continueCheckerFrom(proj, base, srcName string, from int) error {
	dst, next, err := r.truncateCheckerTranscript(proj, base, srcName, from)
	if err != nil {
		return err
	}
	r.log.Log(0, "[checker]", base, "从第", strconv.Itoa(from), "条记录继续核对：新尝试写",
		filepath.Base(dst), "（源转录保留；产物不动，历史原样续行）")
	return r.continueCheckerAfterCut(proj, base, next)
}

// continueCheckerAfterCut 是"前缀已经切好"之后的那一半（T228）：切口可能是提交
// 方（复盘会话）**当场**做完的，派发端就不该再切一次，只把这条核对会话推下去。
func (r *Runner) continueCheckerAfterCut(proj, base string, next int) error {
	if next < 2 {
		return fmt.Errorf("核对新尝试号不合法: %d", next)
	}
	r.bumpCheckerAttemptNo(base, next)
	// 旧结论留一份档（与 forceCheckerRetry 同款）：新的那条核对会重写
	// <章>.tex.checker，旧的留着还能对照。
	r.setRetryNote(base, "网页重试请求（从记录继续核对）")
	texPath := filepath.Join(proj, "work", "chapters", base+".tex")
	if fileExists(texPath + ".checker") {
		_ = copyFile(texPath+".checker", fmt.Sprintf("%s.checker.try%d", texPath, next-1))
	}
	if _, issues := r.recheckChapter(proj, base, 0); issues != "" {
		r.log.LogInfo(0, "[checker]", base, "从记录继续后的结论:", issues)
	}
	return nil
}

// truncateCheckerTranscript 把核对转录的前 from 行抄成一条新的核对尝试。
// 源转录一个字不动（与章节转换那边同一套语义）。
func (r *Runner) truncateCheckerTranscript(proj, base, srcName string, from int) (string, int, error) {
	if from < 1 {
		return "", 0, fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	name := filepath.Base(strings.TrimSpace(srcName))
	if name == "" || !strings.HasSuffix(name, ".jsonl") {
		return "", 0, fmt.Errorf("源转录不合法: %q", srcName)
	}
	dir := filepath.Join(proj, "work", "sessions")
	src := filepath.Join(dir, name)
	// T201：递归子章节的核对转录在父工作区的细分树里，不在 work/sessions/ 下。
	// 请求带的是项目内相对路径（`work/temp/conv_<父>/work/_subdivision/…`）或绝对
	// 路径；新尝试仍然写在 work/sessions/ 下（那里不会随父节点临时树被清掉，
	// recheckChapter 也按同一位置找转录）。
	if strings.ContainsAny(strings.TrimSpace(srcName), "/\\") {
		p := strings.TrimSpace(srcName)
		if !filepath.IsAbs(p) {
			p = filepath.Join(proj, filepath.FromSlash(p))
		}
		if rel, rerr := filepath.Rel(filepath.Join(proj, "work"), p); rerr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if fileExists(p) {
				src = p
			}
		}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", 0, fmt.Errorf("读不到源转录 %s: %w", name, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	asked := from
	from, snapped, err := snapCutLine(lines, from)
	if err != nil {
		return "", 0, err
	}
	if snapped && r.log != nil {
		r.log.LogWarning(0, "[checker]", base, "切口从第", strconv.Itoa(asked), "条挪到第",
			strconv.Itoa(from), "条：那一行是用量/元信息行，不进消息流")
	}
	baseTr := filepath.Join(dir, "checker_"+base+".jsonl")
	next := 2 // 核对尝试从 try2 起（try1 是转换阶段自带的那次核对）
	for n := 2; n <= 999; n++ {
		if !fileExists(attemptTranscriptPath(baseTr, n)) {
			next = n
			break
		}
	}
	if prev := r.checkerAttemptNo(base); prev >= next {
		next = prev + 1
	}
	dst := attemptTranscriptPath(baseTr, next)
	head := strings.Join(lines[:from], "\n") + "\n"
	if err := os.WriteFile(dst, []byte(head), 0o644); err != nil {
		return "", 0, fmt.Errorf("写新尝试的转录失败: %w", err)
	}
	return dst, next, nil
}

// chapterMarkdownFor 找出这一章的源 md。顶层章节在 `chapters/<章>.md`；递归子章节
// 的 md 在父节点的细分树里（`work/temp/conv_<父>*/work/_subdivision/**/<章>.md`），
// 核对视图要用它，所以不能写死顶层路径。
func chapterMarkdownFor(proj, base string) string {
	if p := filepath.Join(proj, "chapters", base+".md"); fileExists(p) {
		return p
	}
	if i := strings.Index(base, "_child_"); i > 0 {
		if p := findChildMarkdown(proj, base[:i], base); p != "" {
			return p
		}
	}
	return filepath.Join(proj, "chapters", base+".md")
}

// findChildMarkdown 在父节点的临时细分树里递归找子章节的 md。用 WalkDir 而不是
// Glob：`**` 在 filepath.Glob 里只匹配一个路径段，深层文件根本查不到（曾因此把
// "父节点正在跑这一支"误判成"找不到输入"）。
// childIDRe 认章节 id 里的层级编号（child_04、child_02…）。
var childIDRe = regexp.MustCompile(`child_\d+`)

// childInputRel 是子章节输入 md 相对**根章节**细分树根的路径（T201）：
// 顶层子章节是 `child_04/input/chapter.md`，嵌套时中间要经过父节点自己的
// `node/work/_subdivision`。用它做整段比较而不是"文件名匹配"——同一棵细分树里
// 不同层级都可能出现 `child_04`，只比名字会把 child_01 底下的 child_04 认成
// 顶层的 child_04（真数据里就这样认错过）。
func childInputRel(base string) string {
	i := strings.Index(base, "_child_")
	if i < 0 {
		return ""
	}
	ids := childIDRe.FindAllString(base[i+1:], -1)
	if len(ids) == 0 {
		return ""
	}
	parts := []string{}
	for k, id := range ids {
		if k > 0 {
			parts = append(parts, "node", "work", "_subdivision")
		}
		parts = append(parts, id)
	}
	parts = append(parts, "input", "chapter.md")
	return strings.Join(parts, "/")
}
func findChildMarkdown(proj, parent, base string) string {
	// 走**根章节**的细分树：嵌套子章节的输入在父节点自己的
	// `node/work/_subdivision` 底下，只按 parent 拼路径找不到。
	rootChapter := parent
	if i := strings.Index(base, "_child_"); i > 0 {
		rootChapter = base[:i]
	}
	roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+rootChapter+"*", "work", "_subdivision"))
	if len(roots) == 0 {
		roots, _ = filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+rootChapter, "work", "_subdivision"))
	}
	want := childInputRel(base)
	for _, root := range roots {
		hit := ""
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || hit != "" {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			// ① 真身：父节点细分这一支时写的输入 `…/<id>/input/chapter.md`（T201：
			//    以前只认 `<章>.md`，靠核对视图里那份同名副本碰巧命中，视图一清就
			//    "找不到章节 markdown"）。按整段相对路径比，避免认错层级。
			if want != "" {
				if rel, rerr := filepath.Rel(root, p); rerr == nil && filepath.ToSlash(rel) == want {
					hit = p
					return nil
				}
			}
			// ② 兼容：核对视图/报告里留下的 `<章>.md` 副本。
			if d.Name() == base+".md" {
				hit = p
			}
			return nil
		})
		if hit != "" {
			return hit
		}
	}
	return ""
}

// chapterProductsFor 找一章**已提交产物**的两个路径：主 .tex 与分片目录。
//
// 顶层就在 work/chapters/<章>.tex；递归子章节的产物在它上面某一层的
// work/chapters/<父>/children/<子>.tex（层数不定），所以按文件名在 work/chapters
// 下找一遍——暂存目录 children.tmp 里也有同名副本，必须跳过，否则会拿半成品去
// 核对。（以前这里写死 work/chapters/<章>.tex，子章节的「重新核对」一律报
// "这一章还没有提交产物"，按钮看起来就是坏的。）
func chapterProductsFor(proj, base string) (string, string) {
	root := filepath.Join(proj, "work", "chapters")
	top := filepath.Join(root, base+".tex")
	if fileExists(top) {
		return top, strings.TrimSuffix(top, ".tex")
	}
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "children.tmp" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == base+".tex" {
			found = p
		}
		return nil
	})
	if found == "" {
		if found = tempProductFor(proj, base); found == "" {
			return top, filepath.Join(root, base)
		}
	}
	return found, strings.TrimSuffix(found, ".tex")
}

// tempProductFor 在父节点的临时细分树里找子章节**已提交**的产物（T201）。
//
// 子章节核对通过、父节点还没整合时，产物不在 work/chapters 下，而在父工作区里：
//
//	· 父节点收集孩子产物的暂存池：<父工作区>/<父章>/children/<子>.tex
//	· 自己那一支的节点产物：<节点>/result/<子>.tex、<节点>/integrated-stage/<子>.tex
//	  （后者是"这一支还要继续细分"时的落点）
//
// 以前只认 work/chapters，于是"子章节转换完、核对也跑完了，点重新核对却说还没有
// 提交产物"——产品明明在临时树里等着父节点整合。
func tempProductFor(proj, base string) string {
	if i := strings.Index(base, "_child_"); i > 0 {
		roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+base[:i]+"*", "work"))
		for _, root := range roots {
			if p := filepath.Join(root, base[:i], "children", base+".tex"); fileExists(p) {
				return p
			}
		}
	}
	// 顶层拆分父节点的暂存区（T210）：细分父节点提交进
	// `work/temp/conv_<章>/integrated-stage/`，等核对通过才晋级到 work/chapters。
	// 以前只认 work/chapters 与**子树**里的 result/integrated-stage，于是活的
	// 核对会话正在看那份产物、页面上点「重新核对」却说"还没有提交产物"——同一份
	// 文件，两个判据。
	for _, stage := range []string{"integrated-stage", "result"} {
		if p := filepath.Join(proj, "work", "temp", "conv_"+base, stage, base+".tex"); fileExists(p) {
			return p
		}
	}
	roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_*", "work", "_subdivision"))
	for _, root := range roots {
		found := ""
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "children.tmp" {
					return fs.SkipDir
				}
				return nil
			}
			if d.Name() != base+".tex" {
				return nil
			}
			switch filepath.Base(filepath.Dir(p)) {
			case "result", "integrated-stage":
				found = p
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// childNodeWork 找递归子章节所在节点的工作区（<节点>/work）。节点的转录目录是
// 同级的 sessions/，文件名带完整章节 ID，所以按转录名定位节点最稳。
func childNodeWork(proj, base string) string {
	i := strings.Index(base, "_child_")
	if i <= 0 {
		return ""
	}
	root := base[:i]
	roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+root+"*", "work", "_subdivision"))
	for _, r := range roots {
		hit := ""
		_ = filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil || hit != "" {
				return nil
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			if !strings.HasPrefix(d.Name(), "convert_"+base) && !strings.HasPrefix(d.Name(), "checker_"+base) {
				return nil
			}
			if filepath.Base(filepath.Dir(p)) != "sessions" {
				return nil
			}
			hit = filepath.Join(filepath.Dir(filepath.Dir(p)), "work")
			return nil
		})
		if hit != "" && dirExists(hit) {
			return hit
		}
	}
	return ""
}

// chapterSessionsDir 找这一章转录所在的目录：顶层章节在 `<proj>/work/sessions/`，
// 递归子章节在它自己节点的 `sessions/` 下（T231）。
//
// 为什么按**文件名精确匹配**走：`childNodeWork` 用的是前缀匹配，
// `chapter_024_child_09` 会先撞上 `convert_chapter_024_child_09_child_01.jsonl`
// （遍历到 child_01 目录就命中并返回），于是"这一章的转录"被解到了孙节点上。
// 转录名带完整章节 ID，精确匹配（`convert_<章>.jsonl` / `convert_<章>.try<N>.jsonl`）
// 才是唯一的。
func chapterSessionsDir(proj, base string) string {
	top := filepath.Join(proj, "work", "sessions")
	if isChapterTranscriptDir(top, base) {
		return top
	}
	if i := strings.Index(base, "_child_"); i > 0 {
		roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+base[:i]+"*", "work", "_subdivision"))
		for _, root := range roots {
			hit := ""
			_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || hit != "" {
					return nil
				}
				if d.IsDir() || filepath.Base(filepath.Dir(p)) != "sessions" {
					return nil
				}
				if !isChapterTranscriptName(d.Name(), base) {
					return nil
				}
				hit = filepath.Dir(p)
				return nil
			})
			if hit != "" {
				return hit
			}
		}
	}
	return top
}

// isChapterTranscriptName 判断转录名是不是**这一章**的（不含孙节点的转录）。
func isChapterTranscriptName(name, base string) bool {
	name = strings.ToLower(name)
	for _, role := range []string{"convert_", "checker_"} {
		stem := role + strings.ToLower(base)
		if name == stem+".jsonl" || strings.HasPrefix(name, stem+".try") {
			return true
		}
	}
	return false
}

func isChapterTranscriptDir(dir, base string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && isChapterTranscriptName(e.Name(), base) {
			return true
		}
	}
	return false
}

// chapterConvertTranscript 这一章第 attempt 次转换的转录路径（T231）。
func chapterConvertTranscript(proj, base string, attempt int) string {
	return attemptTranscriptPath(filepath.Join(chapterSessionsDir(proj, base), "convert_"+base+".jsonl"), attempt)
}

// markChapterVerdict 把核对的结论写到**产出这份产物的那次转换转录**上（T231）。
//
// 页面上转换行看的是它自己那条转录的最后一条 chapter_activity。以前"重新核对"
// 只在核对转录里写结论，转换转录还挂着之前那句 error，于是核对已经通过、界面
// 上仍是"转换 · 会话错误"，看着像这一章永远过不去。
func markChapterVerdict(proj, base string, attempt int, ok bool, issues string) {
	state := "issues"
	switch {
	case ok:
		state = "pass"
	case checkerNoVerdict(issues):
		state = "no_verdict"
	}
	tr := chapterConvertTranscript(proj, base, attempt)
	if tr == "" || !fileExists(tr) {
		return
	}
	_ = appendTranscriptActivity(tr, state)
}

// latestConvertAttempt 找这一章最近一次转换尝试的号（P59）：重新核对产出的
// 锚点必须指向**做出这份产物的那次转换**。顶层章节的转换转录在
// `<proj>/work/sessions/`；递归子章节的转录跟着自己的节点走（`<节点>/sessions/`）。
// 找不到任何转录时按第 1 次算——与 lastAttemptNumber 的既有口径一致。
func latestConvertAttempt(proj, base string) int {
	dir := chapterSessionsDir(proj, base)
	return lastAttemptNumber(filepath.Join(dir, "convert_"+base+".jsonl"))
}

func (r *Runner) recheckChapter(proj, base string, tid int) (bool, string) {
	chapPath := chapterMarkdownFor(proj, base)
	texPath, partsPath := chapterProductsFor(proj, base)
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
	n := r.checkerAttemptNo(base)
	if n < 2 {
		n = 2
	}
	trPath := attemptTranscriptPath(filepath.Join(chapterSessionsDir(proj, base), "checker_"+base+".jsonl"), n)
	view := filepath.Join(proj, "work", "views", fmt.Sprintf("checker_%s_try%d", base, n))
	attempt := latestConvertAttempt(proj, base)
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints, tid, trPath, view, false, attempt)
	if isSessionAPIError(issues) {
		return false, issues
	}
	// T231：结论写回转换转录（页面上那一行的状态就是它）。核对那一边的
	// no_verdict 由 checkChapterAt 自己写在核对转录上，这里不再重复。
	markChapterVerdict(proj, base, attempt, ok, issues)
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

// ---------- P53 阶段二：运行期派发（不等阶段边界、不用重启） ----------

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

// takeContinueFlag 取出并清掉"这一章是人手动推下去的"标记。
//
// 刻意只传一个 bool、不带任何文字：手动继续与中断续跑走同一条规矩
// （2026-09-29 定的）——**有历史就原样续行，不插 user 内容**。这个标记的
// 唯一作用是让那条会话真的再跑一轮（否则"上次已 submit"会被 replay 收账、
// 直接结束，点一下等于没点）。
func (r *Runner) takeContinueFlag(base string) bool {
	if r.continueFlag == nil {
		return false
	}
	if !r.continueFlag[base] {
		return false
	}
	delete(r.continueFlag, base)
	return true
}

// setContinueFlag 记下"这一章要接着跑"；跑完就清掉，免得下一次普通运行也被
// 当成手动继续。
func (r *Runner) setContinueFlag(base string) {
	if r.continueFlag == nil {
		r.continueFlag = map[string]bool{}
	}
	r.continueFlag[base] = true
}

// startRetryWatcher 起一条 goroutine，每 2 秒看一遍重试队列（P53 阶段二）。
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

// retryBlockReason 是派发端统一的"现在还不能跑"判断。
//
// 切回第 N 次尝试（P63）走另一套判据：它不是"跑一条会话"，而是把两个留档目录
// 里的文件互换；"章节 markdown 存在""样式就绪"这些跟它无关，倒是"这一章还在
// 不在写"必须查——所以两套各归各的，别混在一个函数里越堆越长。
func (r *Runner) retryBlockReason(proj, role, base string, req retryq.Request) string {
	if role == "restore" {
		return RestorePrecondition(proj, base, req.Attempt)
	}
	return r.retryPrecondition(proj, role, base)
}

// retryPrecondition 说明"现在能不能跑这条请求"。返回非空 = 还不能跑（请求留在
// 队列里，下一个 tick 再看）。样式未就绪、产物还没有都属于这一类。
func (r *Runner) retryPrecondition(proj, role, base string) string {
	// T201：递归子章节没有 chapters/<章>.md（它的输入在父节点的细分树里），
	// 这里必须走同一个章节 md 解析入口，否则子章节的请求永远停在"markdown 还不存在"。
	if !fileExists(chapterMarkdownFor(proj, base)) {
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
	if role == "checker" {
		if tex, _ := chapterProductsFor(proj, base); !fileExists(tex) {
			return "这一章还没有提交产物"
		}
	}
	// 这里**不再要求工作区还在**（T243）：章节工作树是临时树，一次转换收尾就
	// 删掉了（`keep_temp_dirs` 默认关），而「接着跑」本来就以 resume=true 重建
	// 它。以前这条判据会把整条接口错误重试链挡死——排下的续跑请求每次扫到都被
	// 判成"工作区已经不在了"，这一章永远停在 queued，重启也不会自己动。
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
		// 矢量图重画（P53）：请求没有章节，按转录名定位那一条作图会话。
		if retryRoleOf(f.Request) == "vector" {
			if !r.vectorMu.TryLock() {
				continue // 已经有一张在重画：留队等它跑完（进度文件不能并发写）
			}
			if why := r.vectorRetryPrecondition(proj, f.Request.Path); why != "" {
				r.vectorMu.Unlock()
				if !r.retryWarnOnce(f.Path, f.Request.Path+" "+why) {
					r.log.LogInfo(0, "[vector]", f.Request.Path, "重画请求排队中："+why)
				}
				continue
			}
			if err := retryq.Consume(f.Path); err != nil {
				r.vectorMu.Unlock()
				r.log.LogWarning(0, "[vector]", f.Request.Path, "标记请求完成失败（下次会再处理一次）:", err)
				continue
			}
			started++
			req := f.Request
			r.retryWatcherWG.Add(1)
			go func(req retryq.Request) {
				defer r.retryWatcherWG.Done()
				defer r.vectorMu.Unlock()
				if err := r.redrawVectorImage(proj, req.Path, req.Reason, req.From); err != nil {
					r.log.LogWarning(0, "[vector]", req.Path, "重画失败:", err)
				}
			}(req)
			continue
		}
		base := strings.TrimSuffix(filepath.Base(f.Chapter), filepath.Ext(f.Chapter))
		if base == "" || strings.Contains(base, "..") {
			_ = retryq.Consume(f.Path)
			continue
		}
		role := retryRoleOf(f.Request)
		if role != "convert" && role != "checker" && role != "continue" && role != "restore" {
			r.log.LogWarning(0, "[retry]", base, "未知的请求角色，忽略:", role)
			_ = retryq.Consume(f.Path)
			continue
		}
		// T237：上游接口错误的重试请求带"最早动手时间"——还没到点就安静跳过，
		// 不写日志、不占章节锁、不消耗任何预算（watcher 两秒一 tick，真正动手
		// 只发生在退避时间到了之后）。
		if nb := strings.TrimSpace(f.Request.NotBefore); nb != "" {
			if at, err := time.Parse(time.RFC3339, nb); err == nil && time.Now().Before(at) {
				continue
			}
		}
		// 切回第 N 次尝试（P63）：子章节的尝试号是**父尝试之内**的号，单独把一支
		// 搬回去会把两代子树拼在一起（产物、报告、整合阶段的版本全对不上）。排成
		// "父章节切回"听起来更周到，但父章节那份留档里没有"子章节这一支的第 N 次"
		// 这个粒度——照实说，让它去父章节上点。
		if parent, isChild := childChapterParent(base); isChild && role == "restore" {
			_ = retryq.Consume(f.Path)
			r.log.LogWarning(0, "[restore]", base, "子章节的尝试归它所在的父章节，请对", parent, "切回")
			continue
		}
		// T196/T211：递归子章节的重转 / 接着跑需要父节点的整合上下文（章节 md、
		// 工作区、产物都在父节点的细分树里）。以前这里**只消费、只写日志**，页面
		// 上是"点了没反应"：回执说已排队，磁盘上请求被丢掉，会话一个字节没变。
		// 现在改成真的推进——把它换成父章节的「继续跑」，父节点重新驱动整棵子树
		// （这一支按磁盘上的转录续跑），改完再重新整合。
		// T244：这一章排着「切回第 N 次尝试」时，别的角色先别动手——它们一开跑就
		// 占住这一章的锁（细分父章节是整棵子树），切回又得从头等。请求留队，等切回
		// 搬完再轮到它们。切回自己不受这条限制。
		if role != "restore" {
			if n, ok := r.pendingRestore(proj, base); ok {
				if !r.retryWarnOnce(f.Path+"|waitrestore", base+" 等切回") {
					r.log.LogInfo(0, "[retry]", base, "有切回第", strconv.Itoa(n),
						"次尝试的请求在等，这条", role, "请求先留着（切回落地后再派发）")
				}
				continue
			}
		}
		if role == "restore" && f.Request.Attempt > 0 && rootChapterOf(base) == base {
			// T244：这条切回请求的目标已经是**现在这一次**了——多半是同一个按钮
			// 点了两下（第二条在第一轮搬完之后就没有意义）。以前它会永远留在队列
			// 里：每 5 分钟重排一次、永远排不上、也永远不消失。这里直接收掉。
			if lastAttemptNumber(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")) == f.Request.Attempt {
				_ = retryq.Consume(f.Path)
				r.log.LogInfo(0, "[restore]", base, "切回第", strconv.Itoa(f.Request.Attempt),
					"次尝试的请求作废：它已经是现在这一次（可能点了两次）")
				continue
			}
		}
		if parent, isChild := childChapterParent(base); isChild && (role == "convert" || role == "continue") {
			_ = retryq.Consume(f.Path)
			// 子章节不能单独重跑（没有谁去整合它），但**"从某条记录之后继续"
			// 这个切点必须在这里先落地**：过去这一支直接换成父章节的「继续跑」，
			// 请求里的 From/Path 一个字没用上；而且真要截的那条转录在子节点自己
			// 的 `sessions/` 下（不在 `<proj>/work/sessions/`），按顶层目录拼
			// 路径根本读不到它，失败还只写一行 warning。结果就是页面回执"已截断"、
			// 磁盘上一行没少，父章节重新驱动时子会话照旧从那段坏尾巴接着跑——
			// 截断等于没发生，一轮又一轮地耗 token。
			// T228：CutAttempt > 0 表示切口已经由提交方（复盘会话）当场做完，
			// 这里不再切第二刀——再切一次会多出一条同前缀的尝试，页面上看着像
			// "截了两回"，续跑也会选到重复的那一类文件。
			if role == "continue" && f.Request.From > 0 && f.Request.CutAttempt == 0 &&
				strings.TrimSpace(f.Request.Path) != "" {
				r.cutChapterTranscriptForRetry(proj, base, f.Request.Path, f.Request.From)
			}
			r.queueParentContinueFor(proj, f.Request.Project, parent, base)
			continue
		}
		if !r.lockChapter(base) {
			// 忙：留队、不消费。以前这里连一行日志都不打——页面上点「重新核对」
			// 就像什么都没发生（既不新建尝试、也不显示在排队），用户只能靠截断
			// 绕过。现在第一次跳过就记一行，之后每隔几分钟再说一次，带"等了多久"
			// 与"这一章最后一次输出在多久之前"，卡住的章节能从日志里直接看出来。
			if role == "restore" {
				// 切回要搬的就是工作区本身，绝不能和正在写它的会话并行（P63）：
				// 拿不到锁就等这一章收尾的那一刻再切，句子也换成"在等它停下来"，
				// 免得被当成在排槽位。
				r.logRestoreWait(proj, base, f.Path, f.Request.Attempt)
				continue
			}
			r.logBusySkip(proj, base, role, f.Path)
			continue
		}
		if why := r.retryBlockReason(proj, role, base, f.Request); why != "" {
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

// logRestoreWait 报告"切回请求在等这一章的会话收尾"（P63）。
//
// 用户点「切回第 N 次尝试」时，工作区里可能正有一个会话在写。切回要搬的就是这棵
// 树，所以它和其它请求一样只 TryLock：拿不到就留队，**不打断正在跑的那条**，等
// 这一章收尾的那一刻立刻切。这条日志就是"它在等，不是没反应"。
// T244：这条以前一辈子只打一行（按请求去重）。细分父章节的锁可能被整棵子树占着
// 几十分钟到几小时，用户看到的就是"点了切回、日志里只有一句、然后什么都没有"，
// 于是以为按钮没用、去重启进程。现在按 retryBusyLogEvery 重复，并带上第几次、
// 等了多久，卡没卡一眼能看出来。
func (r *Runner) logRestoreWait(proj, base, reqPath string, attempt int) {
	now := time.Now()
	r.busyMu.Lock()
	if r.retryBusySince == nil {
		r.retryBusySince = map[string]time.Time{}
	}
	if r.retryBusyLogged == nil {
		r.retryBusyLogged = map[string]time.Time{}
	}
	since, ok := r.retryBusySince[reqPath]
	if !ok {
		since = now
		r.retryBusySince[reqPath] = since
	}
	last := r.retryBusyLogged[reqPath]
	if !last.IsZero() && now.Sub(last) < retryBusyLogEvery {
		r.busyMu.Unlock()
		return
	}
	r.retryBusyLogged[reqPath] = now
	r.busyMu.Unlock()

	msg := "切回第 " + strconv.Itoa(attempt) + " 次尝试的请求排队中（已等 " + waitText(now.Sub(since)) +
		"）：先等这一章的会话跑完手上这一轮再搬（不打断它，那一次会整棵留档，之后能切回来接着跑）"
	switch age := r.chapterLastOutputAge(proj, base); {
	case age < 0:
		msg += "；这一章还没有任何输出"
	case age > retryBusyLogEvery:
		msg += "；这一章最后一次输出在 " + waitText(age) + "前——它可能卡住了（实在等不下去就重启程序，队列里的请求不会丢）"
	default:
		msg += "；这一章最后一次输出在 " + waitText(age) + "前，它还在动"
	}
	r.log.LogInfo(0, "[restore]", base, msg)
}

// retryBusyLogEvery 是"这一章忙、请求留队"这类提示的重复间隔：第一次立刻打，
// 之后每隔这么久再说一次。太密会把日志刷爆（tick 是 2 秒一次），太稀又看不出
// 到底卡了多久。
const retryBusyLogEvery = 5 * time.Minute

// logBusySkip 报告"请求排上了，但这一章还有会话在跑"（T232）。
//
// 派发端只 TryLock：拿不到就留队等下一 tick。以前这条分支一声不响，页面上点
// 「重新核对」/「继续跑」既不新建尝试、也不显示在排队——用户看到的就是"点了没
// 反应"，只有截断（换一条干净的尝试）能绕过去。这里把同样的信息写进日志：等了
// 多久、这一章最后一次输出在多久之前；两个数字一起看就知道是"真在跑、等着就行"
// 还是"这一章已经没有输出了、会话卡住了"。
func (r *Runner) logBusySkip(proj, base, role, reqPath string) {
	now := time.Now()
	r.busyMu.Lock()
	if r.retryBusySince == nil {
		r.retryBusySince = map[string]time.Time{}
	}
	if r.retryBusyLogged == nil {
		r.retryBusyLogged = map[string]time.Time{}
	}
	since, ok := r.retryBusySince[reqPath]
	if !ok {
		since = now
		r.retryBusySince[reqPath] = since
	}
	last := r.retryBusyLogged[reqPath]
	if !last.IsZero() && now.Sub(last) < retryBusyLogEvery {
		r.busyMu.Unlock()
		return
	}
	r.retryBusyLogged[reqPath] = now
	r.busyMu.Unlock()

	what := "还有会话在跑"
	switch role {
	case "checker":
		what = "还有核对在跑"
	case "continue", "convert":
		what = "还有转换在跑"
	}
	msg := "请求排队中：" + what + "（已等 " + waitText(now.Sub(since)) + "）"
	switch age := r.chapterLastOutputAge(proj, base); {
	case age < 0:
		msg += "；还没看到这一章的任何输出"
	case age > retryBusyLogEvery:
		msg += "；这一章最后一次输出在 " + waitText(age) + "前——它可能卡住了，等不下去时只能重启进程（队列里的请求不会丢）"
	default:
		msg += "；这一章最后一次输出在 " + waitText(age) + "前，它还在动"
	}
	r.log.LogInfo(0, "[retry]", base, msg)
}

// waitText 把时长写成"19 分钟"这种一眼能读的形式。
func waitText(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(d.Seconds())) + " 秒"
	}
	if d < time.Hour {
		return strconv.Itoa(int(d.Minutes())) + " 分钟"
	}
	return strconv.Itoa(int(d.Hours())) + " 小时"
}

// chapterLastOutputAge 是一章（含它的递归子章节）最近一次写盘的"年岁"：转录与
// 产物都算。找不到任何文件时返回 -1（这一章还没开始过）。
//
// T240：子章节的转录不在 `<proj>/work/sessions/` 里，而在父章节的临时工作区
// `<proj>/work/temp/conv_<父章>/work/_subdivision/…/node/sessions/` 下（P34）。
// 只看顶层目录时，父节点停在「等待子会话」的那几个小时里它自己一行都不写，而
// 子树其实在猛跑——排队提示据此说出"这一章最后一次输出在 1 小时前——它可能卡住
// 了、等不下去只能重启"，把"还在动"误报成"卡住"，用户就去重启一个干得好好的
// 进程。所以这里把子树里属于这一章的转录也算进来。
func (r *Runner) chapterLastOutputAge(proj, base string) time.Duration {
	newest := time.Time{}
	consider := func(p string) {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	sessDir := filepath.Join(proj, "work", "sessions")
	if ents, err := os.ReadDir(sessDir); err == nil {
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") || !strings.Contains(e.Name(), base) {
				continue
			}
			consider(filepath.Join(sessDir, e.Name()))
		}
	}
	// ② 递归子章节的转录：父章节临时工作区里的 node/sessions/ 与 .partial。
	// 走这棵树时按名字过滤（子节点名一定以父章节名开头），只看 .jsonl 与
	// .partial；条目数设上限，几千个文件的父章不至于把 tick 拖住。
	walkRoot := filepath.Join(proj, "work", "temp", "conv_"+topChapterOf(base), "work")
	seen := 0
	_ = filepath.WalkDir(walkRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		seen++
		if seen > 20000 {
			return fs.SkipAll
		}
		name := d.Name()
		if !strings.Contains(name, base) {
			return nil
		}
		if strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".partial") {
			consider(p)
		}
		return nil
	})
	consider(filepath.Join(proj, "work", "chapters", base+".tex"))
	if newest.IsZero() {
		return -1
	}
	return time.Since(newest)
}

// topChapterOf 把递归子章节名收敛到最外层的顶层章节（chapter_004_child_02_child_02
// → chapter_004）；顶层章节原样返回。子节点的转录都挂在顶层章节的临时工作区下。
func topChapterOf(base string) string {
	if i := strings.Index(base, "_child_"); i > 0 {
		return base[:i]
	}
	return base
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

// childChapterParent 拆出递归子章节所属的顶层父章节
// （chapter_004_child_02 → chapter_004；嵌套子章节取到最外层）。
func childChapterParent(base string) (string, bool) {
	i := strings.Index(base, "_child_")
	if i <= 0 {
		return "", false
	}
	return base[:i], true
}

// requeueParentAfterChildPass：子章节的核对这一轮**通过了**，父节点却还停在
// 「等待子会话」——它只能靠人点一下「继续跑」才醒。这里替人点这一下（T231）。
// 顶层章节没有父节点，什么也不做。
func (r *Runner) requeueParentAfterChildPass(proj, project, base string) {
	parent, isChild := childChapterParent(base)
	if !isChild {
		return
	}
	r.queueParentContinueFor(proj, project, parent, base)
}

// queueParentContinueFor 把"子章节单独重转"改成排队父章节的「继续跑」（T211）。
//
// 为什么不能单独重跑：子章节的产物与整合都归父节点——重跑完这一支没有谁去整合
// 它，父节点的 children-summary 与问题继承清单也会与产物脱节。父章节已经排着
// 一条「继续跑」时不重复排。
func (r *Runner) queueParentContinueFor(proj, project, parent, child string) {
	for _, f := range retryq.Pending(proj) {
		if f.Chapter == parent && retryRoleOf(f.Request) == "continue" {
			r.log.LogInfo(0, "[retry]", child, "父章节 "+parent+" 已有「继续跑」在排队，不重复排")
			return
		}
	}
	if _, err := retryq.Queue(proj, retryq.Request{
		Project: project, Chapter: parent, Role: "continue",
		Reason: "子章节 " + child + " 的重试（改为继续跑父章节）", Source: "latex",
	}); err != nil {
		r.log.LogWarning(0, "[retry]", child, "改为继续跑父章节失败:", err)
		return
	}
	r.log.LogInfo(0, "[retry]", child, "子章节不能单独重跑，已排队父章节 "+parent+" 的「继续跑」")
}

// runRetryTask 执行一条重试请求（已在章节锁里）。
func (r *Runner) runRetryTask(proj, base string, req retryq.Request) {
	role := retryRoleOf(req)
	reason := strings.TrimSpace(req.Reason)
	// T232：重跑之前先把这一支里"提醒已经打满"的子章节核对转录换新尝试。不换
	// 的话父章节的「继续跑」会在子章节的核对入口守卫上当场失败，请求被消费、
	// 页面什么都没变——用户看到的就是"点了没反应"。
	if role != "checker" {
		r.rotateSpentChildCheckers(proj, base)
	}
	switch role {
	case "convert":
		r.forceChapterRetry(proj, base, reason)
		if err := r.convertChapterNow(proj, base); err != nil {
			r.log.LogWarning(0, "[convert]", base, "手动重试失败:", err)
		}
	case "checker":
		if req.From > 0 && req.CutAttempt > 0 {
			// T228：切口已由提交方当场做完，只把这条核对会话推下去。
			r.log.LogInfo(0, "[checker]", base, "截断已在提交时当场落到磁盘（第",
				strconv.Itoa(req.CutAttempt), "次尝试），直接继续核对")
			if err := r.continueCheckerAfterCut(proj, base, req.CutAttempt); err != nil {
				r.log.LogWarning(0, "[checker]", base, "从记录继续核对失败:", err)
			}
		} else if req.From > 0 {
			// T184：核对的"从某条记录之后继续"——只换核对会话，产物一个字不动。
			// 不走 forceCheckerRetry：它自己会分配下一个尝试号，和截断这条撞车。
			if err := r.continueCheckerFrom(proj, base, req.Path, req.From); err != nil {
				r.log.LogWarning(0, "[checker]", base, "从记录继续核对失败:", err)
			}
		} else {
			r.forceCheckerRetry(proj, base)
			ok, issues := r.recheckChapter(proj, base, 0)
			if issues != "" {
				r.log.LogInfo(0, "[checker]", base, "重新核对的结论:", issues)
			}
			// T231：子章节这一支由父节点负责整合，它的核对后来通过了，父节点
			// 不会自己醒过来——于是"子会话全部已通过、父会话一直等待子会话"。
			// 这里替父节点排一条「继续跑」，让它重新收尾。
			if ok {
				r.requeueParentAfterChildPass(proj, req.Project, base)
			}
		}
	case "restore":
		// 切回第 N 次尝试（P63）：纯文件搬运，不跑任何会话。跑完了提示一句
		// "要不要接着核对"——自动开一条核对会立刻烧掉一轮 token，而用户往往
		// 只是想先看一眼那一版的产物。
		msg, err := RestoreChapterAttempt(proj, base, req.Attempt)
		if err != nil {
			r.log.LogWarning(0, "[restore]", base, "切回失败:", err)
			break
		}
		r.log.Log(0, "[restore]", base, msg)
		r.log.LogInfo(0, "[restore]", base, "产物已就位；要更新核对结论请点「重新核对这一章」")
	case "continue":
		if req.From > 0 && req.CutAttempt == 0 {
			if err := r.continueFromChapter(proj, base, req.Path, req.From); err != nil {
				if isPaused(err) {
					// T243：接口重试用满 = 暂停，不改口叫"失败"。
					r.log.LogWarning(0, "[convert]", base, "上游接口仍不通，这一章保持暂停（重启或点「继续跑」再试）：", err)
					r.phaseNote()("[convert] %s 因上游接口错误暂停", base)
					return
				}
				r.log.LogError(0, "[convert]", base, "从记录继续失败:", err)
				r.phaseNote()("[convert] %s 从记录继续失败：%v", base, err)
			}
			return
		}
		if req.From > 0 {
			// T228：切口在提交时就已经落在磁盘上了，这里只负责接着那条新尝试跑。
			r.log.LogInfo(0, "[convert]", base, "截断已在提交时当场落到磁盘（第",
				strconv.Itoa(req.CutAttempt), "次尝试），直接继续")
		}
		if err := r.continueChapter(proj, base, reason); err != nil {
			if kids, retryingKids := isRetryingChildren(err); retryingKids {
				// T247：子节点还在重试队列里，父章节这次没提交——不是失败。
				r.log.LogWarning(0, "[convert]", base, "子章节还在重试，这次先不整合（不记失败）:",
					strings.Join(kids, ", "))
			} else if isPaused(err) {
				// T243：接口重试用满 = 暂停，不是"继续跑失败"。
				r.log.LogWarning(0, "[convert]", base, "上游接口仍不通，这一章保持暂停（重启或点「继续跑」再试）：", err)
			} else {
				r.log.LogError(0, "[convert]", base, "继续跑失败:", err)
			}
		}
	}
}

// convertChapterNow 在运行期把一章转换跑掉（主流程之外的那条路）。
func (r *Runner) convertChapterNow(proj, base string) error {
	clsName := classNameOfFile(filepath.Join(proj, "style"))
	if clsName == "" {
		return fmt.Errorf("样式还没就绪（style 阶段未完成）")
	}
	chapPath := filepath.Join(proj, "chapters", base+".md")
	if !fileExists(chapPath) {
		return fmt.Errorf("章节不存在: %s", base)
	}
	return r.convertOneChapter(proj, clsName, filepath.Join(proj, "style", "manual.md"), chapPath,
		filepath.Join(proj, "work"), 1, 0, true)
}

// continueChapter 让这一章**接着上一次尝试干**（P53 阶段二）：不新建尝试、不
// 归档、不动已提交的产物，只是让那条会话再跑一轮、自己把活收尾。
//
// 现场是这样的：会话因为厂商报错/上下文跑偏/被手动改过文件而停住，重启整个
// 流程要重烧一整本的 token。这里给的是"接着这一条继续"。
//
// **不发任何 user 内容**：历史原样续行（与中断续跑同一条规矩），模型接着它
// 最后那句往下说。真跑完没 submit 时，T94 的"没提交"提醒会补一条——那才是
// 唯一真会插进去的 user 消息。
func (r *Runner) continueChapter(proj, base, reason string) error {
	if reason == "" {
		reason = "手动继续"
	}
	r.log.Log(0, "[convert]", base, "手动继续：接着最近一次尝试跑（不新建尝试、不插消息）")
	r.setContinueFlag(base)
	defer r.setContinueFlag(base)
	return r.convertChapterNow(proj, base)
}

// continueFromChapter 从转录的第 from 条记录之后继续（P53 阶段二 的"从某个错误的地方
// 开始"）：把前 from 行抄成**一条新尝试**，后面的轮次丢掉，然后让它接着干。
//
// 源转录一个字不动——原记录还在「历史」里，看得见、也删得掉。这样"回滚到某一
// 步"不会毁掉现场，只是多出一条从那儿分叉出去的尝试。
//
// P55：**连工作区一起退**。每落一条消息都拍过快照（见 rounds.go），这里把
// 本章工作树 reset 到那一行对应的 commit——只退对话会让模型困惑：它记得的是
// 那一刻的文件，一 read_file 却发现后面乱改的还在。快照缺失（老会话/git 不
// 可用）时降级成只退对话，并在日志里说清。提交过的产物一并留档挪走：那一刻
// 未必已经提交，"未来"交出来的 .tex 留在原地只会把模型带偏。
func (r *Runner) continueFromChapter(proj, base, srcName string, from int) error {
	dst, next, err := r.truncateChapterTranscript(proj, base, srcName, from)
	if err != nil {
		return err
	}
	rolled, ok := r.applyWorkspaceCut(proj, base, srcName, from)
	if r.retryAttempt == nil {
		r.retryAttempt = map[string]int{}
	}
	r.retryAttempt[base] = next
	r.setContinueFlag(base)
	defer r.setContinueFlag(base)
	note := "（源转录保留；这条会话没有工作区快照，只回退对话、工作区保持最后一次的样子；不发 user 内容）"
	if ok {
		note = fmt.Sprintf("（源转录保留；工作区回退到第 %d 行；不发 user 内容）", rolled)
	}
	r.log.Log(0, "[convert]", base, "从第", strconv.Itoa(from), "条记录继续：新尝试写",
		filepath.Base(dst), note)
	return r.convertChapterNow(proj, base)
}

// applyWorkspaceCut 把工作区退到第 from 行那一刻（P55），返回退到的行号与是否
// 真退了。真退了才挪走已提交的产物——退不了还挪走 `.tex`，就成了"工作区留着
// 后来的改动、产物却没了"的半新半旧状态，比不回退更让人困惑；退不了时行为与
// P55 之前完全一致（只退对话、工作区与产物都不动）。
func (r *Runner) applyWorkspaceCut(proj, base, srcName string, from int) (int, bool) {
	return r.applyWorkspaceCutTo(proj, base, srcName, from, "")
}

// applyWorkspaceCutTo 与 applyWorkspaceCut 相同，只是把"已提交的产物"挪到
// archiveDir（空 = work/attempts/<章>/try<源尝试>/）。T241：复盘原地截断时尝试号
// 不变，产物不能挪进 attempts——那会把"作废留档"和"现役这一次"搅成同一个
// 目录，切回第 N 次尝试时搬回来的会是它自己刚作废的产物。改挪进
// work/rescue/cuts/<章>/，只作证据。
func (r *Runner) applyWorkspaceCutTo(proj, base, srcName string, from int, archiveDir string) (int, bool) {
	rolled, ok := r.rollbackChapterWork(proj, base, srcName, from)
	if ok {
		if archiveDir != "" {
			archiveChapterProductsTo(r, proj, base, archiveDir)
		} else {
			archiveChapterProducts(r, proj, base, srcName)
		}
	}
	return rolled, ok
}

// archiveChapterProducts 把这一章已提交的产物与报告挪进 work/attempts/<章>/
// try<源尝试>/，不动工作树——工作树要留着给 P55 回退。
func archiveChapterProducts(r *Runner, proj, base, srcName string) {
	attempt := 1
	if _, n, ok := transcriptAttempt(srcName); ok && n > 0 {
		attempt = n
	}
	archiveChapterProductsTo(r, proj, base, filepath.Join(proj, "work", "attempts", base, fmt.Sprintf("try%d", attempt)))
}

// archiveChapterProductsTo 是 archiveChapterProducts 的目的地显式版。
func archiveChapterProductsTo(r *Runner, proj, base, dst string) {
	submitRoot := filepath.Join(proj, "work", "chapters")
	targets := []struct{ src, name string }{
		{filepath.Join(submitRoot, base+".tex"), base + ".tex"},
		{filepath.Join(submitRoot, base), base},
		{filepath.Join(proj, "work", "reports", base+".md"), base + ".report.md"},
	}
	moved := 0
	for _, t := range targets {
		if !fileExists(t.src) {
			continue
		}
		if err := os.MkdirAll(dst, 0o755); err != nil {
			r.log.LogWarning(0, "[convert]", base, "产物留档目录创建失败:", err)
			return
		}
		if err := movePath(t.src, filepath.Join(dst, t.name)); err != nil {
			r.log.LogWarning(0, "[convert]", base, "产物留档失败:", filepath.Base(t.src), err)
			continue
		}
		moved++
	}
	_ = os.Remove(filepath.Join(submitRoot, base+".tex.checker"))
	if moved > 0 {
		shown := dst
		if rel, err := filepath.Rel(proj, dst); err == nil && !strings.HasPrefix(rel, "..") {
			shown = filepath.ToSlash(rel)
		}
		r.log.Log(0, "[convert]", base, "已提交的产物挪进 "+shown+"/")
	}
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
	// 转录不一定在顶层 `work/sessions/`：递归子章节的转录跟着自己的节点走
	// （`<节点>/sessions/`），拿顶层目录硬拼一条路径出来只会"读不到源转录"。
	src := findChapterTranscript(proj, base, name)
	if src == "" {
		return "", 0, fmt.Errorf("读不到源转录 %s: 顶层 work/sessions/ 与细分节点里都没有这个文件", name)
	}
	floor := 2
	if prev := r.retryAttemptOf(base); prev >= floor {
		floor = prev + 1
	}
	return cutTranscriptAt(src, from, floor)
}

// cutTranscriptNow 在**作出截断结论的这一刻**把前缀切成一条新尝试并落盘（T228）。
//
// 与 truncateChapterTranscript 的区别：源转录是已知的那一条——复盘目标带着完整
// 路径（递归子章节的转录在父工作区的细分树里），不用再按章节名去猜；尝试号是
// "这条转录的号 + 1"，新尝试就排在它后面，续跑时按磁盘推导会自动选中它
// （convertOneChapterSeq：retryAttemptOf 优先，其次是 lastAttemptNumber）。
//
// 这里只做纯文件操作：源转录一字不动、页面的「历史」立刻多一条。调用方负责把
// 工作区一起退回那一刻（applyWorkspaceCut）与排进重试队列。
func (r *Runner) cutTranscriptNow(proj, base, role, trPath string, from int) (string, int, error) {
	trPath = strings.TrimSpace(trPath)
	if trPath == "" || !fileExists(trPath) {
		return "", 0, fmt.Errorf("源转录不存在: %s", trPath)
	}
	if role == "checker" {
		// 核对转录：新尝试照旧写在 work/sessions/ 下（recheckChapter 按那儿找）。
		rel, err := filepath.Rel(proj, trPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", 0, fmt.Errorf("核对转录不在本项目之下: %s", trPath)
		}
		return r.truncateCheckerTranscript(proj, base, filepath.ToSlash(rel), from)
	}
	floor := transcriptAttemptOf(trPath) + 1
	if prev := r.retryAttemptOf(base); prev >= floor {
		floor = prev + 1
	}
	dst, next, err := cutTranscriptAt(transcriptStem(trPath), from, floor)
	if err != nil {
		return "", 0, err
	}
	if r.retryAttempt == nil {
		r.retryAttempt = map[string]int{}
	}
	r.retryAttempt[base] = next
	return dst, next, nil
}

// cutChapterTranscriptForRetry 是重试队列用的"先截断、再排队"这一步：请求里
// 的 Path 是转录文件名，From 是 1 基行号。切成功返回新转录名，失败返回空串
// 并**大声**记一条错误（旧代码在这里只写一行 warning，页面上却已经回执"已
// 截断"，于是"截断"看起来发生了、实际一行没少，子会话一轮又一轮重放坏尾巴）。
func (r *Runner) cutChapterTranscriptForRetry(proj, base, srcName string, from int) string {
	// 请求里带的行号可能落在用量/元信息行上（复盘会话常把"最后一条好消息的下
	// 一行"当切口）：先落位，日志与调用都用真正切到的位置（T245）。
	if src := findChapterTranscript(proj, base, srcName); src != "" {
		if fixed, moved := snapTranscriptFile(src, from); moved {
			r.log.LogWarning(0, "[retry]", base, "请求的切口在第", strconv.Itoa(from),
				"行（用量/元信息行），挪到第", strconv.Itoa(fixed), "行")
			from = fixed
		}
	}
	dst, next, err := r.truncateChapterTranscript(proj, base, srcName, from)
	if err != nil {
		r.log.LogError(0, "[retry]", base, "按请求截断转录失败（继续排父章节，但这一支没被截）:", err)
		r.phaseNote()("[retry] %s 截断失败：%v", base, err)
		return ""
	}
	r.log.LogInfo(0, "[retry]", base, "已按请求截断到第", strconv.Itoa(from), "行，新尝试写",
		filepath.Base(dst), "（第", strconv.Itoa(next), "次尝试）")
	return filepath.Base(dst)
}

// transcriptStem 把 `convert_<章>[.tryN].jsonl` 还原成"第 1 次尝试"的名字：
// 新的尝试号一律从它派生，避免切一条 `.try3` 却写出 `.try3.try2.jsonl`。
func transcriptStem(path string) string {
	ext := filepath.Ext(path)
	return trimAttemptSuffix(strings.TrimSuffix(path, ext)) + ext
}

// cutTranscriptAt 把 src 的前 from 行抄成**同目录**下的下一条尝试，返回新文件
// 与它的尝试号（floor 是允许的最小号）。源转录一个字不动——保留现场，回滚只
// 是多出一条从那儿分叉的尝试。
func cutTranscriptAt(src string, from, floor int) (string, int, error) {
	if from < 1 {
		return "", 0, fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", 0, fmt.Errorf("读不到源转录 %s: %w", filepath.Base(src), err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	// 只能切在消息行上：meta/usage 行不进消息流，切在那儿等于没切。挪不过去
	// （整条转录没有消息行）才算错。
	from, _, err = snapCutLine(lines, from)
	if err != nil {
		return "", 0, err
	}
	baseTr := transcriptStem(src)
	if floor < 2 {
		floor = 2
	}
	next := floor
	for n := floor; n <= floor+999; n++ {
		if !fileExists(attemptTranscriptPath(baseTr, n)) {
			next = n
			break
		}
	}
	dst := attemptTranscriptPath(baseTr, next)
	head := strings.Join(lines[:from], "\n") + "\n"
	if err := os.WriteFile(dst, []byte(head), 0o644); err != nil {
		return "", 0, fmt.Errorf("写新尝试的转录失败: %w", err)
	}
	return dst, next, nil
}

// snapCutLine 把切口挪到最近的一条消息行上（先往前找，前面没有才往后找）。
//
// 为什么不再直接拒绝：`from` 是"保留到第几条记录"的物理行号，可转录里夹着
// t=meta / t=usage 行（只记录、不回放）。复盘会话读到"最后一条是第 237 行"，
// 很自然就填 238 —— 那正好是紧跟其后的用量行；于是每一次截断都被判"不是一条
// 消息记录"，请求原样再派一次、还是同一个数，这一章就再也续不上（实测两章连
// 撞两次，一次在原地、一次在派发端，每次都要重跑一整轮父节点）。往前挪到最近
// 的消息行，会话读到的内容一条不少，切口也仍然落在消息流上。
//
// 返回挪后的行号与是否挪过；整条转录里一条消息行都没有才算错。
func snapCutLine(lines []string, from int) (int, bool, error) {
	if from < 1 {
		return 0, false, fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	if from > len(lines) {
		return 0, false, fmt.Errorf("截断点 %d 超出转录长度 %d", from, len(lines))
	}
	isMsg := func(i int) bool {
		var probe struct {
			T string `json:"t"`
		}
		if err := json.Unmarshal([]byte(lines[i]), &probe); err != nil {
			return false
		}
		return probe.T == "msg"
	}
	for i := from - 1; i >= 0; i-- {
		if isMsg(i) {
			return i + 1, i+1 != from, nil
		}
	}
	for i := from; i < len(lines); i++ {
		if isMsg(i) {
			return i + 1, true, nil
		}
	}
	return 0, false, fmt.Errorf("第 %d 行前后都没有一条消息记录（整条转录只有用量/元信息行？）", from)
}

// snapTranscriptFile 读转录文件、把切口挪到消息行上；读不到就原样返回。
func snapTranscriptFile(path string, from int) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return from, false
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	fixed, moved, err := snapCutLine(lines, from)
	if err != nil {
		return from, false
	}
	return fixed, moved
}

// balancedCutFrom 把切口往前挪到"所有工具调用都已经拿到回执"的那一条记录上。
//
// 为什么必须挪：截在一条还在等回执的 assistant 消息后面，续跑时那些调用只能
// 补占位回执（"这个调用没有回执，已跳过"）。模型看到自己上一轮"调了四个工具、
// 一个结果都没有"，下一句就是"这个会话里工作区/编译/提交工具都不可用"——实测
// 一族章节连切十次全栽在这句话上，每切一次就多一条同样退化、同样只活几轮的
// 尝试。切口挪回真正平衡的那一条，回放的上下文里工具全部是有结果的。
//
// 返回保留的前缀行数（1 基，等于"保留到第几条记录"）；0 = 整段前缀都不平衡。
// 老转录没有 tool_call_id 时按先后顺序一对一对销（tool 行的 role 是 tool）。
func balancedCutFrom(lines []string, from int) int {
	type probe struct {
		T     string `json:"t"`
		Role  string `json:"role"`
		Calls []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
		CallID string `json:"tool_call_id"`
	}
	var pending []string
	last := 0
	for i := 0; i < from && i < len(lines); i++ {
		var p probe
		if err := json.Unmarshal([]byte(lines[i]), &p); err != nil || p.T != "msg" {
			continue
		}
		for _, c := range p.Calls {
			pending = append(pending, c.ID)
		}
		if p.Role == "tool" {
			switch {
			case p.CallID != "":
				for j, id := range pending {
					if id == p.CallID {
						pending = append(pending[:j], pending[j+1:]...)
						break
					}
				}
			case len(pending) > 0:
				pending = pending[1:]
			}
		}
		if len(pending) == 0 {
			last = i + 1
		}
	}
	return last
}

// extendPastToolRound 把切口后移到"最后一个完整工具轮"的末尾（T269）。
//
// 场景：复盘会话指的切口正好落在一条工具调用消息上（例如细分父节点申请子会话
// 的那一条）。按 T241 的口径，这一条连同它下一行的回执一起被裁掉——平衡是保住
// 了，可"我申请过子章节"这个事实也从回放的上下文里消失了（实测一章的
// request_chapter_split 就这样丢过一次，只能手工从 cuts 备份里补回来）。
//
// 只要这一轮的调用都在紧跟的行里拿到了回执，就把切口挪到回执之后：丢掉的仍然
// 是复盘判定的退化部分，只是不再切在"调用与回执之间"。找不到完整回执就原样不动。
func extendPastToolRound(lines []string, keep int) int {
	if keep < 1 || keep >= len(lines) {
		return keep
	}
	var head struct {
		T     string `json:"t"`
		Role  string `json:"role"`
		Calls []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(lines[keep]), &head); err != nil {
		return keep
	}
	if head.T != "msg" || head.Role == "tool" || len(head.Calls) == 0 {
		return keep
	}
	pending := make(map[string]bool, len(head.Calls))
	var order []string
	for _, c := range head.Calls {
		pending[c.ID] = true
		order = append(order, c.ID)
	}
	lastReceipt := 0
	for i := keep + 1; i < len(lines); i++ {
		var p struct {
			T      string `json:"t"`
			Role   string `json:"role"`
			CallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal([]byte(lines[i]), &p); err != nil {
			return keep
		}
		switch p.T {
		case "meta", "usage":
			continue
		case "msg":
			if p.Role != "tool" {
				return keep
			}
			if p.CallID != "" {
				delete(pending, p.CallID)
			} else {
				// 老转录没有 tool_call_id：按先后顺序一对一销。
				for _, id := range order {
					if pending[id] {
						delete(pending, id)
						break
					}
				}
			}
			lastReceipt = i + 1
			if len(pending) == 0 {
				return lastReceipt
			}
		default:
			return keep
		}
	}
	return keep
}

// trimTranscriptInPlace 把一条转录**原地**截到第 from 条记录之前（T241）：同一条
// 文件、同一个尝试号，页面的「历史」不多出一条。
//
// 复盘截断以前是"抄前 N 行成下一条尝试"（cutTranscriptNow），于是一次真实会话
// 被复盘十次就在历史里变成十条：每条都是同一个退化现场的前缀，谁也看不出它们
// 本来是同一件事，还跟"核对多次打回"、"手动重试"、"子会话切换"这些**真**换尝试
// 的操作混在一起。复盘是"这条会话跑歪了、退回去重说"，它不该占一次尝试。
//
// 丢掉的那一段不销毁：抄进 `<proj>/work/rescue/cuts/<章>/`（只给查证据用，不进
// 历史、不可切回）。切口上补一条 `t=meta kind=cut` 记录，历史和转录里都能看出
// "这里被复盘截过、截到第几条"。
func (r *Runner) trimTranscriptInPlace(proj, base, trPath string, from int, reason string) (int, int, error) {
	trPath = strings.TrimSpace(trPath)
	if trPath == "" || !fileExists(trPath) {
		return 0, 0, fmt.Errorf("源转录不存在: %s", trPath)
	}
	if from < 1 {
		return 0, 0, fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	data, err := os.ReadFile(trPath)
	if err != nil {
		return 0, 0, fmt.Errorf("读不到源转录 %s: %w", filepath.Base(trPath), err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	asked := from
	from, snapped, err := snapCutLine(lines, from)
	if err != nil {
		return 0, 0, err
	}
	if snapped && r.log != nil {
		r.log.LogWarning(0, "[rescue]", base, "切口从第", strconv.Itoa(asked), "条挪到第",
			strconv.Itoa(from), "条：那一行是用量/元信息行，不进消息流")
	}
	keep := balancedCutFrom(lines, from)
	if keep < 1 {
		return 0, 0, fmt.Errorf("前 %d 行里没有一步是工具调用都已回执的，截了也接不下去", from)
	}
	if keep < from {
		r.log.LogWarning(0, "[rescue]", base, "切口从第", strconv.Itoa(from), "条前移到第",
			strconv.Itoa(keep), "条：后面那条还在等工具回执，截在那儿续跑只会读到占位回执")
	}
	// T269：切口若正落在一条工具调用上，把这一轮（调用 + 回执）留住再切——
	// 复盘判的是"后面那一段退化"，不是"你刚才申请过子章节"。
	if adv := extendPastToolRound(lines, keep); adv > keep {
		r.log.LogInfo(0, "[rescue]", base, "切口从第", strconv.Itoa(keep), "条后移到第",
			strconv.Itoa(adv), "条：保留最后一个完整的工具调用轮（调用与回执不能被切开）")
		keep = adv
	}
	tail := lines[keep:]
	if len(tail) > 0 {
		if p, err := saveCutEvidence(proj, base, trPath, tail); err != nil {
			r.log.LogWarning(0, "[rescue]", base, "丢掉的那一段没能留档:", err)
		} else {
			r.log.LogInfo(0, "[rescue]", base, "丢掉的那一段留档:", p)
		}
	}
	meta, err := json.Marshal(map[string]any{
		"t": "meta", "kind": "cut", "ts": time.Now().Format(time.RFC3339),
		"from": keep, "dropped": len(tail), "reason": reason,
	})
	if err != nil {
		return 0, 0, err
	}
	head := strings.Join(lines[:keep], "\n") + "\n" + string(meta) + "\n"
	// 先写临时文件再 rename：转录随时可能被预览页/复盘读到，读到一个半截文件
	// 比什么都没做更糟。
	tmp := trPath + ".cut-tmp"
	if err := os.WriteFile(tmp, []byte(head), 0o644); err != nil {
		return 0, 0, fmt.Errorf("写截断后的转录失败: %w", err)
	}
	if err := os.Rename(tmp, trPath); err != nil {
		_ = os.Remove(tmp)
		return 0, 0, fmt.Errorf("截断后的转录就位失败: %w", err)
	}
	return keep, len(tail), nil
}

// saveCutEvidence 把复盘截断丢掉的那一段抄进 work/rescue/cuts/<章>/。
func saveCutEvidence(proj, base, trPath string, tail []string) (string, error) {
	dir := filepath.Join(proj, "work", "rescue", "cuts", base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := time.Now().Format("20060102-150405") + "-" + filepath.Base(trPath)
	if !strings.HasSuffix(name, ".jsonl") {
		name += ".jsonl"
	}
	dst := filepath.Join(dir, name)
	return dst, os.WriteFile(dst, []byte(strings.Join(tail, "\n")+"\n"), 0o644)
}

// findChapterTranscript 在磁盘上找这一章的一条转录（只按文件名找）：先看顶层
// `work/sessions/`，再顺着这本书的细分树找 `<节点>/sessions/`。
func findChapterTranscript(proj, base, name string) string {
	if p := filepath.Join(proj, "work", "sessions", name); fileExists(p) {
		return p
	}
	top := base
	if i := strings.Index(top, "_child_"); i > 0 {
		top = top[:i]
	}
	if top == "" {
		return ""
	}
	roots, _ := filepath.Glob(filepath.Join(proj, "work", "temp", "conv_"+top+"*", "work", "_subdivision"))
	for _, root := range roots {
		hit := ""
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || hit != "" || d.IsDir() {
				return nil
			}
			if d.Name() == name && filepath.Base(filepath.Dir(p)) == "sessions" {
				hit = p
			}
			return nil
		})
		if hit != "" {
			return hit
		}
	}
	return ""
}

// vectorRetryPrecondition 判断"重画这张图"现在能不能做（空串 = 可以）。
func (r *Runner) vectorRetryPrecondition(proj, trName string) string {
	name := filepath.Base(strings.TrimSpace(trName))
	if !strings.HasPrefix(name, "vector_") || !strings.HasSuffix(name, ".jsonl") {
		return "只认 vector_*.jsonl 这条转录"
	}
	if !fileExists(filepath.Join(proj, "source", "progress_items")) {
		return "这个项目还没有逐图进度（images 阶段没跑过）"
	}
	// 作图转录在 <proj>/source/sessions/ 里（images 阶段的 outDir），不在 work。
	if !fileExists(filepath.Join(proj, "source", "sessions", name)) {
		return "这条作图转录已经不在了"
	}
	return ""
}

// redrawVectorImage 重画一张矢量图（P53 的"矢量图那一侧"）：把这张图的作图
// 会话作废留档、清掉它的成品与状态，然后只让那一条图重新做一遍。
//
// 与章节重试同一个思路——**开新尝试**：旧转录与旧工作区挪进
// work/attempts/vector_<成品名>/try<N>/，新会话从零开始（图片重画没有"接着
// 上次画错的那半张继续"的意义）。分类与 dedupe 不动：类别、分组、代表图都是
// 上一次已经定下来的，重画只该换这张图的画法。
//
// 注意下游：作图结果会重新嵌进 md，样式/划分/转换/组装要看新图就得重跑。
func (r *Runner) redrawVectorImage(proj, trName, reason string, from int) error {
	key, rec, archived, err := prepareVectorRedraw(proj, trName, from)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	what := "手动重画"
	if from > 0 {
		what = "从第 " + strconv.Itoa(from) + " 条记录继续"
	}
	if reason != "" {
		what += "（" + reason + "）"
	}
	archNote := "没有旧转录"
	if archived != "" {
		archNote = "旧转录留档为 " + archived
	}
	r.log.Log(0, "[vector]", filepath.Base(rec.ImgPath), what+"："+archNote+
		" | 类别/分组不变，只重做这一张")

	inline := r.inline || looksInlineProject(proj)
	srcDir := r.imagesSrcDir
	if srcDir == "" {
		srcDir = r.cfg.Paths.OutputDir
	}
	err = r.RunImages(ImagesOptions{
		Inline:    inline,
		SourceDir: srcDir,
		OutDir:    filepath.Join(proj, "source"),
		Only:      []string{key},
	})
	if err != nil {
		return err
	}
	r.log.Log(0, "[vector]", filepath.Base(rec.ImgPath), "重画完成——下游（样式/划分/转换/组装）需要重跑才能看到新图")
	return nil
}

// archiveVectorAttempt 把一张图的旧作图会话改名成 .try<N>.jsonl 留档（与章节
// 尝试同一个套路：旧的那次仍能在预览页「矢量图」里看到，不是删掉），旧工作区
// 挪进 work/attempts/vector_<成品名>/try<N>.work/。返回留档后的转录名。
//
// 为什么是改名而不是挪走：扫描器把根下**任何** *.jsonl 都当转录，挪进
// work/attempts 反而会让它变成一条名字古怪的会话；就地加 .tryN 既能留档，
// 又能让 vectorImageParts 继续把它归到同一张图。
func archiveVectorAttempt(proj, texBase string) string {
	sessDir := filepath.Join(proj, "source", "sessions")
	tr := filepath.Join(sessDir, "vector_"+texBase+".jsonl")
	work := filepath.Join(sessDir, "vector_"+texBase+".work")
	if !fileExists(tr) && !fileExists(work) {
		return ""
	}
	n := 1
	for ; n < 999; n++ {
		if !fileExists(filepath.Join(sessDir, "vector_"+texBase+".try"+strconv.Itoa(n)+".jsonl")) {
			break
		}
	}
	dst := filepath.Join(sessDir, "vector_"+texBase+".try"+strconv.Itoa(n)+".jsonl")
	if fileExists(tr) {
		if err := os.Rename(tr, dst); err != nil {
			return ""
		}
	}
	if fileExists(work) {
		_ = os.Rename(work, filepath.Join(proj, "work", "attempts", "vector_"+texBase,
			"try"+strconv.Itoa(n)+".work"))
	}
	return filepath.Base(dst)
}

// looksInlineProject 猜一个项目是不是档位1（矢量图内嵌 tikz）。
// 用于"页面上的重画请求"落到一个不是本次进程跑出来的项目时：读一份项目 md 看
// 它是不是内嵌 tikz 的形态。
func looksInlineProject(proj string) bool {
	mds, _ := filepath.Glob(filepath.Join(proj, "source", "*.md"))
	for _, md := range mds {
		data, err := os.ReadFile(md)
		if err != nil {
			continue
		}
		return strings.Contains(string(data), "\\input{tikz/") || strings.Contains(string(data), "\\input{figures/")
	}
	return false
}

// prepareVectorRedraw 是"重画这张图"的文件部分（可单独测）：按转录名找回那
// 张图的进度记录、把旧作图会话留档成 .tryN、清掉成品与状态。类别、标签、去重
// 结论一律保留——重画只换画法，不动分组。
//
// 返回进度记录的键（`<md>::<图片>`，RunImages 的 Only 用它点名）、记录本身、
// 留档后的转录名。
func prepareVectorRedraw(proj, trName string, from int) (string, *imageProgress, string, error) {
	name := filepath.Base(strings.TrimSpace(trName))
	if !strings.HasPrefix(name, "vector_") || !strings.HasSuffix(name, ".jsonl") {
		return "", nil, "", fmt.Errorf("只认 vector_*.jsonl 这条转录: %s", name)
	}
	texBase := strings.TrimSuffix(strings.TrimPrefix(name, "vector_"), ".jsonl")
	if texBase == "" {
		return "", nil, "", fmt.Errorf("转录名里没有成品名: %s", name)
	}
	progDir := filepath.Join(proj, "source", "progress_items")
	prog := map[string]*imageProgress{}
	loadProgress(progDir, prog)
	key, rec := "", (*imageProgress)(nil)
	for k, p := range prog {
		if vectorTexBase(p.MDName, p.ImgPath, p.Label) == texBase {
			key, rec = k, p
			break
		}
	}
	if rec == nil {
		return "", nil, "", fmt.Errorf("找不到这张图对应的进度记录（成品名 %s）", texBase)
	}
	if rec.EquivalentTo != "" && prog[rec.EquivalentTo] != nil {
		return "", nil, "", fmt.Errorf("这张图是另一张代表图的副本（dedupe 跟随），重画请点代表图那张: %s", rec.EquivalentTo)
	}
	// from > 0 = 从某条记录之后继续（不是整张重画）：把前 from 行留下当会话的
	// 续跑起点，完整的旧转录抄一份 .tryN 留档；from = 0 = 整张重画，旧会话整个
	// 让开。两条路都清成品，好让这一张重新做。
	var archived string
	if from > 0 {
		var err error
		archived, err = truncateVectorTranscript(proj, texBase, from)
		if err != nil {
			return "", nil, "", err
		}
	} else {
		archived = archiveVectorAttempt(proj, texBase)
	}
	rec.Status = "classified"
	rec.TikzCode, rec.FigPDF, rec.FigPNG, rec.FigSVG = "", "", "", ""
	rec.Error, rec.SVGFail, rec.Kept, rec.Content = "", false, false, ""
	saveProgress(progDir, rec)
	return key, rec, archived, nil
}

// truncateVectorTranscript 是"从某条记录之后继续"的文件部分（作图会话版）：
// 把前 from 行留在原地当续跑起点（会话启动时会读它续上），完整的那份抄成
// vector_<成品名>.try<N>.jsonl 留档——原对话不会因为回滚而消失。
//
// 与章节那边的差别：章节把前缀写成**另一条**新尝试的转录，作图会话没有尝试
// 号那套管道（tikz 只认 vector_<成品名>.jsonl 这一个文件名），所以是"前缀留
// 原名、全量留档"，但对外语义一样：历史里两份都在、源内容不丢。
//
// 同样只回滚**对话**：scratch 工作区还是最后一次的样子（没有按轮次快照文件
// 树），所以要它"按现在的样子看看还差什么再画"，靠的是续跑时的那句说明。
func truncateVectorTranscript(proj, texBase string, from int) (string, error) {
	if from < 1 {
		return "", fmt.Errorf("截断点必须 ≥ 1（转录里的行号）")
	}
	sessDir := filepath.Join(proj, "source", "sessions")
	name := "vector_" + texBase + ".jsonl"
	src := filepath.Join(sessDir, name)
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("读不到作图转录 %s: %w", name, err)
	}
	text := string(data)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	// 只能切在消息行上：meta/usage 行不进消息流，切在那儿等于没切。
	snappedFrom, _, err := snapCutLine(lines, from)
	if err != nil {
		return "", err
	}
	from = snappedFrom
	n := 1
	for ; n < 999; n++ {
		if !fileExists(filepath.Join(sessDir, "vector_"+texBase+".try"+strconv.Itoa(n)+".jsonl")) {
			break
		}
	}
	archived := "vector_" + texBase + ".try" + strconv.Itoa(n) + ".jsonl"
	if err := os.WriteFile(filepath.Join(sessDir, archived), data, 0o644); err != nil {
		return "", fmt.Errorf("留档旧转录失败: %w", err)
	}
	head := strings.Join(lines[:from], "\n") + "\n"
	if err := os.WriteFile(src, []byte(head), 0o644); err != nil {
		return "", fmt.Errorf("写回截断后的转录失败: %w", err)
	}
	return archived, nil
}

// ---- 上游接口错误的原地重试（T237） ----

// apiRetryMu 保护 apiRetrySeen。
var (
	apiRetryMu   sync.Mutex
	apiRetrySeen = map[string]int{}
)

// isSessionUpstreamError 判断一条失败是不是"上游接口不通"（T237）。
//
// 为什么单独分一类：连接超时、网关 5xx、限流是**上游**的事，与这条会话做过
// 什么无关——换一条尝试、整章重转，照样在同一个请求上撞同一堵墙，代价却是一
// 整章的转换重跑。这类错误只该在同一条会话上退避重试，不该进复盘、不该截断、
// 更不该开新尝试。
//
// 明确标了"不可重试"的 4xx（配置/模型名写错）不算：重试多少次还是那个错。
func isSessionUpstreamError(msg string) bool {
	if strings.Contains(msg, "不可重试") {
		return false
	}
	for _, mark := range []string{
		"SESSION_CONNECTION_TIMEOUT", "SESSION_API_ERROR",
		"SESSION_RATE_LIMIT_EXCEEDED", "SESSION_TIMEOUT", "connection reset",
	} {
		if strings.Contains(msg, mark) {
			return true
		}
	}
	return false
}

// apiRetryDelay 是第 n 次原地重试前等多久：30 秒起翻倍，封顶 10 分钟。
//
// 上限 10 分钟不是怕等，而是怕页面上一条会话几小时都停在"排队"——用满默认的
// 30 次大约 4.5 小时，足够熬过常见的上游抖动；真断这么久，这一章就暂停（T243）。
func apiRetryDelay(n int) time.Duration {
	if n < 1 {
		n = 1
	}
	d := 30 * time.Second
	for i := 1; i < n; i++ {
		d *= 2
		if d >= 10*time.Minute {
			return 10 * time.Minute
		}
	}
	return d
}

// pausedMarker 让"暂停"的两种承载（单章、整轮收尾）都能被 isPaused 认出来。
type pausedMarker interface{ pausedMarker() }

// pausedError 是"这一章暂停"（T243）：上游接口错误连 api_error_retries 次原地
// 重试仍不通，这一章不再往下跑，它也**不是失败**——不换尝试、不归档产物、转录
// 里不写 error（页面写「已暂停」）。重试更多次没有意义（还是同一个接口错误），
// 正确的下一步是人来定：重启程序会接着这条会话跑，网页上也能点「继续跑」。
type pausedError struct {
	Base    string
	Retries int
	Cause   string
}

func (e *pausedError) pausedMarker() {}

func (e *pausedError) Error() string {
	return fmt.Sprintf("%s 因上游接口错误暂停（连 %d 次原地重试仍不通，不是这一章的失败：不换尝试、不归档产物）："+
		"重启程序会接着这条会话跑，也可以在网页点「继续跑」。最后一条错误：%s",
		e.Base, e.Retries, e.Cause)
}

// pausedChaptersError 是一轮转换的收尾错误：这一批里有章节因上游接口错误暂停
// 了（T243）。措辞刻意不叫"转换失败"——失败会让人以为这一章废了、要重转；暂停
// 只是停在原地等人接着推。
type pausedChaptersError struct{ Chapters []string }

func (e *pausedChaptersError) pausedMarker() {}

func (e *pausedChaptersError) Error() string {
	return fmt.Sprintf("%s 因上游接口错误暂停（原地重试次数已用满；不是失败：尝试、产物与核对预算都没动）："+
		"重启程序会接着跑，也可以在网页点「继续跑」", strings.Join(e.Chapters, "; "))
}

// pauseError 把这一批里暂停的章节包成整轮收尾的错误（T243）。
func pauseError(pauses []string) error {
	if len(pauses) == 0 {
		return nil
	}
	sorted := append([]string(nil), pauses...)
	sort.Strings(sorted)
	return &pausedChaptersError{Chapters: sorted}
}

// isPaused 判断错误是不是"暂停"（T243）。
func isPaused(err error) bool {
	var m pausedMarker
	return errors.As(err, &m)
}

// queueUpstreamRetry 把"这一章过一会儿再续跑同一条会话"排进队列，返回是否排上。
//
// 排的是 role=continue（同一转录、同一工作区、不建新尝试），带 NotBefore 退避
// 时间；派发端到点才动手。计数按目标（章/角色/转录）算，用满就返回 false，让
// 调用方按"暂停"收尾（T243：不重试、不记失败、不换尝试）。
func (r *Runner) queueUpstreamRetry(tid int, tag, label string, target rescueTarget, cause string) bool {
	if target.ProjDir == "" || target.Base == "" || target.TrPath == "" {
		return false
	}
	limit := r.cfg.Latex.APIErrorRetryLimit()
	key := target.Base + "/" + target.Role + "/" + filepath.Base(target.TrPath)
	apiRetryMu.Lock()
	apiRetrySeen[key]++
	n := apiRetrySeen[key]
	if n > limit {
		delete(apiRetrySeen, key)
	}
	apiRetryMu.Unlock()
	if n > limit {
		// T243：次数用完 = 暂停，不是失败。不再排队（再排一次还是同一个接口错误），
		// 也不写「会话错误」；重启进程或网页「继续跑」会接着这条会话干。
		r.log.LogWarning(tid, tag, label, "上游接口错误已重试 "+strconv.Itoa(limit)+
			" 次仍不通，这一章暂停（重启程序或网页「继续跑」接着跑，不重转）："+cause)
		return false
	}
	wait := apiRetryDelay(n)
	at := time.Now().Add(wait)
	_, err := retryq.Queue(target.ProjDir, retryq.Request{
		Project: filepath.Base(target.ProjDir), Chapter: target.Base, Role: "continue",
		Source: "upstream", NotBefore: at.Format(time.RFC3339),
		Reason: fmt.Sprintf("上游接口错误，第 %d/%d 次原地重试：%s", n, limit, cause),
	})
	if err != nil {
		r.log.LogWarning(tid, tag, label, "上游接口错误，但排队重试失败:", err)
		return false
	}
	why := fmt.Sprintf("上游接口错误（第 %d/%d 次重试，%s 后续跑同一条会话，不重转）",
		n, limit, wait)
	// 用复盘那条"已排队"标记：调用方的失败终态要写 queued 而不是 error——它
	// 几分钟后就会被这条请求续上，界面不该停在「会话错误」。
	r.markRescueQueued(target.TrPath)
	r.markTargetActivity(tid, tag, label, target, "queued", why)
	r.log.LogWarning(tid, tag, label, fmt.Sprintf(
		"%s：%s 后再续跑这条会话（第 %d/%d 次，不截断、不换尝试）", why, wait, n, limit))
	r.phaseNote()("[retry] %s 上游接口错误，%s 后原地重试（第 %d/%d 次）", target.Base, wait, n, limit)
	return true
}
