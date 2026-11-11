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

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
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

// T318：交付物字段被清过（早期翻"待处理"清掉的）但人单独给这张图设了阈值——
// 照样退回重画，否则它会永远停在 "done 且成绩低于自己的门槛"。
func TestRequeueEmptyDeliverableWithPerFigureThreshold(t *testing.T) {
	dir := t.TempDir()
	prog := map[string]*imageProgress{
		"cleared":    {Status: "done", FigBestOverlap: 0.179, FigMinOverlap: 0.20},
		"cleared-gl": {Status: "done", FigBestOverlap: 0.10}, // 没被单独点名：不动
	}
	if n := requeueBelowThreshold(prog, dir, 0.17, 0.10); n != 1 {
		t.Fatalf("只该退回被点名的那一张，实际 %d", n)
	}
	if prog["cleared"].Status != "classified" {
		t.Fatalf("单独设了阈值的应当退回：%+v", prog["cleared"])
	}
	if prog["cleared-gl"].Status != "done" {
		t.Fatalf("没点名的空交付物不该动：%+v", prog["cleared-gl"])
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
	// old 补重叠+偏差；hasScore 只补偏差；missing 读不到文件不写。
	if n != 2 {
		t.Fatalf("应当补算 2 张（一张补重叠、一张只补偏差），实际 %d", n)
	}
	old := prog["old"]
	if old.FigBestOverlap <= 0.9 || !old.FigScoreBackfilled || old.FigScaleDev > 0.01 {
		t.Fatalf("自成一对应接近满分且尺寸偏差≈0：overlap=%v dev=%v backfilled=%v",
			old.FigBestOverlap, old.FigScaleDev, old.FigScoreBackfilled)
	}
	if prog["missing"].FigBestOverlap != 0 || prog["missing"].FigScoreBackfilled {
		t.Fatal("读不到文件的图不该被写成绩")
	}
	// 已有重叠但缺尺寸偏差的：只补偏差、不覆盖已有成绩、不标"补算"
	if hs := prog["hasScore"]; hs.FigBestOverlap != 0.5 || hs.FigScoreBackfilled || hs.FigScaleDev > 0.01 {
		t.Fatalf("已有成绩的图应只补尺寸偏差：overlap=%v dev=%v backfilled=%v",
			hs.FigBestOverlap, hs.FigScaleDev, hs.FigScoreBackfilled)
	}
}

// 工具清单里一个字都不许提"尽力还原"：它只在交够次数之后的回执里出现。
// 这是用户的口径——schema 里写着等于一上手就告诉模型有白旗可举。
func TestSubmitSchemaDoesNotAdvertiseAcceptAsIs(t *testing.T) {
	tool := &SubmitFigureTool{State: &tikzState{}, MinOverlap: 0.17, MaxScaleDev: 0.10, AcceptMinSubmits: 8}
	raw, err := json.Marshal(tool.Definition())
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"accept_as_is", "note", "best-effort", "尽力"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("submit 的 schema 不该出现 %q：%s", banned, raw)
		}
	}
}

// 交够次数之前，打回回执同样不能提这个出口；够了才提。
func TestAcceptAsIsHintOnlyAfterEnoughSubmits(t *testing.T) {
	state := &tikzState{compileOK: true, compareCalled: true, lastCode: "x", bestOverlap: 0.05}
	tool := &SubmitFigureTool{State: state, MinOverlap: 0.17, MaxScaleDev: 0.10, AcceptMinSubmits: 3}
	args, _ := json.Marshal(map[string]any{"code": "x"})
	for i := 1; i <= 2; i++ {
		res, err := tool.Execute(string(args))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.Text, "accept_as_is") {
			t.Fatalf("第 %d 次就打回时不该提这个出口：%s", i, res.Text)
		}
		if !strings.Contains(res.Text, "12.30%") && !strings.Contains(res.Text, "5.00%") {
			t.Fatalf("回执仍要给出差在哪：%s", res.Text)
		}
	}
	res, err := tool.Execute(string(args))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "accept_as_is") {
		t.Fatalf("交够次数后应当提示这个出口：%s", res.Text)
	}
}

// "尽力还原"要真交够次数：第 8 次才放行（含被打回的那几次），不放在第一版。
func TestAcceptAsIsNeedsEnoughSubmits(t *testing.T) {
	st := &tikzState{compareCalled: true, compileOK: true, lastCode: "x", bestOverlap: 0.05, scaleDevAtBest: 0.3}
	tool := &SubmitFigureTool{State: st, MinOverlap: 0.17, MaxScaleDev: 0.10, AcceptMinSubmits: 8}
	body := `{"code":"x","accept_as_is":true,"note":"笔画太粗，扫描变形"}`
	for i := 1; i <= 7; i++ {
		res, err := tool.Execute(body)
		if err != nil {
			t.Fatal(err)
		}
		if res.Terminal {
			t.Fatalf("第 %d 次就接受了，应当先被打回", i)
		}
		if !strings.Contains(res.Text, "只交过") {
			t.Fatalf("回执要说清还差几次：%s", res.Text)
		}
	}
	res, err := tool.Execute(body)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Terminal {
		t.Fatalf("第 8 次应当放行：%s", res.Text)
	}
	if !st.acceptedAsIs {
		t.Fatal("接受标记没记上")
	}
}

// 负数是"不限次数"（老行为），0 由 config 的默认值兜成 8。
func TestAcceptAsIsSubmitLimitDisabled(t *testing.T) {
	st := &tikzState{compareCalled: true, compileOK: true, lastCode: "x", bestOverlap: 0.05}
	tool := &SubmitFigureTool{State: st, MinOverlap: 0.17, AcceptMinSubmits: -1}
	res, err := tool.Execute(`{"code":"x","accept_as_is":true,"note":"就这样"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Terminal {
		t.Fatalf("负数是关闭这条限制，第一次就该放行：%s", res.Text)
	}
}

// T308：点了"重画"却在 images 阶段整段持锁期间毫无动静——请求必须留在队列里
// （不能被悄悄吃掉），并且在页面上仍然算"待重试"。这里钉住"不消费"这一半。
func TestVectorRetryStaysQueuedWhileImagesHoldsLock(t *testing.T) {
	proj := t.TempDir()
	trName := "vector_book__abc__fig.jsonl"
	for _, d := range []string{
		filepath.Join(proj, "source", "sessions"),
		filepath.Join(proj, "source", "progress_items", "book.md"),
		filepath.Join(proj, "work"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "source", "sessions", trName), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := retryq.Queue(proj, retryq.Request{Project: "book", Role: "vector", Path: trName, Reason: "测试"}); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger(filepath.Join(proj, "run.log"), filepath.Join(proj, "err.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(&config.Config{}, log)
	if why := r.vectorRetryPrecondition(proj, trName); why != "" {
		t.Fatalf("前置条件该通过：%s", why)
	}
	r.vectorMu.Lock() // 模拟 images 阶段整段持锁
	defer r.vectorMu.Unlock()
	if n := r.dispatchRetries(proj); n != 0 {
		t.Fatalf("锁被占着时不该派发，实际 %d", n)
	}
	if got := len(retryq.Pending(proj)); got != 1 {
		t.Fatalf("请求必须留在队列里等着，实际剩 %d", got)
	}
}
