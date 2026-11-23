package latex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSharedLibrarySymlinkKeepsLiteralTargetDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlinks and ldd")
	}
	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	libDir := filepath.Join(physical, "packages", "runtime", "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(physical, "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(libDir, "libfixture.so")
	if err := os.WriteFile(library, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../packages/runtime/lib/libfixture.so", filepath.Join(physical, "lib", "libfixture.so")); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	// ldd names the alias, but its library is a relative symlink through a
	// directory not covered by mounting the alias's lib/ alone.
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' 'libfixture.so => " + filepath.Join(alias, "lib", "libfixture.so") + " (0x0)'\n"
	if err := os.WriteFile(filepath.Join(bin, "ldd"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dirs := sharedLibDirs("fixture-binary")
	for _, want := range []string{filepath.Join(alias, "lib"), filepath.Join(alias, "packages", "runtime", "lib"), libDir} {
		found := false
		for _, dir := range dirs {
			if dir == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing dependency directory %s in %s", want, strings.Join(dirs, ", "))
		}
	}
}
