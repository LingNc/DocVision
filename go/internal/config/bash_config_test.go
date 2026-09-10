package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCfg writes a minimal config with the given extra YAML body and loads it.
func writeCfg(t *testing.T, extra string) *Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "mineru:\n  token: \"abc\"\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// The bash tool settings live in tools.bash.*; the old latex.bash_* keys
// must keep working (with a migration hint).
func TestBashToolConfigLocation(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := writeCfg(t, "")
		if !cfg.BashSandboxEnabled() {
			t.Error("sandbox should default to enabled")
		}
		if got := cfg.BashMaxOutput(); got != 5000 {
			t.Errorf("max output default = %d, want 5000", got)
		}
		if cfg.UsesDeprecatedBashKeys() {
			t.Error("no deprecated keys were written")
		}
	})

	t.Run("tools.bash is authoritative", func(t *testing.T) {
		cfg := writeCfg(t, "tools:\n  bash:\n    sandbox: false\n    max_output: 900\n")
		if cfg.BashSandboxEnabled() {
			t.Error("tools.bash.sandbox: false should disable the sandbox")
		}
		if got := cfg.BashMaxOutput(); got != 900 {
			t.Errorf("max output = %d, want 900", got)
		}
	})

	t.Run("tools.bash wins over deprecated latex keys", func(t *testing.T) {
		cfg := writeCfg(t, "tools:\n  bash:\n    sandbox: false\n    max_output: 900\nlatex:\n  bash_sandbox: true\n  bash_max_output: 100\n")
		if cfg.BashSandboxEnabled() {
			t.Error("tools.bash.sandbox should win over latex.bash_sandbox")
		}
		if got := cfg.BashMaxOutput(); got != 900 {
			t.Errorf("max output = %d, want 900 (tools.bash wins)", got)
		}
		if !cfg.UsesDeprecatedBashKeys() {
			t.Error("deprecated keys were used and should be reported")
		}
	})

	t.Run("deprecated latex keys still honoured", func(t *testing.T) {
		cfg := writeCfg(t, "latex:\n  bash_sandbox: false\n  bash_max_output: 1234\n")
		if cfg.BashSandboxEnabled() {
			t.Error("latex.bash_sandbox: false should still disable the sandbox")
		}
		if got := cfg.BashMaxOutput(); got != 1234 {
			t.Errorf("max output = %d, want 1234 (legacy key)", got)
		}
		if !cfg.UsesDeprecatedBashKeys() {
			t.Error("legacy usage should be flagged for a migration hint")
		}
	})

	t.Run("negative max output rejected in both locations", func(t *testing.T) {
		dir := t.TempDir()
		for name, body := range map[string]string{
			"tools": "mineru:\n  token: \"abc\"\ntools:\n  bash:\n    max_output: -1\n",
			"latex": "mineru:\n  token: \"abc\"\nlatex:\n  bash_max_output: -1\n",
		} {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Errorf("%s: negative max_output should be rejected", name)
			}
		}
	})
}
