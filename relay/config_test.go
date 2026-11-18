package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaultsWhenNoFile(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("缺文件应纯默认: %v", err)
	}
	if cfg.Listen != ":8787" || cfg.Repo != "LingNc/DocVision" || cfg.Rate != 5 {
		t.Fatalf("默认不符: %+v", cfg)
	}
}

func TestLoadConfigFileAndEnvPrecedence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "relay.yaml")
	os.WriteFile(p, []byte("listen: \":9999\"\nrepo: \"a/b\"\nrate: 9\n"), 0o600)
	t.Setenv("RELAY_LISTEN", "")   // 清环境干扰
	t.Setenv("RELAY_REPO", "")
	t.Setenv("RELAY_RATE", "")
	cfg, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9999" || cfg.Repo != "a/b" || cfg.Rate != 9 {
		t.Fatalf("文件值未生效: %+v", cfg)
	}
	// 环境变量覆盖文件
	t.Setenv("RELAY_LISTEN", ":1234")
	t.Setenv("RELAY_RATE", "3")
	cfg, err = loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":1234" || cfg.Rate != 3 || cfg.Repo != "a/b" {
		t.Fatalf("环境未覆盖文件: %+v", cfg)
	}
	// 未写的键保留默认
	os.WriteFile(p, []byte("rate: 7\n"), 0o600)
	t.Setenv("RELAY_LISTEN", "")
	t.Setenv("RELAY_RATE", "")
	cfg, err = loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8787" || cfg.Repo != "LingNc/DocVision" || cfg.Rate != 7 {
		t.Fatalf("零值未保留默认: %+v", cfg)
	}
}

func TestLoadConfigBadFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "relay.yaml")
	os.WriteFile(p, []byte("listen: [broken"), 0o600)
	if _, err := loadConfig(p); err == nil {
		t.Fatal("坏文件必须报错，不许静默用默认")
	}
}
