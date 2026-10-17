package latex

import (
	"fmt"
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
