package latex

import (
	"fmt"
	"path/filepath"
	"sort"
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
	cum := ""
	if p.cum.Total > 0 {
		cum = fmt.Sprintf(" · 全书 %d/%d（画好 %d、退回原图 %d、失败 %d、未处理 %d）",
			p.cum.Settled, p.cum.Total, p.cum.OK, p.cum.Fallback, p.cum.Error, p.cum.Pending)
	}
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
type imageStatusCounts struct {
	Total    int
	Settled  int // done + fallback（这一轮不会再动）
	OK       int // done：真画出来了
	Fallback int // 退回原图/栅格
	Error    int
	Pending  int // classified 或空：还没处理
	Raster   int // 类别是 raster 的（直接保留原图，不画）
}

func countImageStatuses(prog map[string]*imageProgress) imageStatusCounts {
	var c imageStatusCounts
	for _, p := range prog {
		if p == nil {
			continue
		}
		c.Total++
		if p.Class == ClassRaster {
			c.Raster++
		}
		switch {
		case imageStatusSettled(p.Status):
			c.Settled++
			if p.Status == ImageStatusDone {
				c.OK++
			} else {
				c.Fallback++
			}
		case p.Status == ImageStatusError:
			c.Error++
		default:
			c.Pending++
		}
	}
	return c
}

// summary 是进度行尾巴上的全书口径（P64）：续跑时这一轮可能只处理一两张，
// 只看本轮队列会显示成 1/1，看不出整本书还剩多少。Total 为 0（还没有任何
// progress_items）时返回空串，不给分类阶段平白加噪音。
func (c imageStatusCounts) summary() string {
	if c.Total == 0 {
		return ""
	}
	return fmt.Sprintf(" · 全书 %d/%d（画好 %d、退回原图 %d、失败 %d、未处理 %d）",
		c.Settled, c.Total, c.OK, c.Fallback, c.Error, c.Pending)
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
