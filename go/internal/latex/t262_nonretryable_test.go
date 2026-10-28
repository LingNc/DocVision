package latex

import (
	"path/filepath"
	"testing"
)

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

// T263：代表图只是"还没收尾"（待处理 / 刚失败）时，跟随链接必须留着——它有下
// 一轮，跟随者复用同一张成品就行。现场一次模型名写错，18 张代表图全失败，356
// 张跟随被当成"代表图没产出"清空，下一轮要画 379 张而不是 23 张。
func TestFollowerKeepsLinkWhileLeaderUnresolved(t *testing.T) {
	dir := t.TempDir()
	progDir := filepath.Join(dir, "progress_items")
	a := &task{mdName: "book.md", imgPath: "images/book/a.jpg"}
	b := &task{mdName: "book.md", imgPath: "images/book/b.jpg"}
	pa := &imageProgress{Key: a.key(), MDName: a.mdName, ImgPath: a.imgPath, Class: ClassVector,
		Status: ImageStatusFallback, FallbackReason: "会话错误: HTTP 404", Kept: true}
	pb := &imageProgress{Key: b.key(), MDName: b.mdName, ImgPath: b.imgPath, Class: ClassVector,
		Status: ImageStatusClassified}
	pb.EquivalentTo = a.key()
	pb.DedupeChecked = true
	pb.DedupeLeaderHash = "hash-a"
	prog := map[string]*imageProgress{a.key(): pa, b.key(): pb}
	r := testRunner(t, true)

	// ① 代表图失败待重试：跟随者不动。
	r.clearUnresolvedMotifLinks([]*task{a, b}, prog, progDir)
	if pb.EquivalentTo != a.key() || !pb.DedupeChecked {
		t.Fatalf("代表图还没收尾时不该拆组: %+v", pb)
	}

	// ② 代表图收尾了却没有成品：这时才解开，而且要把"核过"的记号放开，
	// 否则这张图以后再也挂不回任何代表图。
	pa.Status = ImageStatusDone
	pa.Class = ClassVector
	pa.TikzCode = ""
	pa.FallbackReason = ""
	r.clearUnresolvedMotifLinks([]*task{a, b}, prog, progDir)
	if pb.EquivalentTo != "" || pb.DedupeLeaderHash != "" || pb.DedupeChecked {
		t.Fatalf("代表图完成却没成品时应解开并放开核过记号: %+v", pb)
	}
}
