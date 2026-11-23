package luadraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixture2DSHA = "93706afa92a6ccbcce4fc4f67268ed49e50a472dfc68366404c2d65cdf7e4dab"
	fixture3DSHA = "af411326f1ed64fab6a6d3ab113199ada4a45d66aeb54873b692a1e13db58547"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func verifyFixtureSHA(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	got := hex.EncodeToString(digest[:])
	if got != want {
		t.Fatalf("fixture SHA mismatch for %s: got %s want %s", path, got, want)
	}
}

func compileComplexFixture(t *testing.T, name, engine, wantSHA string) map[string]any {
	t.Helper()
	source := fixturePath(t, name)
	verifyFixtureSHA(t, source, wantSHA)
	if engine == "lualatex" {
		if _, err := exec.LookPath("lualatex"); err != nil {
			t.Skipf("required external engine lualatex is unavailable: %v", err)
		}
	}
	root := projectRoot(t)
	result, err := Compile(context.Background(), source, t.TempDir(), map[string]any{
		"resource_root":   root,
		"helper_path":     helperSearch(root),
		"engine":          engine,
		"compile_timeout": 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true {
		t.Fatalf("%s fixture did not compile with %s: %v", name, engine, result["error"])
	}
	if engine == "fallback" {
		used := result["engine"]
		if used != "xelatex" && used != "pdflatex" {
			t.Fatalf("%s fixture used unexpected fallback engine %v", name, used)
		}
		if result["backend"] != "standalone-lua+generated-TikZ" {
			t.Fatalf("%s fixture used unexpected fallback backend %v", name, result["backend"])
		}
	} else if result["engine"] != "lualatex" || result["backend"] != "luadraw.sty" {
		t.Fatalf("%s fixture used unexpected direct result: engine=%v backend=%v", name, result["engine"], result["backend"])
	}
	tikz, ok := result["tikz"].(string)
	if !ok || tikz == "" {
		t.Fatalf("%s fixture returned no generated TikZ: %#v", name, result["tikz"])
	}
	body, err := os.ReadFile(tikz)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 1000 || !strings.Contains(string(body), "tikzpicture") || !strings.Contains(string(body), "\\draw") {
		t.Fatalf("%s fixture generated suspiciously small TikZ (%d bytes)", name, len(body))
	}
	pdf, _ := result["pdf"].(string)
	pdfData, pdfErr := os.ReadFile(pdf)
	if pdfErr != nil || len(pdfData) < 512 || !strings.HasPrefix(string(pdfData), "%PDF") {
		t.Fatalf("missing valid PDF: %v", pdfErr)
	}
	if os.Getenv("COMPARE_FIGURE_FULL_LUADRAW_TEST") == "1" {
		pdf, _ := result["pdf"].(string)
		if _, err := os.Stat(pdf); err != nil {
			t.Fatal(err)
		}
		if _, err := exec.LookPath("pdftoppm"); err != nil {
			t.Skipf("required preview tool pdftoppm is unavailable: %v", err)
		}
		base := filepath.Join(filepath.Dir(pdf), name+"-preview")
		cmd := exec.Command("pdftoppm", "-png", "-f", "1", "-singlefile", pdf, base)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("render PNG preview: %v: %s", err, out)
		}
		t.Logf("%s PDF: %s; PNG: %s.png; TikZ: %s", name, pdf, base, tikz)
	}
	return result
}

func TestLuaDrawComplex2D(t *testing.T) {
	for _, engine := range []string{"lualatex", "fallback"} {
		t.Run(engine, func(t *testing.T) {
			result := compileComplexFixture(t, "luadraw-test-2d.lua", engine, fixture2DSHA)
			body, err := os.ReadFile(result["tikz"].(string))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"decorate", "pattern", "mark="} {
				if !strings.Contains(string(body), marker) {
					t.Fatalf("2D output lost coverage marker %q", marker)
				}
			}
		})
	}
}

func TestLuaDrawComplex3D(t *testing.T) {
	for _, engine := range []string{"lualatex", "fallback"} {
		t.Run(engine, func(t *testing.T) {
			result := compileComplexFixture(t, "luadraw-test-3d.lua", engine, fixture3DSHA)
			body, err := os.ReadFile(result["tikz"].(string))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"\\pgfsetfillcolor{rgb", "Stealth", "A(2,0", "B(2,0"} {
				if !strings.Contains(string(body), marker) {
					t.Fatalf("3D output lost coverage marker %q", marker)
				}
			}
		})
	}
}
