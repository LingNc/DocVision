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
