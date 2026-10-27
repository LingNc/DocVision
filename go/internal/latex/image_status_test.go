package latex

import "testing"

// P64：完成口径只有一处——status。raster/退回原图也算收尾，但要能和"真画出来"分开。
func TestImageStatusSettledAndQuality(t *testing.T) {
	cases := []struct {
		status  string
		settled bool
		quality string
	}{
		{ImageStatusDone, true, "ok"},
		{ImageStatusFallback, true, "degraded"},
		{ImageStatusClassified, false, "pending"},
		{ImageStatusError, false, "pending"},
		{"", false, "pending"},
		{"whatever", false, "pending"},
	}
	for _, c := range cases {
		if got := imageStatusSettled(c.status); got != c.settled {
			t.Errorf("imageStatusSettled(%q) = %v, want %v", c.status, got, c.settled)
		}
		if got := imageStatusQuality(c.status); got != c.quality {
			t.Errorf("imageStatusQuality(%q) = %q, want %q", c.status, got, c.quality)
		}
	}
}
