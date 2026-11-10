package sessionview

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/reviewpage"
)

// T307/P78：逐图门槛的写入口。两处界面共用——核对页按 (md, img) 定位，会话页
// 手里只有作图转录名（vector_<成品名>.jsonl），两条路都必须落到同一条记录上。
func figureOverrideServer(t *testing.T) (*viewerServer, string, string) {
	t.Helper()
	v, proj := retryTestServer(t, true)
	mdName := "书A.md"
	imgName := "images/书A/a1b2c3.png"
	prog := map[string]any{
		"md": mdName, "img": imgName, "class": "vector", "label": "vector", "status": "done",
		"fig_best_overlap": 0.12, "tikz_code": "\\draw (0,0)--(1,1);",
	}
	data, _ := json.MarshalIndent(prog, "", "  ")
	dir := filepath.Join(proj, "source", "progress_items", reviewpage.SanitizeName(mdName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, reviewpage.SanitizeName(imgName)+".json")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return v, proj, file
}

func postFigureOverride(t *testing.T, v *viewerServer, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/figure-override", bytes.NewBuffer(raw))
	rec := httptest.NewRecorder()
	v.ServeHTTP(rec, req)
	return rec
}

func readOverrideFile(t *testing.T, file string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// 按 md+img 设阈值：写进 fig_min_overlap，0 表示回到全局（删掉这个键）。
func TestFigureOverrideSetsThreshold(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0.25,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("应成功：%d %s", rec.Code, rec.Body.String())
	}
	if got := readOverrideFile(t, file)["fig_min_overlap"]; got != 0.25 {
		t.Fatalf("fig_min_overlap 应为 0.25，实际 %v", got)
	}
	// 0 = 用全局：字段删掉，不写 0（否则会被当成"这张图不卡"）。
	rec = postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("应成功：%d %s", rec.Code, rec.Body.String())
	}
	if _, ok := readOverrideFile(t, file)["fig_min_overlap"]; ok {
		t.Fatal("0 应当删掉字段（回到全局），不能落盘成 0")
	}
}

// 按作图转录名定位（会话页只有它）：成品名规则必须与 latex 那边一致。
func TestFigureOverrideResolvesTranscriptPath(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	texBase := vectorTexBaseOf("书A.md", "images/书A/a1b2c3.png", "vector")
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "path": "vector_" + texBase + ".jsonl", "min_overlap": 0.3,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("按转录名应成功：%d %s", rec.Code, rec.Body.String())
	}
	if got := readOverrideFile(t, file)["fig_min_overlap"]; got != 0.3 {
		t.Fatalf("fig_min_overlap 应为 0.3，实际 %v", got)
	}
}

// "尽力还原"要一句 note；撤销会把这套标记删掉。
func TestFigureOverrideAcceptNeedsNote(t *testing.T) {
	v, _, file := figureOverrideServer(t)
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "accept_as_is": true,
	})
	if rec.Code == http.StatusOK {
		t.Fatal("没有 note 的接受必须被拒")
	}
	rec = postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png",
		"accept_as_is": true, "note": "字号压不下来",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("带 note 应成功：%d %s", rec.Code, rec.Body.String())
	}
	rec2 := readOverrideFile(t, file)
	if rec2["fig_accept_as_is"] != true || rec2["fig_accept_note"] != "字号压不下来" {
		t.Fatalf("接受标记没落盘：%v", rec2)
	}
	rec = postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "accept_as_is": false,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("撤销应成功：%d %s", rec.Code, rec.Body.String())
	}
	if _, ok := readOverrideFile(t, file)["fig_accept_as_is"]; ok {
		t.Fatal("撤销后不该还留着接受标记")
	}
}

// 越界项目名、比率越界、没给定位、总开关关掉——四种拒绝都要有话说。
func TestFigureOverrideRejectsBadInput(t *testing.T) {
	v, _, _ := figureOverrideServer(t)
	cases := []map[string]any{
		{"project": "../..", "md": "书A.md", "img": "x.png", "min_overlap": 0.2},
		{"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 1.5},
		{"project": "书A"},
		{"project": "书A", "md": "书A.md", "img": "images/书A/nope.png", "min_overlap": 0.2},
	}
	for _, body := range cases {
		if rec := postFigureOverride(t, v, body); rec.Code == http.StatusOK {
			t.Fatalf("应当拒绝：%v", body)
		}
	}
	v.allowRetry = false
	rec := postFigureOverride(t, v, map[string]any{
		"project": "书A", "md": "书A.md", "img": "images/书A/a1b2c3.png", "min_overlap": 0.2,
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("--no-retry 时应当 403：%d", rec.Code)
	}
}

// 核对页：矢量图卡片上要挂控件（实时渲染），静态导出没有控件。
func TestReviewPageRendersThresholdControls(t *testing.T) {
	v, proj, _ := figureOverrideServer(t)
	_ = v
	page, ok := renderLiveImageReview(filepath.Join(proj, "source"), "书A")
	if !ok {
		t.Fatal("有图片记录就该能渲染")
	}
	html := string(page)
	for _, want := range []string{"/api/figure-override", "data-act=\"save\"", "data-act=\"accept\"", "上次 12.0%"} {
		if !strings.Contains(html, want) {
			t.Fatalf("核对页缺 %q", want)
		}
	}
	static, ok := renderLiveImageReview(filepath.Join(proj, "source"), "")
	if !ok {
		t.Fatal("静态渲染也该成功")
	}
	if strings.Contains(string(static), "data-act=\"save\"") {
		t.Fatal("静态导出（没有项目名）不该有写控件")
	}
}
