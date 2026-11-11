package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T320：图代码顶端的 `\usepackage` / `\standaloneconfig` 是前言行，粘进章节
// 正文会直接编译失败。围栏里只留能进正文的部分；`\usetikzlibrary` 要留着。
func TestStripFigurePreamble(t *testing.T) {
	in := "\\documentclass[tikz]{standalone}\n" +
		"\\usepackage{tikz-3dplot}\\standaloneconfig{border=.5pt}\n" +
		"\\usetikzlibrary{arrows.meta}\n" +
		"\\begin{document}\n" +
		"\\begin{tikzpicture}\n" +
		"\\draw (0,0)--(1,1);\n" +
		"\\end{tikzpicture}\n" +
		"\\end{document}\n"
	got := stripFigurePreamble(in)
	for _, bad := range []string{"\\documentclass", "\\usepackage", "\\standaloneconfig", "\\begin{document}", "\\end{document}"} {
		if strings.Contains(got, bad) {
			t.Fatalf("前言行没去干净（%s）：\n%s", bad, got)
		}
	}
	for _, keep := range []string{"\\usetikzlibrary{arrows.meta}", "\\draw (0,0)--(1,1);"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("正文内容被误删（%s）：\n%s", keep, got)
		}
	}
	if !strings.HasPrefix(got, "\\usetikzlibrary") {
		t.Fatalf("首行应当是库声明：%q", got)
	}
	if stripFigurePreamble("") != "" {
		t.Fatal("空代码应当原样返回空")
	}
}

// 样式阶段要拿到"本书的矢量图需要哪些宏包/库"的清单：转换会话加不了宏包，
// 缺一个就会被逼着重画整张图。
func TestFigurePackageNeeds(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "book")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, code string) {
		p := imageProgress{TikzCode: code}
		data, _ := json.Marshal(p)
		if err := os.WriteFile(filepath.Join(sub, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.json", "\\usepackage{tikz-3dplot}\\usetikzlibrary{arrows.meta,calc}\n\\begin{tikzpicture}\\end{tikzpicture}")
	write("b.json", "\\usepackage{tikz-3dplot}\\usetikzlibrary{arrows.meta}")
	write("c.json", "") // 没代码的图不参与
	needs := figurePackageNeeds(dir)
	if needs.empty() {
		t.Fatal("应当统计出宏包与库")
	}
	joined := strings.Join(needs.Packages, ",")
	if !strings.Contains(joined, "tikz-3dplot (2)") {
		t.Fatalf("包统计不对：%v", needs.Packages)
	}
	lj := strings.Join(needs.Libs, ",")
	if !strings.Contains(lj, "arrows.meta (2)") || !strings.Contains(lj, "calc (1)") {
		t.Fatalf("库统计不对：%v", needs.Libs)
	}
	if figurePackageNeeds(filepath.Join(dir, "nope")).empty() != true {
		t.Fatal("目录不存在时应当返回空")
	}
}

// T321：样式类漏加载图代码要的宏包/库时，转换前置检查要喊出来——否则会话只能
// 编译失败或者背着人重画（3D 图被改成 2D 就是这么来的）。
func TestMissingFigurePackages(t *testing.T) {
	needs := figureNeeds{
		PkgCount: map[string]int{"tikz-3dplot": 18, "amsmath": 3, "bm": 1},
		LibCount: map[string]int{"arrows.meta": 71, "3d": 2},
	}
	cls := `\RequirePackage{tikz}
\RequirePackage{amsmath,amssymb}
\usetikzlibrary{arrows.meta,patterns,calc}
`
	pkgs, libs := missingFigurePackages(cls, needs)
	got := strings.Join(pkgs, ",")
	if strings.Contains(got, "amsmath") {
		t.Fatalf("已加载的 amsmath 不该报缺：%v", pkgs)
	}
	for _, want := range []string{"tikz-3dplot", "bm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("应当报缺 %s：%v", want, pkgs)
		}
	}
	if len(libs) != 1 || !strings.Contains(libs[0], "3d") {
		t.Fatalf("应当只报缺 3d 库：%v", libs)
	}
	// 都加载了就一条都不报。
	ok, okl := missingFigurePackages(cls+"\n\\RequirePackage{tikz-3dplot,bm}\n\\usetikzlibrary{3d}\n", needs)
	if len(ok)+len(okl) != 0 {
		t.Fatalf("全加载齐了还报缺：%v / %v", ok, okl)
	}
}
