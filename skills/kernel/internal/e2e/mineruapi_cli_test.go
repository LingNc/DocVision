package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMineruAPIViaCLI：prep-mineru-api 命令级测试——假 API 服务端 + config-set
// 注入 base_url + MINERU_TOKEN 环境变量，走真实 dvagent 二进制。
// 覆盖：断点续跑（batch_id 落盘重跑）+ 解压归位 + 图源发现链路数据就位。
func TestMineruAPIViaCLI(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("无 python3")
	}
	kernelRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	bin := filepath.Join(t.TempDir(), "dvagent")
	if out, err := exec.Command("go", "-C", kernelRoot, "build", "-buildvcs=false", "-o", bin, "./cmd/dvagent").CombinedOutput(); err != nil {
		t.Skipf("构建失败: %v %.100s", err, out)
	}

	// 假 MinerU 服务端
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for name, body := range map[string]string{
		"p1/auto/p1_content_list.json": `[{"type":"text","page_idx":0,"text":"正文"}]`,
		"p1/auto/layout.json":          `{"pdf_info":[{"page_size":[595,842]}]}`,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()

	var uploads int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/api/v1/file-urls/batch", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"batch_id": "bb1", "file_urls": []string{srv.URL + "/up"}}})
	})
	mux.HandleFunc("/up", func(w http.ResponseWriter, r *http.Request) { uploads++; w.WriteHeader(200) })
	mux.HandleFunc("/api/v1/extract-results/batch/bb1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{
			"extract_result": []any{map[string]any{"state": "done", "full_zip_url": srv.URL + "/dl"}}}})
	})
	mux.HandleFunc("/dl", func(w http.ResponseWriter, r *http.Request) { w.Write(zbuf.Bytes()) })

	// 工作区 + 项目 + part 文件
	root := t.TempDir()
	bookDir := filepath.Join(root, "book")
	partsDir := filepath.Join(bookDir, "source", "parts")
	os.MkdirAll(partsDir, 0o755)
	os.WriteFile(filepath.Join(partsDir, "p1.pdf"), []byte("%PDF-fake"), 0o644)

	env := append(os.Environ(),
		"MINERU_TOKEN=test-token",
		"PATH="+filepath.Dir(bin)+":"+os.Getenv("PATH"),
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
	mustDV("register", "demo", bookDir)
	mustDV("activate", "demo")
	// config-set 注入假 base_url
	mustDV("config-set", "demo", "mineru.api_base_url", srv.URL+"/api/v1")
	mustDV("config-set", "demo", "mineru.poll_interval", "1")

	// 第一次跑：上传→done→解压归位
	s := mustDV("prep-mineru-api", "demo", filepath.Join(partsDir, "p1.pdf"))
	if !contains(s, `"status": "done"`) {
		t.Fatalf("prep-mineru-api 应 done: %.200s", s)
	}
	// 归一契约=part 根（对齐老 DocVision 结构约定，无 auto 层）
	if _, err := os.Stat(filepath.Join(bookDir, "mineru", "p1", "p1_content_list.json")); err != nil {
		t.Fatal("content_list 应收敛到 mineru/p1 根")
	}
	if _, err := os.Stat(filepath.Join(bookDir, "mineru", "p1", "auto")); err == nil {
		t.Fatal("不应存在 auto 层")
	}
	// 断点续跑：清掉产物、保留 .mineru_task.json → 重跑不应再上传（uploads 不变）
	os.RemoveAll(filepath.Join(bookDir, "mineru", "p1", "p1_content_list.json"))
	uploadsBefore := uploads
	s = mustDV("prep-mineru-api", "demo", filepath.Join(partsDir, "p1.pdf"))
	if !contains(s, `"status": "done"`) {
		t.Fatalf("断点重跑应 done: %.200s", s)
	}
	if uploads != uploadsBefore {
		t.Fatalf("断点重跑不应重传：uploads %d→%d", uploadsBefore, uploads)
	}
	t.Logf("prep-mineru-api CLI 全链+断点续跑通过（重跑零重传）")
}
