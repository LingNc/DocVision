package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

func TestOfferMermaidInstallDeclined(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a shell script")
	}
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "npm"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)

	var out bytes.Buffer
	if err := offerMermaidInstall(strings.NewReader("n\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "跳过 Mermaid CLI 安装") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestOfferMermaidInstallAccepted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "installed")
	npm := filepath.Join(dir, "npm")
	writeExecutable(t, npm, "#!/bin/sh\nprintf installed > \"$MERMAID_TEST_MARKER\"\n")
	t.Setenv("PATH", dir)
	t.Setenv("MERMAID_TEST_MARKER", marker)

	var out bytes.Buffer
	if err := offerMermaidInstall(strings.NewReader("\n"), &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("npm was not invoked: %v; output=%q", err, out.String())
	}
	if !strings.Contains(out.String(), "Mermaid CLI 安装完成") {
		t.Fatalf("output = %q", out.String())
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

// preview.enabled 必须真的把服务起起来（用户要的是"跑起来就能在浏览器看"），
// 而且只读、默认只监听本机；端口 0 时日志里给的那个 URL 必须能打开。
func TestStartPreviewServesViewerWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	log, err := logger.NewLogger(filepath.Join(dir, "run.log"), filepath.Join(dir, "err.log"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()

	// 关闭时：什么都不起（不能占端口）。
	off := &config.Config{Preview: config.PreviewConfig{Enabled: false, Host: "127.0.0.1", Port: 0}}
	if stop := startPreview(off, log); stop != nil {
		stop()
		t.Fatal("preview.enabled=false 时不该启动服务")
	}

	// 打开时（端口 0 = 内核挑）：拿到 URL 后必须真能拉到页面。
	on := &config.Config{Preview: config.PreviewConfig{Enabled: true, Host: "127.0.0.1", Port: 0},
		Paths: config.PathsConfig{LatexProject: dir, LatexOutput: dir}}
	stop := startPreview(on, log)
	if stop == nil {
		t.Fatal("preview.enabled=true 必须启动服务")
	}
	defer stop()

	logPath := filepath.Join(dir, "run.log")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "[preview] 实时会话预览: http://127.0.0.1:") {
		t.Fatalf("启动日志里必须给出确切 URL（端口 0 时由内核挑）: %q", text)
	}
	i := strings.Index(text, "http://")
	url := strings.TrimSpace(text[i:])
	if j := strings.IndexAny(url, " \n"); j > 0 {
		url = url[:j]
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(page) < 500 {
		t.Fatalf("预览页面不可用: status=%d len=%d", resp.StatusCode, len(page))
	}
	stop()
}
