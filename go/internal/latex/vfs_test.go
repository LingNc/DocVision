package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVFSResolve covers the virtual workspace mount table: default
// mount, explicit name:path / /name/path addressing, write rights and
// traversal rejection.
func TestVFSResolve(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	v := &VFS{Mounts: []Mount{
		{Name: "work", Dir: work, Writable: true},
		{Name: "project", Dir: proj},
	}}

	full, label, err := v.Resolve("chapters/ch1.tex", true)
	if err != nil {
		t.Fatal(err)
	}
	if label != "work" || full != filepath.Join(work, "chapters/ch1.tex") {
		t.Fatalf("plain path = %q (%s)", full, label)
	}

	for _, p := range []string{"project:chapters/a.md", "/project/chapters/a.md"} {
		full, label, err = v.Resolve(p, false)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", p, err)
		}
		if label != "project" || full != filepath.Join(proj, "chapters/a.md") {
			t.Errorf("Resolve(%q) = %q (%s)", p, full, label)
		}
	}

	if _, _, err := v.Resolve("project:x.md", true); err == nil {
		t.Error("writing to a read-only mount must fail")
	}
	if _, _, err := v.Resolve("nope:x.md", false); err == nil || !strings.Contains(err.Error(), "未知挂载点") {
		t.Errorf("unknown mount error = %v", err)
	}
	if _, _, err := v.Resolve("../escape", false); err == nil {
		t.Error("traversal must be rejected")
	}
}

// TestReadFileToolMountSyntax: the unified read tool understands the
// mount table, so one namespace covers the workspace and the project.
func TestReadFileToolMountSyntax(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "ch1.tex"), []byte("own"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "ch1.md"), []byte("# original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &ReadFileTool{Mounts: []Mount{
		{Name: "work", Dir: work, Writable: true},
		{Name: "project", Dir: proj},
	}}
	res, err := tool.Execute(`{"path":"project:ch1.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "project:ch1.md") || !strings.Contains(res.Text, "# original") {
		t.Fatalf("mount read = %q", res.Text)
	}
	if _, err := tool.Execute(`{"path":"/project/missing.md"}`); err == nil {
		t.Error("missing file under an explicit mount must be an error")
	}
}

// TestGrepToolAltRoots: grep searches the read-only roots too and labels
// their hits.
func TestGrepToolAltRoots(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "a.tex"), []byte("needle in work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "b.md"), []byte("needle in project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := &GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: proj}}}
	res, err := tool.Execute(`{"pattern":"needle"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "a.tex:1:needle in work") {
		t.Errorf("workspace hit missing: %q", res.Text)
	}
	if !strings.Contains(res.Text, "project/b.md:1:needle in project") {
		t.Errorf("project hit missing: %q", res.Text)
	}
}
