package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// writeFile is a tiny helper: create parent dirs and write content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newProjectTestRunner builds a Runner with a real (file-backed) logger so
// tests can assert the decision lines that reach the log.
func newProjectTestRunner(t *testing.T, logPath string, cfg *config.Config) *Runner {
	t.Helper()
	log, err := logger.NewLogger(logPath, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	log.SetQuiet(true)
	if cfg == nil {
		cfg = &config.Config{}
	}
	return NewRunner(cfg, log)
}

// TestResolveProjectDirNewLayout: 空的输出根 → <root>/<主题名>/ 才是工作区
// （多项目布局）。把这条分支改成"直接返回 root"必须让本测试红。
func TestResolveProjectDirNewLayout(t *testing.T) {
	root := t.TempDir()
	lay, err := resolveProjectDir(root, "", "测试-概率论")
	if err != nil {
		t.Fatal(err)
	}
	if lay.Legacy {
		t.Fatalf("空目录不应判成旧版单项目布局: %+v", lay)
	}
	if want := filepath.Join(root, "测试-概率论"); lay.Dir != want {
		t.Fatalf("Dir = %q, want %q", lay.Dir, want)
	}
	if lay.Name != "测试-概率论" {
		t.Fatalf("Name = %q", lay.Name)
	}
	if !strings.Contains(lay.Note, lay.Dir) {
		t.Fatalf("Note 应说明工作区路径: %q", lay.Note)
	}
}

// TestResolveProjectDirLegacyRootKept: 输出根本身已经是工程（含 work/ 或
// source/ 或 progress.json 等）→ 继续沿用根目录（旧行为，绝不搬迁）。
func TestResolveProjectDirLegacyRootKept(t *testing.T) {
	for _, marker := range []string{"work", "source", "progress.json", "doc_index", "progress_items", "chapters"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			if strings.HasSuffix(marker, ".json") {
				writeFile(t, filepath.Join(root, marker), "{}")
			} else if err := os.MkdirAll(filepath.Join(root, marker), 0o755); err != nil {
				t.Fatal(err)
			}
			lay, err := resolveProjectDir(root, "", "测试-概率论")
			if err != nil {
				t.Fatal(err)
			}
			if !lay.Legacy {
				t.Fatalf("含 %s/ 的输出根应判成旧版单项目布局: %+v", marker, lay)
			}
			if lay.Dir != root {
				t.Fatalf("Dir = %q, want root %q", lay.Dir, root)
			}
			// 必须明确提示"沿用旧目录"以及新项目会去哪 —— 否则用户不知道
			// 怎么在同一目录下开第二本书。
			if !strings.Contains(lay.Note, "旧版单项目布局") || !strings.Contains(lay.Note, "--project") {
				t.Fatalf("Note 缺少沿用/新建提示: %q", lay.Note)
			}
			if !strings.Contains(lay.Note, filepath.Join(root, "<项目名>")) {
				t.Fatalf("Note 未说明新项目位置: %q", lay.Note)
			}
		})
	}
}

// TestProjectFlagWinsOverLegacyRoot: 显式 --project 是"在旧工程所在的输出根
// 下新开一本书"的出口——即使 root 命中旧布局标记也必须进子目录。
func TestProjectFlagWinsOverLegacyRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "progress.json"), "{}")

	lay, err := resolveProjectDir(root, "线性代数", "测试-概率论")
	if err != nil {
		t.Fatal(err)
	}
	if lay.Legacy {
		t.Fatalf("显式 --project 时不得沿用旧布局: %+v", lay)
	}
	if want := filepath.Join(root, "线性代数"); lay.Dir != want {
		t.Fatalf("Dir = %q, want %q", lay.Dir, want)
	}
	if !strings.Contains(lay.Note, "--project") {
		t.Fatalf("Note 应说明是显式指定: %q", lay.Note)
	}
}

