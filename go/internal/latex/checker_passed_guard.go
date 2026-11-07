package latex

import (
	"os"
	"strings"
)

// T303：产物已经核对通过、之后一个字都没改时，**自动**提出的「接着核对 / 继续跑」
// 没有任何意义。
//
// 实测事故：一章的核对在 12:43 交了 pass，紧接着队列里另一条（更早排下、一直在等
// 这一章的锁）"没结论 → 接着重核"的请求就派发出去了——刚通过的章于是又开了一条
// 全新的核对会话。它没有可核的东西，几轮工具调用之后退化；复盘要求截断又被"同一
// 位置反复切"的闸门拒掉，于是按 no_verdict 自动再换两条全新尝试（第 1/2、2/2 次）：
// 前后三条会话、四十来分钟，全部烧在一份已经通过的产物上。更早的那些自动请求会
// 在队列里等上一两个小时（等这一章的锁），所以"通过"与"派发"之间的时间差可以很
// 大，光靠"提交那一刻清队列"是不够的——派发时再问一句磁盘最稳。
//
// 页面点「重新核对」的那条不受影响（Source=web，人是想再看一遍，T260 也允许人工
// 打回同一份产物）。判据与 T260 的陈旧反馈完全一致：记号里的指纹必须等于当前
// 产物，且没有新的 `.tex.checker` 把这一版打回。
func chapterPassedAndUnchanged(proj, base string) bool {
	tex, parts := chapterProductsFor(proj, base)
	if tex == "" || !fileExists(tex) {
		return false
	}
	if fileExists(tex + ".checker") {
		return false
	}
	b, err := os.ReadFile(passMarkerPath(tex))
	if err != nil {
		return false
	}
	want := strings.TrimSpace(string(b))
	return want != "" && want == productFingerprint(tex, parts)
}

// autoRetryRequest 报告这条请求是不是"程序自己提的"（不是页面/命令行点的）。
// 自动请求在产物已经通过之后一律作废；页面提的照旧跑。
func autoRetryRequest(src string) bool { return src != "web" }
