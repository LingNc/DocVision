// End-to-end check of the three-way accounting on a synthetic log that
// reproduces the shape of the real 2026-09-12 run that triggered the
// question "warning 也算失败吗？":
//
//	81 images: 42 ✓ DONE, 39 ✗ FAILED [IMG_MERMAID_INVALID], 0 hard errors.
//
// The runner's own progress line said `errors: 0, warns: 39`, while
// `docvision analyze` reported "失败 39 / 成功率 51.9%". The report must
// now agree with the progress line: 42 success, 39 warning, 0 failed.
package analyze

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected and returns what it
// printed, so report wording can be asserted.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// buildRunLog writes a log shaped like the runner's output: one START per
// image, one close line, thread ids reused across the run.
func buildRunLog(t *testing.T, dir string, ok, warn, fail int) string {
	t.Helper()
	var b strings.Builder
	sec := 0
	step := func() string {
		sec++
		return fmt.Sprintf("%02d:%02d:%02d", 9+sec/3600, (sec/60)%60, sec%60)
	}
	write := func(key, closeLine string) {
		fmt.Fprintf(&b, "[%s][T01] ▶ START %s\n", step(), key)
		fmt.Fprintf(&b, "[%s][T01] %s\n", step(), closeLine)
	}
	for i := 0; i < ok; i++ {
		key := fmt.Sprintf("书.md::images/ok%03d.jpg", i)
		write(key, "✓ [1.00s] DONE")
	}
	for i := 0; i < warn; i++ {
		key := fmt.Sprintf("书.md::images/warn%03d.jpg", i)
		write(key, "✗ [1.00s] FAILED [IMG_MERMAID_INVALID]")
	}
	for i := 0; i < fail; i++ {
		key := fmt.Sprintf("书.md::images/fail%03d.jpg", i)
		write(key, "✗ [1.00s] FAILED [IMG_API_ERROR: HTTP 400]")
	}
	path := filepath.Join(dir, "img2text_20260912_012048.log")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

func TestAnalyze_RealRunShape_WarningsAreNotFailures(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 42, 39, 0)

	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	if len(sessions) != 81 {
		t.Fatalf("sessions = %d, want 81", len(sessions))
	}

	st := ComputeStatistics(sessions, nil)
	if st.Success != 42 {
		t.Errorf("Success = %d, want 42", st.Success)
	}
	if st.Warning != 39 {
		t.Errorf("Warning = %d, want 39 (validation failures are warnings, not failures)", st.Warning)
	}
	if st.Failed != 0 {
		t.Errorf("Failed = %d, want 0 — this run had no hard errors", st.Failed)
	}
	if st.Incomplete != 0 {
		t.Errorf("Incomplete = %d, want 0", st.Incomplete)
	}
	if got, want := st.SuccessRate, 42.0/81.0*100; got < want-0.01 || got > want+0.01 {
		t.Errorf("SuccessRate = %.2f, want %.2f", got, want)
	}
	// The old bug folded warnings into failures, which made the report
	// print "失败 39" and a 51.9% success rate that looked like breakage.
	if st.ErrorDistribution["mermaid_invalid"] != 39 {
		t.Errorf("warning distribution = %v", st.ErrorDistribution)
	}
	for k := range st.ErrorDistribution {
		if !isRetryableError(k) {
			t.Errorf("unexpected hard failure in distribution: %s", k)
		}
	}
}

// A real hard error must still be a failure, and warnings must not hide
// it.
func TestAnalyze_MixedOutcome(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 5, 3, 2)
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	st := ComputeStatistics(sessions, nil)
	if st.Success != 5 || st.Warning != 3 || st.Failed != 2 {
		t.Fatalf("success=%d warning=%d failed=%d, want 5/3/2", st.Success, st.Warning, st.Failed)
	}
	if st.ErrorDistribution["mermaid_invalid"] != 3 || st.ErrorDistribution["api_error"] != 2 {
		t.Fatalf("distribution = %v", st.ErrorDistribution)
	}
}

// The report text must say "警告" and never present warnings as failures;
// the retry behaviour must be spelled out.
func TestPrintReport_ShowsWarningsSeparately(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 42, 39, 0)
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	st := ComputeStatistics(sessions, nil)

	out := captureStdout(t, func() { PrintReport(st, filepath.Base(logPath), true) })

	if !strings.Contains(out, "警告:") || !strings.Contains(out, "39") {
		t.Errorf("report does not show the warning count:\n%s", out)
	}
	if !strings.Contains(out, "不是失败") {
		t.Errorf("report does not say a warning is not a failure:\n%s", out)
	}
	if !strings.Contains(out, "下轮") {
		t.Errorf("report does not mention the retry behaviour:\n%s", out)
	}
	if !strings.Contains(out, "失败:          0") {
		t.Errorf("report does not show 0 hard failures:\n%s", out)
	}

	// The per-file summary uses the same three-way split.
	out = captureStdout(t, func() { PrintRoundFileSummary(filepath.Base(logPath), sessions) })
	if !strings.Contains(out, "警告:") || !strings.Contains(out, "下轮重试") {
		t.Errorf("per-file summary does not separate warnings:\n%s", out)
	}
}

// The CSV keeps the status column honest ("warning", not "failed").
func TestExportCSV_WarningStatus(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 1, 1, 0)
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	csvPath := filepath.Join(t.TempDir(), "out.csv")
	_ = captureStdout(t, func() {
		if err := ExportCSV(sessions, csvPath); err != nil {
			t.Fatalf("ExportCSV: %v", err)
		}
	})
	f, err := os.Open(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]int{}
	for _, r := range rows[1:] {
		statuses[r[3]]++
	}
	if statuses[StatusWarning] != 1 || statuses[StatusSuccess] != 1 {
		t.Fatalf("csv statuses = %v", statuses)
	}
}
