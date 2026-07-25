package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
