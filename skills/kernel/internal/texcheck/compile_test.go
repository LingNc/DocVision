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


// SK-I25：NeedsLuaLaTeX 探测口径——环境块即使用 + 注释行不算使用（幂等标记不失效）。
func TestNeedsLuaLaTeXDetectsEnvBlockAndIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	// 只有 \begin{luadraw} 无 \usepackage → 也要探测到（旧口径漏）
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("前文\n\\begin{luadraw}{name=x}\ncode\n\\end{luadraw}\n"), 0o644)
	if !NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("环境块写法应探测为需要 lualatex（旧口径只认 \\usepackage 会漏）")
	}
	// 预编译产物：活跃代码无 luadraw、原块全在 % 注释里 → 不得再判"需要"（否则幂等失效）
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("% luadraw-precompiled\n\\input{x.tkz}\n% \\begin{luadraw}{name=x}\n% code\n% \\end{luadraw}\n"), 0o644)
	if NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("注释里的 luadraw 块不算使用（预编译后应判不需要，幂等）")
	}
	// 真无 luadraw
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("\\chapter{a}\n正文\n"), 0o644)
	if NeedsLuaLaTeX(dir, "main.tex") {
		t.Fatal("无 luadraw 不应误报")
	}
}
