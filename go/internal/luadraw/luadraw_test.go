package luadraw

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := Resources()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestLuaQuoteUnicodePath(t *testing.T) {
	got := luaQuote("a/图.lua")
	if got != `"a/\229\155\190.lua"` {
		t.Fatalf("unexpected Lua quote: %s", got)
	}
}

func TestDocsSearchReadViewAndWarmCache(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	cache := t.TempDir()
	out := t.TempDir()
	d, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	text, img, e := d.Execute(ctx, map[string]any{"mode": "search", "query": "Regardless of global options chosen package loads", "limit": 4}, out)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(text, "index_sha256") || !strings.Contains(text, "luadraw-doc-en.pdf") {
		t.Fatalf("expected official manual hit, got %s", text)
	}
	if img != "" {
		t.Fatal("query-only search should not attach a PDF crop by default")
	}
	symbolText, symbolImage, e := d.Execute(ctx, map[string]any{"mode": "search", "query": "Dpolyline3d", "limit": 1}, out)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(symbolText, "Dpolyline3d(L") || !strings.Contains(symbolText, "draw_options") {
		t.Fatalf("exact symbol search must lead with its complete API signature: %s", symbolText)
	}
	boxMatch := regexp.MustCompile(`bbox_pts=([0-9.]+),([0-9.]+),([0-9.]+),([0-9.]+)`).FindStringSubmatch(symbolText)
	if len(boxMatch) != 5 {
		t.Fatalf("expected a real PDF bbox, got %s", symbolText)
	}
	var coords [4]float64
	for i := range coords {
		coords[i], e = strconv.ParseFloat(boxMatch[i+1], 64)
		if e != nil {
			t.Fatal(e)
		}
	}
	if coords[2] <= coords[0] || coords[3] <= coords[1] {
		t.Fatalf("bbox must have positive area, got %v", coords)
	}
	if symbolImage != "" {
		t.Fatal("exact PDF hit should remain text-only by default")
	}
	text, symbolImage, e = d.Execute(ctx, map[string]any{"mode": "search", "query": "Dpolyline3d", "limit": 1, "include_image": true}, out)
	if e != nil || symbolImage == "" {
		t.Fatalf("explicit PDF crop failed: %v %s", e, text)
	}
	if _, e = os.Stat(symbolImage); e != nil {
		t.Fatalf("explicit symbol crop missing: %v", e)
	}
	src := "doc/src/luadraw-doc-en.tex"
	text, img, e = d.Execute(ctx, map[string]any{"mode": "read", "doc": src, "start_line": 1, "end_line": 3}, out)
	if e != nil || img != "" || !strings.Contains(text, "lines 1-3/") {
		t.Fatalf("read failed: %v %s", e, text)
	}
	text, img, e = d.Execute(ctx, map[string]any{"mode": "read", "doc": "files/luadraw_graph3d.lua", "start_line": 1, "end_line": 2}, out)
	if e != nil || !strings.Contains(text, "files/luadraw_graph3d.lua lines 1-2/") || img != "" {
		t.Fatalf("direct source read failed: %v %s", e, text)
	}
	list, _, e := d.Execute(ctx, map[string]any{"mode": "list", "doc": "files"}, out)
	if e != nil || !strings.Contains(list, "files/luadraw_graph3d.lua") {
		t.Fatalf("list failed: %v %s", e, list)
	}
	found, _, e := d.Execute(ctx, map[string]any{"mode": "find", "query": "graph3d.lua"}, out)
	if e != nil || !strings.Contains(found, "files/luadraw_graph3d.lua") {
		t.Fatalf("find failed: %v %s", e, found)
	}
	freq, e := d.FrequentSnippet(ctx, 8, 256, 2048)
	entries, freqErr := d.frequencySnapshot(d.index.Key)
	foundRead := false
	for _, entry := range entries {
		if entry.Doc == "files/luadraw_graph3d.lua" && entry.Source == "read" {
			foundRead = true
		}
	}
	if e != nil || freqErr != nil || freq == "" || !foundRead {
		t.Fatalf("frequency cache failed or omitted the explicitly read source entry: %v %v %s", e, freqErr, freq)
	}
	text, img, e = d.Execute(ctx, map[string]any{"mode": "view", "doc": "doc/luadraw-doc-en.pdf", "page": 1, "left": 10, "top": 15, "right": 85, "bottom": 90, "zoom": 400}, out)
	if e != nil || img == "" || !strings.Contains(text, "page 1/") {
		t.Fatalf("view failed: %v %s", e, text)
	}
	viewEntries, e := d.frequencySnapshot(d.index.Key)
	if e != nil {
		t.Fatal(e)
	}
	viewRecorded := false
	for _, entry := range viewEntries {
		if entry.Source == "view" && entry.Doc == "doc/luadraw-doc-en.pdf#page=1" {
			viewRecorded = true
			break
		}
	}
	if !viewRecorded {
		t.Fatal("explicit PDF view did not record the displayed page segments")
	}
	indexPath := filepath.Join(cache, "luadraw-doc-index-v4.json")
	before, e := os.Stat(indexPath)
	if e != nil {
		t.Fatal(e)
	}
	d2, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = d2.Execute(ctx, map[string]any{"mode": "search", "query": "LuaDraw", "include_image": false}, out)
	if e != nil {
		t.Fatal(e)
	}
	after, e := os.Stat(indexPath)
	if e != nil {
		t.Fatal(e)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("warm index unexpectedly rewritten")
	}
}
func TestSearchMergesAdjacentPDFBlocksWithoutCrossColumnCollapse(t *testing.T) {
	b1, b2, b3, dup := bbox{10, 10, 100, 20}, bbox{12, 22, 105, 32}, bbox{300, 10, 390, 20}, bbox{10, 10, 100, 20}
	idx := &docIndex{Docs: []docEntry{{Path: "doc/test.pdf", Kind: "pdf", Pages: []page{{Number: 1, Width: 400, Height: 500, Blocks: []block{{Text: "API first", BBox: b1}, {Text: "API second", BBox: b2}, {Text: "API column", BBox: b3}, {Text: "API first", BBox: dup}}}}}}}
	d := &Docs{}
	hits := d.search(idx, "API", 10, 0)
	if len(hits) != 2 {
		t.Fatalf("expected adjacent merge plus separate column, got %d: %#v", len(hits), hits)
	}
	if !strings.Contains(hits[0].text, "API first") || !strings.Contains(hits[0].text, "API second") {
		t.Fatalf("adjacent blocks not merged: %#v", hits[0])
	}
	if hits[0].box[3] < b2[3] {
		t.Fatalf("merged bbox did not expand: %#v", hits[0].box)
	}
}

