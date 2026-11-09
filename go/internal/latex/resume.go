package latex

// 续跑收口（T28/T29）：
//   - T28：续上的会话要「接着上次干」——控制台进度行从历史轮次/工具数/
//     已用时间继续；转录里已经 submit 过的（进程死在提交后的收尾路上）
//     重放那次提交调用直接完成本阶段，而不是几乎重新开一场。
//   - T29：续跑不该往会话里插内容——历史以 tool 回执收尾时原样续行
//     （session.Run 空参），只有模型自己停了（最后是普通 assistant 文本）
//     才发一句最小推动；原图也不重附（历史里已经有）。

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"mineru-tools/internal/logger"
	"mineru-tools/internal/session"
)

// replaySubmit re-executes the recorded submit call of a resumed transcript.
// Returns true when the transcript contained a completed submit* call and the
// replay succeeded — the caller then finishes the phase without another Run.
// The submit tools are pure state writers over the persisted workspace, so
// replaying the recorded arguments against the freshly built state reproduces
// the original submission exactly.
func replaySubmit(log *logger.Logger, tid int, tag string, tool session.Tool, st session.ResumeStats) bool {
	if st.SubmitName == "" || st.SubmitArgs == "" {
		return false
	}
	res, err := tool.Execute(st.SubmitArgs)
	if err != nil {
		log.LogWarning(tid, "["+tag+"] 重放已记录的提交调用失败（按未提交继续）:", err)
		return false
	}
	// 提交被计划清单/校验打回时，工具回的是一句以 REJECTED 起头的**正常回执**
	// （不是 error）；只有真被收下的提交才带 Terminal（T92）。只看 error 会把
	// "被打回"当成"重放成功"，阶段便带着未提交的状态一路往下跑（T298）。
	if !res.Terminal {
		log.LogWarning(tid, "["+tag+"] 重放的提交未被收下（", receiptLine(res.Text), "）——按未提交继续")
		return false
	}
	log.Log(tid, "["+tag+"] 转录中已提交（", st.SubmitName, "）——重放收账，本阶段直接完成")
	return true
}

// receiptLine 取回执第一行并截短：日志里只留一眼能看懂的那半句。
func receiptLine(s string) string {
	s = strings.TrimSpace(strings.SplitN(strings.TrimSpace(s), "\n", 2)[0])
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}

// resumeUserText picks the wire entry for a resumed session: a history that
// ends on tool receipts (or on a synthesized interrupted-receipt) continues
// by itself with NO new content ("" → session.Run sends nothing); only a
// session the model stopped mid-flight gets the nudge text.
func resumeUserText(msgs []session.ChatMessage, nudge string) string {
	// 用户口径（2026-09-29）：续跑一律不插 user 内容、原样续行——
	// 回复一半中断的转录末尾本来就是 user/tool（死掉的回复不落盘），
	// 没 submit 的由 remindSubmit 提醒兜底；插一段"你被中断过"反而
	// 稀释上下文。空历史（全新会话）才用首句任务文本。
	if len(msgs) == 0 {
		return nudge
	}
	return ""
}

// resumeLog summarizes the loaded history the way the console shows it, so
// an interrupted-then-resumed session reads as one continuous story.
func resumeLog(log *logger.Logger, tid int, tag, file string, msgs []session.ChatMessage, st session.ResumeStats) {
	// 回放的内存量一并写出来（T162）：续跑会同时恢复十几条会话，每条都把
	// 转录里的图片重新读成 data URL，这是启动阶段最大的一次内存尖峰。
	images, bytes := session.ReplayWeight(msgs)
	line := "[" + tag + "] 恢复中断的会话: " + file + " (" + strconv.Itoa(len(msgs)) + " 条历史消息"
	if images > 0 {
		line += " · 图片 " + strconv.Itoa(images) + " 张 " + fmtMiB64(bytes)
	}
	line += " · 轮次 " + strconv.Itoa(st.Rounds) + " · 工具调用 " + strconv.Itoa(st.Tools) +
		" · 已用 " + st.Active.Round(time.Second).String() + ")"
	log.Log(tid, line)
	// 单条会话回放就要几十 MiB 时明确提醒：并发恢复几条就足以把可用内存吃掉。
	if bytes >= 32<<20 {
		log.LogWarning(tid, "["+tag+"] 这条会话的光图片回放就要 "+fmtMiB64(bytes)+
			"；同时恢复多条会话时内存会成倍上涨，必要时调低 latex.concurrency")
	}
}

// fmtMiB64 renders a byte count as MiB/GiB for the log.
func fmtMiB64(bytes int64) string {
	if bytes <= 0 {
		return "0"
	}
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1fGiB", float64(bytes)/(1<<30))
	}
	return fmt.Sprintf("%.1fMiB", float64(bytes)/(1<<20))
}
