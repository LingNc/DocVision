package latex

// 父章节不再把「还在重试的子节点」判成失败（T247）。
//
// 细分父章节的收尾逻辑一直是这么写的：任一支子节点这一轮没交上来，就把
// "child_NN: <原因>" 记进失败清单，父章节立刻报
// 「子章节转换或 checker 失败；父章节禁止提交」。问题在于**那一支并没有死**：
// 复盘已经把它原地截断并排进重试队列（转录状态写成 queued），几分钟后它会
// 接着跑，跑通了还会照 T231 替父章节排一条「继续跑」。父章节却在那一刻就
// 把失败锁死了——页面上一会儿"父已通过"，一会儿又冒出几小时前那句失败，
// 阶段汇总也一直带着它。
//
// 现在的口径：已经排进重试的子节点不算失败，父章节这次**不提交也不记失败**，
// 如实说"还在重试、等它们通过后自动接手"。真正跑到死（提醒用满、复盘没救
// 回来、没排任何重试）的子节点照旧是失败。

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// retryingChildrenError 表示：这次没交上来，但那几支子节点已经在重试队列里，
// 过一会儿会自己接着跑。父章节这次既不提交也不失败。
type retryingChildrenError struct {
	Base     string
	Children []string
}

func (e *retryingChildrenError) Error() string {
	kids := append([]string(nil), e.Children...)
	sort.Strings(kids)
	return fmt.Sprintf("子章节还在重试（已排进队列，父章节这次不提交、不记失败）: %s",
		strings.Join(kids, ", "))
}

// isRetryingChildren 判断这个错误是不是"子节点还在重试"。
func isRetryingChildren(err error) ([]string, bool) {
	var e *retryingChildrenError
	if errors.As(err, &e) {
		return e.Children, true
	}
	return nil, false
}