// TestResolveProjectDirSameBookStable: 同一本书重复运行必须落到同一个工作区
// （断点续传/幂等），换一本书用同一个名字才去重到 -2。
func TestResolveProjectDirSameBookStable(t *testing.T) {
	root := t.TempDir()

	// 第一本书：显式名字 + 写入标记（模拟运行一次）
	lay1, err := resolveProjectDir(root, "练习册", "书A")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lay1.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeProjectMarker(lay1, "书A", 1); err != nil {
		t.Fatal(err)
	}

	// 同一本书再来一次 → 同一目录
	again, err := resolveProjectDir(root, "练习册", "书A")
	if err != nil {
		t.Fatal(err)
	}
	if again.Dir != lay1.Dir {
		t.Fatalf("同一本书重复运行换了目录: %q != %q", again.Dir, lay1.Dir)
	}

	// 另一本书抢同一个名字 → 退到 <名字>-2，绝不混进别人的工作区
	other, err := resolveProjectDir(root, "练习册", "书B")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "练习册-2"); other.Dir != want {
		t.Fatalf("同名项目的另一本书应退到 %q，实际 %q", want, other.Dir)
	}
	// 手工建的空目录（无标记）不参与去重：直接复用
	if err := os.MkdirAll(filepath.Join(root, "手工目录"), 0o755); err != nil {
		t.Fatal(err)
	}
	manual, err := resolveProjectDir(root, "手工目录", "书C")
	if err != nil {
		t.Fatal(err)
	}
	if manual.Dir != filepath.Join(root, "手工目录") {
		t.Fatalf("无标记目录应被复用，实际 %q", manual.Dir)
	}
}

