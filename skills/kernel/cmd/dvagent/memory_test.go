package main

import (
	"os"
	"strings"
	"testing"
)

func TestSanitizeMemoryKey(t *testing.T) {
	if _, err := sanitizeMemoryKey("../escape"); err == nil {
		t.Fatal(".. 应被拒绝")
	}
	if _, err := sanitizeMemoryKey("/abs/path"); err == nil {
		t.Fatal("绝对路径应被拒绝")
	}
	if k, err := sanitizeMemoryKey("notes/第一章要点.md"); err != nil || k != "notes/第一章要点.md" {
		t.Fatalf("嵌套 key 应合法: %q %v", k, err)
	}
}

func TestMemoryRoundtrip(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(root+"/memory", 0o755)
	k, n, err := memoryPut(root, "digest/luadraw-3d.md", []byte("中央透视=投影中心+视锥"))
	if err != nil || k != "digest/luadraw-3d.md" || n == 0 {
		t.Fatalf("put: %q %d %v", k, n, err)
	}
	data, err := memoryGet(root, "digest/luadraw-3d.md")
	if err != nil || string(data) != "中央透视=投影中心+视锥" {
		t.Fatalf("get: %q %v", data, err)
	}
	keys, err := memoryList(root, "digest/")
	if err != nil || len(keys) != 1 || keys[0] != "digest/luadraw-3d.md" {
		t.Fatalf("list: %v %v", keys, err)
	}
	if err := memoryRm(root, "digest/luadraw-3d.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := memoryGet(root, "digest/luadraw-3d.md"); err == nil {
		t.Fatal("rm 后 get 应失败")
	}
}

func TestMemoryAtomicReplace(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(root+"/memory", 0o755)
	memoryPut(root, "notes/a.md", []byte("v1"))
	memoryPut(root, "notes/a.md", []byte("v2"))
	data, _ := memoryGet(root, "notes/a.md")
	if string(data) != "v2" {
		t.Fatalf("替换失败: %q", data)
	}
	entries, _ := os.ReadDir(root + "/memory/notes")
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatal("有 .tmp 残留")
		}
	}
}
