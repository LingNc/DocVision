package sessionview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/reviewpage"
)

// P64：核对页必须按**磁盘现状**渲染——状态来自 progress_items，成品来自
// figures/，链接只在文件真存在时出现。静态 review.html 是 dedupe 收尾时的快照，
// 那时 process 还没跑，照它显示会把全书都说成"已分类待处理"。
func TestRenderLiveImageReviewUsesDiskFacts(t *testing.T) {
	src := t.TempDir()
	mk := func(rel string, data []byte) {
		t.Helper()
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hashA := "0123456789abcdef0123456789abcdef"
	groups := map[string]any{
		"fingerprint": "fp-abc",
		"refs":        3,
		"distinct":    2,
		"rules":       map[string]any{"align_min": 0.6, "align_shift": 0.1, "icon_auto": 0.75},
		"groups": []any{map[string]any{
			"key_image": "images/book/a.png",
			"class":     "vector",
			"source":    "session",
			"members": []any{
				map[string]any{"ref": "book.md::images/book/a.png", "image": "images/book/a.png", "hash": hashA, "class": "vector"},
				map[string]any{"ref": "book.md::images/book/b.png", "image": "images/book/b.png", "hash": "fedcba9876543210fedcba9876543210", "class": "vector", "align_score": 0.91},
			},
		}},
		"independent": []any{map[string]any{"ref": "book.md::images/book/c.png", "image": "images/book/c.png", "hash": "aaaabbbbccccddddaaaabbbbccccdddd", "class": "raster"}},
	}
	blob, _ := json.Marshal(groups)
	mk("dedupe/groups.json", blob)
	mk("dedupe/review.html", []byte("<html>旧快照</html>"))
	mk("dedupe/thumbs/"+hashA[:12]+".jpg", []byte("jpg"))
	mk("figures/a.png", []byte("png"))
	mk("tikz/a.tex", []byte(`\draw`))
	mk("sessions/vector_a.jsonl", []byte("{}\n"))
	mk("progress_items/book.md/"+mustSanitize("images/book/a.png")+".json", progressJSON(t, map[string]any{
		"md": "book.md", "img": "images/book/a.png", "class": "vector", "status": "done",
		"figure_png": "figures/a.png", "figure_pdf": "figures/a.pdf", "tikz_code": `\draw`,
	}))
	// 退回原图的图：磁盘上还留着上一次重画失败前的旧成品（Go 的单图重画不删
	// 旧文件），核对页不能把它当"这一版的成品"显示出来。
	mk("figures/b.png", []byte("stale"))
	mk("sessions/vector_b.jsonl", []byte("{}\n"))
	mk("tikz/b.tex", []byte(`\stale`))
	mk("progress_items/book.md/"+mustSanitize("images/book/b.png")+".json", progressJSON(t, map[string]any{
		"md": "book.md", "img": "images/book/b.png", "class": "vector", "status": "fallback",
		"fallback_reason": "TikZ 重画没有通过校验", "equivalent_to": "book.md::images/book/a.png",
		"figure_png": "figures/b.png",
	}))
	mk("progress_items/book.md/"+mustSanitize("images/book/c.png")+".json", progressJSON(t, map[string]any{
		"md": "book.md", "img": "images/book/c.png", "class": "classified", "status": "classified",
	}))

	page, ok := renderLiveImageReview(src)
	if !ok {
		t.Fatal("有 groups.json 就该渲染成功")
	}
	html := string(page)
	for _, want := range []string{
		"已完成",                        // status=done
		"../figures/a.png",           // 成品并排
		"判定记录",                       // progress json 链接
		"../tikz/a.tex",              // LaTeX 源
		"../sessions/vector_a.jsonl", // 作图会话
		"TikZ 重画没有通过校验",              // 退回原因
		"退回原图的（已收尾但没画出成品）（1 张）", // degraded 段
		"等待转换的原图（1 张）",          // pending 段
		"引用 3 条",                // 统计口径来自 groups.json
		"已收尾（画好 1、退回原图 1、失败 0、未处理 1）",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("核对页缺少 %q", want)
		}
	}
	// 没有成品的那张不能出现"成品"栏。
	if strings.Count(html, "shot conv") != 1 {
		t.Fatalf("只有一张图有成品，实际出现 %d 次", strings.Count(html, "shot conv"))
	}
	// 退回原图的旧产物不许出现在页面上（既不并排，也不给 PDF/LaTeX 源链接）。
	if strings.Contains(html, "../figures/b.png") || strings.Contains(html, "../tikz/b.tex") {
		t.Fatal("退回原图的图不该显示上一版产物")
	}
	// 但它的会话仍然可点：失败原因要去那条会话里看。
	if !strings.Contains(html, "../sessions/vector_b.jsonl") {
		t.Fatal("退回原图的图仍应给出会话入口")
	}
}

// 没有 groups.json 时如实返回 false，让调用方退回静态页。
func TestRenderLiveImageReviewNeedsGroups(t *testing.T) {
	if _, ok := renderLiveImageReview(t.TempDir()); ok {
		t.Fatal("没有 groups.json 不应渲染")
	}
}

func progressJSON(t *testing.T, v map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustSanitize(s string) string { return reviewpage.SanitizeName(s) }