func TestDocsPathBoundary(t *testing.T) {
	d, e := NewDocs(projectRoot(t), t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "../../etc/passwd"}, t.TempDir())
	if e == nil {
		t.Fatal("expected path boundary rejection")
	}
}
func TestDocsIndexInvalidationAndSymlinkBoundary(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "doc", "src")
	if e := os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(base, "notes.tex")
	if e := os.WriteFile(source, []byte("before\n"), 0600); e != nil {
		t.Fatal(e)
	}
	cache := t.TempDir()
	d, e := NewDocs(root, cache)
	if e != nil {
		t.Fatal(e)
	}
	first, _, e := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/notes.tex"}, t.TempDir())
	if e != nil || !strings.Contains(first, "before") {
		t.Fatalf("initial index failed: %v", e)
	}
	if e = os.WriteFile(source, []byte("before\nafter\n"), 0600); e != nil {
		t.Fatal(e)
	}
	future := time.Now().Add(2 * time.Second)
	if e = os.Chtimes(source, future, future); e != nil {
		t.Fatal(e)
	}
	second, _, e := d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, t.TempDir())
	if e != nil || !strings.Contains(second, "2: after") {
		t.Fatalf("stale source index: %v %s", e, second)
	}
	outside := filepath.Join(t.TempDir(), "secret.tex")
	if e = os.WriteFile(outside, []byte("secret"), 0600); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(base, "escape.tex")
	if e = os.Symlink(outside, link); e != nil {
		t.Skipf("symlink unavailable: %v", e)
	}
	_, _, e = d.Execute(context.Background(), map[string]any{"mode": "read", "doc": "doc/src/escape.tex"}, t.TempDir())
	if e == nil {
		t.Fatal("symlink escape should be rejected")
	}
}

