package session

// 续跑前的历史清洗（T159）。
//
// 转录是 append-only 的真实现场，续跑把历史原样回放——包括那些"厂商拒收"
// 的坏消息。两类真实事故：
//
//  1. 模型输出的工具调用参数不是合法 JSON（流式被截断，例如
//     `{"path": "chapter_01.md"` 少了右括号）。回放时原样塞进 assistant
//     消息，网关校验工具调用失败 → 整个请求 400 upstream_request_rejected，
//     会话从此再也起不来（现场两例：第一次请求成功、带着坏参数的第二
//     次请求立刻 400）。
//  2. finish_reason=length 的真空白回复（正文为空、无工具调用）。回放一条
//     内容为空的 assistant 消息既没用也可能被拒（现场两例 "empty final
//     answer"）。
//
// 清洗只改**回放给模型的那份内存历史**，绝不改写磁盘转录（事故现场要留
// 原样）；改动范围限"确定无法被服务端接受"的两类，其余一律原样保留。

import (
	"encoding/json"
	"strings"
)

// replayRepairNote replaces a tool result whose call carried malformed
// arguments: the call is replayed as `{}`, so the model must re-issue it.
const replayRepairNote = "TOOL ERROR: 上一步工具参数不是合法 JSON，已按空参数重放；请重新调用该工具并给出完整参数。"

// sanitizeReplayMessages returns a wire-safe copy of a replayed history:
// malformed tool-call arguments become `{}` (with their tool result
// rewritten to say so) and empty assistant messages are dropped.
func sanitizeReplayMessages(msgs []ChatMessage) (out []ChatMessage, dropped, repaired int) {
	// Pass 1: find malformed calls and their ids.
	badCall := map[string]bool{}
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			if !validToolArguments(tc.Function.Arguments) {
				badCall[tc.ID] = true
			}
		}
	}

	out = make([]ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "assistant" &&
			strings.TrimSpace(ContentString(m)) == "" &&
			len(m.ToolCalls) == 0 {
			// 空回复（多为被 max_tokens 截断的纯思考）：回放它没有任何
			// 信息量，还可能被服务端当非法消息拒收。
			dropped++
			continue
		}
		if len(m.ToolCalls) > 0 {
			fixedCalls := make([]ToolCall, len(m.ToolCalls))
			copy(fixedCalls, m.ToolCalls)
			for i := range fixedCalls {
				args := fixedCalls[i].Function.Arguments
				if validToolArguments(args) {
					continue
				}
				fixedCalls[i].Function.Arguments = "{}"
				repaired++
			}
			m.ToolCalls = fixedCalls
		}
		if m.Role == "tool" && badCall[m.ToolCallID] {
			m.Content = replayRepairNote
			m.ImageTokens = nil
		}
		out = append(out, m)
	}
	return out, dropped, repaired
}

// validToolArguments reports whether a tool call's arguments are a JSON
// object (empty counts as "no arguments", which is accepted as `{}`).
func validToolArguments(args string) bool {
	if strings.TrimSpace(args) == "" {
		return true
	}
	var probe map[string]any
	return json.Unmarshal([]byte(args), &probe) == nil
}
