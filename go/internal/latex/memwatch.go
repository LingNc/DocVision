package latex

// 运行期内存自证（T158，T162 扩展）。
//
// 现场：档位1 跑到一半，终端只留一句「已杀死」，进程消失、章节转录停在
// waiting_children，没有任何错误行。SIGKILL 无法被捕获，所以唯一能做的
// 是**在活着的时候把证据写进日志**：
//
//   - 启动时打一行基线（本进程 RSS/峰值 + 系统可用内存/总量 + swap 用量）；
//   - 运行期间采样：**前 3 分钟每 10 秒一次**（绝大多数「已杀死」都发生在
//     起步阶段——续跑要同时恢复十几条会话，每条都把转录里的图片重新读成
//     data URL，这是内存最大的一次尖峰），之后每 60 秒一次；
//   - 每一行都同时追加到**项目内**的 `<proj>/work/mem.log`：终端被刷掉、
//     窗口关掉、甚至进程被杀，这个文件都还在，事后直接看最后几行；
//   - 可用内存过低（<1GiB 或 <10%）立刻警告，<256MiB 或 <3% 明说随时可能
//     被 OOM 杀掉；swap 已被大量吃掉时也提醒——那时「可用内存」的余量是
//     假的，随时可能雪崩。
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
	TotalKiB     int // /proc/meminfo MemTotal
	AvailKiB     int // /proc/meminfo MemAvailable（判据用它，不用 MemFree）
	SwapTotalKiB int // /proc/meminfo SwapTotal
	SwapFreeKiB  int // /proc/meminfo SwapFree
	RSSKiB       int // 本进程 /proc/self/status VmRSS（当前）
	PeakRSSKiB   int // 本进程 VmHWM（内核记的峰值，采样间隙也不会漏）
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

// swapPressure reports whether the system is already swapping heavily: swap
// used > 60% and > 1GiB absolute. In that state MemAvailable is optimistic —
// the machine has been under sustained pressure and pages are being shuttled
// between RAM and disk, so a spike kills something.
func swapPressure(s memSnapshot) (usedKiB int, pressured bool) {
	if s.SwapTotalKiB <= 0 {
		return 0, false
	}
	used := s.SwapTotalKiB - s.SwapFreeKiB
	if used <= 0 {
		return 0, false
	}
	return used, used > 1024*1024 && used*10 > s.SwapTotalKiB*6
}

// parseMemInfo reads MemTotal/MemAvailable/SwapTotal/SwapFree out of a
// /proc/meminfo body.
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

// parseSwap reads SwapTotal/SwapFree out of a /proc/meminfo body.
func parseSwap(r io.Reader) (swapTotalKiB, swapFreeKiB int) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, 0
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
		case "SwapTotal:":
			swapTotalKiB = val
		case "SwapFree:":
			swapFreeKiB = val
		}
	}
	return swapTotalKiB, swapFreeKiB
}

// parseRSS reads VmRSS out of a /proc/<pid>/status body.
func parseRSS(r io.Reader) int {
	return parseStatusField(r, "VmRSS:")
}

// parsePeakRSS reads VmHWM (the kernel's high-water mark for this process)
// out of a /proc/<pid>/status body.
func parsePeakRSS(r io.Reader) int {
	return parseStatusField(r, "VmHWM:")
}

func parseStatusField(r io.Reader, name string) int {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != name {
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
	if f, err := os.Open("/proc/meminfo"); err == nil {
		s.SwapTotalKiB, s.SwapFreeKiB = parseSwap(f)
		f.Close()
	}
	if f, err := os.Open("/proc/self/status"); err == nil {
		s.RSSKiB = parseRSS(f)
		s.PeakRSSKiB = parsePeakRSS(f)
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
func memLine(s memSnapshot, peakRSSKiB, minAvailKiB int, label string) string {
	line := fmt.Sprintf("[mem] %s：进程峰值 RSS %s / 系统可用 %s（最低 %s，总 %s）",
		label, fmtMiB(peakRSSKiB), fmtMiB(s.AvailKiB), fmtMiB(minAvailKiB), fmtMiB(s.TotalKiB))
	if used, pressured := swapPressure(s); pressured {
		line += fmt.Sprintf(" · swap 已用 %s/%s（系统在持续换页）", fmtMiB(used), fmtMiB(s.SwapTotalKiB))
	}
	return line
}

// startMemWatch logs a baseline immediately, then samples every interval
// until the returned stop function is called. When memLogPath is non-empty
// every line is also appended there (project-local evidence that survives
// the console and the kill). It never blocks the caller and never fails: a
// missing /proc just yields "?" fields.
func (r *Runner) startMemWatch(interval time.Duration, memLogPath string) func() {
	if r == nil || r.log == nil {
		return func() {}
	}
	appendMem := func(line string) {
		if memLogPath == "" {
			return
		}
		f, err := os.OpenFile(memLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
	}
	base := readMemSnapshot()
	peakRSS, minAvail := base.RSSKiB, base.AvailKiB
	if base.PeakRSSKiB > peakRSS {
		peakRSS = base.PeakRSSKiB
	}
	line := memLine(base, peakRSS, minAvail, "基线")
	// 采样行只写日志文件与 mem.log：普通模式的终端里不再出现 [mem] 噪音
	// （用户要求）。真正需要人看见的只有下面的低内存告警。
	r.log.Trace(0, line)
	appendMem(line)
	if level := memLevel(base); level == "low" || level == "critical" {
		r.warnMemory(level, base)
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		sample := 0
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
			}
			sample++
			s := readMemSnapshot()
			if s.PeakRSSKiB > peakRSS {
				peakRSS = s.PeakRSSKiB
			} else if s.RSSKiB > peakRSS {
				peakRSS = s.RSSKiB
			}
			if s.AvailKiB > 0 && (minAvail == 0 || s.AvailKiB < minAvail) {
				minAvail = s.AvailKiB
			}
			level := memLevel(s)
			out := memLine(s, peakRSS, minAvail, fmt.Sprintf("采样#%d", sample))
			if level == "ok" || level == "unknown" {
				r.log.Trace(0, out)
			} else {
				r.log.LogWarning(0, out)
				r.warnMemory(level, s)
			}
			appendMem(out)
			// 前 3 分钟按固定 10s；之后自然退化成 60s（每 6 个 tick 记一次，
			// 其余 tick 只更新峰值，仍然能抓住尖峰）。
			next := interval
			if sample >= 18 && next < time.Minute {
				next = time.Minute
			}
			timer.Reset(next)
		}
	}()
	return func() {
		close(stop)
		<-done
		final := readMemSnapshot()
		if final.PeakRSSKiB > peakRSS {
			peakRSS = final.PeakRSSKiB
		} else if final.RSSKiB > peakRSS {
			peakRSS = final.RSSKiB
		}
		if final.AvailKiB > 0 && (minAvail == 0 || final.AvailKiB < minAvail) {
			minAvail = final.AvailKiB
		}
		out := memLine(final, peakRSS, minAvail, "结束")
		r.log.Trace(0, out)
		appendMem(out)
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
	line := fmt.Sprintf("[mem] 系统可用内存紧张（%s / 总 %s）：%s", fmtMiB(s.AvailKiB), fmtMiB(s.TotalKiB), hint)
	if used, pressured := swapPressure(s); pressured {
		line += fmt.Sprintf("（swap 已用 %s，说明这台机器已经在用磁盘换页，实际余量比这个数字更小）", fmtMiB(used))
	}
	r.log.LogWarning(0, line)
}
