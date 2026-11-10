package latex

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
	if n := requeueBelowThreshold(prog, dir, 0.17, 0.10); n != 1 {
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

// 补算出来的成绩只用来展示与"按图设阈值后自动重判"：全局门槛一调不能把几十张
// 老图一起退回重画（那是白烧 token）。单独设了 fig_min_overlap 的才算人点了名。
func TestRequeueIgnoresBackfilledUnlessPerFigureThreshold(t *testing.T) {
	dir := t.TempDir()
	prog := map[string]*imageProgress{
		"old-low": {Status: "done", TikzCode: "x", FigBestOverlap: 0.05, FigScaleDev: 0.40,
			FigScoreBackfilled: true},
		"old-named": {Status: "done", TikzCode: "x", FigBestOverlap: 0.05,
			FigScoreBackfilled: true, FigMinOverlap: 0.20},
	}
	if n := requeueBelowThreshold(prog, dir, 0.17, 0.10); n != 1 {
		t.Fatalf("只该退回被点名的那一张，实际 %d", n)
	}
	if prog["old-low"].Status != "done" {
		t.Fatal("补算成绩的老图不该被全局门槛退回")
	}
	if prog["old-named"].Status != "classified" {
		t.Fatal("单独设了阈值的图应当退回重画")
	}
}

// 尺寸偏差也是门槛：重叠够但画小了（宽高只有原图六成）同样退回。
func TestRequeueOnScaleDeviation(t *testing.T) {
	dir := t.TempDir()
	prog := map[string]*imageProgress{
		"too-small": {Status: "done", TikzCode: "x", FigBestOverlap: 0.42, FigScaleDev: 0.39},
		"fine":      {Status: "done", TikzCode: "x", FigBestOverlap: 0.42, FigScaleDev: 0.02},
	}
	if n := requeueBelowThreshold(prog, dir, 0.17, 0.10); n != 1 {
		t.Fatalf("只该退回画小了的那张，实际 %d", n)
	}
	if prog["too-small"].Status != "classified" || prog["fine"].Status != "done" {
		t.Fatalf("尺寸判据没生效：%+v / %+v", prog["too-small"], prog["fine"])
	}
}

// backfillFigureScores：给升级前画好的图本地补算成绩（不调模型）。同一张图
// 自成一对时重叠应该接近满分；找不到成品/原图的图保持原样（只记录，不判死）。
func TestBackfillFigureScores(t *testing.T) {
	dir := t.TempDir()
	imgDir := filepath.Join(dir, "images")
	figDir := filepath.Join(dir, "figures")
	for _, d := range []string{imgDir, figDir, filepath.Join(dir, "prog")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 白底黑框：墨迹外接框可辨，比对不会退化。
	src := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			src.Set(x, y, color.White)
		}
	}
	for x := 0; x < 120; x++ {
		src.Set(x, 0, color.Black)
		src.Set(x, 79, color.Black)
	}
	for y := 0; y < 80; y++ {
		src.Set(0, y, color.Black)
		src.Set(119, y, color.Black)
	}
	origPath := filepath.Join(imgDir, "a.png")
	candPath := filepath.Join(figDir, "a.png")
	for _, p := range []string{origPath, candPath} {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, src); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	prog := map[string]*imageProgress{
		"old":      {ImgPath: "a.png", FigPNG: "figures/a.png", Status: "done", TikzCode: "x"},
		"missing":  {ImgPath: "nope.png", FigPNG: "figures/nope.png", Status: "done", TikzCode: "x"},
		"hasScore": {ImgPath: "a.png", FigPNG: "figures/a.png", Status: "done", TikzCode: "x", FigBestOverlap: 0.5},
	}
	n, _ := backfillFigureScores(dir, imgDir, prog, filepath.Join(dir, "prog"))
	if n != 1 {
		t.Fatalf("只该补算 1 张，实际 %d", n)
	}
	old := prog["old"]
	if old.FigBestOverlap <= 0.9 || !old.FigScoreBackfilled || old.FigScaleDev > 0.01 {
		t.Fatalf("自成一对应接近满分且尺寸偏差≈0：overlap=%v dev=%v backfilled=%v",
			old.FigBestOverlap, old.FigScaleDev, old.FigScoreBackfilled)
	}
	if prog["missing"].FigBestOverlap != 0 || prog["missing"].FigScoreBackfilled {
		t.Fatal("读不到文件的图不该被写成绩")
	}
	if prog["hasScore"].FigScoreBackfilled {
		t.Fatal("已有成绩的图不该被覆盖")
	}
}
