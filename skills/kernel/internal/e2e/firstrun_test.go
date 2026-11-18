package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFirstRunCLI：首秀手册（skills/docs/05-first-run.md 的 0–2 步）端到端走 CLI。
// 真 poppler 拆书；假 mineru 是 PATH 里的 shell 脚本（写 content_list+layout+图）——
// 证明手册里每个命令在真实 dvagent 二进制上都可用，而不只是 Go 函数级 e2e。
func TestFirstRunCLI(t *testing.T) {
	if _, err := exec.LookPath("pdfseparate"); err != nil {
		t.Skip("无 poppler")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("无 python3")
	}
	// 构建 dvagent 二进制（测试的工作目录是包目录，用 -C 回模块根）
	kernelRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	bin := filepath.Join(t.TempDir(), "dvagent")
	out, err := exec.Command("go", "-C", kernelRoot, "build", "-buildvcs=false", "-o", bin, "./cmd/dvagent").CombinedOutput()
	if err != nil {
		t.Skipf("构建失败（环境无常驻工具链可换路径）: %v %.120s", err, out)
	}
	runtime.KeepAlive(bin)

	// 假 mineru：PATH 注入（prep-mineru 命令经 CLI 调 mineru）
	binDir := t.TempDir()
	fakeMineru := filepath.Join(binDir, "mineru")
	script := `#!/usr/bin/env bash
# 假 MinerU：-p <pdf> → 写 auto/<名>/auto/{content_list,layout} + 抽图到 source/images
pdf=""; while [ $# -gt 0 ]; do case "$1" in -p) pdf="$2"; shift 2;; *) shift;; esac; done
base=$(basename "$pdf"); name="${base%.pdf}"
root="$(dirname "$(dirname "$(dirname "$pdf")")")"  # book/source/parts/x.pdf → book
auto="$root/mineru/$name/auto"
mkdir -p "$auto" "$root/source/images"
echo '[{"type":"text","page_idx":0,"text":"监督学习的基础"},{"type":"image","page_idx":0,"img_path":"images/'"$name"'.jpg","image_caption":["图"]}]' > "$auto/$name""_content_list.json"
echo '{"pdf_info":[{"page_size":[595,842]}]}' > "$auto/layout.json"
python3 -c "
from PIL import Image
img=Image.new('RGB',(60,60),(255,255,255))
for y in range(15,45):
    for x in range(15,45): img.putpixel((x,y),(0,0,0))
img.save('$root/source/images/$name.jpg')
" 2>/dev/null || cp "$pdf" "$root/source/images/$name.jpg"
echo "ok $name"
`
	if err := os.WriteFile(fakeMineru, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	env := append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"DOCVISION_PDF_SPLIT_TOOL=pdfcpu",
	)
	dv := func(args ...string) (string, error) {
		cmd := exec.Command(bin, append([]string{"-workspace", root}, args...)...)
		cmd.Env = env
		b, err := cmd.CombinedOutput()
		return string(b), err
	}
	mustDV := func(args ...string) string {
		t.Helper()
		s, err := dv(args...)
		if err != nil {
			t.Fatalf("dvagent %v: %v\n%s", args, err, s)
		}
		return s
	}

	// 0. 建区 + 造书 + 登记
	bookDir := filepath.Join(root, "book")
	os.MkdirAll(bookDir, 0o755)
	py := fmt.Sprintf("from reportlab.pdfgen import canvas\nc=canvas.Canvas(%q)\n[c.drawString(72,720,'p %%d'%%i) or c.showPage() for i in range(3)]\nc.save()", filepath.Join(bookDir, "book.pdf"))
	if b, err := exec.Command("python3", "-c", py).CombinedOutput(); err != nil {
		t.Skipf("reportlab: %v %.100s", err, b)
	}
	mustDV("register", "demo", bookDir)
	mustDV("activate", "demo")

	// 1. PREP 四命令
	s := mustDV("prep-split", "demo", filepath.Join(bookDir, "book.pdf"), "2")
	if !strings.Contains(s, "book_part1") || !strings.Contains(s, "book_part2.pdf") {
		t.Fatalf("prep-split 应出 2 part: %.200s", s)
	}
	s = mustDV("prep-mineru", "demo", filepath.Join(bookDir, "source/parts/book_part1.pdf"), filepath.Join(bookDir, "source/parts/book_part2.pdf"))
	if !strings.Contains(s, "content_list") {
		t.Fatalf("prep-mineru 应定位 content_list: %.200s", s)
	}
	s = mustDV("prep-organize", "demo")
	if !strings.Contains(s, "book.md") {
		t.Fatalf("prep-organize 应出 book.md: %.200s", s)
	}
	s = mustDV("prep-index", "demo")
	if !strings.Contains(s, `"entries": 4`) { // 2 part × (text+image)
		t.Fatalf("prep-index 应有 4 条目: %.200s", s)
	}

	// 2. 图片阶段起步：gen-classify 铺任务（假 mineru 抽了 2 张图）
	s = mustDV("gen-classify", "demo")
	if !strings.Contains(s, `"added": 2`) {
		t.Fatalf("gen-classify 应铺 2 任务: %.200s", s)
	}
	// imgxxx 编号登记
	s = mustDV("ids-list", "demo", "img")
	if !strings.Contains(s, "img001") {
		t.Fatalf("img001 应已登记: %.100s", s)
	}
	// 检索链活：doc-search 命中内容
	s = mustDV("doc-search", "demo", "监督学习")
	var ds struct {
		Hits []struct {
			Text string `json:"text"`
		} `json:"hits"`
	}
	json.Unmarshal([]byte(s), &ds)
	if len(ds.Hits) == 0 {
		t.Fatalf("doc-search 应命中: %.150s", s)
	}
	// 看图台产物
	s = mustDV("viewer-build", "demo")
	if !strings.Contains(s, `"viewer"`) || !strings.Contains(s, "work/viewer/index.html") {
		t.Fatalf("viewer-build 应出页面路径: %.200s", s)
	}
	if _, err := os.Stat(filepath.Join(bookDir, "work", "viewer", "index.html")); err != nil {
		t.Fatal("看图台 HTML 应存在")
	}
	t.Log("05-first-run 的 0–2 步 CLI 端到端全通")
}
