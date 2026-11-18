// Package checkspec 是核对（T4）收尾判据的纯函数部分（旧线 checker.go 移植）。
// "没给结论"的收尾判定 + 重核挂起标记。会话编排（核对会话本身）不进本包。
package checkspec

import (
	"os"
	"strings"
)

// MaxNoVerdictRetries 是"核对没给结论"的最大重核次数（T233，旧线默认 6）。
// 每次换一条全新的核对尝试、转换会话不重跑——产物没被判定过，重转既不会更有
// 结论，也没理由假设新的转换会话能救回一个坏掉的核对。
const MaxNoVerdictRetries = 6

// noVerdictPrefixes 是 checkerNoVerdict 认得的文案前缀（旧线逐字）。
var noVerdictPrefixes = []string{
	"核对始终没有给出结论",
	"checker 多次未能提交结论",
	"checker API failure",
	"checker 会话已退化",
	"checker 复盘判定会话无法挽救",
	"checker 只读视图无法准备",
}

// IsNoVerdict 报告 issues 文案是不是"核对没给结论"那一类（T222）。
func IsNoVerdict(issues string) bool {
	for _, p := range noVerdictPrefixes {
		if strings.HasPrefix(issues, p) {
			return true
		}
	}
	return false
}

// VerdictPath 是核对结论的落盘路径约定：`<产物>.checker`。
func VerdictPath(texPath string) string { return texPath + ".checker" }

// WriteVerdict 落盘核对结论（issues 为空且 pass 时可写空串表示 pass）。
func WriteVerdict(texPath, issues string) error {
	return os.WriteFile(VerdictPath(texPath), []byte(issues), 0o644)
}

// RecheckPending 报告这一章的产物在、但上一次核对**没有下过判定**
// （`.checker` 里是 IsNoVerdict 认得的文案）。
//
// T222：这种产物不能被"已有产物，跳过"静默采纳——跳过意味着整章连核对都不
// 跑，一份从没被判定过的东西就这样进了书。带这个标记的章节重跑时先重核这一份。
func RecheckPending(texPath string) bool {
	data, err := os.ReadFile(VerdictPath(texPath))
	if err != nil {
		return false
	}
	return IsNoVerdict(string(data))
}
