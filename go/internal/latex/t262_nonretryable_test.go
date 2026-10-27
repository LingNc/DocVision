package latex

import "testing"

// T262：模型名/参数写错的 4xx 判"不可重试"——重试与重开只会撞同一堵墙。它同时
// 也是接口错误（子集），所以两把尺子都必须认它，只是处置不同：接口错误保留原图
// 下次重试，不可重试的则整批停下并如实报。
func TestNonRetryableSessionError(t *testing.T) {
	modelErr := `会话错误: api error: [SESSION_API_ERROR: HTTP 404 (不可重试): HTTP 404: {"error":{"message":"Model \"x\" is not supported by any configured account in this group","type":"model_not_found"}}]`
	if !isSessionAPIError(modelErr) {
		t.Fatal("不可重试的 4xx 仍是接口错误，应被 isSessionAPIError 认出来")
	}
	if !isSessionNonRetryableError(modelErr) {
		t.Fatal("model_not_found 应判不可重试")
	}
	// 会自动退避重试的那类（限流/网关）不能误判成不可重试，否则整批被无谓停下。
	for _, msg := range []string{
		"[SESSION_RATE_LIMIT_EXCEEDED]",
		"[SESSION_API_ERROR: HTTP 502: bad gateway]",
		"[SESSION_CONNECTION_TIMEOUT]",
		"[SESSION_TIMEOUT]",
		"TikZ 编译失败：Undefined control sequence",
		"",
	} {
		if isSessionNonRetryableError(msg) {
			t.Fatalf("%q 不该判不可重试", msg)
		}
	}
}
