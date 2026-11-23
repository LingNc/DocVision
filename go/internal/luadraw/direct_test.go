package luadraw

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaLaTeXSharedCacheDirectCompile(t *testing.T) {
	if lookup("lualatex") == "" {
		t.Skip("LuaLaTeX is not installed")
	}
	root := projectRoot(t)
	info, err := Probe(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if info["lualatex_usable"] != true {
		t.Skipf("host LuaLaTeX direct route unavailable: %v", info["probe_log"])
	}
	source := filepath.Join(t.TempDir(), "figure.lua")
	if err := os.WriteFile(source, []byte(`local ld=luadraw; local g=ld.graph:new{window={-2,2,-2,2},bbox=false}; g:Dline(-1,1,"red"); g:Show()`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Compile(context.Background(), source, filepath.Join(t.TempDir(), "direct"), map[string]any{"resource_root": root, "helper_path": helperSearch(root), "engine": "lualatex"})
	if err != nil || result["ok"] != true || result["engine"] != "lualatex" {
		t.Fatalf("direct compile: %v %v", err, result)
	}
	pdf, _ := result["pdf"].(string)
	data, err := os.ReadFile(pdf)
	if err != nil || !strings.HasPrefix(string(data), "%PDF") {
		t.Fatalf("missing actual PDF: %v", err)
	}
}
