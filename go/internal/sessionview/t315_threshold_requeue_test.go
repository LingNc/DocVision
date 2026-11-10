package sessionview

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// T315：把门槛抬到高于这张图上次成绩时，写阈值当场就把它标回待处理——
// 免得"改完还要等下一轮启动才退回"。降门槛/改用全局/已接受"尽力还原"/没成绩
// 的图都不动状态。
func TestFigureOverrideRaiseThresholdRequeues(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0.25,
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"requeued":true`) {
		t.Fatalf("回执要说清已当场退回：%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "待处理") {
		t.Fatalf("回执要给人一句说明：%s", rec.Body.String())
	}
	after := readOverrideFile(t, file)
	if after["status"] != "classified" {
		t.Fatalf("抬高门槛后应当回到待处理，实际 %v", after["status"])
	}
	if _, ok := after["tikz_code"]; ok {
		t.Fatalf("重画前要清掉成品字段")
	}
	if after["fig_min_overlap"] != 0.25 {
		t.Fatalf("阈值要写进去，实际 %v", after["fig_min_overlap"])
	}
	if after["fig_best_overlap"] != 0.12 {
		t.Fatalf("上次成绩要留着给人看，实际 %v", after["fig_best_overlap"])
	}
}

// 门槛没超过成绩（或改成用全局）时不动状态。
func TestFigureOverrideRaiseBelowScoreKeepsDone(t *testing.T) {
	for _, tc := range []struct {
		name string
		min  float64
	}{
		{"低于成绩", 0.10},
		{"用全局", 0},
	} {
		v, _, file := figureOverrideServer(t)
		rec := postFigureOverride(t, v, map[string]any{
			"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": tc.min,
		})
		if rec.Code != 200 {
			t.Fatalf("%s: status = %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"requeued":true`) {
			t.Fatalf("%s 不该退回：%s", tc.name, rec.Body.String())
		}
		if after := readOverrideFile(t, file); after["status"] != "done" {
			t.Fatalf("%s 不该改状态，实际 %v", tc.name, after["status"])
		}
	}
}

// 「尽力还原」接受过的图：抬门槛也不退回（要人先点「撤销接受」）。
func TestFigureOverrideAcceptedFigureNotRequeued(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	if rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png",
		"accept_as_is": true, "note": "扫描件太小，尽力了",
	}); rec.Code != 200 {
		t.Fatalf("接受失败: %d %s", rec.Code, rec.Body.String())
	}
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0.9,
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"requeued":true`) {
		t.Fatalf("接受过的图不该被门槛退回：%s", rec.Body.String())
	}
	if after := readOverrideFile(t, file); after["status"] != "done" || after["fig_accept_as_is"] != true {
		t.Fatalf("接受标记与状态都不该变: %+v", after)
	}
}

// 没有成绩的老图：抬门槛不动状态（没东西可判，下一轮也不会自己退回）。
func TestFigureOverrideNoScoreNotRequeued(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	after := readOverrideFile(t, file)
	delete(after, "fig_best_overlap")
	if err := os.WriteFile(file, mustJSON(t, after), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0.9,
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"requeued":true`) {
		t.Fatalf("没成绩不该退回：%s", rec.Body.String())
	}
	if got := readOverrideFile(t, file)["status"]; got != "done" {
		t.Fatalf("状态不该变，实际 %v", got)
	}
}
