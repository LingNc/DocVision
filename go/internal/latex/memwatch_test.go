package latex

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/logger"
)

// TestParseMemInfo pins the /proc/meminfo reader: MemAvailable is the
// judgement value (MemFree alone looks healthy right before an OOM).
func TestParseMemInfo(t *testing.T) {
	body := "MemTotal:       16316364 kB\nMemFree:          745000 kB\n" +
		"MemAvailable:     740000 kB\nBuffers:           10000 kB\n"
	total, avail, ok := parseMemInfo(strings.NewReader(body))
	if !ok || total != 16316364 || avail != 740000 {
		t.Fatalf("parseMemInfo = %d/%d ok=%v", total, avail, ok)
	}
	if _, _, ok := parseMemInfo(strings.NewReader("garbage\n")); ok {
		t.Fatal("a body without MemTotal must not report ok")
	}
}

// TestParseRSS pins the VmRSS reader.
func TestParseRSS(t *testing.T) {
	body := "Name:\tdocvision\nVmPeak:\t 2000000 kB\nVmRSS:\t 1234567 kB\nThreads:\t42\n"
	if got := parseRSS(strings.NewReader(body)); got != 1234567 {
		t.Fatalf("parseRSS = %d", got)
	}
	if got := parseRSS(strings.NewReader("none\n")); got != 0 {
		t.Fatalf("parseRSS on a body without VmRSS = %d, want 0", got)
	}
}

// TestMemLevel pins the headroom thresholds so a low-memory run warns
// BEFORE the kernel kills it: <1GiB or <10% is low, <256MiB or <3% is
// critical (a SIGKILL leaves no error line in the transcript).
func TestMemLevel(t *testing.T) {
	gib := 1024 * 1024
	cases := []struct {
		total, avail int
		want         string
	}{
		{16 * gib, 8 * gib, "ok"},
		{16 * gib, 2 * gib, "ok"},
		{16 * gib, 512 * 1024, "low"},      // <1GiB
		{16 * gib, 1536 * 1024, "low"},     // <10% of 16GiB
		{16 * gib, 200 * 1024, "critical"}, // <256MiB
		{16 * gib, 480 * 1024, "critical"}, // <3% of 16GiB
		{0, 0, "unknown"},
	}
	for _, c := range cases {
		if got := memLevel(memSnapshot{TotalKiB: c.total, AvailKiB: c.avail}); got != c.want {
			t.Errorf("memLevel(total=%d, avail=%d) = %q, want %q", c.total, c.avail, got, c.want)
		}
	}
}

// TestStartMemWatchLogsBaselineAndStop checks the watchdog writes its
// baseline + final line and stops cleanly (the OOM evidence trail).
func TestStartMemWatchLogsBaselineAndStop(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "memwatch.log")
	log, err := logger.NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	// 采样行是 Trace 级（普通模式终端与 info 日志里都不出现），所以这里显式
	// 打开 trace 来验证"全量采样确实落在 debug 日志里"。
	log.SetLevel(logger.LevelTrace)
	r := &Runner{log: log}
	memLogPath := filepath.Join(t.TempDir(), "mem.log")
	stop := r.startMemWatch(20*time.Millisecond, memLogPath)
	if stop == nil {
		t.Fatal("startMemWatch must return a stop function")
	}
	time.Sleep(60 * time.Millisecond)
	stop() // must not deadlock or panic
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "[mem]") || !strings.Contains(string(data), "基线") {
		t.Errorf("log must carry the memory baseline:\n%s", data)
	}
	if !strings.Contains(string(data), "结束") {
		t.Errorf("log must carry the final memory line:\n%s", data)
	}
	// T162：采样必须同时落到项目内的 mem.log（终端刷掉/进程被杀后唯一证据）。
	memData, err := os.ReadFile(memLogPath)
	if err != nil {
		t.Fatalf("read mem.log: %v", err)
	}
	if !strings.Contains(string(memData), "基线") || !strings.Contains(string(memData), "结束") {
		t.Errorf("mem.log must carry baseline and final lines:\n%s", memData)
	}
	if !strings.Contains(string(memData), "[mem]") {
		t.Errorf("mem.log lines must be tagged [mem]:\n%s", memData)
	}
}

// TestMemSnapshotParsesSwapAndPeak pins the extra fields the OOM forensics
// relies on: swap usage (the machine may already be paging) and VmHWM (the
// kernel's peak, which a sampling gap cannot miss).
func TestMemSnapshotParsesSwapAndPeak(t *testing.T) {
	meminfo := "MemTotal:       16316364 kB\nMemAvailable:   6291456 kB\n" +
		"SwapTotal:      28311552 kB\nSwapFree:        4194304 kB\n"
	total, avail := 0, 0
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = atoiK(t, fields[1])
		case "MemAvailable:":
			avail = atoiK(t, fields[1])
		}
	}
	if total == 0 || avail == 0 {
		t.Fatalf("fixture parse failed: %d/%d", total, avail)
	}
	swapTotal, swapFree := parseSwap(strings.NewReader(meminfo))
	if swapTotal != 28311552 || swapFree != 4194304 {
		t.Fatalf("parseSwap = %d/%d", swapTotal, swapFree)
	}
	if used, pressured := swapPressure(memSnapshot{SwapTotalKiB: swapTotal, SwapFreeKiB: swapFree}); !pressured || used != 28311552-4194304 {
		t.Fatalf("swapPressure = %d/%v, want ~24GiB/pressured", used, pressured)
	}
	if _, pressured := swapPressure(memSnapshot{SwapTotalKiB: 28311552, SwapFreeKiB: 28311552 - 512*1024}); pressured {
		t.Fatal("half a GiB of swap in use is not pressure")
	}
	status := "Name:\tdocvision\nVmPeak:\t 2000000 kB\nVmHWM:\t 1700000 kB\nVmRSS:\t 1234567 kB\n"
	if got := parsePeakRSS(strings.NewReader(status)); got != 1700000 {
		t.Fatalf("parsePeakRSS = %d", got)
	}
}

func atoiK(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestMemWatchQuietAtInfoLevel pins the user's requirement (T162 追加)：普通
// 模式（info）下 [mem] 采样行既不上控制台、也不进 info 日志，但 mem.log 里
// 必须仍然全量——被杀之后唯一的证据不能跟着日志等级一起消失。
func TestMemWatchQuietAtInfoLevel(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "memwatch-info.log")
	log, err := logger.NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	log.SetLevel(logger.LevelInfo)
	r := &Runner{log: log}
	memLogPath := filepath.Join(t.TempDir(), "mem.log")
	stop := r.startMemWatch(20*time.Millisecond, memLogPath)
	if stop == nil {
		t.Fatal("startMemWatch must return a stop function")
	}
	time.Sleep(60 * time.Millisecond)
	stop()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(data), "[mem]") {
		t.Errorf("info log must not carry [mem] sampling lines:\n%s", data)
	}
	memData, err := os.ReadFile(memLogPath)
	if err != nil {
		t.Fatalf("read mem.log: %v", err)
	}
	if !strings.Contains(string(memData), "基线") || !strings.Contains(string(memData), "结束") {
		t.Errorf("mem.log must stay complete regardless of the log level:\n%s", memData)
	}
}
