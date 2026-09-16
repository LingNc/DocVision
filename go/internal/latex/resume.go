package latex

// 续跑收口（T28/T29）：
//   - T28：续上的会话要「接着上次干」——控制台进度行从历史轮次/工具数/
//     已用时间继续；转录里已经 submit 过的（进程死在提交后的收尾路上）
//     重放那次提交调用直接完成本阶段，而不是几乎重新开一场。
//   - T29：续跑不该往会话里插内容——历史以 tool 回执收尾时原样续行
//     （session.Run 空参），只有模型自己停了（最后是普通 assistant 文本）
//     才发一句最小推动；原图也不重附（历史里已经有）。

import (
	"strconv"
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
	if _, err := tool.Execute(st.SubmitArgs); err != nil {
		log.LogWarning(tid, "["+tag+"] 重放已记录的提交调用失败（按未提交继续）:", err)
		return false
	}
	log.Log(tid, "["+tag+"] 转录中已提交（", st.SubmitName, "）——重放收账，本阶段直接完成")
	return true
}

// resumeUserText picks the wire entry for a resumed session: a history that
// ends on tool receipts (or on a synthesized interrupted-receipt) continues
// by itself with NO new content ("" → session.Run sends nothing); only a
// session the model stopped mid-flight gets the nudge text.
func resumeUserText(msgs []session.ChatMessage, nudge string) string {
	if len(msgs) == 0 {
		return nudge
	}
	if msgs[len(msgs)-1].Role == "tool" {
		return ""
	}
	return nudge
}

// resumeLog summarizes the loaded history the way the console shows it, so
// an interrupted-then-resumed session reads as one continuous story.
func resumeLog(log *logger.Logger, tid int, tag, file string, msgs []session.ChatMessage, st session.ResumeStats) {
	log.Log(tid, "["+tag+"] 恢复中断的会话:", file, "(", strconv.Itoa(len(msgs)), "条历史消息 · 轮次 ",
		strconv.Itoa(st.Rounds), " · 工具调用 ", strconv.Itoa(st.Tools),
		" · 已用 ", st.Active.Round(time.Second), ")")
}
