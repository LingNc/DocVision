package latex

import (
	"os"
	"path/filepath"
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
	r := &Runner{log: log}
	stop := r.startMemWatch(20 * time.Millisecond)
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
}