func TestFrequencyPassiveAndExplicitSelection(t *testing.T) {
	d := &Docs{cache: t.TempDir()}
	idx := &docIndex{Key: "fingerprint"}
	d.recordFrequency(idx.Key, "files/a.lua", 1, 2, "first paragraph 中文", "read", 1)
	d.recordFrequency(idx.Key, "files/b.lua", 4, 4, "second paragraph", "search", 2)
	before, _ := os.ReadFile(d.frequencyPath())
	entries, _ := d.frequencySnapshot(idx.Key)
	passive, _ := formatFrequency(entries, 8, 80, 180)
	after, _ := os.ReadFile(d.frequencyPath())
	if passive == "" || string(before) != string(after) {
		t.Fatal("passive frequent changed cache")
	}
	out, err := d.frequent(context.Background(), idx, map[string]any{"limit": 1, "line_chars": 80, "max_chars": 180})
	if err != nil || out == "" {
		t.Fatal(err, out)
	}
	selected, _ := d.frequencySnapshot(idx.Key)
	if selected[0].Hits < 3 {
		t.Fatalf("top entry not incremented: %#v", selected[0])
	}
	id := selected[0].ID
	if _, err = d.frequent(context.Background(), idx, map[string]any{"entry_id": id[:6], "limit": -1, "line_chars": 80, "max_chars": 180}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, id[:12]) || len([]rune(out)) > 180 {
		t.Fatalf("frequency output budget/id failed: %q", out)
	}
}

func TestCompileLuaFallbackAndSourcePolicy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "图形.lua")
	content := `local ld=luadraw; local g=ld.graph:new{window={-2,2,-2,2},bbox=false}; g:Dline(-1,1,"red"); g:Show()` + "\n"
	if e := os.WriteFile(src, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
	work := filepath.Join(t.TempDir(), "输出目录 with spaces")
	if e := os.MkdirAll(work, 0700); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(ctx, src, work, map[string]any{"resource_root": root, "helper_path": helperSearch(root), "mask_executables": []string{"lualatex"}})
	if res["ok"] != true {
		if strings.Contains(res["error"].(string), "no fallback combination") {
			t.Skipf("real fallback unavailable on this host: %v", res["error"])
		}
		t.Fatalf("fallback compile failed: %v", res["error"])
	}
	if res["backend"] != "standalone-lua+generated-TikZ" {
		t.Fatalf("unexpected backend %v", res["backend"])
	}
	if res["engine"] != "xelatex" && res["engine"] != "pdflatex" {
		t.Fatalf("unexpected engine %v", res["engine"])
	}
	if _, e := os.Stat(src); e != nil {
		t.Fatal("source was removed", e)
	}
	if res["generated_lua"] == nil || res["tikz"] == nil {
		t.Fatal("missing generated artifact paths")
	}
}
func TestCompileFallbackFailureCollectsLogsOnFreshProbe(t *testing.T) {
	root := projectRoot(t)
	helperSource := helperSearch(root)
	body, e := os.ReadFile(helperSource)
	if e != nil {
		t.Fatal(e)
	}
	helper := filepath.Join(t.TempDir(), "shared helper.lua")
	if e = os.WriteFile(helper, body, 0600); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(t.TempDir(), "图.lua")
	lua := `local ld=luadraw; local g=ld.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()` + "\n"
	if e = os.WriteFile(src, []byte(lua), 0600); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(context.Background(), src, t.TempDir(), map[string]any{"resource_root": root, "helper_path": helper, "mask_executables": []string{"lualatex"}, "tex_packages": []string{"luadrawtest_missing_package_zz"}})
	if res["ok"] == true {
		t.Fatal("expected the deliberately missing package to fail")
	}
	if strings.Contains(res["error"].(string), "no fallback combination") {
		t.Skipf("real fallback unavailable on this host: %v", res["error"])
	}
	logs, ok := res["logs"].([]any)
	if !ok {
		t.Fatalf("logs must have consistent []any type, got %T", res["logs"])
	}
	found := false
	for _, v := range logs {
		m, ok := v.(map[string]any)
		if ok && m["engine"] == "xelatex" || ok && m["engine"] == "pdflatex" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback TeX failure log missing: %#v", logs)
	}
}
func TestSelfContainedTeXPreservesAcceptedPreamble(t *testing.T) {
	accepted := "\\documentclass{article}\n\\usepackage{tikz}\n\\usetikzlibrary{calc,arrows.meta}\n\\usepackage[3d]{luadraw}\n\\begin{document}\n\\begin{luadraw}{name=accepted}\nlocal ld=luadraw; local g=ld.graph3d:new{}; g:Show()\n\\end{luadraw}\n\\end{document}\n"
	tex := SelfContainedTeX(accepted, map[string]any{}, true)
	if !strings.Contains(tex, `\usetikzlibrary{calc,arrows.meta}`) || !strings.Contains(tex, `\usepackage[3d]{luadraw}`) {
		t.Fatalf("custom preamble or 3D package option was lost:\n%s", tex)
	}
	if strings.Count(tex, `\usepackage[3d]{luadraw}`) != 1 {
		t.Fatalf("LuaDraw package must be emitted once by the exporter:\n%s", tex)
	}
	if !strings.Contains(tex, `\begin{luadraw}{name=submitted}`) || strings.Contains(tex, `\begin{luadraw}{name=accepted}`) {
		t.Fatalf("accepted Lua code was not wrapped in the canonical environment:\n%s", tex)
	}
}

