package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// 现场缺陷复现：md 里写的是 `images/测试-概率论/<sha>.jpg`（相对 md 自身），
// 图片实体却只在全局 paths.images_dir（output/images/测试-概率论/）下，项目
// source 树里的 images/ 是空的 → 样式会话 view_image 拿着 md 里的路径直接
// 报"文件不存在"，章节里的栅格图也无处可寻。
func TestProjectImagesMaterializedNextToMarkdown(t *testing.T) {
	root := t.TempDir()
	imagesDir := filepath.Join(root, "output", "images")
	sub := filepath.Join(imagesDir, "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "b6f6f41eff8b4ee7f04e713f8e9db7dd4010f68b9f58beba4923d0f340ed5550.jpg"
	if err := os.WriteFile(filepath.Join(sub, name), []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "proj", "source") // 档位1：项目 source 树
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 真实形状：矢量图转换失败 → 状态 fallback、原引用留在 md 里，而
	// embedBlock 的嵌入分支从不会被调用（copyOriginalImage 因此漏掉它）。
	tasks := []*task{{
		mdName:  "测试-概率论.md",
		imgPath: "images/测试-概率论/" + name,
	}}

	n, missing := linkProjectImages(outDir, imagesDir, tasks)
	if n != 1 || missing != 0 {
		t.Fatalf("linked=%d missing=%d, want 1/0", n, missing)
	}
	rel := filepath.Join(outDir, "images", "测试-概率论", name)
	if !fileExists(rel) {
		t.Fatalf("项目树里必须有这张图: %s", rel)
	}
	// 内容必须真能读到（symlink 或 copy 都行）。
	data, err := os.ReadFile(rel)
	if err != nil || string(data) != "jpg-bytes" {
		t.Fatalf("读图失败: %v %q", err, data)
	}
	// 幂等：再跑一次不重复、不报错，也不把已有条目换掉。
	before, err := os.Lstat(rel)
	if err != nil {
		t.Fatal(err)
	}
	n2, _ := linkProjectImages(outDir, imagesDir, tasks)
	if n2 != 0 {
		t.Fatalf("第二次应该什么都不做，linked=%d", n2)
	}
	after, err := os.Lstat(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("已有条目被重建了")
	}
	// 源文件缺失：计数出来但不 panic（图片阶段会重试/保留原图）。
	n3, missing3 := linkProjectImages(outDir, imagesDir, []*task{{
		mdName: "测试-概率论.md", imgPath: "images/测试-概率论/nope.jpg",
	}})
	if n3 != 0 || missing3 != 1 {
		t.Fatalf("缺失源文件应记 missing=1，got linked=%d missing=%d", n3, missing3)
	}
}

// assemble 的 copyDir 用 filepath.Walk（不跟随目录软链）复制 source/images：
// 必须确认铺进去的条目能被复制成 build 树里的真文件。
func TestProjectImagesSurviveAssembleCopy(t *testing.T) {
	root := t.TempDir()
	imagesDir := filepath.Join(root, "output", "images")
	name := "fig.png"
	if err := os.MkdirAll(filepath.Join(imagesDir, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imagesDir, "book", name), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "proj", "source")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if n, _ := linkProjectImages(outDir, imagesDir, []*task{{
		mdName: "book.md", imgPath: "images/book/" + name,
	}}); n != 1 {
		t.Fatalf("linked=%d", n)
	}
	build := filepath.Join(root, "proj", "build")
	if err := copyDir(filepath.Join(outDir, "images"), filepath.Join(build, "images")); err != nil {
		t.Fatalf("copyDir 必须先铺图后可用: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(build, "images", "book", name))
	if err != nil || string(got) != "png" {
		t.Fatalf("build 树里的图不对: %v %q", err, got)
	}
	// build 树里必须是真文件（不是指向项目树的链接）。
	fi, err := os.Lstat(filepath.Join(build, "images", "book", name))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Error("build 树里应是复制的真文件")
	}
}

// 样式会话的 view_image：路径按 md 里的写法解析（Root=项目 source，
// Subject="images"），找不到时要把目录里真实的文件名讲清楚。
func TestViewImageResolvesProjectPathAndHints(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "images", "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "b6f6f41eff8b4ee7f04e713f8e9db7dd4010f68b9f58beba4923d0f340ed5550.jpg"
	if err := os.WriteFile(filepath.Join(sub, name), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ViewImageTool{Root: root, Subject: "images", SoftMax: 100, WarnRatio: 0.5}
	got, err := tool.resolve("images/测试-概率论/" + name)
	if err != nil {
		t.Fatalf("md 里的引用必须能解析: %v", err)
	}
	if got != filepath.Join(sub, name) {
		t.Fatalf("resolve = %q", got)
	}
	// 写错文件名：错误信息里要有真实文件与"直接给这个路径"的提示。
	_, err = tool.resolve("images/测试-概率论/deadbeef.jpg")
	if err == nil {
		t.Fatal("错名必须报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "文件不存在") {
		t.Fatalf("错误信息 = %q", msg)
	}
	if !strings.Contains(msg, "测试-概率论") || !strings.Contains(msg, "实际有") {
		t.Fatalf("错误信息要列出真实目录内容: %q", msg)
	}
	// 同名文件出现在子目录里 → 直接给出可用路径。
	_, err = tool.resolve("deadbeef/" + name)
	if err == nil {
		t.Fatal("错目录必须报错")
	}
	if !strings.Contains(err.Error(), "images/测试-概率论/"+name) {
		t.Fatalf("同名文件的提示 = %q", err.Error())
	}
	// 目录根本不存在时也不能崩。
	empty := &ViewImageTool{Root: filepath.Join(root, "nope"), Subject: "images"}
	if _, err := empty.resolve("images/x.jpg"); err == nil {
		t.Fatal("不存在的根目录应报错")
	}
}

// 配置项 pip_index_url 传给宿主侧 pip（-i）——宿主 pip.conf 不可控时用它。
func TestPythonEnvPipIndexURL(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "pip.log")
	shim := filepath.Join(dir, "python3")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\nif [ \"$1\" = \"-c\" ]; then exit 1; fi\nexit 0\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := NewPythonEnv(config.ToolsPythonConfig{
		Mode: "system", Interpreter: shim,
		PipIndexURL: "https://mirrors.aliyun.com/pypi/simple",
	}, filepath.Join(dir, "report"), nil)
	_ = env.NoteFor("ModuleNotFoundError: No module named 'PIL'\n")
	calls, _ := os.ReadFile(log)
	txt := string(calls)
	if !strings.Contains(txt, "-i https://mirrors.aliyun.com/pypi/simple") {
		t.Fatalf("必须带上显式镜像，got:\n%s", txt)
	}
	if !strings.Contains(txt, "install --user -i https://mirrors.aliyun.com/pypi/simple Pillow") {
		t.Fatalf("镜像 + 映射后的包名都要在, got:\n%s", txt)
	}
}

// 现场缺陷（2026-09-11 20:29 那次运行）：配置里 paths.images_dir 是**相对**
// 路径 `./output/images`，而 os.Symlink 会原样保存目标串、内核按"软链所在
// 目录"解析——于是项目树里落下一批死链：
//
//	<proj>/source/images/测试-概率论/<sha>.jpg -> output/images/测试-概率论/<sha>.jpg
//	（实际被解析成 <proj>/source/images/测试-概率论/output/images/…）
//
// 后果：rebuildPhase 里 copyOriginalImage 复制失败 → 7 条 DOCVISION-VECTOR
// 注释连同 LINK 一起消失、STYLED-TEXT 只剩注释没有 LINK、doc_index 里对应
// 条目的 text 为空；随后 assemble 复制 source/images 时以
// "open …: no such file or directory" 中止整本书。
//
// 老测试用的是**绝对** imagesDir，所以一路绿灯；这里必须用相对路径复现。
func TestProjectImagesLinkWithRelativeImagesDirIsReadable(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)                     // 真实运行里进程 cwd 就是 config.yaml 所在目录
	const relImages = "output/images" // == config 默认值 ./output/images
	name := "19c107f60583a9f492543a1907d5767586b4813c59cc5bb9b29c2cb5e8010b65.jpg"
	sub := filepath.Join(relImages, "测试-概率论")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, name), []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join("latex_project", "测试-概率论", "source")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := []*task{{mdName: "测试-概率论.md", imgPath: "images/测试-概率论/" + name}}
	if n, missing := linkProjectImages(outDir, relImages, tasks); n != 1 || missing != 0 {
		t.Fatalf("linked=%d missing=%d, want 1/0", n, missing)
	}
	dst := filepath.Join(outDir, "images", "测试-概率论", name)
	if !pathExists(dst) {
		t.Fatalf("相对 images_dir 铺出来的条目必须能读到（不能是死链）: %s -> %q", dst, readLink(t, dst))
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "jpg-bytes" {
		t.Fatalf("读图失败: %v %q", err, data)
	}
	// 目标必须是绝对路径：绝对路径才与"软链所在目录"无关。
	if target := readLink(t, dst); target != "" && !filepath.IsAbs(target) {
		t.Errorf("软链目标应为绝对路径，got %q", target)
	}
}

// 死链必须能被修好：旧实现的跳过条件是 `pathExists(dst) || isSymlink(dst)`，
// 死链被当成"已铺好"永久跳过，源路径改对了也不会自愈。
func TestLinkProjectImagesRepairsDanglingLink(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	const relImages = "output/images"
	name := "fig.jpg"
	if err := os.MkdirAll(filepath.Join(relImages, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relImages, "book", name), []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join("proj", "source")
	dst := filepath.Join(outDir, "images", "book", name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	// 模拟上一次运行留下的死链（相对目标）。
	if err := os.Symlink(filepath.Join(relImages, "book", name), dst); err != nil {
		t.Fatal(err)
	}
	if pathExists(dst) {
		t.Fatal("前置条件：这必须是一条死链")
	}
	n, missing := linkProjectImages(outDir, relImages, []*task{{
		mdName: "book.md", imgPath: "images/book/" + name,
	}})
	if n != 1 || missing != 0 {
		t.Fatalf("死链应被修好并计入 linked: linked=%d missing=%d", n, missing)
	}
	if !pathExists(dst) {
		t.Fatalf("修复后仍读不到: %s -> %q", dst, readLink(t, dst))
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "real" {
		t.Fatalf("修复后内容不对: %v %q", err, data)
	}
	// 已就位的条目不该被动：再跑一次 linked=0。
	if n2, _ := linkProjectImages(outDir, relImages, []*task{{
		mdName: "book.md", imgPath: "images/book/" + name,
	}}); n2 != 0 {
		t.Fatalf("已就位条目被重建了: linked=%d", n2)
	}
}

// repairProjectImageLinks：只跑 assemble 的场景（或上一次死在 assemble 的项目）
// 也必须能自愈死链，否则整本书继续以 "no such file or directory" 中止。
func TestRepairProjectImageLinks(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	const relImages = "output/images"
	name := "keep.png"
	if err := os.MkdirAll(filepath.Join(relImages, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relImages, "book", name), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join("latex_project", "book")
	dst := filepath.Join(proj, "source", "images", "book", name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(relImages, "book", name), dst); err != nil {
		t.Fatal(err)
	}
	// 一张真正找不到源的图：只警告，不能崩。
	ghost := filepath.Join(proj, "source", "images", "book", "ghost.png")
	if err := os.Symlink(filepath.Join(relImages, "book", "ghost.png"), ghost); err != nil {
		t.Fatal(err)
	}
	// 一个真文件也不能被动。
	real := filepath.Join(proj, "source", "figures", "fig.pdf")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("pdf"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := testRunnerWithImages(t, relImages)
	if fixed := r.repairProjectImageLinks(proj); fixed != 1 {
		t.Fatalf("只应修好 1 条死链，got %d", fixed)
	}
	if !pathExists(dst) {
		t.Fatalf("死链没修好: %s", dst)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "png" {
		t.Fatalf("修复后内容不对: %v %q", err, data)
	}
	if got, err := os.ReadFile(real); err != nil || string(got) != "pdf" {
		t.Fatalf("真文件被动了: %v %q", err, got)
	}
}

// assemble 复制 source/images：断链只跳过并上报，绝不中止（跑了一刻钟的
// assemble 不该被一张取不到的图毁掉）。
func TestCopyDirReportSkipsDanglingLink(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "book"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "book", "ok.png"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("output/images/book/gone.png", filepath.Join(src, "book", "gone.png")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	skipped, err := copyDirReport(src, dst)
	if err != nil {
		t.Fatalf("断链不该让复制失败: %v", err)
	}
	if len(skipped) != 1 || !strings.HasSuffix(skipped[0], "gone.png") {
		t.Fatalf("应上报 1 条跳过，got %v", skipped)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "book", "ok.png")); err != nil || string(data) != "ok" {
		t.Fatalf("好图必须照拷: %v %q", err, data)
	}
}

// 原图搬不进项目树时，注释与 LINK 必须留下（曾经整条 DOCVISION-VECTOR
// 注释被静默丢掉，导致 doc_index 里 text 为空、下游查不到是哪张图）。
func TestVectorNoteSurvivesMissingOriginal(t *testing.T) {
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	log.SetQuiet(true)
	t.Cleanup(func() { _ = log.Close() })
	r := &Runner{cfg: &config.Config{Paths: config.PathsConfig{ImagesDir: t.TempDir()}},
		log: log, inline: true}
	// 源图不存在：copyOriginalImage 必然失败。
	p := &imageProgress{
		Class: ClassVector, ImgPath: "images/测试-概率论/deadbeef.jpg",
		TikzCode: "\\begin{tikzpicture}\\end{tikzpicture}", Label: "Venn diagram",
	}
	got := r.embedBlock(p, "测试-概率论.md", t.TempDir())
	if !strings.Contains(got, "DOCVISION-VECTOR") {
		t.Fatalf("注释必须保留，got:\n%s", got)
	}
	if !strings.Contains(got, "原图未就位") {
		t.Fatalf("必须标明原图没就位，got:\n%s", got)
	}
	if !strings.Contains(got, "tikzpicture") {
		t.Fatalf("TikZ 代码必须保留，got:\n%s", got)
	}
}

func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return target
}
