package latex

import (
	"encoding/json"
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
	if r.retryCheckerAttempt == nil {
		r.retryCheckerAttempt = map[string]int{}
	}
	r.retryCheckerAttempt[base] = next
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
	if from > len(lines) {
		return "", 0, fmt.Errorf("截断点 %d 超出转录长度 %d", from, len(lines))
	}
	var probe struct {
		T string `json:"t"`
	}
	if err := json.Unmarshal([]byte(lines[from-1]), &probe); err != nil || probe.T != "msg" {
		return "", 0, fmt.Errorf("第 %d 行不是一条消息记录（选了用量/元信息行？）", from)
	}
	baseTr := filepath.Join(dir, "checker_"+base+".jsonl")
	next := 2 // 核对尝试从 try2 起（try1 是转换阶段自带的那次核对）
	for n := 2; n <= 999; n++ {
		if !fileExists(attemptTranscriptPath(baseTr, n)) {
			next = n
			break
		}
	}
	if prev := r.retryCheckerAttempt[base]; prev >= next {
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

// latestConvertAttempt 找这一章最近一次转换尝试的号（P59）：重新核对产出的
// 锚点必须指向**做出这份产物的那次转换**。顶层章节的转换转录在
// `<proj>/work/sessions/`；递归子章节的转录跟着自己的节点走
// （`<节点>/sessions/`），节点工作区由 childNodeWork 反推，再取同级 sessions。
// 找不到任何转录时按第 1 次算——与 lastAttemptNumber 的既有口径一致。
func latestConvertAttempt(proj, base string) int {
	dir := filepath.Join(proj, "work", "sessions")
	if !fileExists(filepath.Join(dir, "convert_"+base+".jsonl")) {
		if nodeWork := childNodeWork(proj, base); nodeWork != "" {
			dir = filepath.Join(filepath.Dir(nodeWork), "sessions")
		}
	}
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
	n := r.retryCheckerAttempt[base]
	if n < 2 {
		n = 2
	}
	trPath := attemptTranscriptPath(filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl"), n)
	view := filepath.Join(proj, "work", "views", fmt.Sprintf("checker_%s_try%d", base, n))
	ok, issues := r.checkChapterAt(proj, base, chapPath, texPath, partsPath, hints, tid, trPath, view, false, latestConvertAttempt(proj, base))
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
	if role == "continue" && !fileExists(filepath.Join(proj, "work", "temp", "conv_"+base, "work")) && childNodeWork(proj, base) == "" {
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
		if role != "convert" && role != "checker" && role != "continue" {
			r.log.LogWarning(0, "[retry]", base, "未知的请求角色，忽略:", role)
			_ = retryq.Consume(f.Path)
			continue
		}
		// T196/T211：递归子章节的重转 / 接着跑需要父节点的整合上下文（章节 md、
		// 工作区、产物都在父节点的细分树里）。以前这里**只消费、只写日志**，页面
		// 上是"点了没反应"：回执说已排队，磁盘上请求被丢掉，会话一个字节没变。
		// 现在改成真的推进——把它换成父章节的「继续跑」，父节点重新驱动整棵子树
		// （这一支按磁盘上的转录续跑），改完再重新整合。
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

// childChapterParent 拆出递归子章节所属的顶层父章节
// （chapter_004_child_02 → chapter_004；嵌套子章节取到最外层）。
func childChapterParent(base string) (string, bool) {
	i := strings.Index(base, "_child_")
	if i <= 0 {
		return "", false
	}
	return base[:i], true
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
			if _, issues := r.recheckChapter(proj, base, 0); issues != "" {
				r.log.LogInfo(0, "[checker]", base, "重新核对的结论:", issues)
			}
		}
	case "continue":
		if req.From > 0 && req.CutAttempt == 0 {
			if err := r.continueFromChapter(proj, base, req.Path, req.From); err != nil {
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
			r.log.LogError(0, "[convert]", base, "继续跑失败:", err)
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
	rolled, ok := r.rollbackChapterWork(proj, base, srcName, from)
	if ok {
		archiveChapterProducts(r, proj, base, srcName)
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
	dst := filepath.Join(proj, "work", "attempts", base, fmt.Sprintf("try%d", attempt))
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
		r.log.Log(0, "[convert]", base, "已提交的产物挪进 work/attempts/"+base+"/try"+strconv.Itoa(attempt)+"/")
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
	if from > len(lines) {
		return "", fmt.Errorf("截断点 %d 超出转录长度 %d", from, len(lines))
	}
	// 只能切在消息行上：meta/usage 行不进消息流，切在那儿等于没切。
	var probe struct {
		T string `json:"t"`
	}
	if err := json.Unmarshal([]byte(lines[from-1]), &probe); err != nil || probe.T != "msg" {
		return "", fmt.Errorf("第 %d 行不是一条消息记录（选了用量/元信息行？）", from)
	}
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
