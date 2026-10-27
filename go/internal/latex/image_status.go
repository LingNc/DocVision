package latex

// P64：图片阶段的"这张图算不算已经完事"只有一个口径——看 progress_items 里的
// status。以前各处各自判断（有的看产物文件在不在、有的看是否 raster），于是
// 进度行、分组视图、单图页能给出三种答案。这里收成一处，谁要判断都调它。
//
// 状态词表（见 runner.go / motif_dedupe.go 的写入点）：
//
//	classified  —— 已分类但还没处理，**不算完事**
//	done        —— 处理完成（重画成 TikZ、复用代表图成品、转文本、保留原图都落这里）
//	fallback    —— 退回原图/raster 交付：图没画出来，但这条图**这一轮就这样收尾了**，
//	               算完事（用户口径："有的是 raster 就是没有处理但是直接算完成了"）
//	其它（error/空）—— 没完事
//
// 只是"已收尾"不等于"结果好"：界面要另外把 fallback 与 done 分开显示，别让退回
// 原图的那些混进"已完成"里看不出问题。
const (
	ImageStatusClassified = "classified"
	ImageStatusDone       = "done"
	ImageStatusFallback   = "fallback"
	ImageStatusError      = "error"
)

// imageStatusSettled 表示这张图这一轮不会再有动作（成功或退回原图）。
func imageStatusSettled(status string) bool {
	switch status {
	case ImageStatusDone, ImageStatusFallback:
		return true
	}
	return false
}

// imageStatusQuality 把"收尾了"再分成三档给界面用：ok（真画出来了）、degraded
// （退回原图）、pending（还没完事，含空状态）。
func imageStatusQuality(status string) string {
	switch status {
	case ImageStatusDone:
		return "ok"
	case ImageStatusFallback:
		return "degraded"
	}
	return "pending"
}
