package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mineru-tools/internal/config"
)

func TestLuaDrawCompileAndSubmitSourceIdentity(t *testing.T) {
	work := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	comp := NewCompiler(config.LatexCompileConfig{Timeout: 90})
	if err := comp.FigureAvailable(); err != nil {
		t.Skipf("LuaDraw toolchain unavailable: %v", err)
	}
	state := &tikzState{workDir: work, requireLuaDraw: true}
	code := `local ld=luadraw; local g=ld.graph:new{window={-2,2,-2,2},bbox=false}; g:Dline(-1,1,"red"); g:Show()`
	source := filepath.Join(work, "figure.lua")
	if err := os.WriteFile(source, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	tool := &CompileFigureTool{Comp: comp, State: state, EngineIsXe: true}
	result, err := tool.Execute(`{"path":"figure.lua"}`)
	if err != nil || !state.compileOK {
		t.Fatalf("compile: %v %s", err, result.Text)
	}
	if !strings.Contains(state.generatedTikZ, "tikzpicture") || !strings.Contains(state.editableTeX, `\begin{luadraw}`) {
		t.Fatal("missing generated backend or editable source")
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != code {
		t.Fatalf("source changed: %v", err)
	}
	compiledSource, err := os.ReadFile(filepath.Join(work, "luadraw-build", "editable.tex"))
	if err != nil || string(compiledSource) != state.editableTeX {
		t.Fatalf("editable delivery differs from compiled source: %v", err)
	}
	bookWork := t.TempDir()
	if err := os.WriteFile(filepath.Join(bookWork, "figure.tex"), []byte(buildStandalone(state.generatedTikZ, true)), 0644); err != nil {
		t.Fatal(err)
	}
	if result := comp.Compile(bookWork, "figure.tex"); !result.OK {
		t.Fatalf("generated TikZ does not compile with the book engine: %s", result.Err)
	}
	submit := &SubmitFigureTool{State: state}
	state.compareCalled = true
	if err := os.WriteFile(source, []byte(code+"\n-- edited after compile"), 0644); err != nil {
		t.Fatal(err)
	}
	rejected, err := submit.Execute(`{"path":"figure.lua"}`)
	if err != nil || !strings.Contains(rejected.Text, "REJECTED") || state.submitted {
		t.Fatalf("changed source accepted: %v %s", err, rejected.Text)
	}
	if err := os.WriteFile(source, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	accepted, err := submit.Execute(`{"path":"figure.lua"}`)
	if err != nil || !state.submitted || !strings.Contains(accepted.Text, "SUBMITTED") {
		t.Fatalf("submit: %v %s", err, accepted.Text)
	}
	// Failed compilation must keep the previous valid preview available.
	before, err := os.ReadFile(state.lastPDF)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("not a LuaDraw program"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _ = tool.Execute(`{"path":"figure.lua"}`)
	after, err := os.ReadFile(filepath.Join(work, "standalone.pdf"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("failed compile lost previous preview: %v", err)
	}
}

func TestLuaDrawDocToolsShareIndexAndSeparatePreviews(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cache := t.TempDir()
	first, err := newLuaDrawDocs(t.TempDir(), cache)
	if err != nil {
		t.Skip(err)
	}
	second, err := newLuaDrawDocs(t.TempDir(), cache)
	if err != nil {
		t.Fatal(err)
	}
	if first.Docs != second.Docs || first.Root == second.Root {
		t.Fatal("documentation index must be shared; previews must be private")
	}
	result, err := first.Execute(`{"mode":"read","doc":"files/luadraw_graph2d.lua","start_line":1,"end_line":5}`)
	if err != nil || !strings.Contains(result.Text, "luadraw_graph2d.lua") {
		t.Fatalf("docs read: %v %s", err, result.Text)
	}
	b, _ := json.Marshal(second.Definition())
	if !strings.Contains(string(b), "frequent") {
		t.Fatal("frequency lookup missing")
	}
}

func TestNewLuaDrawSessionRejectsHandWrittenTikZ(t *testing.T) {
	state := &tikzState{workDir: t.TempDir(), requireLuaDraw: true}
	comp := &Compiler{timeout: time.Second}
	result := compileDrawing(comp, state, `\begin{tikzpicture}\end{tikzpicture}`, "figure.tex", true)
	if result.OK || !strings.Contains(result.Err, "LuaDraw source is required") {
		t.Fatalf("unexpected result: %+v", result)
	}
}
