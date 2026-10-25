package latex

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 切回第 N 次尝试（P63）。
//
// 换尝试一直是**单向**的：作废的那一次整棵被搬进 `work/attempts/<章>/try<N>/`
// （archiveChapterAttempt），新尝试从空工作区开始，旧的只能翻不能回去。用户遇到
// 的是最典型的那种后悔：一次新尝试跑到一半发现"上一版其实是对的"，或者核对会话
// 自己跑坏了、想退回它还没插手那一刻接着干——页面上一排「第 1/2/3 次」，点了也
// 只是切视图，流水线用的还是最新那一次。
//
// 这里把它做成一次**对称的搬运**：
//
//	现役这次（第 L 次）整棵 → work/attempts/<章>/tryL/（含它自己的转录）
//	work/attempts/<章>/tryN/ 整棵 → 工作区（含它自己的转录，如果有）
//
// 搬的是什么和 archiveChapterAttempt 同一张清单（节点工作区三份 + 提交产物 +
// 分片目录 + 报告），所以"切回再切回来"是幂等的，不产生第二套布局。转录也要
// 一起搬：`lastAttemptNumber` 是**按 work/sessions 里最大的 .tryN 号**认"现在这
// 一次"的，只搬文件树不搬转录，切回来以后各条路径还会继续往那个更大的号上写，
// 页面上的「最新」也对不上。
//
// 不做的事（每条都是有意的）：
//   - 不重跑任何会话。切回只是把文件摆回原处，要不要接着干由用户按「继续跑 /
//     重新核对」决定——自动开一条核对会立刻烧掉一轮 token，而用户常常只是想
//     先看一眼那一版。
//   - 不动核对转录。核对结论记在它自己那条转录的 meta 里（含它核的是第几次
//     转换），那是历史事实，搬来搬去只会让"这次尝试当时的结论"消失。
//   - 不碰正在写的东西。守卫见 restorePrecondition：这一章（或它的子节点）的
//     转录 2 分钟内还有写入就不切——半个工作区被搬走时子会话还在里面写，是这
//     个功能唯一能把数据搞坏的方式。
const restoreLiveGrace = 2 * time.Minute

// attemptPath 是"一次尝试在工作区里的那些东西"：留档目录里的名字 → 工作区里的
// 真实路径。
type attemptPath struct {
	src  string // 工作区一侧的路径
	name string // 留档目录里的名字
}

// restoreLivePaths 列出顶层章节一次尝试在工作区里的全部落点。
//
// 顶层章节的节点根是 `work/temp/conv_<章>`（P34），提交产物在 `work/chapters/`
// 下，报告在 `work/reports/` 下——与 archiveChapterAttempt 搬的是同一张清单。
func restoreLivePaths(proj, base string) []attemptPath {
	nodeRoot := filepath.Join(proj, "work", "temp", "conv_"+base)
	chapters := filepath.Join(proj, "work", "chapters")
	return []attemptPath{
		{filepath.Join(nodeRoot, "work"), "work"},
		{filepath.Join(nodeRoot, "result"), "result"},
		{filepath.Join(nodeRoot, "integrated-stage"), "integrated-stage"},
		{filepath.Join(chapters, base+".tex"), base + ".tex"},
		{filepath.Join(chapters, base), base},
		{filepath.Join(proj, "work", "reports", base+".md"), base + ".report.md"},
	}
}

// chapterAttemptArchive 是第 N 次尝试的留档目录。
func chapterAttemptArchive(proj, base string, attempt int) string {
	return filepath.Join(proj, "work", "attempts", base, "try"+strconv.Itoa(attempt))
}

