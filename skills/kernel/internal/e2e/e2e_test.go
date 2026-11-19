// e2e：A1 验收前置——假书全链演练。一条测试打穿：
// 造书 → Split → fake MinerU → Organize → docindex → 注册项目 →
// classify（模拟 agent 验收循环）→ draw → convert → check → deliver。
// 需要 python3+reportlab（造真 PDF）与 poppler；缺则跳过。
package e2e

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/deliver"
	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/gen"
	"github.com/LingNc/DocVision/skills/kernel/internal/prep"
	"github.com/LingNc/DocVision/skills/kernel/internal/project"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

func TestWholeBookPipeline(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("无 python3")
	}
	if _, err := exec.LookPath("pdfseparate"); err != nil {
		t.Skip("无 poppler")
	}
	root := t.TempDir()

	// ① 造书：3 页 PDF
	bookPDF := filepath.Join(root, "book.pdf")
	script := fmt.Sprintf(`
from reportlab.pdfgen import canvas
c = canvas.Canvas(%q)
for i in range(3):
    c.drawString(72, 720, "第%%d章 监督学习无监督学习强化学习" %% (i+1))
    c.showPage()
c.save()
`, bookPDF)
	if out, err := exec.Command("python3", "-c", script).CombinedOutput(); err != nil {
		t.Skipf("reportlab 不可用: %v %.100s", err, out)
	}

	// ② Split：2 页/part → book_part1/2
	parts, err := prep.Split(bookPDF, root, prep.SplitConfig{PagesPerPart: 2}, nil)
	if err != nil || len(parts) != 2 {
		t.Fatalf("拆分: %v %v", parts, err)
	}

	// ③ fake MinerU：每张 part 产 content_list（文字块+图块）+ 真 PNG 图文件
	mineruOut := filepath.Join(root, "mineru")
	imgDir := filepath.Join(root, "source", "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(imgDir, "fig1.png"), 40, 40)
	writePNG(t, filepath.Join(imgDir, "fig2.png"), 40, 40)
	fake := func(name string, args ...string) ([]byte, error) {
		if name != "mineru" {
			return nil, fmt.Errorf("unexpected %s", name)
		}
		pdf := args[1]
		base := filepath.Base(pdf)
		stem := base[:len(base)-len(filepath.Ext(base))]
		auto := filepath.Join(mineruOut, stem, "auto")
		if err := os.MkdirAll(auto, 0o755); err != nil {
			return nil, err
		}
		fig := "fig1.png"
		if stem == "book_part2" {
			fig = "fig2.png"
		}
		cl := fmt.Sprintf(`[{"type":"text","page_idx":0,"text":"%s 监督学习无监督学习强化学习的内容"},{"type":"image","page_idx":0,"img_path":"images/%s","image_caption":["图"]}]`, stem, fig)
		if err := os.WriteFile(filepath.Join(auto, stem+"_content_list.json"), []byte(cl), 0o644); err != nil {
			return nil, err
		}
		return []byte("ok"), nil
	}
	for _, p := range parts {
		if _, err := prep.RunMinerU(filepath.Join(root, p), mineruOut, prep.MinerUConfig{Command: "mineru"}, fake); err != nil {
			t.Fatal(err)
		}
	}

	// ④ Organize + docindex
	auto1 := filepath.Join(mineruOut, "book_part1", "auto")
	auto2 := filepath.Join(mineruOut, "book_part2", "auto")
	mdDir := filepath.Join(root, "chapters")
	outs, err := prep.Organize([]string{auto1, auto2}, mdDir)
	if err != nil || len(outs) != 3 {
		t.Fatalf("organize: %v %v", outs, err)
	}
	idx, pages, err := docindex.Build(mineruOut, "", []string{"book_part1.md", "book_part2.md"}, mdDir, filepath.Join(root, "doc_index.json"))
	if err != nil || idx.Total() != 2 || pages.Total != 0 {
		t.Fatalf("docindex: %v %+v", err, pages)
	}
	if hits := idx.Search("监督学习", 3); len(hits) == 0 {
		t.Fatal("检索应命中")
	}

	// ⑤ 注册项目 + 队列
	ws := filepath.Join(root, "ws")
	reg, _ := project.Load(project.RegistryPath(ws))
	if _, err := reg.Register("demo", root); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Activate("demo"); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Load(queue.ProjectQueuePath(root))

	// ⑥ classify：两图，一 vector 一 styled_text（模拟 agent 看图写 verdict）
	crep, err := gen.GenClassify(q, root, "source/images")
	if err != nil || crep.Added != 2 {
		t.Fatalf("gen-classify: %+v", crep)
	}
	claimed := q.Claim("agent-e2e", 5, 600)
	ck := verify.NewChecker(root)
	for i, tk := range claimed {
		outDir := filepath.Join(root, tk.Sandbox, "out")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		v := `{"class":"vector","confidence":0.9,"reason":"线条图可重绘"}`
		if i == 1 {
			v = `{"class":"styled_text","confidence":0.9,"extracted":"图 2-1 模型评估"}`
		}
		if err := os.WriteFile(filepath.Join(outDir, "verdict.json"), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
		errs, _ := ck.Run(tk)
		if len(errs) != 0 {
			t.Fatalf("classify 验收: %v", errs)
		}
		q.RecordVerdict(tk.ID, task.Verdict{At: time.Now(), Pass: true}, 6)
	}

	// ⑦ draw：vector 图铺任务；agent 交 figure.tex
	drep, err := gen.GenDraw(q, root, nil)
	if err != nil || drep.Added != 1 {
		t.Fatalf("gen-draw: %+v", drep)
	}
	dclaimed := q.Claim("agent-e2e", 5, 600, task.TypeDraw)
	if len(dclaimed) != 1 {
		t.Fatalf("draw 任务应可领: %d", len(dclaimed))
	}
	dout := filepath.Join(root, dclaimed[0].Sandbox, "out")
	if err := os.MkdirAll(dout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dout, "figure.tex"), []byte("\\documentclass{article}\n\\usepackage{tikz}\n\\begin{document}\n\\begin{tikzpicture}\\draw(0,0)--(1,1);\\end{tikzpicture}\n\\end{document}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, _ := ck.Run(dclaimed[0])
	if len(errs) != 0 {
		t.Fatalf("draw 验收: %v", errs)
	}
	q.RecordVerdict(dclaimed[0].ID, task.Verdict{At: time.Now(), Pass: true}, 6)

	// ⑧ convert：chapters 里挑一章（part1 md 改名章）；agent 交自包含 .tex
	chDir := filepath.Join(root, "chapters")
	chapterMD := filepath.Join(chDir, "chapter_001.md")
	if err := os.Rename(filepath.Join(chDir, "book_part1.md"), chapterMD); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(chDir, "book_part2.md"))
	_ = os.Remove(filepath.Join(chDir, "book.md"))
	mdBody := "# 第一章\n"
	for i := 0; i < 30; i++ {
		mdBody += "监督学习与无监督学习的模型评估方法在特征工程中的应用实践案例。\n"
	}
	if err := os.WriteFile(chapterMD, []byte(mdBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.GenConvert(q, root, "chapters"); err != nil {
		t.Fatal(err)
	}
	cclaimed := q.Claim("agent-e2e", 5, 600, task.TypeConvert)
	if len(cclaimed) != 1 {
		t.Fatalf("convert 任务应可领: %d", len(cclaimed))
	}
	cout := filepath.Join(root, cclaimed[0].Sandbox, "out")
	if err := os.MkdirAll(cout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cout, "chapter_001.tex"), []byte("\\chapter{一}\n监督学习与无监督学习的模型评估方法。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, _ = ck.Run(cclaimed[0])
	if len(errs) != 0 {
		t.Fatalf("convert 验收: %v", errs)
	}
	q.RecordVerdict(cclaimed[0].ID, task.Verdict{At: time.Now(), Pass: true}, 6)
	q.Unblock(cclaimed[0].ID)

	// ⑨ check：先铺核对任务（blocked 已随 conv done 自动解除），agent 判 pass
	if _, err := gen.GenCheck(q); err != nil {
		t.Fatal(err)
	}
	kclaimed := q.Claim("agent-e2e", 5, 600, task.TypeCheck)
	if len(kclaimed) != 1 {
		t.Fatalf("check 应解锁: %d", len(kclaimed))
	}
	kout := filepath.Join(root, kclaimed[0].Sandbox, "out")
	if err := os.MkdirAll(kout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kout, "verdict.json"), []byte(`{"result":"pass"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	errs, _ = ck.Run(kclaimed[0])
	if len(errs) != 0 {
		t.Fatalf("check 验收: %v", errs)
	}
	q.RecordVerdict(kclaimed[0].ID, task.Verdict{At: time.Now(), Pass: true}, 6)

	// ⑩ 冷启动重载 + deliver
	q2, _ := queue.Load(queue.ProjectQueuePath(root))
	n := 0
	for _, tk := range q2.Tasks {
		if tk.Status == task.StatusDone {
			n++
		}
	}
	if n != 5 { // 2 classify + 1 draw + 1 convert + 1 check
		t.Fatalf("重载后 done 应=5（A2 冷启动续接）: %d", n)
	}
	rep, err := deliver.Deliver(q2, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean || len(rep.Chapters) != 1 || len(rep.Figures) != 1 || rep.Bookmarks != 1 {
		t.Fatalf("deliver: %+v", rep)
	}
	st, err := os.ReadFile(rep.Standalone)
	if err != nil || !contains(string(st), "chapters/chapter_001.tex") {
		t.Fatalf("standalone 应内联章节: %v %.100s", err, st)
	}
	t.Logf("A1 全链演练通过：%d 任务 done，交付 %s", n, rep.BuildDir)
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for y := 10; y < 30; y++ {
		for x := 10; x < 30; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
