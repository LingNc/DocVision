package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
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