// TestSanitizeProjectName: 项目名必须安全化——保留中文，去掉路径分隔符、
// 控制字符、首尾空白与点，禁止 "."/".."/空串/保留字。
func TestSanitizeProjectName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"测试-概率论", "测试-概率论"},
		{"  线性代数  ", "线性代数"},
		{"a/b", "a-b"},
		{`a\b`, "a-b"},
		{"../etc/passwd", "etc-passwd"},
		{"..", ""},
		{".", ""},
		{"", ""},
		{"   ", ""},
		{"...", ""},
		{"../../etc/passwd", "etc-passwd"},
		{"a..b", "a-b"},
		{"书\x00名", "书名"},
		{"解<析>册", "解-析-册"},
		{"name.", "name"},
		{strings.Repeat("长", 200), strings.Repeat("长", projectNameMaxRunes)},
	}
	for _, c := range cases {
		got := SanitizeProjectName(c.in)
		if got != c.want {
			t.Errorf("SanitizeProjectName(%q) = %q, want %q", c.in, got, c.want)
		}
		if strings.ContainsAny(got, `/\`) || got == "." || got == ".." {
			t.Errorf("SanitizeProjectName(%q) = %q 仍是危险名", c.in, got)
		}
	}
	// 穿越写法不得在名字里留下 ".." 残留，也不得以 '-'/'.' 开头
	// （否则看起来像路径残留，或被当成命令行旗标）。
	for _, in := range []string{"../../etc/passwd", "..", ".", "-flag", "...", "../.."} {
		got := SanitizeProjectName(in)
		if strings.Contains(got, "..") || strings.ContainsAny(got, `/\`) {
			t.Errorf("SanitizeProjectName(%q) = %q 仍含路径残留", in, got)
		}
		if got != "" && (strings.HasPrefix(got, "-") || strings.HasPrefix(got, ".")) {
			t.Errorf("SanitizeProjectName(%q) = %q 不应以 - 或 . 开头", in, got)
		}
	}
	if got := SanitizeProjectName("../../etc/passwd"); got != "etc-passwd" {
		t.Errorf("SanitizeProjectName(../../etc/passwd) = %q, want etc-passwd", got)
	}
	if got := SanitizeProjectName("-flag"); got != "flag" {
		t.Errorf("SanitizeProjectName(-flag) = %q, want flag", got)
	}
}

// TestResolveProjectDirRejectsUnsafeNames: 非法/保留的 --project 值不能逃出
// 输出根，也不能造出与旧布局标记同名的目录（否则下次运行会把输出根误判
// 成旧工程，两本书又混在一起）。
func TestResolveProjectDirRejectsUnsafeNames(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"..", ".", "   ", "work", "source", "progress.json", "doc_index", "out"} {
		lay, err := resolveProjectDir(root, bad, "测试-概率论")
		if err != nil {
			t.Fatalf("want=%q: %v", bad, err)
		}
		if lay.Legacy {
			t.Fatalf("want=%q 落到了旧布局: %+v", bad, lay)
		}
		if lay.Dir == root {
			t.Fatalf("want=%q 逃到了输出根本身", bad)
		}
		if rel, err := filepath.Rel(root, lay.Dir); err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("want=%q 越出输出根: %q (%v)", bad, lay.Dir, err)
		}
		for _, reserved := range legacyProjectEntries {
			if lay.Name == reserved {
				t.Fatalf("want=%q 产生了保留名目录 %q（会污染旧布局判定）", bad, lay.Name)
			}
		}
		// 无可用名字时退回主题名
		if want := filepath.Join(root, "测试-概率论"); lay.Dir != want {
			t.Fatalf("want=%q: Dir = %q, want %q", bad, lay.Dir, want)
		}
	}

	// 空项目名 + 空主题名 → 兜底名字
	lay, err := resolveProjectDir(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if lay.Name != defaultProjectName {
		t.Fatalf("兜底项目名 = %q, want %q", lay.Name, defaultProjectName)
	}

	// 路径穿越写法被安全化后仍在根内
	trav, err := resolveProjectDir(root, "../../escape", "书")
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(root, trav.Dir); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("穿越名逃出输出根: %q", trav.Dir)
	}
}

// TestUseProjectDirLogsDecision: 旧布局兼容与显式 --project 都要在日志里
// 留下可检索的一行（用户据此知道数据落在哪）。
func TestUseProjectDirLogsDecision(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
			t.Fatal(err)
		}
		logPath := filepath.Join(t.TempDir(), "latex_test.log")
		r := newProjectTestRunner(t, logPath, nil)
		dir, err := r.useProjectDir(root, "", "测试-概率论", 1)
		if err != nil {
			t.Fatal(err)
		}
		if dir != root || r.ProjectDir() != root {
			t.Fatalf("旧布局应沿用 %q，得到 %q / %q", root, dir, r.ProjectDir())
		}
		// 旧工程一个字节都不动：不写项目标记
		if _, err := os.Stat(filepath.Join(root, projectMarkerFile)); err == nil {
			t.Fatalf("旧布局下不应写入 %s", projectMarkerFile)
		}
		body, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "旧版单项目布局") || !strings.Contains(string(body), "--project") {
			t.Fatalf("日志缺少沿用提示:\n%s", body)
		}
	})

	t.Run("new", func(t *testing.T) {
		root := t.TempDir()
		logPath := filepath.Join(t.TempDir(), "latex_test.log")
		r := newProjectTestRunner(t, logPath, nil)
		dir, err := r.useProjectDir(root, "", "测试-概率论", 2)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(root, "测试-概率论")
		if dir != want || r.ProjectDir() != want {
			t.Fatalf("新布局工作区 = %q / %q, want %q", dir, r.ProjectDir(), want)
		}
		if !dirExists(want) {
			t.Fatalf("工作区目录未创建: %q", want)
		}
		// 标记写入（同名去重依据）
		if owner := projectOwnerOf(want); owner != "测试-概率论" {
			t.Fatalf("项目标记 = %q, want 主题名", owner)
		}
		body, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "项目工作区") {
			t.Fatalf("日志缺少工作区提示:\n%s", body)
		}
	})
}

// TestProjectSourceName: 默认项目名 = 主题名（主 markdown 主文件名，与
// images/<主题>/ 一致），中文与连字符原样保留。
func TestProjectSourceName(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "测试-概率论.md"), "small")
	writeFile(t, filepath.Join(src, "另一本-习题册.md"), strings.Repeat("x", 100))
	if got := projectSourceName(src, nil); got != "另一本-习题册" {
		t.Fatalf("批量模式应取最大 md 的主题名，得到 %q", got)
	}
	if got := projectSourceName(src, []string{"测试-概率论.md"}); got != "测试-概率论" {
		t.Fatalf("单选应取该文件的主题名，得到 %q", got)
	}
	if got := projectSourceName(src, []string{"/abs/path/线性代数.md"}); got != "线性代数" {
		t.Fatalf("路径参数应取文件名，得到 %q", got)
	}
	if got := projectSourceName(t.TempDir(), nil); got != "" {
		t.Fatalf("无 md 时应返回空串，得到 %q", got)
	}
}

// TestRunImagesUsesProjectWorkspace: 档位2 独立运行的产物目录必须是
// <latex_output>/<项目名>/（不再直接写 latex_output 根），旧版布局则沿用根。
func TestRunImagesUsesProjectWorkspace(t *testing.T) {
	run := func(t *testing.T, legacy bool, project string) (root, outDir string) {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		root = t.TempDir()
		out := filepath.Join(root, "output")
		writeFile(t, filepath.Join(out, "测试-概率论.md"), "# 标题\n\n没有图片的正文\n")
		if legacy {
			if err := os.MkdirAll(filepath.Join(root, "finally_latex", "progress_items"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		logPath := filepath.Join(t.TempDir(), "latex_test.log")
		cfg := &config.Config{}
		cfg.Paths.OutputDir = out
		cfg.Paths.LatexOutput = filepath.Join(root, "finally_latex")
		cfg.Paths.LatexProject = filepath.Join(root, "latex_project")
		cfg.Latex.Concurrency = 1
		r := newProjectTestRunner(t, logPath, cfg)
		if err := r.RunImages(ImagesOptions{Project: project, Verbose: false}); err != nil {
			t.Fatalf("RunImages: %v", err)
		}
		return root, r.ProjectDir()
	}

	t.Run("new layout", func(t *testing.T) {
		root, outDir := run(t, false, "")
		if want := filepath.Join(root, "finally_latex", "测试-概率论"); outDir != want {
			t.Fatalf("档位2 工作区 = %q, want %q", outDir, want)
		}
		for _, sub := range []string{"figures", "images", "tikz", "progress_items"} {
			if !dirExists(filepath.Join(outDir, sub)) {
				t.Fatalf("工作区缺少 %s/（产物没落在项目目录里）", sub)
			}
		}
		// 输出根只应有项目子目录，不能有散落的 images/figures/progress_items
		for _, stray := range []string{"images", "figures", "tikz", "progress_items"} {
			if dirExists(filepath.Join(root, "finally_latex", stray)) {
				t.Fatalf("输出根 %s/ 被档位2 直接写到（多项目布局失效）", stray)
			}
		}
	})

	t.Run("--project override", func(t *testing.T) {
		root, outDir := run(t, false, "线性代数")
		if want := filepath.Join(root, "finally_latex", "线性代数"); outDir != want {
			t.Fatalf("--project 未生效: %q, want %q", outDir, want)
		}
	})

	t.Run("legacy root", func(t *testing.T) {
		root, outDir := run(t, true, "")
		if want := filepath.Join(root, "finally_latex"); outDir != want {
			t.Fatalf("旧版档位2 布局应沿用输出根 %q，得到 %q", want, outDir)
		}
		if !dirExists(filepath.Join(outDir, "figures")) {
			t.Fatalf("旧布局下仍应在根下建 figures/")
		}
	})
}

// TestResolveExistingProjectDir (verify): 核对要能定位到唯一的项目；有多个
// 项目且未指定 --project 时必须报错而不是猜。
func TestResolveExistingProjectDir(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"书A", "书B"} {
		writeFile(t, filepath.Join(root, name, "progress_items", "x.json"), "{}")
	}
	if _, err := resolveExistingProjectDir(root, ""); err == nil {
		t.Fatal("多个项目且未指定 --project 时应报错")
	} else if !strings.Contains(err.Error(), "书A") || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("错误信息应列出项目并提示 --project: %v", err)
	}
	lay, err := resolveExistingProjectDir(root, "书B")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "书B"); lay.Dir != want {
		t.Fatalf("--project 定位 = %q, want %q", lay.Dir, want)
	}
	if _, err := resolveExistingProjectDir(root, "不存在的书"); err == nil {
		t.Fatal("指定不存在的项目应报错")
	}

	// 唯一项目：自动选中
	only := t.TempDir()
	writeFile(t, filepath.Join(only, "测试-概率论", "progress_items", "x.json"), "{}")
	lay, err = resolveExistingProjectDir(only, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(only, "测试-概率论"); lay.Dir != want {
		t.Fatalf("唯一项目应被自动选中，得到 %q", lay.Dir)
	}

	// 旧版布局：就是根目录本身
	legacy := t.TempDir()
	if err := os.MkdirAll(filepath.Join(legacy, "progress_items"), 0o755); err != nil {
		t.Fatal(err)
	}
	lay, err = resolveExistingProjectDir(legacy, "")
	if err != nil {
		t.Fatal(err)
	}
	if !lay.Legacy || lay.Dir != legacy {
		t.Fatalf("旧版布局应沿用根目录: %+v", lay)
	}
}
