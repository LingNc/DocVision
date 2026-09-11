package latex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

func keepTestRunner(t *testing.T, probes ...*bool) *Runner {
	t.Helper()
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	log.SetQuiet(true)
	cfg := &config.Config{}
	if len(probes) >= 1 && probes[0] != nil {
		cfg.Latex.KeepTempDirs = probes[0]
	}
	if len(probes) >= 2 && probes[1] != nil {
		cfg.Latex.KeepSessionRecords = probes[1]
	}
	return &Runner{cfg: cfg, log: log}
}

// TestTempDirLivesInProject: temporary workspaces are created INSIDE the
// project (<proj>/work/temp/...) instead of /tmp, so a run can be
// inspected; deletion is the default.
func TestTempDirLivesInProject(t *testing.T) {
	proj := t.TempDir()
	r := keepTestRunner(t)
	dir, cleanup, err := r.tempDir(proj, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(proj, "work", "temp", "chapters"); dir != want {
		t.Fatalf("temp dir = %q, want %q", dir, want)
	}
	if err := os.WriteFile(filepath.Join(dir, "book.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir must be removed by default, stat err = %v", err)
	}
	// fresh dir again, now kept
	keep := true
	r2 := keepTestRunner(t, &keep, nil)
	dir2, cleanup2, err := r2.tempDir(proj, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir2, "book.md"), []byte("x"), 0o644)
	cleanup2()
	if _, err := os.Stat(filepath.Join(dir2, "book.md")); err != nil {
		t.Errorf("keep_temp_dirs must keep the temp dir: %v", err)
	}
}

