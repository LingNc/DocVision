package latex

import (
	"path/filepath"
	"strings"
	"testing"
)

// 看图软预算：按"对象"计数（view_image 每张图、view_pdf 每个文件每页），
// 用满 warn_ratio 起提醒剩余次数，超出后提示尽快提交——但从不拦截。
func TestViewBudgetNoteBoundaries(t *testing.T) {
	// 30 张上限、0.7 比例 → 向上取整 21，从第 21 次（含）起提醒
	if got := viewBudgetNote("view_image on a.png", 20, 30, 0.7); got != "" {
		t.Errorf("第 20 次不该提醒: %q", got)
	}
	got := viewBudgetNote("view_image on a.png", 21, 30, 0.7)
	if !strings.Contains(got, "21/30") || !strings.Contains(got, "only 9 left") {
		t.Errorf("第 21 次应提醒剩余 9 次: %q", got)
	}
	if g := viewBudgetNote("view_image on a.png", 30, 30, 0.7); !strings.Contains(g, "only 0 left") {
		t.Errorf("第 30 次剩 0: %q", g)
	}
	if g := viewBudgetNote("view_pdf on b.pdf p3", 31, 30, 0.7); !strings.Contains(g, "BUDGET SPENT") || !strings.Contains(g, "31/30") {
		t.Errorf("超出后应提示预算用尽: %q", g)
	}
	// 0 = 不限
	if g := viewBudgetNote("view_image on a.png", 999, 0, 0.7); g != "" {
		t.Errorf("SoftMax=0 应无预算: %q", g)
	}
}

// 24 张上限、0.7 → ceil(16.8)=17（不是 16）
func TestViewBudgetNoteRounding(t *testing.T) {
	if g := viewBudgetNote("x", 16, 24, 0.7); g != "" {
		t.Errorf("第 16 次不该提醒: %q", g)
	}
	if g := viewBudgetNote("x", 17, 24, 0.7); !strings.Contains(g, "only 7 left") {
		t.Errorf("第 17 次应提醒剩余 7 次: %q", g)
	}
}

// 每张图独立计数：同一会话里看另一张图不会继承前一张的用量。
func TestViewImageCountsPerFile(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), 40, 20)
	writePNG(t, filepath.Join(dir, "b.png"), 40, 20)
	tool := &ViewImageTool{Root: dir, SoftMax: 2, WarnRatio: 0.7}
	call := func(name string) string {
		res, err := tool.Execute(`{"path":"` + name + `","zoom":64}`)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res.Text
	}
	call("a.png")
	call("a.png") // 第二张：2/2 用满
	if got := call("b.png"); strings.Contains(got, "BUDGET") {
		t.Errorf("另一张图不该带着上一张的用量: %q", got)
	}
	// 回到 a.png 超出预算：仍是提醒，不是拒绝
	res, err := tool.Execute(`{"path":"a.png","zoom":64}`)
	if err != nil {
		t.Fatalf("超出预算不应报错: %v", err)
	}
	if res.ImageBase64 == "" {
		t.Error("超出预算仍应返回图片（软预算不拦截）")
	}
	if !strings.Contains(res.Text, "BUDGET SPENT") {
		t.Errorf("应提示预算用尽: %q", res.Text)
	}
}
