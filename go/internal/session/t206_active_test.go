package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// T206：续跑时"已用"是累计的**真实工作时间**（每条用量行的请求耗时之和），
// 不是"首行到末行"的墙上跨度——旧口径会把中间隔了几天的空档也算进去，
// 控制台于是出现"已用 2628m56s"这种数字。
func TestResumeActiveSumsRequestDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "convert_chapter_001.jsonl")
	lines := "" +
		`{"t":"msg","role":"user","text":"go","ts":"2026-10-20T14:00:00.000+08:00"}` + "\n" +
		`{"t":"msg","role":"assistant","text":"a","ts":"2026-10-20T14:00:30.000+08:00"}` + "\n" +
		`{"t":"usage","round":1,"prompt_tokens":10,"duration_ms":60000}` + "\n" +
		`{"t":"msg","role":"assistant","text":"b","ts":"2026-10-22T11:00:00.000+08:00"}` + "\n" +
		`{"t":"usage","round":2,"prompt_tokens":20,"duration_ms":30000}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stats, err := LoadTranscriptStats(path)
	if err != nil {
		t.Fatalf("LoadTranscriptStats: %v", err)
	}
	if stats.Active != 90*time.Second {
		t.Fatalf("Active = %v, want 1m30s（请求耗时之和）", stats.Active)
	}
	if stats.Rounds != 2 {
		t.Fatalf("Rounds = %d, want 2", stats.Rounds)
	}
}

// 老转录没有用量行时 Active 保持 0：宁可显示 0，也不要退回那个会膨胀成
// 几十小时的墙上跨度。
func TestResumeActiveWithoutUsageIsZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checker_chapter_001.jsonl")
	lines := `{"t":"msg","role":"user","text":"hi","ts":"2026-10-20T14:00:00.000+08:00"}` + "\n" +
		`{"t":"msg","role":"assistant","text":"ok","ts":"2026-10-22T11:00:00.000+08:00"}` + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stats, err := LoadTranscriptStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Active != 0 {
		t.Fatalf("Active = %v, want 0", stats.Active)
	}
}
