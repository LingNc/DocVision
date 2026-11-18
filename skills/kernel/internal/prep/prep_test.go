package prep

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 用 poppler 自带工具造一本 N 页真 PDF（pdfseparate/pdfunite 从空 PDF 造页不行，
// 用 python reportlab 若可用；否则跳过集成测试）。
func makeTestPDF(t *testing.T, path string, pages int) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("无 python3，跳过真 PDF 集成测试")
	}
	script := fmt.Sprintf(`
from reportlab.pdfgen import canvas
c = canvas.Canvas(%q)
for i in range(%d):
    c.drawString(72, 720, "page %%d" %% (i+1))
    c.showPage()
c.save()
`, path, pages)
	if out, err := exec.Command("python3", "-c", script).CombinedOutput(); err != nil {
		t.Skipf("reportlab 不可用: %v %.120s", err, out)
	}
}

func TestSplitIntegration(t *testing.T) {
	if _, err := exec.LookPath("pdfseparate"); err != nil {
		t.Skip("无 poppler")
	}
	dir := t.TempDir()
	pdf := filepath.Join(dir, "book.pdf")
	makeTestPDF(t, pdf, 5)

	parts, err := Split(pdf, dir, SplitConfig{PagesPerPart: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 || parts[0] != "book_part1.pdf" || parts[2] != "book_part3.pdf" {
		t.Fatalf("5 页按 2 页/part 应得 3 part: %v", parts)
	}
	// 幂等：重拆不重建
	parts2, err := Split(pdf, dir, SplitConfig{PagesPerPart: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts2) != 3 {
		t.Fatalf("重拆应幂等: %v", parts2)
	}
	// part3 应只有 1 页（5 = 2+2+1）
	out, err := exec.Command("pdfinfo", filepath.Join(dir, "book_part3.pdf")).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(out), "Pages:           1") && !contains(string(out), "Pages:         1") && !contains(string(out), "Pages: 1") {
		t.Fatalf("part3 应为 1 页: %.100s", out)
	}
	// 临时目录已清
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			t.Fatalf("临时目录应已清理: %s", e.Name())
		}
	}
}

// fakeRunner 按命令名分派：记录调用并产出假产物。
func fakeMineruRunner(t *testing.T, mineruOut string) Runner {
	return func(name string, args ...string) ([]byte, error) {
		switch name {
		case "mineru":
			// args: -p <pdf> ... → 造 auto/<name>/auto/*_content_list.json
			var pdf string
			for i, a := range args {
				if a == "-p" && i+1 < len(args) {
					pdf = args[i+1]
				}
			}
			base := filepath.Base(pdf)
			name := base[:len(base)-len(filepath.Ext(base))]
			auto := filepath.Join(mineruOut, name, "auto")
			if err := os.MkdirAll(auto, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(auto, name+"_content_list.json"), []byte("[]"), 0o644); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(auto, "layout.json"), []byte("{}"), 0o644); err != nil {
				return nil, err
			}
			return []byte("ok"), nil
		}
		return nil, fmt.Errorf("unexpected command %s", name)
	}
}

func TestRunMinerUWithFakeRunner(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "mineru")
	pdf := filepath.Join(dir, "book_part1.pdf")
	if err := os.WriteFile(pdf, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := RunMinerU(pdf, outDir, MinerUConfig{Command: "mineru"}, fakeMineruRunner(t, outDir))
	if err != nil {
		t.Fatal(err)
	}
	if res.ContentList == "" || res.LayoutJSON == "" || res.AutoDir == "" {
		t.Fatalf("产物定位不全: %+v", res)
	}
	// 缺 content_list 时报错而非静默
	badOut := filepath.Join(dir, "mineru2")
	res2, err := RunMinerU(pdf, badOut, MinerUConfig{Command: "mineru"}, func(name string, args ...string) ([]byte, error) {
		return []byte("ran"), nil // runner 不产物
	})
	if err == nil || res2.ContentList != "" {
		t.Fatalf("无 content_list 应报错: %+v %v", res2, err)
	}
}

func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
