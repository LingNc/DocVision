package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnalyzeLog_FailedBeforeNextStartSameTID guards the per-thread state
// machine against the suspected miscount bug: when a task fails and the
// same TID (worker slot) immediately picks up the next task, the
// FAILED line must still be attributed to the failing session — not to
// the new one or lost as incomplete.
//
// The log layout below mirrors what the runner emits:
//
//	[T03] ▶ START imgA
//	[T03] ✗ [5.0s] FAILED [IMG_MERMAID_INVALID]
//	[T03] ▶ START imgB
//	[T03] ✗ [3.0s] FAILED [IMG_MERMAID_INVALID]
//	[T03] ▶ START imgC
//	[T03] ✓ [2.0s] DONE
//
// Expected: imgA and imgB are reported as failed; imgC as success.
// If the parser mis-ordered them (or marked imgA incomplete because the
// new START overwrote current[T03]), the Failed count would be wrong.
func TestAnalyzeLog_FailedBeforeNextStartSameTID(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "img2text.log")
	lines := []string{
		"[14:24:20][T03] ▶ START ch1.md::images/imgA.jpg",
		"[14:24:25][T03] ✗ [5.0s] FAILED [IMG_MERMAID_INVALID]",
		"[14:24:30][T03] ▶ START ch1.md::images/imgB.jpg",
		"[14:24:33][T03] ✗ [3.0s] FAILED [IMG_MERMAID_INVALID]",
		"[14:24:40][T03] ▶ START ch1.md::images/imgC.jpg",
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

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d (%v)", len(sessions), sessions)
	}
	if s := byKey["ch1.md::images/imgA.jpg"]; s.Status != StatusFailed {
		t.Fatalf("imgA: want %s, got %s (errType=%q errMsg=%q)",
			StatusFailed, s.Status, s.ErrorType, s.ErrorMsg)
	}
	if s := byKey["ch1.md::images/imgB.jpg"]; s.Status != StatusFailed {
		t.Fatalf("imgB: want %s, got %s (errType=%q errMsg=%q)",
			StatusFailed, s.Status, s.ErrorType, s.ErrorMsg)
	}
	if s := byKey["ch1.md::images/imgC.jpg"]; s.Status != StatusSuccess {
		t.Fatalf("imgC: want %s, got %s", StatusSuccess, s.Status)
	}

	// Ensure both failures are classified as mermaid_invalid now that the
	// pattern was added (task 1 wiring).
	for _, key := range []string{
		"ch1.md::images/imgA.jpg",
		"ch1.md::images/imgB.jpg",
	} {
		if et := byKey[key].ErrorType; et != "mermaid_invalid" {
			t.Fatalf("%s: want error type mermaid_invalid, got %q", key, et)
		}
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
