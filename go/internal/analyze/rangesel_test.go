package analyze

import (
	"testing"
	"time"
)

func mkPaths(names []string) []string { return names }

func TestParseRoundSpec(t *testing.T) {
	cases := []struct {
		spec    string
		count   int
		lo, hi  int
		wantErr bool
	}{
		{"0", 5, 0, 0, false},
		{"1", 5, 1, 1, false},
		{"1-3", 5, 1, 3, false},
		{"-2", 5, 0, 2, false},
		{"2-", 5, 2, 4, false},
		{"9", 5, 9, 9, false}, // parsed; selection clamps
		{"x", 5, 0, 0, true},
		{"-1", 5, 0, 1, false}, // "-1" reads as open-ended range 0..1
		{"", 5, 0, 0, true},
	}
	for _, c := range cases {
		lo, hi, err := ParseRoundSpec(c.spec, c.count)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRoundSpec(%q) expected error", c.spec)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRoundSpec(%q) error: %v", c.spec, err)
			continue
		}
		if lo != c.lo || hi != c.hi {
			t.Errorf("ParseRoundSpec(%q) = %d-%d, want %d-%d", c.spec, lo, hi, c.lo, c.hi)
		}
	}
}

func TestSelectLogsByRounds(t *testing.T) {
	// Newest first: a(0) b(1) c(2) d(3)
	paths := []string{"d.log", "c.log", "b.log", "a.log"}
	got := selectLogsByRounds(paths, 1, 2)
	if len(got) != 2 || got[0] != "c.log" || got[1] != "b.log" {
		t.Fatalf("rounds 1-2 = %v", got)
	}
	got = selectLogsByRounds(paths, 0, 0)
	if len(got) != 1 || got[0] != "d.log" {
		t.Fatalf("round 0 = %v", got)
	}
	got = selectLogsByRounds(paths, 2, 99)
	if len(got) != 2 || got[0] != "b.log" || got[1] != "a.log" {
		t.Fatalf("rounds 2- = %v", got)
	}
}

func TestLogNameTime(t *testing.T) {
	tm, ok := logNameTime("/x/img2text_20260902_144733.log")
	if !ok {
		t.Fatal("expected ok")
	}
	want := time.Date(2026, 9, 2, 14, 47, 33, 0, time.Local)
	if !tm.Equal(want) {
		t.Fatalf("logNameTime = %v, want %v", tm, want)
	}
	if _, ok := logNameTime("/x/img2text_error_20260902_144733.log"); ok {
		t.Fatal("error log should not parse (name has error_ before ts; base still matches img2text_ prefix)")
	}
}

func TestParseTimePoint(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.Local)
	cases := []struct {
		in   string
		want time.Time
	}{
		{"+2d", now.Add(-48 * time.Hour)},
		{"+1d12h", now.Add(-36 * time.Hour)},
		{"+90m", now.Add(-90 * time.Minute)},
		{"48h", now.Add(-48 * time.Hour)},
		{"2026Y9M1D", time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)},
		{"2026Y9M1D15H30m", time.Date(2026, 9, 1, 15, 30, 0, 0, time.Local)},
		{"20260902", time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)},
		{"20260902_1530", time.Date(2026, 9, 2, 15, 30, 0, 0, time.Local)},
		{"2026-09-02", time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)},
		{"2026-09-02 15:30:12", time.Date(2026, 9, 2, 15, 30, 12, 0, time.Local)},
	}
	for _, c := range cases {
		got, err := parseTimePoint(c.in, now)
		if err != nil {
			t.Errorf("parseTimePoint(%q) error: %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("parseTimePoint(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := parseTimePoint("nonsense", now); err == nil {
		t.Error("expected error for nonsense time point")
	}
}

func TestSelectLogsTimeRange(t *testing.T) {
	paths := []string{
		"/l/img2text_20260902_144733.log",
		"/l/img2text_20260901_202353.log",
		"/l/img2text_20260830_183619.log",
		"/l/img2text_20260829_174647.log",
	}
	sel, desc, err := SelectLogs(paths, "", "2026Y8M1D-")
	if err != nil {
		t.Fatalf("SelectLogs(open range): %v", err)
	}
	_ = desc
	if len(sel) != 4 {
		t.Fatalf("open range selected %v", sel)
	}
	sel, _, err = SelectLogs(paths, "", "2026Y9M1D-2026Y9M2D")
	if err != nil {
		t.Fatalf("SelectLogs(range): %v", err)
	}
	if len(sel) != 2 || sel[0] != paths[0] || sel[1] != paths[1] {
		t.Fatalf("time range selected %v", sel)
	}
	sel, _, err = SelectLogs(paths, "1", "")
	if err != nil {
		t.Fatalf("SelectLogs(round): %v", err)
	}
	if len(sel) != 1 || sel[0] != paths[1] {
		t.Fatalf("round 1 selected %v", sel)
	}
	if _, _, err := SelectLogs(paths, "9", ""); err == nil {
		t.Fatal("expected error for out-of-range round")
	}
}
