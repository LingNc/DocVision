package latex

import (
	"encoding/json"
	"strings"
	"testing"

)

// T307/P78：重画图的提交门槛。软墨重叠或尺寸偏差不达标就不能交——回执要把
// 两个数字与差值都写出来，而不是一句 REJECTED；"尽力还原"要带 note 才放行。
func TestSubmitFigureThresholdGate(t *testing.T) {
	state := &tikzState{compileOK: true, compareCalled: true, lastCode: "\\draw (0,0)--(1,1);",
		bestOverlap: 0.123, scaleDevAtBest: 0.31, bestScaleX: 0.92, bestScaleY: 0.69}
	tool := &SubmitFigureTool{State: state, MinOverlap: 0.17, MaxScaleDev: 0.10}
	args, _ := json.Marshal(map[string]any{"code": "\\draw (0,0)--(1,1);"})
	res, err := tool.Execute(string(args))
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if state.submitted {
		t.Fatal("成绩不达标不该放行")
	}
	for _, want := range []string{"12.30%", "17.00%", "差", "31.0%", "0.92"} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("回执要给出具体数字与差值，缺 %q：%s", want, res.Text)
		}
	}
}

// 达标就正常交；门槛 <= 0 表示这条不卡（图级别把 min_overlap 设成负数）。
func TestSubmitFigureThresholdPassesOrDisabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		min     float64
		maxDev  float64
		overlap float64
		scale   float64
	}{
		{"达标", 0.17, 0.10, 0.234, 0.05},
		{"重叠门槛关闭", 0, 0.10, 0.01, 0.05},
		{"两条都关闭", 0, 0, 0.01, 0.9},
	} {
		state := &tikzState{compileOK: true, compareCalled: true, lastCode: "x",
			bestOverlap: tc.overlap, scaleDevAtBest: tc.scale}
		tool := &SubmitFigureTool{State: state, MinOverlap: tc.min, MaxScaleDev: tc.maxDev}
		args, _ := json.Marshal(map[string]any{"code": "x"})
		res, err := tool.Execute(string(args))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !state.submitted || !strings.HasPrefix(res.Text, "SUBMITTED") {
			t.Fatalf("%s 应当放行：%s", tc.name, res.Text)
		}
	}
}

// "尽力还原"：没有 note 一律拒绝；有 note 才放行并留下粘性标记与那句原因。
func TestSubmitFigureAcceptAsIsNeedsNote(t *testing.T) {
	state := &tikzState{compileOK: true, compareCalled: true, lastCode: "x", bestOverlap: 0.02}
	tool := &SubmitFigureTool{State: state, MinOverlap: 0.17, MaxScaleDev: 0.10}

	args, _ := json.Marshal(map[string]any{"code": "x", "accept_as_is": true})
	res, err := tool.Execute(string(args))
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if state.submitted || !strings.Contains(res.Text, "note") {
		t.Fatalf("accept_as_is 必须有 note：%s", res.Text)
	}

	args, _ = json.Marshal(map[string]any{"code": "x", "accept_as_is": true, "note": "字号压不下来"})
	res, err = tool.Execute(string(args))
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if !state.submitted || !state.acceptedAsIs || state.acceptNote != "字号压不下来" {
		t.Fatalf("带 note 的尽力还原应当放行并记标记：submitted=%v note=%q text=%s",
			state.submitted, state.acceptNote, res.Text)
	}
	if !strings.Contains(res.Text, "best-effort") {
		t.Fatalf("回执要说明是尽力还原：%s", res.Text)
	}
}

// 提高门槛之后，以前"交过"的图要退回重画；已按尽力还原接受的、没有成绩记录的
// 老图不动——这是用户要的"改了判定之后继续让他跑"，且不重复烧 token。
func TestRequeueBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	prog := map[string]*imageProgress{
		"low":     {Status: "done", TikzCode: "x", FigBestOverlap: 0.12, FigPDF: "a.pdf"},
		"ok":      {Status: "done", TikzCode: "x", FigBestOverlap: 0.40},
		"accept":  {Status: "done", TikzCode: "x", FigBestOverlap: 0.05, FigAcceptAsIs: true},
		"noscore": {Status: "done", TikzCode: "x"},
		"override": {Status: "done", TikzCode: "x", FigBestOverlap: 0.12,
			FigMinOverlap: 0.10},
	}
	if n := requeueBelowThreshold(prog, dir, 0.17); n != 1 {
		t.Fatalf("只该退回 1 张（低的），实际 %d", n)
	}
	if prog["low"].Status != "classified" || prog["low"].TikzCode != "" || prog["low"].FigPDF != "" {
		t.Fatalf("退回的这张要清干净重画：%+v", prog["low"])
	}
	for _, k := range []string{"ok", "accept", "noscore", "override"} {
		if prog[k].Status != "done" {
			t.Fatalf("%s 不该被动（成绩够 / 已接受 / 没成绩 / 单独设了更低阈值）：%+v", k, prog[k])
		}
	}
}
