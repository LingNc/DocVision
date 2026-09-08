package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugGate(t *testing.T) {
	dir := t.TempDir()
	log, err := NewLogger(filepath.Join(dir, "t.log"), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.Debug(1, "hidden message")
	data, _ := os.ReadFile(filepath.Join(dir, "t.log"))
	if strings.Contains(string(data), "hidden message") {
		t.Fatal("debug entry must be gated before SetDebug")
	}

	log.SetDebug(true)
	log.Debug(1, "visible message")
	data, _ = os.ReadFile(filepath.Join(dir, "t.log"))
	if !strings.Contains(string(data), "[DEBUG] visible message") {
		t.Fatalf("debug entry missing after SetDebug: %s", data)
	}
}

// TestTraceLevelIsDeeperThanDebug pins the three-level contract: trace
// entries only appear at LevelTrace, debug entries at both debug and
// trace, info stays silent for both.
func TestTraceLevelIsDeeperThanDebug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	log, err := NewLogger(path, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)

	read := func() string {
		data, _ := os.ReadFile(path)
		return string(data)
	}

	log.SetDebug(true)
	if log.Level() != LevelDebug || !log.DebugEnabled() || log.TraceEnabled() {
		t.Fatalf("level=%d debug=%v trace=%v", log.Level(), log.DebugEnabled(), log.TraceEnabled())
	}
	log.Debug(1, "d")
	log.Trace(1, "t-hidden")
	if !strings.Contains(read(), "[DEBUG] d") {
		t.Fatal("debug entry missing at debug level")
	}
	if strings.Contains(read(), "t-hidden") {
		t.Fatal("trace entry must stay hidden at debug level")
	}

	log.SetLevel(LevelTrace)
	log.Trace(1, "t-visible")
	if !strings.Contains(read(), "[TRACE] t-visible") {
		t.Fatal("trace entry missing at trace level")
	}

	log.SetDebug(false)
	if log.Level() != LevelInfo {
		t.Fatalf("SetDebug(false) level=%d, want info", log.Level())
	}
	log.Debug(1, "d-hidden")
	if strings.Contains(read(), "d-hidden") {
		t.Fatal("debug entry must be hidden at info level")
	}
}
