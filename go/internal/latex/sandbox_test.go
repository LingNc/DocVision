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

// TestSandboxArgsFromMounts pins the sandbox contract: the mount table
// becomes top-level directories inside bubblewrap, writable mounts with
// --bind and the rest with --ro-bind, and a file-level view (symlinks,
// like the original-PDF view) is bound per file.
func TestSandboxArgsFromMounts(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	view := t.TempDir()
	pdf := filepath.Join(t.TempDir(), "real_origin.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(pdf, filepath.Join(view, "book_part1.pdf")); err != nil {
		t.Fatal(err)
	}
	tool := &WorkBashTool{
		Root:   work,
		Mounts: []Mount{{Name: "work", Dir: work, Writable: true}, {Name: "project", Dir: proj}, {Name: "source", Dir: view}},
	}
	args := strings.Join(tool.sandboxArgs("ls -R /work", false), " ")
	for _, want := range []string{
		"--bind " + work + " /work",
		"--ro-bind " + proj + " /project",
		"--ro-bind " + pdf + " /source/book_part1.pdf",
		"--chdir /work",
		"--unshare-net",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("sandbox args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "--bind "+proj) {
		t.Errorf("read-only mount must not be bound writable:\n%s", args)
	}
	if tool.Dir() != work {
		t.Errorf("Dir() = %q, want the work mount %q", tool.Dir(), work)
	}
}

// TestBashSandboxIsolates runs a real command inside the sandbox and
// checks that the workspace is writable, read-only mounts are enforced
// by the kernel and the rest of the host is invisible.
func TestBashSandboxIsolates(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	work := t.TempDir()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "secret.txt"), []byte("project-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	tool := &WorkBashTool{
		Root:    work,
		Mounts:  []Mount{{Name: "work", Dir: work, Writable: true}, {Name: "project", Dir: proj}},
		Sandbox: true, Log: log,
	}
	res, err := tool.Execute(`{"command":"cd /work && echo ok > out.txt && cat /project/secret.txt && echo \"---\"; (echo x > /project/nope 2>&1 | head -1); ls /home 2>&1 | head -1; echo pwd=$(pwd)"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "project-data") {
		t.Errorf("read-only mount must be readable:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "pwd=/work") {
		t.Errorf("cwd must be /work:\n%s", res.Text)
	}
	if _, err := os.Stat(filepath.Join(work, "out.txt")); err != nil {
		t.Errorf("writable mount must accept writes: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "nope")); err == nil {
		t.Errorf("read-only mount must reject writes")
	}
	if !strings.Contains(res.Text, "No such file or directory") && !strings.Contains(res.Text, "没有那个文件或目录") {
		t.Errorf("host paths outside the mounts must not exist:\n%s", res.Text)
	}
	_ = config.Config{}
}

// TestSandboxWritableWorkspaceWithLinks pins the fix for the read-only
// workspace trap: a convert workspace links images/ and figures/ to the
// project's read-only asset trees, and the sandbox used to take that as a
// "view directory" and bind EVERY entry read-only — so in-sandbox editors
// (bash, python) could not touch the session's own .tex files and failed
// with "Read-only file system" while write_file (no sandbox) worked.
func TestSandboxWritableWorkspaceWithLinks(t *testing.T) {
	work := t.TempDir()
	assets := t.TempDir()
	if err := os.WriteFile(filepath.Join(assets, "pic.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(assets, filepath.Join(work, "images")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "chapter_01.tex"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &WorkBashTool{Root: work, Mounts: []Mount{{Name: "work", Dir: work, Writable: true}}}
	args := strings.Join(tool.sandboxArgs("ls", false), " ")
	if !strings.Contains(args, "--bind "+work+" /work") {
		t.Fatalf("a writable workspace must stay a writable bind:\n%s", args)
	}
	if strings.Contains(args, "--ro-bind "+filepath.Join(work, "chapter_01.tex")) {
		t.Fatalf("the session's own files must not be bound read-only:\n%s", args)
	}
	// The link target is bound read-only at its real path so the link
	// resolves inside the sandbox.
	if !strings.Contains(args, "--ro-bind "+assets+" "+assets) {
		t.Fatalf("the symlink target must be bound read-only:\n%s", args)
	}
}

// TestBashSandboxEditsOwnWorkspaceWithLinks is the behavioural half of the
// same contract: a real sandboxed command must be able to overwrite an
// existing workspace file and read through the images/ link.
func TestBashSandboxEditsOwnWorkspaceWithLinks(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	work := t.TempDir()
	assets := t.TempDir()
	if err := os.WriteFile(filepath.Join(assets, "pic.jpg"), []byte("pixels"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(assets, filepath.Join(work, "images")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "chapter_01.tex"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.SetQuiet(true)
	tool := &WorkBashTool{
		Root:    work,
		Mounts:  []Mount{{Name: "work", Dir: work, Writable: true}},
		Sandbox: true, Log: log,
	}
	res, err := tool.Execute(`{"command":"printf new > chapter_01.tex && cat images/pic.jpg && echo && echo done"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "Read-only") || strings.Contains(res.Text, "只读") {
		t.Fatalf("workspace write was rejected by the sandbox:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "pixels") || !strings.Contains(res.Text, "done") {
		t.Fatalf("command did not complete:\n%s", res.Text)
	}
	if b, _ := os.ReadFile(filepath.Join(work, "chapter_01.tex")); string(b) != "new" {
		t.Fatalf("workspace file not updated, got %q", b)
	}
	// The link target must stay read-only.
	res, err = tool.Execute(`{"command":"echo x > images/pic.jpg 2>&1 | head -1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Read-only") && !strings.Contains(res.Text, "只读") {
		t.Fatalf("the asset tree must stay read-only:\n%s", res.Text)
	}
}

// TestProjectViewIsNarrow pins the project-view contract: a session's
// "project" mount contains ONLY source/style/chapters — never the
// internal bookkeeping (work/sessions transcripts, doc_index, pages
// renders, build, out), which is either huge or none of its business.
func TestProjectViewIsNarrow(t *testing.T) {
	proj := t.TempDir()
	for _, d := range []string{"source/images", "style", "chapters", "work/chapters", "work/reports",
		"work/sessions", "pages", "build", "out", "doc_index"} {
		if err := os.MkdirAll(filepath.Join(proj, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"work/sessions/convert_x.jsonl", "pages/p001.png", "doc_index/doc_index.json",
		"out/book.pdf", "source/book.md", "chapters/chapter_01.md", "style/book.cls",
		"work/chapters/chapter_01.tex", "work/reports/chapter_01.md"} {
		if err := os.WriteFile(filepath.Join(proj, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	view := ensureProjectView(proj)
	if view == "" {
		t.Fatal("no view built")
	}
	// Idempotent.
	if again := ensureProjectView(proj); again != view {
		t.Fatalf("view dir changed: %q vs %q", again, view)
	}
	for _, want := range []string{"source/book.md", "chapters/chapter_01.md", "style/book.cls"} {
		if _, err := os.Stat(filepath.Join(view, want)); err != nil {
			t.Errorf("view must expose %s: %v", want, err)
		}
	}
	// The read-only channel to OTHER conversion sessions: their submitted
	// .tex and their work reports — never their transcripts.
	for _, want := range []string{"converted/chapter_01.tex", "reports/chapter_01.md"} {
		if _, err := os.Stat(filepath.Join(view, want)); err != nil {
			t.Errorf("view must expose %s: %v", want, err)
		}
	}
	for _, hidden := range []string{"work", "pages", "build", "out", "doc_index"} {
		if _, err := os.Lstat(filepath.Join(view, hidden)); err == nil {
			t.Errorf("view must NOT expose %s", hidden)
		}
	}
}
