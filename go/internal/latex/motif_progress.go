package latex

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/logger"
)

// motifProgress uses the same live-row owner as classify/process, including
// quiet mode. Scanning is visible before the first candidate/model request.
type motifProgress struct {
	mu                                            sync.Mutex
	row                                           *liveProgress
	started                                       time.Time
	scanned, images, done, total, members, failed int
	phase                                         string
}

func newMotifProgress(log *logger.Logger, images int) *motifProgress {
	p := &motifProgress{started: time.Now(), images: images, phase: "scan"}
	p.row = newLiveProgressRow(log, "dedupe", p.text)
	return p
}

func (p *motifProgress) text() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := int(time.Since(p.started).Seconds())
	switch p.phase {
	case "scan":
		return fmt.Sprintf("[dedupe] 扫描候选 %d/%d · 已用 %ds", p.scanned, p.images, elapsed)
	case "done":
		return fmt.Sprintf("[dedupe %d/%d] done (failed: %d) · 已用 %ds", p.done, p.total, p.failed, elapsed)
	default:
		return fmt.Sprintf("[dedupe %d/%d] 核验整组 %d 张 (failed: %d) · 已用 %ds", p.done, p.total, p.members, p.failed, elapsed)
	}
}

func (p *motifProgress) scan(done, total int) {
	p.mu.Lock()
	p.scanned, p.images = done, total
	p.mu.Unlock()
	p.row.render()
}

func (p *motifProgress) verify(done, total, members, failed int) {
	p.mu.Lock()
	p.phase = "verify"
	p.done, p.total, p.members, p.failed = done, total, members, failed
	p.mu.Unlock()
	p.row.render()
}

func (p *motifProgress) finish() {
	p.mu.Lock()
	p.phase = "done"
	p.mu.Unlock()
	p.row.render()
	p.row.Close()
}

// imageStatusCounts 是**整本书**的图片状态盘点：阶段收尾要报"退回原图共 N 张"。
// 进度行本身只报这一轮的队列，别把全书口径挂到实时行上——它每帧都在刷，太长。
//
// 口径分两层（T262）：**作业项**是真正要跑的东西——非跟随的矢量代表图、文字图、
// 位图；**跟随项**是 dedupe 判为同一内容、复用代表图成品的那批，它们不单独跑，
// 算进"未处理"会让人以为还有几百张没动。文字图/位图完成也不是"画好"，单独一栏。
type imageStatusCounts struct {
	Total      int // 全部引用
	Work       int // 作业项：代表图 + 文字 + 位图
	VectorOK   int // 矢量真画出来的
	TextRaster int // 文字/位图收尾（取文本或保留原图）
	Fallback   int // 以原图交付（TikZ 没过 / 会话错误）
	Error      int // 真失败（下一轮会重试）
	Pending    int // 作业项里还没跑到的
	Followers  int // 跟随代表图的（不单独跑）
}

func countImageStatuses(prog map[string]*imageProgress) imageStatusCounts {
	var c imageStatusCounts
	for _, p := range prog {
		if p == nil {
			continue
		}
		c.Total++
		// 跟随项（dedupe 等价于另一张代表图）不占作业位：它们的成品随代表图
		// 重新物化，算"未处理"会让全书口径永远差着几百张。
		if p.Class == ClassVector && strings.TrimSpace(p.EquivalentTo) != "" {
			c.Followers++
			continue
		}
		c.Work++
		if imageStatusSettled(p.Status) {
			switch {
			case p.Status == ImageStatusDone && p.Class == ClassVector:
				c.VectorOK++
			case p.Status == ImageStatusDone:
				c.TextRaster++
			default:
				c.Fallback++
			}
			continue
		}
		if p.Status == ImageStatusError {
			c.Error++
			continue
		}
		c.Pending++
	}
	return c
}
