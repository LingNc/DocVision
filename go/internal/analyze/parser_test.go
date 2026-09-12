package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnalyzeLog_RetryableBeforeNextStartSameTID guards the per-thread
// state machine against the suspected miscount bug: when a task ends
// without a result and the same TID (worker slot) immediately picks up
// the next task, the ✗ line must still be attributed to the closing
// session — not to the new one or lost as incomplete.
//
// It also pins the warning/failure split. The layout below mirrors what
// the runner emits:
//
//	[T03] ▶ START imgA
//	[T03] ✗ [5.0s] FAILED [IMG_MERMAID_INVALID]   ← skipped, retried next run
//	[T03] ▶ START imgB
//	[T03] ✗ [3.0s] FAILED [IMG_INVALID_FORMAT]    ← skipped, retried next run
//	[T03] ▶ START imgC
//	[T03] ✗ [2.0s] FAILED [IMG_API_ERROR: HTTP 400]  ← a hard failure
//	[T03] ▶ START imgD
//	[T03] ✓ [2.0s] DONE
//
// Expected: imgA/imgB are WARNINGS (validation/format problems the
// runner retries), imgC is a FAILURE, imgD is a success. Reporting the
// first two as failures is exactly the bug that made the report claim
// "失败 39" for a run whose progress line said "errors: 0, warns: 39".
func TestAnalyzeLog_RetryableBeforeNextStartSameTID(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "img2text.log")
	lines := []string{
		"[14:24:20][T03] ▶ START ch1.md::images/imgA.jpg",
		"[14:24:25][T03] ✗ [5.0s] FAILED [IMG_MERMAID_INVALID]",
		"[14:24:30][T03] ▶ START ch1.md::images/imgB.jpg",
		"[14:24:33][T03] ✗ [3.0s] FAILED [IMG_INVALID_FORMAT]",
		"[14:24:36][T03] ▶ START ch1.md::images/imgC.jpg",
		"[14:24:38][T03] ✗ [2.0s] FAILED [IMG_API_ERROR: HTTP 400]",
		"[14:24:40][T03] ▶ START ch1.md::images/imgD.jpg",
		"[14:24:42][T03] ✓ [2.0s] DONE",
	}
	if err := os.WriteFile(logPath, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}

	byKey := map[string]Session{}
	for _, s := range sessions {
		byKey[s.Key] = s
	}

	if len(sessions) != 4 {
		t.Fatalf("expected 4 sessions, got %d (%v)", len(sessions), sessions)
	}
	want := map[string]string{
		"ch1.md::images/imgA.jpg": StatusWarning,
		"ch1.md::images/imgB.jpg": StatusWarning,
		"ch1.md::images/imgC.jpg": StatusFailed,
		"ch1.md::images/imgD.jpg": StatusSuccess,
	}
	for key, wantStatus := range want {
		if got := byKey[key].Status; got != wantStatus {
			t.Fatalf("%s: want %s, got %s (errType=%q errMsg=%q)",
				key, wantStatus, got, byKey[key].ErrorType, byKey[key].ErrorMsg)
		}
	}

	// The error type is still classified, so the report can explain WHY
	// an item was skipped and retried.
	if et := byKey["ch1.md::images/imgA.jpg"].ErrorType; et != "mermaid_invalid" {
		t.Fatalf("imgA: want error type mermaid_invalid, got %q", et)
	}
	if et := byKey["ch1.md::images/imgB.jpg"].ErrorType; et != "invalid_format" {
		t.Fatalf("imgB: want error type invalid_format, got %q", et)
	}
	if et := byKey["ch1.md::images/imgC.jpg"].ErrorType; et != "api_error" {
		t.Fatalf("imgC: want error type api_error, got %q", et)
	}
}

// TestComputeStatistics_SeparatesWarningsFromFailures pins the three-way
// accounting (success / warning / failure) end to end, including the
// retry note the report relies on.
func TestComputeStatistics_SeparatesWarningsFromFailures(t *testing.T) {
	sessions := []Session{
		{Key: "a.md::images/1.jpg", TID: "1", Status: StatusSuccess},
		{Key: "a.md::images/2.jpg", TID: "1", Status: StatusSuccess},
		{Key: "a.md::images/3.jpg", TID: "2", Status: StatusWarning, ErrorType: "mermaid_invalid"},
		{Key: "a.md::images/4.jpg", TID: "2", Status: StatusWarning, ErrorType: "mermaid_invalid"},
		{Key: "a.md::images/5.jpg", TID: "2", Status: StatusWarning, ErrorType: "mermaid_invalid"},
		{Key: "a.md::images/6.jpg", TID: "2", Status: StatusFailed, ErrorType: "api_error"},
		{Key: "a.md::images/7.jpg", TID: "3", Status: StatusIncomplete},
	}

	st := ComputeStatistics(sessions, nil)
	if st.Total != 7 || st.Success != 2 || st.Warning != 3 || st.Failed != 1 || st.Incomplete != 1 {
		t.Fatalf("counts: total=%d success=%d warning=%d failed=%d incomplete=%d",
			st.Total, st.Success, st.Warning, st.Failed, st.Incomplete)
	}
	if got, want := st.SuccessRate, 2.0/7.0*100; got < want-0.01 || got > want+0.01 {
		t.Fatalf("SuccessRate = %.2f, want %.2f", got, want)
	}
	if got, want := st.WarningRate, 3.0/7.0*100; got < want-0.01 || got > want+0.01 {
		t.Fatalf("WarningRate = %.2f, want %.2f", got, want)
	}
	// Warnings appear in the distribution (so the report explains them)
	// but never in the failure count.
	if st.ErrorDistribution["mermaid_invalid"] != 3 {
		t.Fatalf("warning distribution = %v", st.ErrorDistribution)
	}
	if st.ErrorDistribution["api_error"] != 1 {
		t.Fatalf("failure distribution = %v", st.ErrorDistribution)
	}
}

