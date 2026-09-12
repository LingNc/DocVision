package sessionview

import (
	"strings"
	"testing"
)

// 监听地址的**网络族**选择：Go 的 net.Listen("tcp", "0.0.0.0:8849") 遇到通配
// host 会优先拿 IPv6 双栈套接字，内核回报 "[::]:8849"，于是用户按配置写了
// 0.0.0.0、横幅却显示 [::] —— 配置没错，是地址族挑错了。
// 规则：写死的字面 host 说了算（0.0.0.0 → tcp4、:: → tcp6），host 留空才交给
// 双栈。空的 host（":8849"）与 loopback 一律 "tcp"。
func TestListenNetworkFollowsLiteralHost(t *testing.T) {
	cases := map[string]string{
		"0.0.0.0:8849": "tcp4",
		"0.0.0.0:0":    "tcp4",
		"[::]:8849":    "tcp6",
		"[::1]:8849":   "tcp", // IPv6 loopback 用 tcp 即可，只有 :: 需要点明 tcp6
		"127.0.0.1:1":  "tcp",
		"localhost:1":  "tcp",
		":8849":        "tcp",
		"8849":         "tcp",
	}
	for addr, want := range cases {
		if got := listenNetwork(addr); got != want {
			t.Errorf("listenNetwork(%q) = %q，期望 %q", addr, got, want)
		}
	}
}

// 端到端：写 0.0.0.0 就必须**绑在 IPv4 通配**上，横幅照实打印 "0.0.0.0:<port>"。
func TestStartKeepsConfiguredWildcard(t *testing.T) {
	root := t.TempDir()
	bound, _, err := Start(root, "0.0.0.0:0")
	if err != nil {
		t.Skipf("本机不能绑通配地址：%v", err)
	}
	if !strings.HasPrefix(bound.Addr, "0.0.0.0:") {
		t.Fatalf("Addr = %q，期望以 0.0.0.0: 开头（配置写了 0.0.0.0）", bound.Addr)
	}
	if !strings.HasPrefix(bound.Browse, "http://127.0.0.1:") {
		t.Errorf("通配绑定的 Browse = %q，期望给出 loopback 浏览地址", bound.Browse)
	}
}
