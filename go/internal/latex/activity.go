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