// TestDebugAlwaysKeeps: debug logging overrides both switches.
func TestDebugAlwaysKeeps(t *testing.T) {
	off := false
	r := keepTestRunner(t, &off, &off)
	r.log.SetLevel(logger.LevelDebug)
	if !r.keepTemp() || !r.keepRecords() {
		t.Fatalf("debug must keep temps and records (keepTemp=%v keepRecords=%v)", r.keepTemp(), r.keepRecords())
	}
	proj := t.TempDir()
	tr := filepath.Join(proj, "work", "sessions", "convert_chapter_01.jsonl")
	if err := os.MkdirAll(filepath.Dir(tr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.keepSessionFile(tr)
	if _, err := os.Stat(tr); err != nil {
		t.Errorf("debug must keep the transcript: %v", err)
	}
	// and without debug the default removes it
	r2 := keepTestRunner(t)
	r2.keepSessionFile(tr)
	if _, err := os.Stat(tr); !os.IsNotExist(err) {
		t.Errorf("transcript must be removed by default, stat err = %v", err)
	}
}

// TestAlignmentProblemsDetectsDrift: when the OCR index and the real PDF
// disagree about a part's page count, the mismatch is reported (global
// page numbers may drift) — part-based lookups stay exact.
func TestAlignmentProblemsDetectsDrift(t *testing.T) {
	view := &pdfView{Files: []pdfViewFile{
		{Name: "book_part1.pdf", Part: "book_part1", First: 1, Count: 200},
		{Name: "book_part2.pdf", Part: "book_part2", First: 201, Count: 200},
	}}
	pages := &pageIndex{total: 396, srcs: []pageSrc{
		{pdf: "/m/book_part1/x_origin.pdf", first: 1, count: 200},
		{pdf: "/m/book_part2/y_origin.pdf", first: 201, count: 196},
	}}
	msgs := view.alignmentProblems(pages)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "book_part2") {
		t.Fatalf("expected one drift report for book_part2, got %v", msgs)
	}
	if _, _, err := view.LocatePart("book_part2", 196); err != nil {
		t.Errorf("part-based lookup must stay exact: %v", err)
	}
	if _, _, err := view.LocatePart("", 1); err == nil {
		t.Errorf("empty part must never match")
	}
}

// TestChapterWorkTreeIsSelfContained: the conversion session's writable
// tree carries everything the chapter needs (class + manual + example as
// COPIES, illustrations, the compile wrapper, and the two paths it will
// submit) — and the real style package is never touched.
func TestChapterWorkTreeIsSelfContained(t *testing.T) {
	proj := t.TempDir()
	style := filepath.Join(proj, "style")
	if err := os.MkdirAll(style, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"mybook.cls", "mybook.sty", "manual.md", "example.tex", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(style, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(proj, "source", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := keepTestRunner(t)
	work, cleanup, err := r.chapterWorkTree(proj, "mybook", "chapter_03", false, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, f := range []string{"mybook.cls", "mybook.sty", "manual.md", "example.tex", "chapter_03_wrapper.tex"} {
		if _, err := os.Stat(filepath.Join(work, f)); err != nil {
			t.Errorf("工作树缺少 %s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(work, "notes.txt")); err == nil {
		t.Errorf("非样式文件不该进工作树")
	}
	if st, err := os.Stat(filepath.Join(work, "chapter_03")); err != nil || !st.IsDir() {
		t.Errorf("待提交的分片文件夹必须已建好: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "images")); err != nil {
		t.Errorf("插图必须可见: %v", err)
	}
	// 副本：改坏工作树里的类不会碰到真包
	if err := os.WriteFile(filepath.Join(work, "mybook.cls"), []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(style, "mybook.cls")); string(data) != "mybook.cls" {
		t.Errorf("真样式包被改动: %q", data)
	}
	// 续跑：上次留下的文件必须还在
	if err := os.WriteFile(filepath.Join(work, "chapter_03.tex"), []byte("half done"), 0o644); err != nil {
		t.Fatal(err)
	}
	work2, cleanup2, err := r.chapterWorkTree(proj, "mybook", "chapter_03", true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	if work2 != work {
		t.Fatalf("续跑必须复用同一棵树: %q vs %q", work2, work)
	}
	if data, _ := os.ReadFile(filepath.Join(work2, "chapter_03.tex")); string(data) != "half done" {
		t.Errorf("续跑丢了上次的产物: %q", data)
	}
}

// TestChapterWorkTreeResolvesRasterImages: raster figures are NOT part of
// the submission — a chapter references them with the very path its
// markdown uses (`images/<书名>/<sha>.jpg`) and the work tree makes that
// path resolve (symlinked images/ + \graphicspath{{figures/}}), so the
// session can really embed and compile them; the book build tree copies
// the same assets, so the reference stays valid to the end.
func TestChapterWorkTreeResolvesRasterImages(t *testing.T) {
	if _, err := exec.LookPath("xelatex"); err != nil {
		t.Skip("xelatex not installed")
	}
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, "style"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "style", "mybook.cls"), []byte(
		"\\NeedsTeXFormat{LaTeX2e}\n\\ProvidesClass{mybook}\n\\LoadClass{article}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 图片实体住在全局 paths.images_dir 下（output/images/<书>/），项目树里
	// 的 `images/…` 由 images 阶段调用 linkProjectImages 铺好——这正是曾经
	// 缺失的一步（项目树 images/ 是空的，章节与样式会话都找不到图）。
	outer := filepath.Join(proj, "output", "images", "测试-概率论")
	if err := os.MkdirAll(outer, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(outer, "abc123.png"), 8, 8)
	if n, missing := linkProjectImages(filepath.Join(proj, "source"), filepath.Join(proj, "output", "images"),
		[]*task{{mdName: "测试-概率论.md", imgPath: "images/测试-概率论/abc123.png"}}); n != 1 || missing != 0 {
		t.Fatalf("images 阶段必须先铺图: linked=%d missing=%d", n, missing)
	}

	r := keepTestRunner(t)
	work, cleanup, err := r.chapterWorkTree(proj, "mybook", "chapter_003", false, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	frag := "\\section{图}\n\\begin{figure}[htbp]\n\\includegraphics[width=0.3\\textwidth]{images/测试-概率论/abc123.png}\n\\caption{示例图}\n\\end{figure}\n"
	if err := os.WriteFile(filepath.Join(work, "chapter_003.tex"), []byte(frag), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &CompileChapterTool{Comp: NewCompiler(config.LatexCompileConfig{Engine: "xelatex", Timeout: 120}),
		Dir: work, MainFile: "chapter_003.tex", WrapperFile: "chapter_003_wrapper.tex"}
	res, err := tool.Execute("{}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "COMPILE OK") {
		t.Fatalf("章节里的 raster 图必须能编译进产物, got: %s", res.Text)
	}
	// 提交只带两个路径：图不进章节文件夹。
	submitRoot := filepath.Join(proj, "work", "chapters")
	if _, err := placeChapterFiles(work, submitRoot, "chapter_003"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_003", "images")); err == nil {
		t.Errorf("raster 图不该被复制进提交的章节文件夹")
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_003.tex")); err != nil {
		t.Errorf("主文件应在: %v", err)
	}
}

// TestSubmitChapterToolPlacesExactlyTwoPaths: submit hands in the main
// .tex + the parts folder and ONLY those two reach the book; the folder
// is replaced wholesale so leftovers from earlier attempts cannot
// survive, and wrong names / escapes / \documentclass are rejected.
func TestSubmitChapterToolPlacesExactlyTwoPaths(t *testing.T) {
	proj := t.TempDir()
	tree := filepath.Join(proj, "work", "temp", "conv_chapter_03", "work")
	submitRoot := filepath.Join(proj, "work", "chapters")
	for _, d := range []string{filepath.Join(tree, "chapter_03"), filepath.Join(submitRoot, "chapter_03")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite := func(rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tree, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("chapter_03.tex", "\\section{real}")
	mustWrite("chapter_03/part1.tex", "part one")
	mustWrite("probe.tex", "probe")
	mustWrite("probe.pdf", "%PDF-1.4")
	mustWrite("chapter_03/stale-probe.tex", "old")
	if err := os.WriteFile(filepath.Join(submitRoot, "chapter_03", "leftover.tex"), []byte("old submission"), 0o644); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(proj, "work", "reports", "chapter_03.md")
	tool := &SubmitChapterTool{WorkRoot: tree, SubmitRoot: submitRoot, Base: "chapter_03", ReportPath: reportPath}

	bad := []struct{ args, want string }{
		{`{"dir":"chapter_03","report":{"status":"pass"}}`, "`path`"},
		{`{"path":"chapter_03.tex","report":{"status":"pass"}}`, "`dir`"},
		{`{"path":"chapter_03.tex","dir":"chapter_03"}`, "`report`"},
		{`{"path":"probe.tex","dir":"chapter_03","report":{"status":"pass"}}`, "must be named chapter_03.tex"},
		{`{"path":"chapter_03.tex","dir":"probe1","report":{"status":"pass"}}`, "must be named chapter_03/"},
		{`{"path":"../chapter_03.tex","dir":"chapter_03","report":{"status":"pass"}}`, "越界"},
		{`{"path":"chapter_03.tex","dir":"sub/chapter_03","report":{"status":"pass"}}`, "NOT FOUND"},
	}
	for _, c := range bad {
		res, err := tool.Execute(c.args)
		if err != nil {
			t.Fatalf("%s: %v", c.args, err)
		}
		if !strings.Contains(res.Text, c.want) {
			t.Errorf("args %s => %q, want %q", c.args, res.Text, c.want)
		}
		if tool.Submitted {
			t.Fatalf("拒绝的提交不能算成功: %s", c.args)
		}
	}
	// \documentclass 的分片必须被拒
	mustWrite("chapter_03.tex", "\\documentclass{book}\n\\begin{document}x\\end{document}")
	if res, _ := tool.Execute(`{"path":"chapter_03.tex","dir":"chapter_03","report":{"status":"pass"}}`); !strings.Contains(res.Text, "\\documentclass") {
		t.Errorf("自带前导的分片必须被拒: %q", res.Text)
	}
	mustWrite("chapter_03.tex", "\\section{real}")
	res, err := tool.Execute(`{"path":"chapter_03.tex","dir":"chapter_03","report":{"status":"issues","issues":"表 3-2 缺行","suggestions":"加 longtable"}}`)
	if err != nil || !tool.Submitted {
		t.Fatalf("合法提交失败: %v %s", err, res.Text)
	}
	if !strings.Contains(res.Text, "SUBMITTED") {
		t.Errorf("回执应确认提交: %q", res.Text)
	}
	placed, err := os.ReadFile(filepath.Join(submitRoot, "chapter_03.tex"))
	if err != nil || string(placed) != "\\section{real}" {
		t.Fatalf("主文件未落盘: %v %q", err, placed)
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_03", "part1.tex")); err != nil {
		t.Errorf("分片未落盘: %v", err)
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_03", "leftover.tex")); err == nil {
		t.Errorf("提交文件夹必须整棵替换（旧残留不该留下）")
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "probe.tex")); err == nil {
		t.Errorf("草稿文件绝不能进书")
	}
	// 分片文件夹本身整棵就是提交内容，里面的文件照交（探针请放文件夹外）。
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_03", "stale-probe.tex")); err != nil {
		t.Errorf("分片文件夹里的文件应当一起提交: %v", err)
	}
	if tool.Status != "issues" || tool.Issues != "表 3-2 缺行" {
		t.Errorf("结论字段未记录: status=%q issues=%q", tool.Status, tool.Issues)
	}
	if report, err := os.ReadFile(reportPath); err != nil || !strings.Contains(string(report), "表 3-2 缺行") {
		t.Errorf("工作汇报未实时落盘: %v %q", err, report)
	}
	// 重复提交：同一章再交一次也必须成功（反馈轮会重交）
	tool2 := &SubmitChapterTool{WorkRoot: tree, SubmitRoot: submitRoot, Base: "chapter_03"}
	if res, err := tool2.Execute(`{"path":"chapter_03.tex","dir":"chapter_03","report":{"status":"pass"}}`); err != nil || !tool2.Submitted {
		t.Fatalf("重交失败: %v %s", err, res.Text)
	}
	if _, err := os.Stat(filepath.Join(submitRoot, "chapter_03.tex")); err != nil {
		t.Errorf("重交后主文件必须仍在: %v", err)
	}
}
