package latex

// 「进行中」状态收口（T246）。
//
// 转录里的 chapter_activity 是页面那一行状态的唯一来源。只要上一轮进程在
// "写进进行中、还没写终态"之间消失（被杀、阶段收尾、请求刚被队列消费就退出），
// 这一章就会永远停在 queued/running——页面上是一句「等待并发槽」，而实际上
// 既没有进程在等，也没有槽位会被发下来。实测：一章在继续跑时写下 queued 之后
// 再无下文，两天后重跑走「已有产物，跳过」，那句 queued 一直留到现在。
//
// 收口只做一件保守的事：**只**在确认这一章不会被本轮再驱动时（"已有产物，
// 跳过"这一条路，且持有章节锁）才动手，并且新状态直接照磁盘事实写：
// 有 .checker 就是 issues（有产物但核对没过），只剩产物就是 checking_result。
// 有产物又没结论的第三种情况不存在——那正是上面分支先重核的场合。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mineru-tools/internal/session"
)

// staleActivity reports whether state is an in-flight marker that only a live
// session could justify.
func staleActivity(state string) bool {
	switch state {
	case "queued", "running", "rescuing":
		return true
	}
	return false
}

// settleStaleActivity rewrites a leftover in-flight state to what the disk
// actually says. It returns the state it wrote ("" = nothing to do): the
// transcript is missing, the last state is not in-flight, or there is no
// product to describe.
func settleStaleActivity(trPath, texPath string) string {
	if trPath == "" || !staleActivity(lastActivityState(trPath)) {
		return ""
	}
	state := ""
	switch {
	case fileExists(texPath + ".checker"):
		state = "issues"
	case fileExists(texPath):
		state = "checking_result"
	default:
		// 没有产物：这一章的真相是"还没转出来"，但这里没有会话可写，
		// 留空让调用方按自己的语义处理。
		return ""
	}
	w, err := session.NewTranscript(trPath)
	if err != nil {
		return ""
	}
	defer w.Close()
	if err := w.AppendNote("chapter_activity", map[string]any{
		"state": state,
		"note":  "上一轮停在排队/运行态就没了下文，本次按磁盘上的产物收口（T246）",
	}); err != nil {
		return ""
	}
	return state
}

// latestConvertTranscript returns the newest convert transcript of a top-level
// chapter ("" when there is none). Attempts live side by side as
// convert_<章>.jsonl / convert_<章>.tryN.jsonl, so "newest" is by mtime.
func latestConvertTranscript(proj, base string) string {
	dir := filepath.Join(proj, "work", "sessions")
	matches, err := filepath.Glob(filepath.Join(dir, "convert_"+base+".jsonl"))
	if err != nil {
		return ""
	}
	tries, _ := filepath.Glob(filepath.Join(dir, "convert_"+base+".try*.jsonl"))
	matches = append(matches, tries...)
	newest, newestMod := "", time.Time{}
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		if st.ModTime().After(newestMod) {
			newest, newestMod = m, st.ModTime()
		}
	}
	return newest
}

// chapterPassedOnDisk reports whether the chapter's product is on disk and was
// last seen passing (T247)：这一章在本次运行里先失败、后来又被重试队列推通了。
//
// 判据刻意取"产物在 + 没有 .checker（核对通过时会被删掉）+ 最新一条转换转录
// 的最后状态是 pass"三条同时成立。只有"确实通过了"才成立，产物在但核对没过
// （有 .checker）或状态不是 pass 的一概不算。
func chapterPassedOnDisk(proj, base string) bool {
	texPath := filepath.Join(proj, "work", "chapters", base+".tex")
	if !fileExists(texPath) || fileExists(texPath+".checker") {
		return false
	}
	tr := latestConvertTranscript(proj, base)
	if tr == "" {
		return false
	}
	return lastActivityState(tr) == "pass"
}

// dropHealedFailures 把"本次运行里先失败、后来通过"的章节从失败清单里摘出来。
//
// 失败清单是按"哪一章的 goroutine 返回了什么"追加的，没有生命周期——重试队列
// 在外面把同一章推通了，那条旧失败仍然留着，阶段汇总就会一边打印"通过"一边
// 报它失败（实测一次运行里三章报失败，其中一章的核对早已 pass）。这里只做
// 一件事：返回时按磁盘事实把它们摘掉，并把摘掉的章名交回去记一行日志。
func dropHealedFailures(proj string, fails []string) (kept, healed []string) {
	for _, f := range fails {
		base := f
		if i := strings.Index(base, ":"); i > 0 {
			base = base[:i]
		}
		base = strings.TrimSpace(strings.TrimSuffix(base, ".md"))
		if chapterPassedOnDisk(proj, base) {
			healed = append(healed, base)
			continue
		}
		kept = append(kept, f)
	}
	return kept, healed
}

// chapterFailure 把一批失败变成阶段错误：先摘掉"后来已经通过"的章，再把剩下的
// 拼成原来那句话（T247）。全都被摘掉时返回 nil——这一轮其实没有失败。
func (r *Runner) chapterFailure(proj string, fails []string) error {
	kept, healed := dropHealedFailures(proj, fails)
	if len(healed) > 0 {
		r.log.LogWarning(0, "[convert]", "这些章后来已经通过（核对 pass、产物在），不再计入失败:",
			strings.Join(healed, ", "))
	}
	if len(kept) == 0 {
		return nil
	}
	return fmt.Errorf("%d 个章节转换失败: %s", len(kept), strings.Join(kept, "; "))
}
