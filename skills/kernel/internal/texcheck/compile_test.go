package texcheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsLuaLaTeX(t *testing.T) {
	dir := t.TempDir()
	// 主文件直接用 luadraw
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("\\documentclass{article}\n\\usepackage[3d]{luadraw}\n"), 0o644)
	if !NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("主文件 luadraw 应探测到")
	}
	// 无 luadraw
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("\\documentclass{article}\n\\usepackage{tikz}\n"), 0o644)
	if NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("无 luadraw 不应误报")
	}
	// 同目录被 \\input 的文件用了 luadraw（主文件本身没有）
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("\\input{figs}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "figs.tex"), []byte("\\usepackage{luadraw}\n"), 0o644)
	if !NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("同目录子文件 luadraw 应探测到（按需引擎的章级判据）")
	}
}
