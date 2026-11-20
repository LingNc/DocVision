package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWorkflowDestDir 钉 T333：配置了 paths.finally_dir 就交到那儿（相对路径
// 按当前工作目录解析，因为 workflow 之后会 chdir 进 job 目录），没配才退回
// 源文件所在目录。
func TestWorkflowDestDir(t *testing.T) {
	src := t.TempDir()

	got, err := workflowDestDir("", src)
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Fatalf("未配置时应回落源目录，got %q want %q", got, src)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err = workflowDestDir("out/final", src)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(wd, "out", "final"); got != want {
		t.Fatalf("相对 finally_dir 应按工作目录解析，got %q want %q", got, want)
	}

	abs := filepath.Join(src, "交付")
	got, err = workflowDestDir(abs, src)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Fatalf("绝对 finally_dir 应原样使用，got %q want %q", got, abs)
	}
}
