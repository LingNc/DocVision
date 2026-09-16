// End-to-end check of the three-way accounting under the T36 semantics:
//
//   - 失败 = 没有产出可用结果（含 mermaid_invalid / invalid_format /
//     empty_response 这类"跳过、下轮重试"——重试不改变命运）；
//   - 警告 = 输出有偏离但被程序纠正、结果仍可用（✓ DONE 且会话期间
//     出现 [WARNING] 行，如剥离 '[IMG_TYPE:' 前的 prose）。
//
// The synthetic log below reproduces the shape of the real 2026-09-12
// run (42 ✓ DONE, 39 ✗ FAILED [IMG_MERMAID_INVALID]) plus two corrected
// successes: the report must now say 42 success, 2 warning, 39 failed —
// agreeing with the runner's own progress line (T15 made it count
// invalid responses as errors).
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
// image, one close line, thread ids reused across the run. `warn` items
// are corrected successes: the runner logged a [WARNING] mid-session and
// still closed with ✓ DONE.
// ok = 干净成功；warn = 自纠正成功（[WARNING] 后仍 ✓ DONE）；retryFail =
// 校验失败（跳过、下轮重试）；fail = 硬错误。
func buildRunLog(t *testing.T, dir string, ok, warn, retryFail, fail int) string {
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
		fmt.Fprintf(&b, "[%s][T01] ▶ START %s\n", step(), key)
		fmt.Fprintf(&b, "[%s][T01] [WARNING] Unexpected prefix before '[IMG_TYPE:' in %s: \"The figure shows\" (前缀 18 字符，已丢弃前缀)\n", step(), key)
		fmt.Fprintf(&b, "[%s][T01] ✓ [1.00s] DONE\n", step())
	}
	for i := 0; i < retryFail; i++ {
		key := fmt.Sprintf("书.md::images/retry%03d.jpg", i)
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
	logPath := buildRunLog(t, t.TempDir(), 42, 2, 39, 0)

	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	if len(sessions) != 83 {
		t.Fatalf("sessions = %d, want 83", len(sessions))
	}

	st := ComputeStatistics(sessions, nil)
	if st.Success != 42 {
		t.Errorf("Success = %d, want 42", st.Success)
	}
	// T36：两个 [WARNING]+✓ DONE 的项是警告（程序自纠正、结果可用）。
	if st.Warning != 2 {
		t.Errorf("Warning = %d, want 2 (corrected deviations, result usable)", st.Warning)
	}
	// 39 个 mermaid_invalid 是失败（下轮重试），与 runner 进度行一致。
	if st.Failed != 39 {
		t.Errorf("Failed = %d, want 39 — validation failures are failures under T36", st.Failed)
	}
	if st.Incomplete != 0 {
		t.Errorf("Incomplete = %d, want 0", st.Incomplete)
	}
	if got, want := st.SuccessRate, 42.0/83.0*100; got < want-0.01 || got > want+0.01 {
		t.Errorf("SuccessRate = %.2f, want %.2f", got, want)
	}
	if st.ErrorDistribution["mermaid_invalid"] != 39 {
		t.Errorf("error distribution = %v", st.ErrorDistribution)
	}
	for k := range st.ErrorDistribution {
		if !retriesNextRun(k) {
			t.Errorf("unexpected hard failure in distribution: %s", k)
		}
	}
}

// A real hard error must still be a failure, and warnings must not hide
// it.
func TestAnalyze_MixedOutcome(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 5, 3, 1, 2)
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	st := ComputeStatistics(sessions, nil)
	if st.Success != 5 || st.Warning != 3 || st.Failed != 3 {
		t.Fatalf("success=%d warning=%d failed=%d, want 5/3/3", st.Success, st.Warning, st.Failed)
	}
	// 3 个失败里 1 个 mermaid_invalid（下轮重试）、2 个 api_error（硬失败）。
	if st.ErrorDistribution["mermaid_invalid"] != 1 || st.ErrorDistribution["api_error"] != 2 {
		t.Fatalf("distribution = %v", st.ErrorDistribution)
	}
}

// T36：报告必须把警告（自纠正、结果可用）与失败（无产出）分开呈现，
// 且校验/格式失败要以"失败（跳过、下轮重试）"标注。
func TestPrintReport_ShowsWarningsSeparately(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 42, 2, 39, 0)
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	st := ComputeStatistics(sessions, nil)

	out := captureStdout(t, func() { PrintReport(st, filepath.Base(logPath), true) })

	if !strings.Contains(out, "警告:") || !strings.Contains(out, " 2 ") {
		t.Errorf("report does not show the warning count:\n%s", out)
	}
	if !strings.Contains(out, "已被程序纠正") {
		t.Errorf("report does not explain warnings as auto-corrected:\n%s", out)
	}
	if !strings.Contains(out, "失败:          39") {
		t.Errorf("report does not show 39 validation failures:\n%s", out)
	}
	if !strings.Contains(out, "失败（跳过、下轮重试）") {
		t.Errorf("report does not annotate retryable failures:\n%s", out)
	}

	// The per-file summary uses the same three-way split.
	out = captureStdout(t, func() { PrintRoundFileSummary(filepath.Base(logPath), sessions) })
	if !strings.Contains(out, "警告:") || !strings.Contains(out, "已被程序纠正") {
		t.Errorf("per-file summary does not separate warnings:\n%s", out)
	}
}

// The CSV keeps the status column honest ("warning", not "failed").
func TestExportCSV_WarningStatus(t *testing.T) {
	logPath := buildRunLog(t, t.TempDir(), 1, 1, 0, 0)
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