// TestRoundFileStat_SeparatesWarnings pins the per-file summary.
func TestRoundFileStat_SeparatesWarnings(t *testing.T) {
	sessions := []Session{
		{Key: "a.md::images/1.jpg", Status: StatusSuccess},
		{Key: "a.md::images/2.jpg", Status: StatusWarning},
		{Key: "a.md::images/3.jpg", Status: StatusFailed},
		{Key: "b.md::images/4.jpg", Status: StatusIncomplete},
	}
	byFile := GroupSessionsByFile(sessions)
	a := byFile["a.md"]
	if a == nil || a.Total != 3 || a.Success != 1 || a.Warning != 1 || a.Failed != 1 {
		t.Fatalf("a.md: %+v", a)
	}
	b := byFile["b.md"]
	if b == nil || b.Total != 1 || b.Incomplete != 1 || b.Warning != 0 {
		t.Fatalf("b.md: %+v", b)
	}
}

// TestAnalyzeLog_StartOverwritesOrphanedSession verifies the existing
// "new START closes the previous session as incomplete" behaviour. The
// state machine never deletes a session for which no DONE/FAILED has
// been seen — for example if a worker crashed between START and DONE.
func TestAnalyzeLog_StartOverwritesOrphanedSession(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "img2text.log")
	lines := []string{
		"[14:24:20][T03] ▶ START ch1.md::images/imgA.jpg",
		// No DONE/FAILED for imgA — simulates a crashed worker.
		"[14:24:30][T03] ▶ START ch1.md::images/imgB.jpg",
		"[14:24:32][T03] ✓ [2.0s] DONE",
	}
	if err := os.WriteFile(logPath, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	byKey := map[string]Session{}
	for _, s := range sessions {
		byKey[s.Key] = s
	}
	if s := byKey["ch1.md::images/imgA.jpg"]; s.Status != StatusIncomplete {
		t.Fatalf("imgA: want %s, got %s", StatusIncomplete, s.Status)
	}
	if s := byKey["ch1.md::images/imgB.jpg"]; s.Status != StatusSuccess {
		t.Fatalf("imgB: want %s, got %s", StatusSuccess, s.Status)
	}
}

// joinLines glues lines with \n so the test log has the same shape as
// the real one produced by the logger.
func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

// TestAnalyzeLog_CountsToolCalls pins the tool-call accounting: both the
// img2text "[ToolCall] ..." lines and the session engine's
// "[tool:<name>] ok|error ..." lines count towards the session that is
// open on that thread.
func TestAnalyzeLog_CountsToolCalls(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "latex.log")
	lines := []string{
		"[19:29:46][T01] ▶ START 书.md::images/书/a.jpg",
		"[19:29:51][T01] [tikz] [tool:view_image] ok (145 chars result)",
		"[19:29:51][T01] [tikz] [tool:image_context] ok (4387 chars result)",
		"[19:29:51][T01] [tikz] [tool:view_image] error (已返回模型，会话继续): 文件不存在",
		"[19:30:28][T01] ✓ [22.75s] DONE [IMG_TYPE: vector]",
		"[19:30:30][T01] ▶ START 书.md::images/书/b.jpg",
		"[19:30:31][T01]   [ToolCall] AI wants +5up/-0down -> window 10/5>15/5",
		"[19:30:33][T01] ✓ [2.86s] DONE [IMG_TYPE: text]",
		"[19:30:40][T01] ▶ START 书.md::images/书/c.jpg",
		"[19:30:42][T01] ✓ [2.00s] DONE [IMG_TYPE: text]",
	}
	if err := os.WriteFile(logPath, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	sessions, err := AnalyzeLog(logPath)
	if err != nil {
		t.Fatalf("AnalyzeLog: %v", err)
	}
	got := map[string]int{}
	for _, s := range sessions {
		got[s.Key] = s.ToolCalls
	}
	want := map[string]int{
		"书.md::images/书/a.jpg": 3,
		"书.md::images/书/b.jpg": 1,
		"书.md::images/书/c.jpg": 0,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ToolCalls[%s] = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
}