func TestSelfContainedTeXBuildsWithoutLuaOrTikZInputs(t *testing.T) {
	root := projectRoot(t)
	code := "local ld=luadraw\nlocal g=ld.graph:new{window={-2,2,-1.5,1.5},bbox=false}\ng:Dline(-1,1,\"red,thick\")\ng:Show()\n"
	tex := SelfContainedTeX(code, map[string]any{}, false)
	if !strings.Contains(tex, `\begin{luadraw}{name=submitted}`) || strings.Contains(tex, `dofile(`) || strings.Contains(tex, `\input{`) {
		t.Fatalf("export is not self-contained:\n%s", tex)
	}
	for _, fallback := range []bool{false, true} {
		name := "lualatex"
		options := map[string]any{"resource_root": root, "output_name": "standalone"}
		if fallback {
			name = "fallback"
			options["mask_executables"] = []string{"lualatex"}
		}
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			source := filepath.Join(work, "submitted.tex")
			if err := os.WriteFile(source, []byte(tex), 0600); err != nil {
				t.Fatal(err)
			}
			result, _ := Compile(context.Background(), source, work, options)
			if result["ok"] != true {
				errText, _ := result["error"].(string)
				if fallback && strings.Contains(errText, "no fallback combination") {
					t.Skipf("fallback tools unavailable: %s", errText)
				}
				t.Fatalf("standalone TeX compile failed: %v", result["error"])
			}
			if _, err := os.Stat(filepath.Join(work, "standalone.pdf")); err != nil {
				t.Fatalf("compiled PDF missing: %v", err)
			}
		})
	}
}

func TestMapSliceAcceptsFreshAndCachedRepresentations(t *testing.T) {
	m := map[string]any{"usable": true}
	if len(mapSlice([]map[string]any{m})) != 1 || len(mapSlice([]any{m})) != 1 {
		t.Fatal("candidate conversion rejected a supported slice representation")
	}
}

func TestCompileChineseMath3DEnvironment(t *testing.T) {
	root := projectRoot(t)
	src := filepath.Join(t.TempDir(), "图形.tex")
	code := "-- 中文数学标签：x^2+y^2=z^2\nlocal ld=luadraw; local g=ld.graph3d:new{window={-2,2,-2,2},bbox=false}; g:Show()\n"
	body := SelfContainedTeX(code, map[string]any{}, false)
	if e := os.WriteFile(src, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := Probe(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	work := t.TempDir()
	res, _ := Compile(context.Background(), src, work, map[string]any{"resource_root": root, "mask_executables": []string{"lualatex"}})
	if res["ok"] != true {
		if info["standalone_lua_usable"] != true {
			t.Skipf("real fallback unavailable on this host: %v", res["error"])
		}
		t.Fatalf("3D Chinese environment failed: %v", res["error"])
	}
	if res["backend"] != "standalone-lua+generated-TikZ" {
		t.Fatalf("expected fallback backend, got %v", res["backend"])
	}
	if res["generated_lua"] == nil || res["tikz"] == nil {
		t.Fatal("expected extracted Lua and generated TikZ")
	}
}

func TestCompileRejectsHandwrittenTikZ(t *testing.T) {
	root := projectRoot(t)
	src := filepath.Join(t.TempDir(), "figure.lua")
	if e := os.WriteFile(src, []byte("tex.print('\\\\draw (0,0)--(1,1);')\n"), 0600); e != nil {
		t.Fatal(e)
	}
	res, _ := Compile(context.Background(), src, t.TempDir(), map[string]any{"resource_root": root})
	if res["ok"] == true || !strings.Contains(res["error"].(string), "LuaDraw is mandatory") {
		t.Fatalf("expected policy rejection, got %#v", res)
	}
}
