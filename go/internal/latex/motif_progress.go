package latex

import (
	"fmt"
	"path/filepath"
	"sort"
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
	// cumulative 是**整本书**的口径，done/total 是这一轮的队列（P64）：
	// 续跑时队列常常只剩一两张，只看它就会显示成 1/1，让人以为整本书才刚开头。
	cum   imageStatusCounts
	phase string
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
	cum := p.cum.summary()
	switch p.phase {
	case "scan":
		return fmt.Sprintf("[dedupe] 扫描候选 %d/%d%s · 已用 %ds", p.scanned, p.images, cum, elapsed)
	case "done":
		return fmt.Sprintf("[dedupe %d/%d] done (failed: %d)%s · 已用 %ds", p.done, p.total, p.failed, cum, elapsed)
	default:
		return fmt.Sprintf("[dedupe %d/%d] 核验整组 %d 张 (failed: %d)%s · 已用 %ds", p.done, p.total, p.members, p.failed, cum, elapsed)
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

// cumulative 刷新全书盘点（P64）：从 progress_items 汇总，而不是这一轮的队列。
func (p *motifProgress) cumulative(c imageStatusCounts) {
	p.mu.Lock()
	p.cum = c
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

// imageStatusCounts 是**整本书**的图片状态盘点（P64）。进度行只显示本轮队列，
// 于是续跑时永远看到 1/1；这里从 progress_items 汇总出全书口径，让终端和页面
// 都能回答"到底还剩多少张没处理"。
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

// summary 是进度行尾巴上的全书口径（P64/T262）：续跑时这一轮可能只处理一两张，
// 只看本轮队列会显示成 1/1。零的分栏不打印，省得"失败 0、未处理 0"占地方。
func (c imageStatusCounts) summary() string {
	if c.Total == 0 || c.Work == 0 {
		return ""
	}
	done := c.VectorOK + c.TextRaster + c.Fallback
	var parts []string
	add := func(label string, n int) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", label, n))
		}
	}
	add("矢量画好", c.VectorOK)
	add("文字/位图", c.TextRaster)
	add("退回原图", c.Fallback)
	add("失败", c.Error)
	add("未处理", c.Pending)
	line := fmt.Sprintf(" · 全书 %d/%d 项", done, c.Work)
	if len(parts) > 0 {
		line += "（" + strings.Join(parts, "、") + "）"
	}
	if c.Followers > 0 {
		line += fmt.Sprintf(" · 跟随代表图 %d 张", c.Followers)
	}
	return line
}

// fallbackImageNames 列出"退回原图"的图名（按名字排序，去掉 md 前缀里的重复）。
// 日志里给的是文件名，够对上 md 里的引用；同名的不同目录加一层父目录区分。
func fallbackImageNames(prog map[string]*imageProgress) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range prog {
		if p == nil || p.Status != ImageStatusFallback {
			continue
		}
		name := filepath.Base(p.ImgPath)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
