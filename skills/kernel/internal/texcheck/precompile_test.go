package texcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 离线断言：环境块正则与 name 提取。
func TestLuadrawEnvRe(t *testing.T) {
	src := "前文\n\\begin{luadraw}{name=fig1, exec=true}\nlocal g = ld.graph3d:new{}\n\\end{luadraw}\n后文\n"
	m := luadrawEnvRe.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("应匹配 luadraw 环境块")
	}
	if nm := nameOptRe.FindStringSubmatch(m[1]); nm == nil || nm[1] != "fig1" {
		t.Fatalf("块名应取 name= 选项: %q", m[1])
	}
	if !strings.Contains(m[2], "graph3d:new") {
		t.Fatalf("块体应含绘图代码: %q", m[2])
	}
}

// 实测：2D 与 3D 两个块预编译出 .tkz、源文件改写、幂等。
// 用例改编自 lingnc-memory/toolchain/luadraw/bin/user-2d.lua 与 user-complex.lua
// （那边的 2D/3D 压力测试是边界基准：complex 3502 行产物过 tectonic），
// 这里走内核完整路径（提取→lua 子进程→改写）钉住集成正确性。
func TestPrecompileLuaDrawLive(t *testing.T) {
	if LuaAvailable() == "" {
		t.Skip("无 lua 解释器")
	}
	rel := filepath.Join("..", "..", "..", "docs", "luadraw", "files")
	filesDir, err := filepath.Abs(rel) // runner 在临时目录里跑，必须绝对路径
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filesDir); err != nil {
		t.Skip("luadraw 模块目录不在（非仓库环境）")
	}
	block2D := `local g = ld.graph:new{window={xmin=-2.2,xmax=2.2,ymin=-1.2,ymax=1.6}, size={10,6}}
g:Dgrid{stepx=0.5, stepy=0.5, style="gray!35, line width=0.3pt"}
g:Daxes{xmin=-2.2, xmax=2.2, ymin=-1.2, ymax=1.6, style="->", labelx="$x$", labely="$y$"}
g:Dcartesian(sin, {xmin=-math.pi, xmax=math.pi, step=0.05, style="blue, line width=0.8pt", plot="smooth"})
g:Show()`
	block3D := `local g = ld.graph3d:new{viewdir={30,30}, size={10,8}}
g:Dboxaxes3d{grid=true, xstyle="->", ystyle="->", zstyle="->", labelx="$x$", labely="$y$", labelz="$z$"}
g:Show()`
	dir := t.TempDir()
	main := "main.tex"
	src := "前文\n" +
		"\\begin{luadraw}{name=fig2d, exec=true}\n" + block2D + "\n\\end{luadraw}\n" +
		"\\begin{luadraw}{name=fig3d}\n" + block3D + "\n\\end{luadraw}\n" +
		"后文\n"
	os.WriteFile(filepath.Join(dir, main), []byte(src), 0o644)
	changed, produced, err := PrecompileLuaDraw(dir, main, filesDir)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 || produced != 2 {
		t.Fatalf("应改写 1 文件产 2 块: %d %d", changed, produced)
	}
	for _, name := range []string{"fig2d", "fig3d"} {
		tkz, err := os.ReadFile(filepath.Join(dir, name+".tkz"))
		if err != nil || !strings.Contains(string(tkz), "tikzpicture") {
			t.Fatalf("%s 应产出完整 tikzpicture: %v", name, err)
		}
	}
	newSrc, _ := os.ReadFile(filepath.Join(dir, main))
	if strings.Contains(string(newSrc), "begin{luadraw}") ||
		!strings.Contains(string(newSrc), "fig2d.tkz") ||
		!strings.Contains(string(newSrc), precompiledMarker) {
		t.Fatalf("源文件应改写为 input+标记: %q", newSrc)
	}
	c2, p2, err := PrecompileLuaDraw(dir, main, filesDir)
	if err != nil || c2 != 0 || p2 != 0 {
		t.Fatalf("幂等: %d %d %v", c2, p2, err)
	}
}

// DecideEngine：无 luadraw 的章直返空引擎。
func TestDecideEngineNoLuaDraw(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.tex"), []byte("\\documentclass{article}\\begin{document}x\\end{document}"), 0o644)
	e, pre, err := DecideEngine(dir, "main.tex", t.TempDir())
	if err != nil || e != "" || pre {
		t.Fatalf("无 luadraw 应直返: %q %v %v", e, pre, err)
	}
}
