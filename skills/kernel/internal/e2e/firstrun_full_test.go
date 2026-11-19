package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFirstRunCLIFull：05-first-run 手册全流程 CLI 验收（0–4 步直到 deliver）。
// agent 循环用 shell 模拟（claim → 沙箱写产物 → submit），与真 agent 的工具序列一致。
func TestFirstRunCLIFull(t *testing.T) {
	if _, err := exec.LookPath("pdfseparate"); err != nil {
		t.Skip("无 poppler")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("无 python3")
	}
	kernelRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	bin := filepath.Join(t.TempDir(), "dvagent")
	if out, err := exec.Command("go", "-C", kernelRoot, "build", "-buildvcs=false", "-o", bin, "./cmd/dvagent").CombinedOutput(); err != nil {
		t.Skipf("构建失败: %v %.100s", err, out)
	}

	// 假 mineru（同 FirstRunCLI）
	binDir := t.TempDir()
	fakeMineru := filepath.Join(binDir, "mineru")
	script := `#!/usr/bin/env bash
pdf=""; while [ $# -gt 0 ]; do case "$1" in -p) pdf="$2"; shift 2;; *) shift;; esac; done
base=$(basename "$pdf"); name="${base%.pdf}"
root="$(dirname "$(dirname "$(dirname "$pdf")")")"
auto="$root/mineru/$name/auto"
mkdir -p "$auto" "$root/source/images"
echo '[{"type":"text","page_idx":0,"text":"监督学习无监督学习"},{"type":"image","page_idx":0,"img_path":"images/'"$name"'.jpg","image_caption":["图"]}]' > "$auto/$name""_content_list.json"
echo '{"pdf_info":[{"page_size":[595,842]}]}' > "$auto/layout.json"
python3 -c "
from PIL import Image
img=Image.new('RGB',(60,60),(255,255,255))
for y in range(15,45):
    for x in range(15,45): img.putpixel((x,y),(0,0,0))
img.save('$root/source/images/$name.jpg')
" 2>/dev/null || cp "$pdf" "$root/source/images/$name.jpg"
echo ok`
	os.WriteFile(fakeMineru, []byte(script), 0o755)

	root := t.TempDir()
	env := append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "DOCVISION_PDF_SPLIT_TOOL=pdfcpu")
	dv := func(args ...string) (string, error) {
		cmd := exec.Command(bin, append([]string{"-workspace", root}, args...)...)
		cmd.Env = env
		b, err := cmd.CombinedOutput()
		return string(b), err
	}
	must := func(args ...string) string {
		t.Helper()
		s, err := dv(args...)
		if err != nil {
			t.Fatalf("dvagent %v: %v\n%s", args, err, s)
		}
		return s
	}

	// 0-1. 建区 + PREP（1 part 足够全流程）
	bookDir := filepath.Join(root, "book")
	partsDir := filepath.Join(bookDir, "source", "parts")
	os.MkdirAll(partsDir, 0o755)
	py := fmt.Sprintf("from reportlab.pdfgen import canvas\nc=canvas.Canvas(%q)\nfor i in range(2):\n    c.drawString(72,720,'hello')\n    c.showPage()\nc.save()", filepath.Join(bookDir, "book.pdf"))
	if b, err := exec.Command("python3", "-c", py).CombinedOutput(); err != nil {
		t.Skipf("reportlab: %v %.100s", err, b)
	}
	must("register", "demo", bookDir)
	must("activate", "demo")
	must("prep-split", "demo", filepath.Join(bookDir, "book.pdf"), "1") // 1页/part → 2 part → 2 图
	must("prep-mineru", "demo", filepath.Join(partsDir, "book_part1.pdf"), filepath.Join(partsDir, "book_part2.pdf"))
	must("prep-organize", "demo")
	must("prep-index", "demo")

	// 2. classify（agent 循环：claim → verdict → submit）
	must("gen-classify", "demo")
	out := must("claim", "demo", "agent-e2e", "5", "classify")
	// 解析任务 id：从沙箱目录反查更稳——直接对两个 cls_ 任务写 verdict 再 submit
	ids := taskIDsByPrefix(t, filepath.Join(bookDir, "skills_queue.json"), "cls_")
	if len(ids) != 2 {
		t.Fatalf("应有 2 classify 任务: %v", ids)
	}
	for _, id := range ids {
		v := `{"class":"vector","styled":false,"confidence":0.9,"reason":"线条图"}`
		if id == "cls_book_part2" {
			v = `{"class":"text","styled":true,"style_note":"测试样式线索","confidence":0.9,"extracted":"图注文字"}`
		}
		writeFile(t, filepath.Join(bookDir, "tasks", id, "out", "verdict.json"), v)
	}
	s := must("submit", "demo", joinIDs(ids))
	if !contains(s, `"status": "done"`) {
		t.Fatalf("classify submit: %.200s", s)
	}

	// draw：vector 那张铺任务（假 mineru 两图内容不同，不会并组覆盖）
	must("gen-draw", "demo")
	out = must("claim", "demo", "agent-e2e", "5", "draw")
	_ = out
	drawIDs := taskIDsByPrefix(t, filepath.Join(bookDir, "skills_queue.json"), "drw_")
	if len(drawIDs) < 1 {
		t.Fatalf("应有 draw 任务")
	}
	writeFile(t, filepath.Join(bookDir, "tasks", drawIDs[0], "out", "figure.tex"),
		"\\documentclass{article}\n\\usepackage{tikz}\n\\begin{document}\n\\begin{tikzpicture}\\draw(0,0)--(1,1);\\end{tikzpicture}\n\\end{document}\n")
	s = must("submit", "demo", drawIDs[0])
	if !contains(s, `"status": "done"`) {
		t.Fatalf("draw submit: %.200s", s)
	}

	// 3. convert：准备一章（organize 产物改名）→ 铺任务 → 写 .tex → submit
	chMD := filepath.Join(bookDir, "chapters", "chapter_001.md")
	os.Rename(filepath.Join(bookDir, "chapters", "book.md"), chMD)
	os.Remove(filepath.Join(bookDir, "chapters", "book_part1.md"))
	os.Remove(filepath.Join(bookDir, "chapters", "book_part2.md"))
	md := "# 第一章\n"
	for i := 0; i < 30; i++ {
		md += "监督学习与无监督学习的模型评估方法在特征工程中的应用实践案例。\n"
	}
	os.WriteFile(chMD, []byte(md), 0o644)
	must("gen-convert", "demo")
	must("claim", "demo", "agent-e2e", "5", "convert")
	convIDs := taskIDsByPrefix(t, filepath.Join(bookDir, "skills_queue.json"), "conv_")
	writeFile(t, filepath.Join(bookDir, "tasks", convIDs[0], "out", "chapter_001.tex"),
		"\\chapter{一}\n监督学习与无监督学习的模型评估方法。\n")
	s = must("submit", "demo", convIDs[0])
	if !contains(s, `"status": "done"`) {
		t.Fatalf("convert submit: %.200s", s)
	}

	// check + final（blocked 自动解锁后交 verdict）
	must("gen-check", "demo")
	must("claim", "demo", "agent-e2e", "5", "check")
	chkIDs := taskIDsByPrefix(t, filepath.Join(bookDir, "skills_queue.json"), "chk_")
	writeFile(t, filepath.Join(bookDir, "tasks", chkIDs[0], "out", "verdict.json"), `{"result":"pass"}`)
	s = must("submit", "demo", chkIDs[0])
	if !contains(s, `"status": "done"`) {
		t.Fatalf("check submit: %.200s", s)
	}
	must("gen-final", "demo")
	must("claim", "demo", "agent-e2e", "1", "final")
	writeFile(t, filepath.Join(bookDir, "tasks", "fin_book", "out", "verdict.json"), `{"result":"pass"}`)
	s = must("submit", "demo", "fin_book")
	if !contains(s, `"status": "done"`) {
		t.Fatalf("final submit: %.200s", s)
	}

	// 4. deliver
	s = must("deliver", "demo")
	if !contains(s, `"clean": true`) {
		t.Fatalf("deliver 应干净: %.200s", s)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "build", "main.tex")); err != nil {
		t.Fatal("build/main.tex 应生成")
	}
	t.Log("05-first-run 全流程 CLI 端到端通过（含 agent 循环模拟直到 deliver）")
}

// --- 小工具 ---

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func taskIDsByPrefix(t *testing.T, queuePath, prefix string) []string {
	t.Helper()
	raw, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	var q struct {
		Tasks map[string]json.RawMessage `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &q); err != nil {
		t.Fatal(err)
	}
	var out []string
	for id := range q.Tasks {
		if strings.HasPrefix(id, prefix) {
			out = append(out, id)
		}
	}
	return out
}

func joinIDs(ids []string) string { return strings.Join(ids, ",") }
