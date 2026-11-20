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
	// API 以 doc-src-en 原文为准（11-20 实跑纠正：Dgrid/Daxes 位置参数，命名表静默不画）；
	// local ld 自带——真实 lualatex 章内无 runner 的 ld 别名（对方会话实跑发现）。
	block2D := `local ld = luadraw
local g = ld.graph:new{window={xmin=-2.2,xmax=2.2,ymin=-1.2,ymax=1.6}, size={10,6}}
g:Dgrid({Z(-2.2,-1.2), Z(2.2,1.6)}, {gridstyle="gray!35, line width=0.3pt"})
g:Daxes({0,0.5,0.5}, {arrows="->", legend={"$x$","$y$"}})
g:Dcartesian(function(x) return math.sin(x) end, {x={-math.pi,math.pi}, draw_options="blue, line width=0.8pt", nbdots=60})
g:Show()`
	// 刻度键名 xlabels/ylabels/zlabels（数值刻度）。图例走 Dlabel3d 手动标注——
	// xlegend 在 3.7 文档在但实渲染静默（上游问题，记入 FAQ，不阻断我们的门禁）。
	block3D := `local ld = luadraw
local g = ld.graph3d:new{viewdir={30,30}, size={10,8}}
g:Dboxaxes3d{grid=true, drawbox=true, xlabels={-2,0,2}, ylabels={-1,0,1}, zlabels={-1,0,1}}
g:Dlabel3d("$z$", M(0,0,3), {})
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
	// 断言防"空图骗过门禁"（11-20 实跑教训：tikzpicture 子串空图也有）——体积+内容双验。
	for _, cs := range []struct {
		name      string
		minBytes  int
		minDraws  int // \\draw 指令最少条数
		mustHave  string
	}{
		{"fig2d", 800, 3, "$x$"},   // 网格+双轴+正弦曲线，轴标签存活
		// 3D 轴名走 Dlabel3d 显式标注（xlegend 在 standalone 预编译下静默不渲染，绕坑务实解）；
		// "$z$" 由 Dlabel3d 保证出现，比刻度值更硬。
		{"fig3d", 1500, 6, "$z$"}, // 网格盒+显式轴标注
	} {
		tkz, err := os.ReadFile(filepath.Join(dir, cs.name+".tkz"))
		if err != nil {
			t.Fatalf("%s 读取失败: %v", cs.name, err)
		}
		if len(tkz) < cs.minBytes {
			t.Fatalf("%s 疑似空图（%d B < %d B）——API 静默不画的老坑: %s", cs.name, len(tkz), cs.minBytes, tkz[:min(120, len(tkz))])
		}
		if strings.Count(string(tkz), "\\draw") < cs.minDraws {
			t.Fatalf("%s \\draw 指令不足（%d < %d）——内容缺失: %.200s", cs.name, strings.Count(string(tkz), "\\draw"), cs.minDraws, tkz)
		}
		if !strings.Contains(string(tkz), cs.mustHave) {
			t.Fatalf("%s 缺预期内容 %q", cs.name, cs.mustHave)
		}
	}
	newSrc, _ := os.ReadFile(filepath.Join(dir, main))
	// 现行内核保留原块为 % 注释（可追溯），故剥注释行后查活跃代码无 luadraw 环境。
	var active []string
	for _, ln := range strings.Split(string(newSrc), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(ln), "%") {
			active = append(active, ln)
		}
	}
	activeSrc := strings.Join(active, "\n")
	if strings.Contains(activeSrc, "begin{luadraw}") ||
		!strings.Contains(activeSrc, "fig2d.tkz") ||
		!strings.Contains(string(newSrc), precompiledMarker) {
		t.Fatalf("源文件应改写为 input+标记（活跃代码无 luadraw 环境）: %q", newSrc[:min(300, len(newSrc))])
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
