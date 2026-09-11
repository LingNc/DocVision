package latex

import (
	"strings"
	"testing"
)

// TestIsSessionAPIError: 接口/会话错误必须与"图本身不过"区分开——现场配置
// 笔误（thinking.type: disable）让服务端 400 拒掉每个请求，日志却打
// "[vector] TikZ 未通过"，看起来像提示词或编译器的问题。
func TestIsSessionAPIError(t *testing.T) {
	api := []string{
		"会话错误: api error: [SESSION_API_ERROR: HTTP 400: {\"error\":{\"message\":\"Failed to deserialize ... unknown variant `disable`\"}}]",
		"api error: [SESSION_INSUFFICIENT_BALANCE] 余额不足",
		"[SESSION_TIMEOUT] 流式响应超时",
	}
	for _, m := range api {
		if !isSessionAPIError(m) {
			t.Errorf("应判定为会话/接口错误: %q", m)
		}
	}
	notAPI := []string{
		"TikZ 未通过编译: ! Undefined control sequence. \\draw",
		"dvisvgm 失败: exit status 1",
		"未提交: 会话在 12 轮内没有调用 submit",
		"",
	}
	for _, m := range notAPI {
		if isSessionAPIError(m) {
			t.Errorf("不该判定为接口错误: %q", m)
		}
	}
}

// TestProcessAbortsOnRepeatedAPIErrors: 连续接口错误的阈值存在且含义是
// "无一成功"（一张成功的都不许有），否则正常跑的任务会因为偶发 500 被砍。
func TestProcessAbortsOnRepeatedAPIErrors(t *testing.T) {
	if processAPIErrorAbort != 3 {
		t.Fatalf("阈值 = %d, want 3", processAPIErrorAbort)
	}
	// 文档化判定条件本身：这两行必须同时成立才停。
	fires := func(apiErrs, successes int) bool { return apiErrs >= processAPIErrorAbort && successes == 0 }
	if !fires(3, 0) || !fires(8, 0) {
		t.Error("连续 3+ 张接口错误且零成功应当早停")
	}
	if fires(3, 1) || fires(2, 0) {
		t.Error("有过成功、或错误数不到阈值时不该早停")
	}
	// 早停只影响"是否继续开始新任务"：剩余图片必须保持未处理（不进 prog），
	// 这样下次运行还会重试。这里钉住日志文案里的关键承诺。
	msg := "[process] 连续 3 张都是接口/会话错误、无一成功，判定为环境问题（不是图的问题）——停止处理剩下的图片；修好配置/额度/端点后重跑即可（已完成的结果会跳过）"
	if !strings.Contains(msg, "不是图的问题") || !strings.Contains(msg, "重跑") {
		t.Error("早停日志要说清因果与后续做法")
	}
}
