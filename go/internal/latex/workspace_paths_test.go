package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 视图软链必须存**绝对**目标：配置里的路径是相对的（"./mineru_output"），
// 而内核按"链接所在目录"解析相对目标 → 悬空 → 会话报"文件不存在:
// source:<part>.pdf"，沙箱里 /source、/project 是空目录。
func TestLinkAbsStoresAbsoluteTarget(t *testing.T) {
	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()

	// 真实文件用**相对**路径（模拟 paths.mineru_output 的默认值）
	if err := os.MkdirAll("mineru_output/book_part1", 0o755); err != nil {
		t.Fatal(err)
	}
	pdf := filepath.Join("mineru_output", "book_part1", "uuid_origin.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	viewDir := filepath.Join("latex_project", "work", "pdfview")
	if err := os.MkdirAll(viewDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 反面：裸 os.Symlink 存相对目标 → 悬空
	naive := filepath.Join(viewDir, "naive.pdf")
	if err := os.Symlink(pdf, naive); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(naive); err == nil {
		t.Fatalf("裸 os.Symlink 竟然可用，测试前提不成立")
	}

	// 正面：linkAbs 可用，且重复调用是幂等的
	good := filepath.Join(viewDir, "book_part1.pdf")
	for i := 0; i < 2; i++ {
		if err := linkAbs(pdf, good); err != nil {
			t.Fatalf("linkAbs 第 %d 次失败: %v", i+1, err)
		}
	}
	if _, err := os.Stat(good); err != nil {
		t.Fatalf("linkAbs 生成的链接不可用: %v", err)
	}
	target, err := os.Readlink(good)
	if err != nil || !filepath.IsAbs(target) {
		t.Fatalf("链接目标应为绝对路径，实际 %q (%v)", target, err)
	}
}

// 目录软链（project 窄视图）同样必须绝对。
func TestLinkAbsDirectoryTarget(t *testing.T) {
	root := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := os.MkdirAll(filepath.Join("latex_project", "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("latex_project", "source", "book.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	viewDir := filepath.Join("latex_project", "work", "views", "project")
	if err := os.MkdirAll(viewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(viewDir, "source")
	if err := linkAbs(filepath.Join("latex_project", "source"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(link, "book.md")); err != nil {
		t.Fatalf("project 视图里的 source/ 不可用: %v", err)
	}
	// 已存在真实目录时不得覆盖
	real := filepath.Join(viewDir, "style")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := linkAbs(filepath.Join("latex_project", "source"), real); err == nil {
		t.Errorf("真实目录不应被软链覆盖")
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		t.Errorf("真实目录被破坏了: %v", err)
	}
}

// 会话把两种路径写法混在一起（"source:/source/x.pdf"）时应能解析，
// 而不是报"路径越界（绝对路径）"。
func TestVFSResolveToleratesRedundantMountPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "book_part1.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := &VFS{Mounts: []Mount{{Name: "work", Dir: t.TempDir(), Writable: true}, {Name: "source", Dir: dir}}}
	for _, p := range []string{"source:book_part1.pdf", "/source/book_part1.pdf", "source:/source/book_part1.pdf", "/source/source/book_part1.pdf"} {
		full, label, err := v.Resolve(p, false)
		if err != nil {
			t.Errorf("Resolve(%q) 失败: %v", p, err)
			continue
		}
		if label != "source" || filepath.Base(full) != "book_part1.pdf" {
			t.Errorf("Resolve(%q) = %q (%s)", p, full, label)
		}
	}
}

// 找不到文件时要给出邻居列表，否则模型会连续几轮猜名字。
func TestSuggestNearListsSiblings(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"book_part1.pdf", "book_part2.pdf", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := suggestNear(filepath.Join(dir, "book_part9.pdf"), 12)
	for _, want := range []string{"book_part1.pdf", "book_part2.pdf", "notes.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("提示 %q 缺少 %s", got, want)
		}
	}
	// 目录不存在时回退到最近的已存在祖先
	got = suggestNear(filepath.Join(dir, "missing", "deep", "x.pdf"), 12)
	if !strings.Contains(got, "notes.txt") {
		t.Errorf("应回退到最近存在的祖先目录，实际 %q", got)
	}
	if s := suggestNear(filepath.Join(t.TempDir(), "x.pdf"), 12); !strings.Contains(s, "空") {
		t.Errorf("空目录提示 = %q", s)
	}
}

// 沙箱 bind 的源路径必须绝对：bwrap 自己解析源路径（不看我们的 CWD），
// 默认配置给的是 "./latex_project" 这类相对路径 → 每个会话的 bash 直接
// 以 "Can't find source path" 失败。
func TestSandboxArgsUsesAbsoluteSources(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "proj", "work")
	src := filepath.Join(root, "proj", "source")
	for _, d := range []string{work, src} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	relWork, err := filepath.Rel(mustGetwd(t), work)
	if err != nil {
		t.Skip("无法构造相对路径")
	}
	relSrc, err := filepath.Rel(mustGetwd(t), src)
	if err != nil {
		t.Skip("无法构造相对路径")
	}
	tool := &WorkBashTool{Mounts: []Mount{
		{Name: "work", Dir: relWork, Writable: true},
		{Name: "project", Dir: relSrc},
	}}
	args := tool.sandboxArgs("echo hi", false)
	// 相对形式绝不能出现；且每个 --bind/--ro-bind 的源都必须是绝对路径
	for i, a := range args {
		if a == relWork || a == relSrc {
			t.Errorf("参数 %d 传入了非绝对源路径: %q", i, a)
		}
	}
	for i, a := range args {
		if (a == "--bind" || a == "--ro-bind") && i+1 < len(args) {
			if !filepath.IsAbs(args[i+1]) {
				t.Errorf("%s 的源不是绝对路径: %q", a, args[i+1])
			}
		}
	}
	if !containsArg(args, "--chdir") {
		t.Fatalf("缺少 --chdir: %v", args)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
