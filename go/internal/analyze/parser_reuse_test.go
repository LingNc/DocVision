package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnalyzeLogTidReuse covers old logs where a thread id was reused while
// the previous task on that id was still in flight: the close line of the
// first task lands AFTER the START of the second, which the old per-thread
// state machine misread as an incomplete session.
func TestAnalyzeLogTidReuse(t *testing.T) {
	log := "[10:00:00][T01] \u25b6 START a.md::images/1.jpg\n" +
		"[10:00:05][T01] \u25b6 START a.md::images/2.jpg\n" + // reuse while 1.jpg running
		"[10:00:10][T01] \u2713 [10.00s] DONE\n" + // belongs to 1.jpg (10:00:00+10s)
		"[10:00:12][T01] \u2713 [7.00s] DONE\n" + // belongs to 2.jpg (10:00:05+7s)
		"[10:00:13][T02] \u25b6 START b.md::images/3.jpg\n" +
		"[10:01:20][T02] \u2717 [60.00s] FAILED [IMG_API_ERROR: HTTP 400]\n"
	p := filepath.Join(t.TempDir(), "img2text_test.log")
	if err := os.WriteFile(p, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	sessions, err := AnalyzeLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("got %d sessions: %+v", len(sessions), sessions)
	}
	byKey := map[string]Session{}
	for _, s := range sessions {
		byKey[s.Key] = s
	}
	s1 := byKey["a.md::images/1.jpg"]
	if s1.Status != StatusSuccess || s1.Elapsed != 10.0 {
		t.Fatalf("1.jpg: %+v", s1)
	}
	s2 := byKey["a.md::images/2.jpg"]
	if s2.Status != StatusSuccess || s2.Elapsed != 7.0 {
		t.Fatalf("2.jpg: %+v", s2)
	}
	s3 := byKey["b.md::images/3.jpg"]
	if s3.Status != StatusFailed {
		t.Fatalf("3.jpg: %+v", s3)
	}
}

// TestAnalyzeLogUnmatchedStartIncomplete keeps the semantics that a start
// with no close line reports incomplete.
func TestAnalyzeLogUnmatchedStartIncomplete(t *testing.T) {
	log := "[10:00:00][T01] \u25b6 START a.md::images/1.jpg\n" +
		"[10:00:30][T01] [WARNING] Skipped invalid response, will retry next run.\n"
	p := filepath.Join(t.TempDir(), "img2text_test.log")
	if err := os.WriteFile(p, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	sessions, err := AnalyzeLog(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Status != StatusIncomplete {
		t.Fatalf("sessions: %+v", sessions)
	}
}
