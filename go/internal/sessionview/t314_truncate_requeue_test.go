package sessionview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// T314：截断作图会话之后，这张图的进度必须改回"待处理"。
// 只改转录不动 progress 的话，它下次运行仍然是 done、根本不会被派下去——
// 用户截断的意义（让它带着干净的上下文接着画）就没了。
func TestPostTruncateVectorRequeuesProgress(t *testing.T) {
	v, proj := retryTestServer(t, true)
	hash := strings.Repeat("ab", 32)
	md := "书A.md"
	progDir := filepath.Join(proj, "source", "progress_items", md)
	sessDir := filepath.Join(proj, "source", "sessions")
	for _, d := range []string{progDir, sessDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	progPath := filepath.Join(progDir, "images_书A_"+hash+".json")
	rec := map[string]any{
		"key": md + "::images/书A/" + hash + ".jpg", "md": md,
		"img": "images/书A/" + hash + ".jpg", "class": "vector", "label": "plot",
		"status": "done", "tikz_code": "\\draw (0,0)--(1,1);",
		"figure_pdf": "figures/x.pdf", "figure_png": "figures/x.png",
		"fig_best_overlap": 0.42, "fig_scale_dev": 0.05, "fig_min_overlap": 0.2,
		"fig_accept_as_is": true, "fig_accept_note": "尽力还原",
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(progPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	name := "vector_书A__" + hash + "__plot.jsonl"
	full := filepath.Join(sessDir, name)
	body := "{\"n\":1}\n{\"n\":2}\n{\"n\":3}\n"
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/truncate",
		strings.NewReader(`{"project":"书A","path":"`+name+`","line":2}`))
	w := httptest.NewRecorder()
	v.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if note, _ := resp["note"].(string); !strings.Contains(note, "待处理") {
		t.Fatalf("回执要说明这张图已标回待处理：%q", resp["note"])
	}
	if got, _ := os.ReadFile(full); string(got) != "{\"n\":1}\n{\"n\":2}\n" {
		t.Fatalf("截断结果不对: %q", string(got))
	}
	// 备份仍在 .trash/ 下，可回退。
	if trash, _ := filepath.Glob(filepath.Join(proj, "work", "sessions", ".trash", name+".*.jsonl")); len(trash) != 1 {
		t.Fatalf("备份缺失: %v", trash)
	}

	after := map[string]any{}
	data, err := os.ReadFile(progPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	if after["status"] != "classified" {
		t.Fatalf("截断后这张图应当回到待处理，实际 status = %v", after["status"])
	}
	for _, k := range []string{"tikz_code", "figure_pdf", "figure_png", "fig_best_overlap", "fig_accept_as_is", "fig_accept_note"} {
		if _, ok := after[k]; ok {
			t.Errorf("重画前要清掉 %s（不能带着上次的成品/成绩/接受标记）", k)
		}
	}
	// 分类、dedupe 与这张图自己的阈值覆盖必须留着。
	if after["class"] != "vector" || after["label"] != "plot" {
		t.Errorf("分类不该被清掉: %+v", after)
	}
	if after["fig_min_overlap"] != 0.2 {
		t.Errorf("这张图自己的阈值覆盖要留着，实际 %v", after["fig_min_overlap"])
	}
}

// dedupe 跟随（副本）不单独重画：截断也不该把它从 done 翻成待处理。
func TestPostTruncateVectorSkipsDedupeFollower(t *testing.T) {
	v, proj := retryTestServer(t, true)
	hash := strings.Repeat("cd", 32)
	md := "书A.md"
	progDir := filepath.Join(proj, "source", "progress_items", md)
	sessDir := filepath.Join(proj, "source", "sessions")
	for _, d := range []string{progDir, sessDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	progPath := filepath.Join(progDir, "images_书A_"+hash+".json")
	raw, _ := json.Marshal(map[string]any{
		"md": md, "img": "images/书A/" + hash + ".jpg", "status": "done",
		"equivalent_to": "书A.md::images/书A/leader.jpg",
	})
	if err := os.WriteFile(progPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	name := "vector_书A__" + hash + "__plot.jsonl"
	full := filepath.Join(sessDir, name)
	if err := os.WriteFile(full, []byte("{\"n\":1}\n{\"n\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/truncate",
		strings.NewReader(`{"project":"书A","path":"`+name+`","line":1}`))
	w := httptest.NewRecorder()
	v.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if note, _ := resp["note"].(string); !strings.Contains(note, "副本") {
		t.Fatalf("副本要说明为什么不改状态：%q", resp["note"])
	}
	data, _ := os.ReadFile(progPath)
	var after map[string]any
	_ = json.Unmarshal(data, &after)
	if after["status"] != "done" {
		t.Fatalf("副本不该被翻成待处理: %v", after["status"])
	}
}

// 章节转录截断不碰逐图进度（只处理作图会话）。
func TestPostTruncateChapterLeavesProgressAlone(t *testing.T) {
	v, proj := retryTestServer(t, true)
	hash := strings.Repeat("ef", 32)
	md := "书A.md"
	progDir := filepath.Join(proj, "source", "progress_items", md)
	sessDir := filepath.Join(proj, "work", "sessions")
	for _, d := range []string{progDir, sessDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	progPath := filepath.Join(progDir, "images_书A_"+hash+".json")
	raw, _ := json.Marshal(map[string]any{"md": md, "img": "images/书A/" + hash + ".jpg", "status": "done"})
	if err := os.WriteFile(progPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	name := "convert_chapter_002.jsonl"
	full := filepath.Join(sessDir, name)
	if err := os.WriteFile(full, []byte("{\"n\":1}\n{\"n\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(full, old, old); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/truncate",
		strings.NewReader(`{"project":"书A","path":"`+name+`","line":1}`))
	w := httptest.NewRecorder()
	v.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if note, _ := resp["note"].(string); note != "" {
		t.Fatalf("章节转录不该带回作图回执：%q", note)
	}
	data, _ := os.ReadFile(progPath)
	var after map[string]any
	_ = json.Unmarshal(data, &after)
	if after["status"] != "done" {
		t.Fatalf("章节截断不该动画的进度: %v", after["status"])
	}
}
