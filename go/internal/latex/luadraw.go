package latex

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/luadraw"
	"mineru-tools/internal/session"
)

// LuaDrawDocTool shares a persistent index and paragraph-frequency cache across
// drawing sessions; previews remain private to each figure workspace.
type LuaDrawDocTool struct {
	Docs *luadraw.Docs
	Root string
}

func (t *LuaDrawDocTool) Name() string { return "luadraw_doc" }

func (t *LuaDrawDocTool) Definition() map[string]any { return t.Docs.Definition() }
func (t *LuaDrawDocTool) Execute(argsJSON string) (session.ToolResult, error) {
	args, err := parseJSONObject(argsJSON)
	if err != nil {
		return session.ToolResult{}, err
	}
	text, img, err := t.Docs.Execute(context.Background(), args, filepath.Join(t.Root, "luadraw-doc-preview"))
	if err != nil {
		return session.ToolResult{Text: "LuaDraw documentation: " + err.Error()}, nil
	}
	result := session.ToolResult{Text: text}
	if img != "" {
		b, err := os.ReadFile(img)
		if err != nil {
			return session.ToolResult{}, err
		}
		result.ImageBase64 = base64.StdEncoding.EncodeToString(b)
		result.ImageMIME = "image/png"
	}
	return result, nil
}

func (c *Compiler) FigureAvailable() error {
	if _, err := exec.LookPath(c.raster); err != nil {
		return err
	}
	root, err := luadraw.Resources()
	if err != nil {
		return err
	}
	info, err := luadraw.Probe(context.Background(), root)
	if err != nil {
		return err
	}
	if info["ready"] != true {
		return fmt.Errorf("no working LuaDraw compiler route: %v", info["probe_log"])
	}
	return nil
}

var luaDrawDocsMu sync.Mutex
var luaDrawDocs = map[string]*luadraw.Docs{}

func newLuaDrawDocs(workDir, sharedCache string) (*LuaDrawDocTool, error) {
	root, err := luadraw.Resources()
	if err != nil {
		return nil, err
	}
	key, err := filepath.Abs(sharedCache)
	if err != nil {
		return nil, err
	}
	luaDrawDocsMu.Lock()
	defer luaDrawDocsMu.Unlock()
	docs := luaDrawDocs[key]
	if docs == nil {
		docs, err = luadraw.NewDocs(root, key)
		if err != nil {
			return nil, err
		}
		luaDrawDocs[key] = docs
	}
	return &LuaDrawDocTool{Docs: docs, Root: workDir}, nil
}
func isLuaDrawSource(code, path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".lua") || strings.Contains(code, `\begin{luadraw}`) || strings.Contains(code, `\begin{luadrawenv}`)
}

// compileDrawing keeps legacy TikZ resumable, while new LuaDraw figures use the
// reference runtime's real capability probes, LuaLaTeX preference and fallback.
func compileDrawing(comp *Compiler, state *tikzState, code, source string, engineIsXe bool) CompileResult {
	if !isLuaDrawSource(code, source) {
		if state.requireLuaDraw {
			return CompileResult{Err: "LuaDraw source is required: write figure.lua or a TeX luadraw environment."}
		}
		path := filepath.Join(state.workDir, "standalone.tex")
		if err := os.WriteFile(path, []byte(buildStandalone(code, engineIsXe)), 0644); err != nil {
			return CompileResult{Err: err.Error()}
		}
		result := comp.Compile(state.workDir, "standalone.tex")
		if result.OK {
			state.generatedTikZ = ""
			state.editableTeX = ""
		}
		return result
	}
	root, err := luadraw.Resources()
	if err != nil {
		return CompileResult{Err: err.Error()}
	}
	// A separate build directory preserves the last successful PDF if compilation fails.
	build := filepath.Join(state.workDir, "luadraw-build")
	sourceIsTeX := strings.EqualFold(filepath.Ext(source), ".tex")
	customPreamble := luadraw.OriginalTeXPreamble(code, sourceIsTeX)
	editableTeX := luadraw.SelfContainedTeX(code, map[string]any{"tex_packages": []string{"amsmath", "amssymb"}}, sourceIsTeX)
	if err := os.MkdirAll(build, 0700); err != nil {
		return CompileResult{Err: err.Error()}
	}
	editableSource := filepath.Join(build, "editable.tex")
	if err := os.WriteFile(editableSource, []byte(editableTeX), 0600); err != nil {
		return CompileResult{Err: err.Error()}
	}
	result, err := luadraw.Compile(context.Background(), editableSource, build, map[string]any{"resource_root": root, "output_name": "standalone", "timeout": int(comp.timeout / time.Second), "tex_packages": []string{"amsmath", "amssymb"}, "preamble": customPreamble})
	if err != nil {
		return CompileResult{Err: err.Error()}
	}
	if result["ok"] != true {
		return CompileResult{Err: fmt.Sprint(result["error"]), Log: fmt.Sprint(result["logs"])}
	}
	tkz, _ := result["tikz"].(string)
	body, err := os.ReadFile(tkz)
	if err != nil {
		return CompileResult{Err: "read generated TikZ: " + err.Error()}
	}
	pdf := filepath.Join(state.workDir, "standalone.pdf")
	if err := copyFile(fmt.Sprint(result["pdf"]), pdf); err != nil {
		return CompileResult{Err: err.Error()}
	}
	state.generatedTikZ = customPreamble + "\\usetikzlibrary{patterns,patterns.meta,plotmarks,arrows.meta,bending,decorations.markings,backgrounds}\n" + string(body)
	state.editableTeX = editableTeX
	return CompileResult{OK: true, PDF: pdf, Log: fmt.Sprintf("LuaDraw backend=%v engine=%v", result["backend"], result["engine"])}
}
