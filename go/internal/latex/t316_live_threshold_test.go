package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/session"
)

// T316：门槛在会话跑着的时候被改（网页写进度记录），下一次 submit 就按新值判。
// 一直在重复交不过的图，人把阈值降下来之后当轮就能过，不必等重跑。
func TestSubmitFigureLiveThresholdOverridesSnapshot(t *testing.T) {
	state := &tikzState{compileOK: true, compareCalled: true, lastCode: "x",
		bestOverlap: 0.12, scaleDevAtBest: 0.01}
	live := 0.10 // 网页上把它从 0.30 降到 0.10
	tool := &SubmitFigureTool{State: state, MinOverlap: 0.30, MaxScaleDev: 0.10,
		LiveThreshold: func() (float64, bool) { return live, true }}
	args, _ := json.Marshal(map[string]any{"code": "x"})
	res, err := tool.Execute(string(args))
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if !state.submitted || !strings.HasPrefix(res.Text, "SUBMITTED") {
		t.Fatalf("放宽后的门槛应当放行：%s", res.Text)
	}

	// 反过来：跑着的时候把门槛抬高，下一次提交要立刻拦住。
	state2 := &tikzState{compileOK: true, compareCalled: true, lastCode: "x",
		bestOverlap: 0.12, scaleDevAtBest: 0.01}
	tool2 := &SubmitFigureTool{State: state2, MinOverlap: 0.10, MaxScaleDev: 0.10,
		LiveThreshold: func() (float64, bool) { return 0.30, true }}
	if res, err := tool2.Execute(string(args)); err != nil {
		t.Fatalf("不该报错：%v", err)
	} else if state2.submitted {
		t.Fatalf("抬高的门槛应当拦住：%s", res.Text)
	}

	// 读不到（或 0 = 用全局）时退回派发时的快照。
	state3 := &tikzState{compileOK: true, compareCalled: true, lastCode: "x",
		bestOverlap: 0.12, scaleDevAtBest: 0.01}
	tool3 := &SubmitFigureTool{State: state3, MinOverlap: 0.10, MaxScaleDev: 0.10,
		LiveThreshold: func() (float64, bool) { return 0, true }}
	if res, err := tool3.Execute(string(args)); err != nil {
		t.Fatalf("不该报错：%v", err)
	} else if !state3.submitted {
		t.Fatalf("0 表示用全局，应按快照 0.10 放行：%s", res.Text)
	}
}

// T316：转录里最后一次提交的回执是 SUBMITTED 时，续跑统计要能看出来——据此判
// 断"这张图其实早画完了"，不再重放工具（新进程必然被 compile/compare 门槛打回）。
func TestLoadTranscriptStatsSubmitAccepted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vector_x.jsonl")
	lines := []string{
		`{"t":"msg","role":"user","text":"draw it"}`,
		`{"t":"msg","role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit","arguments":"{\"path\":\"figure.tex\"}"}}]}`,
		`{"t":"msg","role":"tool","text":"SUBMITTED. The figure is accepted (overlap 21.48%, size deviation 9.6%).","tool_call_id":"c1"}`,
	}
	if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, st, err := session.LoadTranscriptStats(file)
	if err != nil {
		t.Fatal(err)
	}
	if st.SubmitName != "submit" || !st.SubmitAccepted {
		t.Fatalf("应当认出「最后一次提交已被收下」：name=%q accepted=%v", st.SubmitName, st.SubmitAccepted)
	}

	// 被打回的那一版不算收下（否则会把一张没画成的图当已完成）。
	bad := filepath.Join(dir, "vector_y.jsonl")
	lines[2] = `{"t":"msg","role":"tool","text":"REJECTED: call compare_figure at least once after a successful compile.","tool_call_id":"c1"}`
	if err := os.WriteFile(bad, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, st, err := session.LoadTranscriptStats(bad); err != nil {
		t.Fatal(err)
	} else if st.SubmitAccepted {
		t.Fatal("被打回的提交不该算收下")
	}
}
