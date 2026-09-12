package sessionview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeBannerReportsProvenance 钉住启动横幅：三行必须说清"扫哪个目录、
// 监听哪个地址、用的哪个配置文件"，并且各自带来源（不许出现解释性散文）。
func TestServeBannerReportsProvenance(t *testing.T) {
	dir := t.TempDir()
	lines := strings.Split(strings.TrimRight(serveBanner("http://127.0.0.1:8848/", ServeOptions{
		Root:       dir,
		Addr:       "127.0.0.1:8848",
		DirSource:  "config paths.latex_project",
		AddrSource: "config preview.host/port",
		ConfigNote: "/home/share/samba-share/PDF2MD/config.yaml",
	}), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("横幅不是三行：%q", lines)
	}
	if !strings.Contains(lines[0], "http://127.0.0.1:8848/") || !strings.Contains(lines[0], "只读服务，Ctrl+C 停止") {
		t.Errorf("第一行没有给出 URL 与停止方式：%q", lines[0])
	}
	if !strings.Contains(lines[1], dir) || !strings.Contains(lines[1], "来源 config paths.latex_project") {
		t.Errorf("第二行没有给出目录与来源：%q", lines[1])
	}
	if !strings.Contains(lines[2], "/home/share/samba-share/PDF2MD/config.yaml") ||
		!strings.Contains(lines[2], "监听地址来源 config preview.host/port") {
		t.Errorf("第三行没有给出配置文件与地址来源：%q", lines[2])
	}

	// 没有配置时也要说清：配置未找到 + 地址用的是内置默认。
	none := strings.Split(strings.TrimRight(serveBanner("http://127.0.0.1:8848/", ServeOptions{
		Root: dir, Addr: DefaultAddr, DirSource: "当前目录（未找到 config）", AddrSource: "内置默认",
	}), "\n"), "\n")
	if len(none) != 3 || !strings.Contains(none[2], "配置: 未找到") || !strings.Contains(none[2], "内置默认") {
		t.Fatalf("无配置时的横幅说不清来源：%q", none)
	}
}

// TestServeBindsConfiguredAddress 钉住 --serve 真的按给定地址起服务（配置改造后
// 的地址链路端到端走一遍，端口 0 由内核分配）。
func TestServeBindsConfiguredAddress(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "s.jsonl"), `{"t":"msg","role":"user","text":"hi"}`+"\n")
	url, stop, err := Start(dir, PreviewAddr("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = stop }()
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("URL = %q", url)
	}
	if strings.HasSuffix(url, ":0/") {
		t.Fatalf("端口 0 没有被解析成实际端口：%q", url)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}
