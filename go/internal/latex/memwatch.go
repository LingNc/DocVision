package latex

// 运行期内存自证（T158）。
//
// 现场：档位1 跑到一半，终端只留一句「已杀死」，进程消失、章节转录停在
// waiting_children，没有任何错误行。SIGKILL 无法被捕获，所以唯一能做的
// 是**在活着的时候把证据写进日志**：
//
//   - 启动时打一行基线（本进程 RSS + 系统可用内存/总量）；
//   - 运行期间每 60s 采样一次，记 RSS 峰值与系统可用内存的最低值；
//   - 可用内存过低（<1GiB 或 <10%）时立刻警告，并提示调低并发。
//
// 这样下一次被 OOM 杀掉时，日志最后几行就能看出「是内存被吃光」还是
// 「别的原因」，不必再去翻需要 root 权限的 kernel log。

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// memSnapshot is one reading of the memory picture we care about.
type memSnapshot struct {
	TotalKiB int // /proc/meminfo MemTotal
	AvailKiB int // /proc/meminfo MemAvailable（判据用它，不用 MemFree）
	RSSKiB   int // 本进程 VmRSS
}

// memLevel classifies the headroom: "ok", "low" (<1GiB or <10%) or
// "critical" (<256MiB or <3%), so the caller can warn before an OOM kill
// rather than after it.
func memLevel(s memSnapshot) string {
	if s.TotalKiB <= 0 || s.AvailKiB <= 0 {
		return "unknown"
	}
	avail := int64(s.AvailKiB)
	switch {
	case avail < 256*1024 || avail*100 < int64(s.TotalKiB)*3:
		return "critical"
	case avail < 1024*1024 || avail*10 < int64(s.TotalKiB):
		return "low"
	}
	return "ok"
}

// parseMemInfo reads MemTotal/MemAvailable out of a /proc/meminfo body.
func parseMemInfo(r io.Reader) (totalKiB, availKiB int, ok bool) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKiB = val
		case "MemAvailable:":
			availKiB = val
		}
	}
	return totalKiB, availKiB, totalKiB > 0 && availKiB > 0
}

// parseRSS reads VmRSS out of a /proc/<pid>/status body.
func parseRSS(r io.Reader) int {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "VmRSS:" {
			continue
		}
		if v, err := strconv.Atoi(fields[1]); err == nil {
			return v
		}
	}
	return 0
}

// readMemSnapshot samples the current picture (zero values where /proc is
// unavailable, e.g. a non-Linux host).
func readMemSnapshot() memSnapshot {
	var s memSnapshot
	if f, err := os.Open("/proc/meminfo"); err == nil {
		s.TotalKiB, s.AvailKiB, _ = parseMemInfo(f)
		f.Close()
	}
	if f, err := os.Open("/proc/self/status"); err == nil {
		s.RSSKiB = parseRSS(f)
		f.Close()
	}
	return s
}

func fmtMiB(kiB int) string {
	if kiB <= 0 {
		return "?"
	}
	return fmt.Sprintf("%.1fGiB", float64(kiB)/1024/1024)
}

// memLine renders one sampling line for the log.
func memLine(s memSnapshot, peakRSSKiB, minAvailKiB int, level string) string {
	return fmt.Sprintf("[mem] %s：进程峰值 RSS %s / 系统可用 %s（最低 %s，总 %s）",
		level, fmtMiB(peakRSSKiB), fmtMiB(s.AvailKiB), fmtMiB(minAvailKiB), fmtMiB(s.TotalKiB))
}

// startMemWatch logs a baseline immediately, then samples every interval
// until the returned stop function is called. It never blocks the caller
// and never fails: a missing /proc just yields "?" fields.
func (r *Runner) startMemWatch(interval time.Duration) func() {
	if r == nil || r.log == nil {
		return func() {}
	}
	base := readMemSnapshot()
	peakRSS, minAvail := base.RSSKiB, base.AvailKiB
	r.log.LogInfo(0, memLine(base, peakRSS, minAvail, "基线"))
	if level := memLevel(base); level == "low" || level == "critical" {
		r.warnMemory(level, base)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s := readMemSnapshot()
				if s.RSSKiB > peakRSS {
					peakRSS = s.RSSKiB
				}
				if s.AvailKiB > 0 && (minAvail == 0 || s.AvailKiB < minAvail) {
					minAvail = s.AvailKiB
				}
				level := memLevel(s)
				if level == "ok" || level == "unknown" {
					r.log.Trace(0, memLine(s, peakRSS, minAvail, level))
					continue
				}
				r.log.LogWarning(0, memLine(s, peakRSS, minAvail, level))
				r.warnMemory(level, s)
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		final := readMemSnapshot()
		if final.RSSKiB > peakRSS {
			peakRSS = final.RSSKiB
		}
		r.log.LogInfo(0, memLine(final, peakRSS, minAvail, "结束"))
	}
}

// warnMemory turns a low reading into an actionable line: the run is at
// risk of being killed by the kernel's OOM killer, which leaves no error
// in the transcript.
func (r *Runner) warnMemory(level string, s memSnapshot) {
	hint := "建议调低 latex.concurrency 或缩小 --project 范围"
	if level == "critical" {
		hint = "已接近内核 OOM 阈值，随时可能被 SIGKILL（日志不会留下错误行）；请立即调低 latex.concurrency"
	}
	r.log.LogWarning(0, fmt.Sprintf(
		"[mem] 系统可用内存紧张（%s / 总 %s）：%s", fmtMiB(s.AvailKiB), fmtMiB(s.TotalKiB), hint))
}
