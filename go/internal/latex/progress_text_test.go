package latex

import "testing"

// TestPhaseProgressCountsThisRunOnly pins the compact phase lines of a
// level-1 run to THIS RUN's numbers: the resumed baseline is printed once
// on the "Already done: N | to process: M" line, and folding it into the
// progress numbers is what made a resumed run open at 97.93% with only a
// few hundred images left (user report, img2text; same convention here).
func TestPhaseProgressCountsThisRunOnly(t *testing.T) {
	cases := []struct{ got, want string }{
		{classifyProgressText(0, 396, 0, 10), "[classify 0/396] 0.00% (failed: 0, running: 10)"},
		{classifyProgressText(208, 396, 2, 10), "[classify 208/396] 52.53% (failed: 2, running: 10)"},
		{classifyProgressText(0, 0, 0, 0), "[classify 0/0] 0.00% (failed: 0, running: 0)"},
		{processProgressText(0, 396, 0, 0, 0, 0, 10), "[process 0/396] 0.00% (done: 0, errors: 0, fallback: 0, raster: 0, running: 10)"},
		{processProgressText(396, 396, 390, 3, 3, 12, 0), "[process 396/396] 100.00% (done: 390, errors: 3, fallback: 3, raster: 12, running: 0)"},
		// ok 为负（failed+warned 超过 done 的瞬时状态）时钳到 0，且不崩。
		{processProgressText(1, 10, -1, 1, 1, 0, 2), "[process 1/10] 10.00% (done: 0, errors: 1, fallback: 1, raster: 0, running: 2)"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("progress line = %q, want %q", c.got, c.want)
		}
	}
}
