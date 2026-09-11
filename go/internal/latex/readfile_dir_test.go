package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// read_file 把目录当文件读时，原先只把 os.ReadFile 的 EISDIR 原样丢回去
// （"read latex_project/…/work: is a directory"）——既没说是哪个引用、也
// 没说里面有什么。现场（2026-09-11 21:17）style-fix 会话正是这样被卡住
// 一轮，用户看到的是"文件不可读取"。现在必须回报目录里的真实条目。
func TestReadFileOnDirectoryListsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "chapter_002.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "parts"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFileTool{Root: root}
	_, err := tool.Execute(`{"path":"."}`)
	if err == nil {
		t.Fatal("读目录必须报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "是目录不是文件") || !strings.Contains(msg, "chapter_002.tex") {
		t.Fatalf("目录错误必须列出真实条目，got: %s", msg)
	}
	if strings.Contains(msg, "is a directory") {
		t.Fatalf("不该把 EISDIR 原样丢给模型: %s", msg)
	}
}

// read_file 明确点到子目录时同样要列内容（现场是 read_file {"path":"work:"}）。
func TestReadFileOnExplicitSubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "a.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFileTool{Root: root}
	_, err := tool.Execute(`{"path":"sub"}`)
	if err == nil || !strings.Contains(err.Error(), "a.tex") {
		t.Fatalf("读子目录必须列出内容，got: %v", err)
	}
}
