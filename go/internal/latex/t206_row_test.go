package latex

import (
	"testing"
	"time"
)

// T206：长任务的"已用"用 h 读，不再把 43 小时折成 2628m。
func TestFmtDurationHours(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Second:                   "5.0s",
		90 * time.Second:                  "1m30s",
		59*time.Minute + 59*time.Second:   "59m59s",
		2628*time.Minute + 56*time.Second: "43h48m",
		2 * time.Hour:                     "2h00m",
	}
	for d, want := range cases {
		if got := fmtDuration(d); got != want {
			t.Errorf("fmtDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// 实时行的状态词（T206）：只有"运行"让计时器走，其余状态把时间冻结；
// 续跑的行一开始就带真实轮次与真实工作时间。
func TestLiveRowTextStates(t *testing.T) {
	fresh := liveSeed{}
	if got := liveRowText("convert:chapter_001", rowStateQueued, fresh, 0, 0, 0); got != "[convert:chapter_001] 排队 · 轮次 0 · 工具调用 0 · 已用 0.0s" {
		t.Errorf("排队: %q", got)
	}
	resumed := liveSeed{rounds: 67, tools: 127, active: 43 * time.Minute, resumed: true}
	got := liveRowText("convert:chapter_014_child_04", rowStateRunning, resumed, 68, 130, 43*time.Minute+90*time.Second)
	want := "[convert:chapter_014_child_04] 续跑 · 运行 · 轮次 68 · 工具调用 130 · 已用 44m30s"
	if got != want {
		t.Errorf("续跑运行: %q, want %q", got, want)
	}
	if got := liveRowText("convert:chapter_014", rowStateWaiting+" 13", fresh, 0, 0, 0); got != "[convert:chapter_014] 等子会话 13 · 轮次 0 · 工具调用 0 · 已用 0.0s" {
		t.Errorf("等子会话: %q", got)
	}
}
