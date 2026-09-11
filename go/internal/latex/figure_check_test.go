package latex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

// figureCheckRunner builds a Runner whose figure-check model is a fake HTTP
// server, so the check can be driven end to end without a network.
func figureCheckRunner(t *testing.T, handler http.HandlerFunc) (*Runner, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	r := keepTestRunner(t)
	off := false
	r.cfg.Latex.DrawingModel = "drawing"
	r.cfg.Latex.FigureCheck = config.FigureCheckConfig{Enabled: true, Model: "verifier", MaxRounds: 2}
	r.cfg.Models = map[string]config.ModelConfig{
		"verifier": {BaseURL: srv.URL, APIKey: "k", Model: "vision-model", Stream: &off},
		"drawing":  {BaseURL: srv.URL, APIKey: "k", Model: "drawing-model", Stream: &off},
	}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	return r, srv
}

// tinyPNG writes a 1x1 PNG so ImageToBase64 has a real file to read.
func tinyPNG(t *testing.T, path string) string {
	t.Helper()
	// 8-byte PNG signature + IHDR/IDAT/IEND of a 1x1 transparent image.
	data := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00,
		0x1f, 0x15, 0xc4, 0x89,
		0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
		0x0d, 0x0a, 0x2d, 0xb4,
		0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 逐图校验必须真的把**两张图**送进会话（原图 + 重画渲染），并把
// report.status=issues 读成"不通过 + 问题清单"、pass 读成通过。
func TestFigureCheckSendsBothImagesAndReadsVerdict(t *testing.T) {
	var bodies []string
	r, _ := figureCheckRunner(t, func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		bodies = append(bodies, string(raw))
		// 一次 check = 一个会话回合：先给工具调用（submit），拿到工具结果后
		// 再给一句收尾文本。第 1 次请求判 issues，第 3 次（第二轮校验）判 pass。
		switch len(bodies) {
		case 1:
			jsonToolReply(w, "submit", `{"report":{"status":"issues","issues":"missing axis label on the y axis"}}`)
		case 3:
			jsonToolReply(w, "submit", `{"report":{"status":"pass"}}`)
		default:
			jsonReply(w, "Noted.")
		}
	})
	dir := t.TempDir()
	orig := tinyPNG(t, filepath.Join(dir, "orig.png"))
	png := tinyPNG(t, filepath.Join(dir, "fig.png"))
	r.projDir = dir

	check := r.newFigureChecker(1, orig, "chapter_001__venn", 2)
	if check == nil {
		t.Fatal("校验开着就必须建出校验器")
	}
	ok, problems := check(png)
	if ok || !strings.Contains(problems, "missing axis label") {
		t.Fatalf("第一轮必须判不通过并带回问题清单: ok=%v problems=%q", ok, problems)
	}
	if ok2, _ := check(png); !ok2 {
		t.Fatal("第二轮 pass 必须判通过")
	}

	if len(bodies) < 3 {
		t.Fatalf("两轮校验至少各一次请求（含收尾回合），got %d", len(bodies))
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &first); err != nil {
		t.Fatalf("请求体不是 JSON: %v", err)
	}
	text := bodies[0]
	// 两条图片消息：原图与重画图都在（每张一个 image_url part；会话统一按
	// image/jpeg 声明，厂商按内容解码，这与其它会话一致）。
	if n := strings.Count(text, `"type":"image_url"`); n != 2 {
		t.Fatalf("校验会话必须同时看到原图与重画图，image parts=%d", n)
	}
	if !strings.Contains(text, "figure_check") && !strings.Contains(text, "original") {
		t.Fatalf("提示词里没提原图/重画图的比较: %.400s", text)
	}
	// 第二轮要说明"这是同一比较的第 2 轮（对方已尝试修正）"。
	if !strings.Contains(bodies[2], "round") {
		t.Fatalf("后续轮次必须说明是同一比较的下一轮: %.300s", bodies[2])
	}
}

// 模型不交结论（跑飞/报错）时按通过处理：校验是额外的网，不是新的失败理由。
func TestFigureCheckFailsOpenWithoutVerdict(t *testing.T) {
	r, _ := figureCheckRunner(t, func(w http.ResponseWriter, req *http.Request) {
		jsonReply(w, "I looked at both images but nothing to report.")
	})
	dir := t.TempDir()
	orig := tinyPNG(t, filepath.Join(dir, "orig.png"))
	png := tinyPNG(t, filepath.Join(dir, "fig.png"))
	r.projDir = dir
	check := r.newFigureChecker(1, orig, "fig_name", 2)
	if ok, problems := check(png); !ok || problems != "" {
		t.Fatalf("没有结论时必须视为通过: ok=%v problems=%q", ok, problems)
	}
}

// 打回会话时给的是校验器的原话 + 两条硬要求（只改这几点、改完重编译重提交）。
func TestFigureCheckFeedbackQuotesProblems(t *testing.T) {
	msg := figureCheckFeedback("the legend is missing")
	if !strings.Contains(msg, "the legend is missing") {
		t.Fatal("打回消息必须带上校验器给的问题")
	}
	for _, want := range []string{"compile", "submit"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("打回消息必须要求 %s: %q", want, msg)
		}
	}
}
