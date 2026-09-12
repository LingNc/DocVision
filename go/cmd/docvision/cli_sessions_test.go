package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"mineru-tools/internal/config"
	"mineru-tools/internal/sessionview"
)

// sessionsCmdFor 造一个只用来解析 flag 的 sessions 命令（不执行 RunE）。
func sessionsCmdFor(t *testing.T) (*cobra.Command, func(args ...string)) {
	t.Helper()
	cmd := newSessionsCmd()
	parse := func(args ...string) {
		if err := cmd.ParseFlags(args); err != nil {
			t.Fatalf("ParseFlags(%v): %v", args, err)
		}
	}
	return cmd, parse
}

// TestSessionsRootPrefersFlag 钉住 --dir 仍然最优先（相对路径按启动目录解析）。
func TestSessionsRootPrefersFlag(t *testing.T) {
	start := t.TempDir()
	cmd, parse := sessionsCmdFor(t)
	parse("--dir", "sub/dir")
	cfg := &config.Config{}
	cfg.Paths.LatexProject = filepath.Join(start, "from-config")

	root, src := sessionsRoot(cmd, cfg, start)
	if root != filepath.Join(start, "sub", "dir") {
		t.Errorf("root = %q，期望 --dir 解析后的路径", root)
	}
	if src != "--dir" {
		t.Errorf("来源 = %q，期望 --dir", src)
	}
}

// TestSessionsRootComesFromConfig 钉住默认目录确实来自配置（用户报的就是这里：
// 单独跑 sessions --serve 时没用配置），档位2 取 latex_output。
func TestSessionsRootComesFromConfig(t *testing.T) {
	start := t.TempDir()
	project := filepath.Join(start, "latex_project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	level2 := filepath.Join(start, "latex_output")
	if err := os.MkdirAll(level2, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Paths.LatexProject = project
	cfg.Paths.LatexOutput = level2

	cmd, _ := sessionsCmdFor(t)
	root, src := sessionsRoot(cmd, cfg, start)
	if root != project || src != "config paths.latex_project" {
		t.Errorf("档位1 root = %q / %q，期望 %q / config paths.latex_project", root, src, project)
	}

	cfg.Latex.Level = 2
	root, src = sessionsRoot(cmd, cfg, start)
	if root != level2 || src != "config paths.latex_output" {
		t.Errorf("档位2 root = %q / %q，期望 %q / config paths.latex_output", root, src, level2)
	}
}

// TestSessionsRootFallsBackAndSaysWhy 钉住退回当前目录时横幅能说出原因（目录不存在
// 与没有配置是两回事）。
func TestSessionsRootFallsBackAndSaysWhy(t *testing.T) {
	start := t.TempDir()
	cmd, _ := sessionsCmdFor(t)

	cfg := &config.Config{}
	cfg.Paths.LatexProject = filepath.Join(start, "does-not-exist")
	root, src := sessionsRoot(cmd, cfg, start)
	if root != start {
		t.Errorf("配置目录不存在时应退回启动目录，得到 %q", root)
	}
	if !strings.Contains(src, "当前目录") || !strings.Contains(src, "paths.latex_project") {
		t.Errorf("来源 = %q，应说明是当前目录且指出配置项", src)
	}

	root, src = sessionsRoot(cmd, nil, start)
	if root != start || !strings.Contains(src, "未找到 config") {
		t.Errorf("无配置时 root = %q / %q，期望启动目录 + 未找到 config", root, src)
	}
}

// TestSessionsAddrPrecedence 钉住监听地址的优先级：
// --addr > --port > 配置 preview.host/port > 内置默认。
func TestSessionsAddrPrecedence(t *testing.T) {
	cfg := &config.Config{}
	cfg.Preview.Host = "0.0.0.0"
	cfg.Preview.Port = 8848

	// 没有配置：内置默认。
	cmd, _ := sessionsCmdFor(t)
	if addr, src := sessionsAddr(cmd, nil); addr != sessionview.DefaultAddr || src != "内置默认" {
		t.Errorf("无配置 addr = %q / %q，期望内置默认", addr, src)
	}
	// 有配置：preview.host/port。
	if addr, src := sessionsAddr(cmd, cfg); addr != "0.0.0.0:8848" || src != "config preview.host/port" {
		t.Errorf("配置 addr = %q / %q，期望 0.0.0.0:8848 / config preview.host/port", addr, src)
	}
	// --port 覆盖端口，host 仍取配置。
	cmd, parse := sessionsCmdFor(t)
	parse("--port", "9000")
	if addr, src := sessionsAddr(cmd, cfg); addr != "0.0.0.0:9000" || src != "--port" {
		t.Errorf("--port addr = %q / %q，期望 0.0.0.0:9000 / --port", addr, src)
	}
	// --addr 最优先。
	cmd, parse = sessionsCmdFor(t)
	parse("--port", "9000", "--addr", "127.0.0.1:7777")
	if addr, src := sessionsAddr(cmd, cfg); addr != "127.0.0.1:7777" || src != "--addr" {
		t.Errorf("--addr addr = %q / %q，期望 127.0.0.1:7777 / --addr", addr, src)
	}
}

// TestSessionsConfigNote 钉住配置那一行的三种事实：读到哪个文件 / 没找到 /
// 文件在但读不了（三种不能长得一样）。
func TestSessionsConfigNote(t *testing.T) {
	saved := resolvedConfigPath
	defer func() { resolvedConfigPath = saved }()

	resolvedConfigPath = "/tmp/x/config.yaml"
	if got := sessionsConfigNote(&config.Config{}, nil); got != "/tmp/x/config.yaml" {
		t.Errorf("读到配置时 = %q", got)
	}
	if got := sessionsConfigNote(nil, os.ErrNotExist); got != "/tmp/x/config.yaml（读取失败）" {
		t.Errorf("配置读不了时 = %q", got)
	}
	resolvedConfigPath = ""
	if got := sessionsConfigNote(nil, os.ErrNotExist); got != "未找到" {
		t.Errorf("没有配置时 = %q", got)
	}
}

// TestSessionsFlagsRegistered 钉住命令行面：--port 存在、--addr 默认留空
// （默认值由配置决定，flag 默认值不能再写死 8848，否则配置永远排不上）。
func TestSessionsFlagsRegistered(t *testing.T) {
	cmd, _ := sessionsCmdFor(t)
	for _, name := range []string{"dir", "serve", "addr", "port", "list", "out", "cost"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("缺少 --%s", name)
		}
	}
	if got := cmd.Flags().Lookup("addr").DefValue; got != "" {
		t.Errorf("--addr 默认值 = %q，必须留空让配置生效", got)
	}
}
