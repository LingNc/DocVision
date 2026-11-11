package latex

import "testing"

// T318：续跑收账要先看门槛——成绩低于这张图现在的门槛时不许收账，否则"抬门槛
// 重画"会被悄悄吞掉（现场：14 张秒 done，其中 9 张还差 0.3～5 个百分点）。
func TestFigureResumeCreditRespectsThreshold(t *testing.T) {
	for _, tc := range []struct {
		name    string
		overlap float64
		scale   float64
		min     float64
		maxDev  float64
		want    bool
	}{
		{"成绩达标（全局门槛）", 0.21, 0.02, 0.17, 0.10, true},
		{"成绩差一点（这张图门槛 20%）", 0.1794, 0.01, 0.20, 0.10, false},
		{"成绩差一点（这张图门槛 35%）", 0.3012, 0.05, 0.35, 0.10, false},
		{"门槛设成负数＝不卡", 0.05, 0.05, -1, 0.10, true},
		{"两条门槛都关", 0.05, 0.50, 0, 0, true},
		{"尺寸不达标也不收账", 0.30, 0.42, 0.17, 0.10, false},
		{"没有成绩但有门槛：继续画", 0, 0, 0.17, 0.10, false},
		{"没有成绩也没有门槛：按已交付", 0, 0, 0, 0, true},
	} {
		got := figureResumeCredit(tc.overlap, tc.scale, tc.min, tc.maxDev)
		if got != tc.want {
			t.Fatalf("%s：figureResumeCredit=%v，期望 %v", tc.name, got, tc.want)
		}
	}
}
