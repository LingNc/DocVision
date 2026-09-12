package sessionview

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeBannerReportsProvenance 钉住启动横幅：必须说清"扫哪个目录、
// 监听哪个地址、用的哪个配置文件"，并且各自带来源（不许出现解释性散文）。
func TestServeBannerReportsProvenance(t *testing.T) {
	dir := t.TempDir()
	loopback := Bound{Addr: "127.0.0.1:8848", URL: "http://127.0.0.1:8848/"}
	lines := strings.Split(strings.TrimRight(serveBanner(loopback, ServeOptions{
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
	none := strings.Split(strings.TrimRight(serveBanner(loopback, ServeOptions{
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
	bound, stop, err := Start(dir, PreviewAddr("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = stop }()
	if !strings.HasPrefix(bound.URL, "http://127.0.0.1:") {
		t.Fatalf("URL = %q", bound.URL)
	}
	if strings.HasSuffix(bound.URL, ":0/") {
		t.Fatalf("端口 0 没有被解析成实际端口：%q", bound.URL)
	}
	// Addr 是 net.Listen 报回来的地址，通配绑定照实打印；loopback 绑定不是通配。
	if bound.Addr != strings.TrimSuffix(strings.TrimPrefix(bound.URL, "http://"), "/") {
		t.Errorf("Addr = %q，应等于实际绑定的 host:port（URL %q）", bound.Addr, bound.URL)
	}
	if bound.Browse != "" {
		t.Errorf("loopback 绑定不该多给一条浏览地址：%q", bound.Browse)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
}

// TestServeBannerPrintsRealBindAddr 钉住"打印的是实际绑定的地址"：
// 通配绑定（0.0.0.0）照实打印 host:port，并额外给一条可点击的 loopback 地址；
// 端口 0 时打印内核实际分配的端口，而不是配置里的 0。
func TestServeBannerPrintsRealBindAddr(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "s.jsonl"), `{"t":"msg","role":"user","text":"hi"}`+"\n")
	if !supportsWildcardBind() {
		t.Skip("本机不允许 0.0.0.0 绑定")
	}
	bound, stop, err := Start(dir, PreviewAddr("0.0.0.0", 0))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = stop }()
	// Go 在双栈机器上把 0.0.0.0 的实现选成 [::]（v6only=false），所以照实
	// 打印的可能是 "0.0.0.0:<port>"，也可能是 "[::]:<port>" —— 两者都是"通配绑定"。
	host, port, err := net.SplitHostPort(bound.Addr)
	if err != nil {
		t.Fatalf("Addr = %q，不是 host:port", bound.Addr)
	}
	if host != "0.0.0.0" && host != "::" {
		t.Fatalf("Addr = %q，期望通配地址（0.0.0.0 或 [::]）", bound.Addr)
	}
	if port == "0" {
		t.Fatalf("Addr = %q，端口 0 应被换成内核分配的端口", bound.Addr)
	}
	if bound.Browse != "http://127.0.0.1:"+port+"/" {
		t.Fatalf("Browse = %q，期望 loopback 地址 http://127.0.0.1:%s/", bound.Browse, port)
	}
	out := serveBanner(bound, ServeOptions{
		Root: dir, Addr: "0.0.0.0:0", DirSource: "--dir", AddrSource: "config preview.host/port",
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("通配绑定的横幅应为四行：%q", lines)
	}
	if !strings.Contains(lines[0], bound.Addr) || strings.Contains(lines[0], ":0（") {
		t.Errorf("第一行应打印实际绑定地址 %q：%q", bound.Addr, lines[0])
	}
	if lines[1] != "浏览 http://127.0.0.1:"+port+"/" {
		t.Errorf("第二行应是可点击的 loopback 地址：%q", lines[1])
	}
	if !strings.Contains(lines[3], "监听地址来源 config preview.host/port") {
		t.Errorf("地址来源标签丢了：%q", lines[3])
	}
}

// supportsWildcardBind reports whether binding 0.0.0.0:0 works here (a
// restricted container may refuse it); the wildcard-banner test then skips
// instead of failing for an environment reason.
func supportsWildcardBind() bool {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
