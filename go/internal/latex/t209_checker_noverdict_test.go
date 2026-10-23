package latex

import "testing"

// T209：只有"核对会话自己没给结论"的失败才跳过反馈轮——真正的核对结论
// （哪怕是 issues 文本）照旧喂回去修。
func TestCheckerNoVerdict(t *testing.T) {
	noVerdict := []string{
		"checker 多次未能提交结论（会话未调用 submit）",
		"checker 多次未能提交结论",
		"checker API failure: 502 bad gateway",
		// T217：一次结论都没拿到时的文案也必须认成"没结论"——否则它会被当成
		// "这一章有问题"喂回转换会话，正是 T209 修掉的那个空转。
		"核对始终没有给出结论（不是内容问题，共尝试 4 次）：会话一次都没有调用 submit，可能把结论写成了正文或直接结束",
		// 用户报的现场：核对会话退化、已经排队"从干净轮次继续核对"，界面一边
		// 写着"已排队"，一边还把这句话当成"checker 发现了问题"喂回转换会话。
		"checker 会话已退化，已排队从干净轮次继续核对",
		// 复盘判定这次核对无法挽救：同样是"没看过章节"，不是章节的结论。
		"checker 复盘判定会话无法挽救：会话反复空响应",
		// 只读视图都没准备好（基础设施失败）：核对根本没开始。
		"checker 只读视图无法准备: mkdir ...",
	}
	for _, s := range noVerdict {
		if !checkerNoVerdict(s) {
			t.Errorf("checkerNoVerdict(%q) = false, want true", s)
		}
	}
	real := []string{
		"",
		"1. Missing Content: Examples 8.29, 8.30 are absent.",
		"2. The table on p58 is split across two pages without a continued caption.",
	}
	for _, s := range real {
		if checkerNoVerdict(s) {
			t.Errorf("checkerNoVerdict(%q) = true, want false（这是真正的核对结论）", s)
		}
	}
}