// RestorePrecondition 说明"现在能不能切回第 attempt 次尝试"，返回非空 = 不能
// （派发端据此把请求留在队列里，页面的写入接口据此直接拒）。
func RestorePrecondition(proj, base string, attempt int) string {
	if attempt <= 0 {
		return "没写要切回第几次尝试（attempt 从 1 开始）"
	}
	base = strings.TrimSuffix(strings.TrimSpace(base), ".md")
	if base == "" || filepath.Clean(base) != base || strings.ContainsAny(base, `/\`) {
		return "章节名不合法: " + base
	}
	if i := strings.Index(base, "_child_"); i > 0 {
		// 子章节的尝试号是**父尝试之内的**号（父尝试作废后子树整棵进父留档、子章节
		// 又从 #1 重来），单独把一支搬回去等于把两代子树拼在一起——产物、报告、
		// 整合阶段的版本全对不上。要回就回父章节那一次。
		return "子章节的尝试归它所在的父章节（" + base[:i] + "），请对父章节切回"
	}
	src := chapterAttemptArchive(proj, base, attempt)
	if !dirExists(src) {
		return fmt.Sprintf("没有第 %d 次尝试的留档（%s）", attempt, restoreRel(proj, src))
	}
	if !restoreArchiveLooksReal(src, base) {
		return fmt.Sprintf("第 %d 次尝试的留档里既没有工作区也没有产物（%s），没什么可切回",
			attempt, restoreRel(proj, src))
	}
	live := lastAttemptNumber(filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl"))
	if live == attempt {
		return fmt.Sprintf("第 %d 次尝试就是现在这一次（没有可切回的旧版）", attempt)
	}
	if why := restoreBusyReason(proj, base); why != "" {
		return "这一章还在被写入（" + why + "），等它停下来再切回"
	}
	return ""
}

// restoreArchiveLooksReal 判断留档里到底有没有东西：只要三份里有一样在，就
// 认为这次尝试留下了可用的工作区/产物。
func restoreArchiveLooksReal(dir, base string) bool {
	for _, p := range []string{
		filepath.Join(dir, "work"),
		filepath.Join(dir, "result"),
		filepath.Join(dir, "integrated-stage"),
		filepath.Join(dir, base+".tex"),
		filepath.Join(dir, base),
	} {
		if pathExistsAny(p) {
			return true
		}
	}
	return false
}

// restoreBusyReason 找"这一章（含递归子章节）最近还在写"的证据：任何一条转录
// 或流式快照 sidecar 的 mtime 落在 restoreLiveGrace 之内就算在跑。
//
// 为什么看 mtime 而不是只看转录末尾的状态：父章节卡在 `waiting_children` 时它
// 自己那条转录几小时不动，真正在写的是子树里那几条；反过来会话刚收尾一两分钟
// 内也不该切（终态可能正在落盘）。mtime 是这两件事唯一的交集。
func restoreBusyReason(proj, base string) string {
	cutoff := time.Now().Add(-restoreLiveGrace)
	// ① 本章自己的转录（含 .partial 快照）。
	sessDir := filepath.Join(proj, "work", "sessions")
	if entries, err := os.ReadDir(sessDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !restoreRelatedTranscript(e.Name(), base) {
				continue
			}
			if st, err := e.Info(); err == nil && st.ModTime().After(cutoff) {
				return restoreRel(proj, filepath.Join(sessDir, e.Name())) + " 刚在写"
			}
		}
	}
	// ② 本章工作区里的子树转录：转换子节点、核对会话都在 `<节点>/sessions/` 下。
	// 遍历跳过媒体/视图这些只读大目录，别为了判断一次把一个几万文件的树扫一遍。
	nodeRoot := filepath.Join(proj, "work", "temp", "conv_"+base)
	hit := ""
	seen := 0
	_ = filepath.WalkDir(nodeRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || hit != "" {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "media", "views", "pdfview", "build", "out", "images", "children.tmp":
				return fs.SkipDir
			}
			return nil
		}
		seen++
		if seen > 20000 {
			return fs.SkipAll
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".partial") {
			return nil
		}
		if st, err := d.Info(); err == nil && st.ModTime().After(cutoff) {
			hit = restoreRel(proj, p) + " 刚在写"
		}
		return nil
	})
	return hit
}

// restoreRelatedTranscript 判断一条转录是不是这一章的（含子章节的）。
func restoreRelatedTranscript(name, base string) bool {
	stem := strings.TrimSuffix(name, ".partial")
	if !strings.HasSuffix(stem, ".jsonl") {
		return false
	}
	id := strings.TrimSuffix(stem, ".jsonl")
	for _, prefix := range []string{"convert_", "checker_"} {
		if s := strings.TrimPrefix(id, prefix); s != id {
			id = s
			break
		}
	}
	// transcriptAttempt 只认 convert_ 前缀与 .tryN 后缀；这里借它剥掉尝试号，
	// 剥完的基名带着 `convert_` 前缀（与转录文件名一致）。
	b, _, ok := transcriptAttempt("convert_" + id + ".jsonl")
	if !ok {
		return false
	}
	return b == "convert_"+base || strings.HasPrefix(b, "convert_"+base+"_child_")
}

// RestoreChapterAttempt 把第 attempt 次尝试搬回工作区，把手边这一次整棵留档，
// 返回一句给人看的说明。能不能切先由 RestorePrecondition 说了算（这里再查一遍，
// 免得调用方漏掉）。
func RestoreChapterAttempt(proj, base string, attempt int) (string, error) {
	base = strings.TrimSuffix(strings.TrimSpace(base), ".md")
	if why := RestorePrecondition(proj, base, attempt); why != "" {
		return "", fmt.Errorf("%s", why)
	}
	sessDir := filepath.Join(proj, "work", "sessions")
	live := lastAttemptNumber(filepath.Join(sessDir, "convert_"+base+".jsonl"))
	parkDst := chapterAttemptArchive(proj, base, live)
	srcDst := chapterAttemptArchive(proj, base, attempt)

	var parked, restored []string
	// ① 现役这一次整棵挪进它自己的留档（撞名不覆盖，见 restoreMoveUnique）。
	for _, p := range restoreLivePaths(proj, base) {
		if !pathExistsAny(p.src) {
			continue
		}
		if err := restoreMoveUnique(p.src, filepath.Join(parkDst, p.name)); err != nil {
			return "", fmt.Errorf("留档第 %d 次尝试失败（%s）: %w", live, p.name, err)
		}
		parked = append(parked, "try"+strconv.Itoa(live)+"/"+p.name)
	}
	// ② 现役转录里"不是要切回的那个号"的一并带走。不带走的话
	// lastAttemptNumber 还报那个更大的号，切回来这一次会被继续当成"上一代"。
	tp, err := parkAttemptTranscripts(proj, base, attempt)
	if err != nil {
		return "", err
	}
	parked = append(parked, tp...)
	// ③ 把第 attempt 次留档搬回工作区。
	for _, p := range restoreLivePaths(proj, base) {
		src := filepath.Join(srcDst, p.name)
		if !pathExistsAny(src) {
			continue
		}
		if pathExistsAny(p.src) {
			// 上面刚清过，还撞就说明有东西不在清单里（别覆盖用户的东西）。
			return "", fmt.Errorf("工作区里的 %s 还占着位置，先手工清掉再切回", restoreRel(proj, p.src))
		}
		if err := movePath(src, p.src); err != nil {
			return "", fmt.Errorf("搬回第 %d 次尝试失败（%s）: %w", attempt, p.name, err)
		}
		restored = append(restored, p.name)
	}
	// ④ 它的转录（如果有）也回 work/sessions；没有就沿用工作区里那条同号的。
	back, err := unparkAttemptTranscripts(proj, base, attempt)
	if err != nil {
		return "", err
	}
	restored = append(restored, back...)

	// 留档目录空了就收掉；还剩东西（checker-issues.md 这类只属于留档的）就留着。
	leftover := restoreDirNames(srcDst)
	if len(leftover) == 0 {
		_ = os.Remove(srcDst)
	}
	sort.Strings(parked)
	sort.Strings(restored)
	msg := fmt.Sprintf("已切回第 %d 次尝试：第 %d 次整棵留档到 %s，搬回 %s",
		attempt, live, restoreRel(proj, parkDst), strings.Join(restored, " / "))
	if len(parked) > 0 {
		msg += "；留档了 " + strings.Join(parked, " / ")
	}
	if len(leftover) > 0 {
		msg += "；" + restoreRel(proj, srcDst) + " 里还留着 " + strings.Join(leftover, " / ")
	}
	return msg, nil
}

// parkAttemptTranscripts 把 work/sessions 里本章 attempt != keep 的转换转录
// （含 .partial 快照）挪进各自的留档目录。
func parkAttemptTranscripts(proj, base string, keep int) ([]string, error) {
	sessDir := filepath.Join(proj, "work", "sessions")
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var moved []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		n, ok := restoreConvertAttemptOf(name, base)
		if !ok || n == keep {
			continue
		}
		dst := filepath.Join(chapterAttemptArchive(proj, base, n), name)
		if err := restoreMoveUnique(filepath.Join(sessDir, name), dst); err != nil {
			return moved, fmt.Errorf("留档第 %d 次尝试的转录失败（%s）: %w", n, name, err)
		}
		moved = append(moved, "try"+strconv.Itoa(n)+"/"+name)
	}
	return moved, nil
}

// unparkAttemptTranscripts 把留档目录里第 attempt 次的转录搬回 work/sessions。
// 工作区里已经有一条同名的不动它（那条才是被继续写过的那份）。
func unparkAttemptTranscripts(proj, base string, attempt int) ([]string, error) {
	src := chapterAttemptArchive(proj, base, attempt)
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, nil
	}
	sessDir := filepath.Join(proj, "work", "sessions")
	var back []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if _, ok := restoreConvertAttemptOf(name, base); !ok {
			continue
		}
		dst := filepath.Join(sessDir, name)
		if pathExistsAny(dst) {
			continue
		}
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			return back, err
		}
		if err := movePath(filepath.Join(src, name), dst); err != nil {
			return back, fmt.Errorf("搬回第 %d 次尝试的转录失败（%s）: %w", attempt, name, err)
		}
		back = append(back, name)
	}
	return back, nil
}

// restoreConvertAttemptOf 从转录名反推"本章第几次转换尝试"，不是本章的转换转录
// 就 false。`.partial` 快照跟着同号走。
func restoreConvertAttemptOf(name, base string) (int, bool) {
	stem := strings.TrimSuffix(name, ".partial")
	if !strings.HasSuffix(stem, ".jsonl") {
		return 0, false
	}
	b, n, ok := transcriptAttempt(stem)
	if !ok || b != "convert_"+base {
		return 0, false
	}
	return n, true
}

// restoreMoveUnique 把 src 挪到 dst；dst 已存在（同一号被反复留档过）时不覆盖，
// 换成 `parked-<名>`，宁可多一份也不丢东西。
func restoreMoveUnique(src, dst string) error {
	if pathExistsAny(dst) {
		dst = filepath.Join(filepath.Dir(dst), "parked-"+filepath.Base(dst))
	}
	return movePath(src, dst)
}

// restoreDirNames 列出目录里剩下的名字（目录不存在算空）。
func restoreDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// restoreRel 把绝对路径换成相对项目根的写法（日志与错误信息里好读）。
func restoreRel(proj, p string) string {
	if rel, err := filepath.Rel(proj, p); err == nil {
		return filepath.ToSlash(rel)
	}
	return p
}

// pathExistsAny 判断路径在不在（文件或目录都算）。
func pathExistsAny(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
