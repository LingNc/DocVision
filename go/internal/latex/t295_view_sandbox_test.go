package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T295：project 视图是"真目录里套软链"（source/ 是真目录，里面的 md 与
// images/figures/tikz 都是软链）。沙箱以前把整个 source/ 一绑了事，里层
// 软链在沙箱里全是死链：`ls /project/source/tikz` 是空的，会话因此以为书里
// 没有矢量图。
func TestSandboxBindsNestedViewLinks(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	view := filepath.Join(proj, "work", "views", "project")
	srcDir := filepath.Join(proj, "source")
	if err := os.MkdirAll(filepath.Join(srcDir, "tikz"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(srcDir, "book.md"), "# t\n")
	writeFile(t, filepath.Join(srcDir, "tikz", "fig.tex"), "\\begin{tikzpicture}\\end{tikzpicture}\n")
	// 图片目录再深一层：images/<项目>/<图>
	imgDir := filepath.Join(root, "images", "书 名 ")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(imgDir, "a.jpg"), "jpeg")
	projImg := filepath.Join(srcDir, "images", "书 名 ")
	if err := os.MkdirAll(projImg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := linkAbs(filepath.Join(imgDir, "a.jpg"), filepath.Join(projImg, "a.jpg")); err != nil {
		t.Fatal(err)
	}

	// 视图：source/ 是真目录，里面 md、tikz、images 都是软链（T290 的形状）。
	if err := os.MkdirAll(filepath.Join(view, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"book.md", "tikz", "images"} {
		if err := linkAbs(filepath.Join(srcDir, name), filepath.Join(view, "source", name)); err != nil {
			t.Fatal(err)
		}
	}

	work := t.TempDir()
	tool := &WorkBashTool{
		Root:   work,
		Mounts: []Mount{{Name: "work", Dir: work, Writable: true}, {Name: "project", Dir: view}},
	}
	args := tool.sandboxArgs("ls /project/source/tikz", false)
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--ro-bind " + filepath.Join(srcDir, "book.md") + " /project/source/book.md",
		"--dir /project/source/tikz",
		"--ro-bind " + filepath.Join(srcDir, "tikz", "fig.tex") + " /project/source/tikz/fig.tex",
		"--dir /project/source/images",
		"--ro-bind " + filepath.Join(imgDir, "a.jpg") + " /project/source/images/书 名 /a.jpg",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("沙箱参数里缺 %q\n实际: %s", want, joined)
		}
	}
}

// 没有软链的普通只读挂载仍然整棵绑定（别把大目录拆成上千条 --ro-bind）。
func TestSandboxPlainReadOnlyMountStaysWhole(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	if err := os.MkdirAll(filepath.Join(plain, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(plain, "a", "x.tex"), "x")
	tool := &WorkBashTool{Root: t.TempDir(), Mounts: []Mount{{Name: "src", Dir: plain}}}
	joined := strings.Join(tool.sandboxArgs("ls", false), " ")
	if !strings.Contains(joined, "--ro-bind "+plain+" /src") {
		t.Fatalf("普通目录应当整棵绑定: %s", joined)
	}
}
