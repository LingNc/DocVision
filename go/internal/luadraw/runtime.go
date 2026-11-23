package luadraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var probeMu sync.Mutex
var probeExecutionMu sync.Mutex
var probes = map[string]map[string]any{}

func abs(p string) string {
	a, e := filepath.Abs(p)
	if e == nil {
		return filepath.Clean(a)
	}
	return filepath.Clean(p)
}
func filesFor(root string) string {
	for _, p := range []string{filepath.Join(root, "files"), root} {
		if exists(filepath.Join(p, "luadraw_base.lua")) && exists(filepath.Join(p, "luadraw_graph2d.lua")) {
			return p
		}
	}
	return ""
}
func exists(p string) bool   { s, e := os.Stat(p); return e == nil && !s.IsDir() }
func lookup(n string) string { p, _ := exec.LookPath(n); return p }
func envSet(env []string, k, v string) []string {
	out := []string{}
	for _, x := range env {
		if !strings.HasPrefix(x, k+"=") {
			out = append(out, x)
		}
	}
	return append(out, k+"="+v)
}
func envFor(root string) []string {
	env := append([]string{}, os.Environ()...)
	f := filesFor(root)
	for _, k := range []string{"TEXINPUTS", "LUAINPUTS"} {
		env = envSet(env, k, f+string(filepath.Separator)+"//"+string(os.PathListSeparator)+os.Getenv(k))
	}
	return env
}
func run(ctx context.Context, bin string, args []string, dir string, env []string, d time.Duration) (int, string) {
	if bin == "" {
		return 127, "executable not found"
	}
	c, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(c, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	setProcessGroup(cmd)
	cmd.Cancel = func() error { killProcessGroup(cmd); return nil }
	cmd.WaitDelay = 5 * time.Second
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	e := cmd.Run()
	s := strings.ToValidUTF8(output.String(), "�")
	if c.Err() == context.DeadlineExceeded {
		return 124, s + "\nTimed out after " + d.String()
	}
	if e == nil {
		return 0, s
	}
	if x, ok := e.(*exec.ExitError); ok {
		return x.ExitCode(), s
	}
	return 127, s + "\n" + e.Error()
}
func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
func copyMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var r map[string]any
	_ = json.Unmarshal(b, &r)
	return r
}
func helperDefault(root string) string {
	for _, p := range []string{filepath.Join(root, "scripts", "luadraw_runtime.lua"), filepath.Join(filepath.Dir(root), "scripts", "luadraw_runtime.lua")} {
		if exists(p) {
			return p
		}
	}
	return ""
}
func helperSearch(root string) string {
	dir := abs(root)
	for depth := 0; depth < 10; depth++ {
		p := filepath.Join(dir, "scripts", "luadraw_runtime.lua")
		if exists(p) {
			return p
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return helperDefault(root)
}
func version(ctx context.Context, p string, e []string) string {
	if p == "" {
		return ""
	}
	_, s := run(ctx, p, []string{"--version"}, "", e, 5*time.Second)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
func drawVersion(root string) string {
	b, _ := os.ReadFile(filepath.Join(filesFor(root), "luadraw.sty"))
	r := regexp.MustCompile(`(?i)luadraw\.sty\s+version\s+([\d.]+)`).FindSubmatch(b)
	if len(r) > 1 {
		return string(r[1])
	}
	b, _ = os.ReadFile(filepath.Join(root, "README.md"))
	r = regexp.MustCompile(`Package Version:\s*\*\*([\d.]+)\*\*`).FindSubmatch(b)
	if len(r) > 1 {
		return string(r[1])
	}
	return ""
}
func probeKey(root, helper string) string {
	h := sha256.New()
	fmt.Fprintln(h, root, runtime.GOOS, runtime.GOARCH)
	for _, k := range []string{"PATH", "TEXINPUTS", "LUAINPUTS", "TEXMFVAR", "TEXMFCACHE", "XDG_CACHE_HOME"} {
		fmt.Fprintln(h, k, os.Getenv(k))
	}
	for _, n := range []string{"lualatex", "texlua", "lua5.4", "lua", "xelatex", "pdflatex", "kpsewhich"} {
		p := lookup(n)
		fmt.Fprintln(h, n, p)
		if s, e := os.Stat(p); e == nil {
			fmt.Fprintln(h, s.Size(), s.ModTime().UnixNano())
		}
	}
	for _, n := range []string{"luadraw.sty", "luadraw_base.lua", "luadraw_graph2d.lua", "luadraw_graph3d.lua"} {
		p := filepath.Join(filesFor(root), n)
		if s, e := os.Stat(p); e == nil {
			fmt.Fprintln(h, n, s.Size(), s.ModTime().UnixNano())
		}
	}
	if helper == "" {
		helper = helperSearch(root)
	}
	fmt.Fprintln(h, "helper", helper)
	if s, e := os.Stat(helper); e == nil {
		fmt.Fprintln(h, s.Size(), s.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil))
}

const tikzRuntimeLibraries = "\\usetikzlibrary{patterns,patterns.meta,plotmarks,arrows.meta,bending,decorations.markings,backgrounds}\n"

func texCacheEnv(env []string, cache string) ([]string, error) {
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	for _, key := range []string{"TEXMFVAR", "TEXMFCACHE", "XDG_CACHE_HOME"} {
		env = envSet(env, key, cache)
	}
	return env, nil
}

func testDirect(ctx context.Context, bin string, env []string) (bool, string) {
	d, e := os.MkdirTemp("", "luadraw-probe-")
	if e != nil {
		return false, e.Error()
	}
	defer os.RemoveAll(d)
	ee, e := texCacheEnv(env, filepath.Join(d, "tex-cache"))
	if e != nil {
		return false, e.Error()
	}
	src := `\documentclass[tikz,border=1pt]{standalone}` + "\n" + `\usepackage[3d]{luadraw}` + "\n" + `\begin{document}` + "\n" + `\directlua{luadraw.graph_name="probe.tkz"; local g=luadraw.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()}` + "\n" + `\end{document}` + "\n"
	_ = os.WriteFile(filepath.Join(d, "probe.tex"), []byte(src), 0600)
	c, l := run(ctx, bin, []string{"-interaction=nonstopmode", "-halt-on-error", "probe.tex"}, d, ee, 40*time.Second)
	return c == 0 && exists(filepath.Join(d, "probe.pdf")) && exists(filepath.Join(d, "probe.tkz")), tail(l, 8000)
}

type fb struct {
	Lua        string `json:"lua"`
	Engine     string `json:"engine"`
	Usable     bool   `json:"usable"`
	Returncode int    `json:"returncode"`
	Log        string `json:"log"`
}

func testFallback(ctx context.Context, root string, env []string, luas []string, tex map[string]string, helper string) ([]fb, string) {
	f := filesFor(root)
	if helper == "" {
		helper = helperSearch(root)
	}
	var out []fb
	var logs []string
	for _, lua := range luas {
		d, e := os.MkdirTemp("", "luadraw-fallback-")
		if e != nil {
			continue
		}
		src := filepath.Join(d, "probe.lua")
		_ = os.WriteFile(src, []byte(`local ld=luadraw; local g=ld.graph:new{window={-1,1,-1,1},bbox=false}; g:Dline(-1,1,"red"); g:Show()`+"\n"), 0600)
		c, l := run(ctx, lua, []string{helper, f, src, d}, d, env, 25*time.Second)
		tkz := filepath.Join(d, "figure.tkz")
		if c != 0 || !exists(tkz) {
			for _, name := range []string{"xelatex", "pdflatex"} {
				if tex[name] != "" {
					out = append(out, fb{lua, name, false, c, tail(l, 6000)})
				}
			}
			logs = append(logs, l)
			os.RemoveAll(d)
			continue
		}
		_ = os.WriteFile(filepath.Join(d, "probe.tex"), []byte("\\documentclass[tikz,border=1pt]{standalone}\n\\usepackage{tikz}\n"+tikzRuntimeLibraries+"\\begin{document}\n\\input{figure.tkz}\n\\end{document}\n"), 0600)
		for _, name := range []string{"xelatex", "pdflatex"} {
			bin := tex[name]
			if bin == "" {
				continue
			}
			c, l = run(ctx, bin, []string{"-interaction=nonstopmode", "-halt-on-error", "probe.tex"}, d, env, 40*time.Second)
			ok := c == 0 && exists(filepath.Join(d, "probe.pdf"))
			out = append(out, fb{lua, name, ok, c, tail(l, 6000)})
			if !ok {
				logs = append(logs, l)
			}
		}
		os.RemoveAll(d)
	}
	return out, tail(strings.Join(logs, "\n"), 12000)
}

// Probe runs real LuaDraw+TeX probes for every available fallback combination.
func Probe(ctx context.Context, root string) (map[string]any, error) {
	return probeWithHelper(ctx, root, "")
}
func probeWithHelper(ctx context.Context, root, helper string) (map[string]any, error) {
	probeExecutionMu.Lock()
	defer probeExecutionMu.Unlock()
	if root == "" {
		return nil, fmt.Errorf("LuaDraw resource root is required")
	}
	root = abs(root)
	env := envFor(root)
	key := probeKey(root, helper)
	probeMu.Lock()
	v, ok := probes[key]
	probeMu.Unlock()
	if ok {
		return copyMap(v), nil
	}
	names := []string{"lualatex", "texlua", "lua5.4", "lua", "xelatex", "pdflatex", "kpsewhich"}
	bins, vers := map[string]string{}, map[string]string{}
	for _, n := range names {
		bins[n] = lookup(n)
		vers[n] = version(ctx, bins[n], env)
	}
	files := filesFor(root)
	pkgs := map[string]string{}
	if bins["kpsewhich"] != "" && files != "" {
		for _, n := range []string{"luadraw.sty", "luadraw_graph2d.lua", "luadraw_graph3d.lua"} {
			c, o := run(ctx, bins["kpsewhich"], []string{n}, root, env, 8*time.Second)
			if c == 0 {
				pkgs[n] = strings.TrimSpace(o)
			} else {
				pkgs[n] = ""
			}
		}
	}
	direct := false
	log := "LuaDraw package files unavailable"
	if files != "" && exists(filepath.Join(files, "luadraw.sty")) {
		direct, log = testDirect(ctx, bins["lualatex"], env)
	}
	var luas []string
	for _, n := range []string{"texlua", "lua5.4", "lua"} {
		if bins[n] != "" {
			luas = append(luas, bins[n])
		}
	}
	cs, flog := testFallback(ctx, root, env, luas, map[string]string{"xelatex": bins["xelatex"], "pdflatex": bins["pdflatex"]}, helper)
	usable := map[string]bool{}
	eng := map[string]bool{}
	var selected *fb
	for i := range cs {
		if cs[i].Usable {
			usable[cs[i].Lua] = true
			eng[cs[i].Engine] = true
			if selected == nil {
				selected = &cs[i]
			}
		}
	}
	if !direct && flog != "" {
		log += "\nStandalone fallback probe: " + tail(flog, 6000)
	}
	backend := "unavailable"
	if direct {
		backend = "lualatex"
	} else if selected != nil {
		backend = "standalone-lua+" + selected.Engine
	}
	ex := map[string]any{}
	for _, n := range names {
		u := false
		switch n {
		case "lualatex":
			u = direct
		case "texlua", "lua5.4", "lua":
			u = usable[bins[n]]
		case "xelatex", "pdflatex":
			u = eng[n]
		case "kpsewhich":
			u = pkgs["luadraw.sty"] != ""
		}
		ex[n] = map[string]any{"path": nilEmpty(bins[n]), "version": vers[n], "available": vers[n] != "", "usable": u}
	}
	fc := []any{}
	for _, c := range cs {
		fc = append(fc, map[string]any{"lua": c.Lua, "engine": c.Engine, "usable": c.Usable, "returncode": c.Returncode, "log": c.Log})
	}
	var fl, fe any
	if selected != nil {
		fl = selected.Lua
		fe = selected.Engine
	}
	result := map[string]any{"ok": direct || selected != nil, "ready": direct || selected != nil, "selected_backend": backend, "luadraw_root": root, "luadraw_files": nilEmpty(files), "luadraw_version": drawVersion(root), "executables": ex, "packages": pkgs, "lualatex_usable": direct, "standalone_lua_usable": selected != nil, "lua_modules_usable": len(usable) > 0, "fallback_engine": fe, "fallback_lua": fl, "fallback_candidates": fc, "selected_strategy": backend, "probe_log": tail(log, 8000)}
	probeMu.Lock()
	probes[key] = copyMap(result)
	probeMu.Unlock()
	return result, nil
}
func nilEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func strOpt(m map[string]any, k, d string) string {
	v, ok := m[k].(string)
	if ok && v != "" {
		return v
	}
	return d
}
func stringsOpt(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		r := []string{}
		for _, v := range x {
			r = append(r, fmt.Sprint(v))
		}
		return r
	}
	return nil
}
func mapSlice(v any) []map[string]any {
	switch items := v.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}
func cp(src, dst string) error {
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	return os.WriteFile(dst, b, 0600)
}

var graphPattern = regexp.MustCompile(`(?:luadraw|\bld)\.graph(?:3d)?\s*:\s*new`)
var showPattern = regexp.MustCompile(`\b\w+\s*:\s*(?:Show|Save)\s*\(`)
var envPattern = regexp.MustCompile(`(?s)\\begin\s*\{(luadraw|luadrawenv)\}(?:\{[^{}]*\})?(.*?)\\end\s*\{(luadraw|luadrawenv)\}`)
var luaCodePattern = regexp.MustCompile(`(?s)\\begin\s*\{(luacode\*?)\}(.*?)\\end\s*\{(luacode\*?)\}`)

func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range []byte(s) {
		if c >= 0x20 && c <= 0x7e && c != '\\' && c != '"' {
			b.WriteByte(c)
		} else if c == '\\' || c == '"' {
			b.WriteByte('\\')
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "\\%03d", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
func hasGraph(s string) bool { return graphPattern.MatchString(s) && showPattern.MatchString(s) }

// Compile validates the LuaDraw source policy, compiles without modifying the source, and returns Python-compatible result keys.
func Compile(ctx context.Context, source, workdir string, options map[string]any) (map[string]any, error) {
	r := map[string]any{"ok": false, "pdf": nil, "path": nil, "wrapper": nil, "engine": nil, "backend": nil, "generated_tikz": []string{}, "tikz": nil, "logs": []any{}, "error": nil, "luadraw_version": nil, "generated_lua": nil}
	if options == nil {
		options = map[string]any{}
	}
	logs := r["logs"].([]any)
	logadd := func(engine, stage string, code int, s string) {
		m := map[string]any{"engine": engine, "returncode": code, "log": tail(s, 12000)}
		if stage != "" {
			m["stage"] = stage
		}
		logs = append(logs, m)
		r["logs"] = logs
	}
	fail := func(e error) (map[string]any, error) {
		msg := e.Error()
		for i := len(logs) - 1; i >= 0; i-- {
			m, ok := logs[i].(map[string]any)
			if !ok {
				continue
			}
			if c, _ := m["returncode"].(int); c != 0 && m["log"] != "" {
				msg += "\nLast stage log:\n" + tail(fmt.Sprint(m["log"]), 9000)
				break
			}
		}
		r["error"] = msg
		return r, nil
	}
	src := abs(source)
	work := abs(workdir)
	if !exists(src) {
		return fail(fmt.Errorf("LuaDraw source not found: %s", src))
	}
	rawb, e := os.ReadFile(src)
	if e != nil {
		return fail(e)
	}
	raw := string(rawb)
	if e = os.MkdirAll(work, 0700); e != nil {
		return fail(e)
	}
	lua := src
	switch strings.ToLower(filepath.Ext(src)) {
	case ".lua":
		if !hasGraph(raw) {
			return fail(fmt.Errorf("LuaDraw is mandatory: figure.lua must construct a LuaDraw graph and call Show/Save"))
		}
		d := filepath.Join(work, "_luadraw_inputs")
		_ = os.MkdirAll(d, 0700)
		lua = filepath.Join(d, filepath.Base(src))
		if src != abs(lua) {
			if e = cp(src, lua); e != nil {
				return fail(e)
			}
		}
	case ".tex":
		ms := envPattern.FindAllStringSubmatch(raw, -1)
		var blocks []string
		for _, m := range ms {
			if m[1] == m[3] {
				blocks = append(blocks, m[2])
			}
		}
		if len(blocks) == 0 {
			for _, m := range luaCodePattern.FindAllStringSubmatch(raw, -1) {
				if m[1] == m[3] {
					blocks = append(blocks, m[2])
				}
			}
		}
		if len(blocks) != 1 {
			return fail(fmt.Errorf("figure.tex must contain exactly one LuaDraw environment or luacode* block"))
		}
		code := strings.ReplaceAll(blocks[0], `\shortcuts`, "")
		if !hasGraph(code) {
			return fail(fmt.Errorf("LuaDraw is mandatory: figure.tex has no graph construction and Show/Save call"))
		}
		lua = filepath.Join(work, "_luadraw_from_tex.lua")
		if e = os.WriteFile(lua, []byte(code), 0600); e != nil {
			return fail(e)
		}
	default:
		return fail(fmt.Errorf("source must be figure.lua or figure.tex"))
	}
	root := strOpt(options, "luadraw_root", strOpt(options, "resource_root", ""))
	if root == "" {
		return fail(fmt.Errorf("resource_root or luadraw_root is required"))
	}
	root = abs(root)
	helperPath := strOpt(options, "helper_path", helperSearch(root))
	info, e := probeWithHelper(ctx, root, helperPath)
	if e != nil {
		return fail(e)
	}
	r["luadraw_version"] = info["luadraw_version"]
	files, _ := info["luadraw_files"].(string)
	if files == "" {
		return fail(fmt.Errorf("LuaDraw modules unavailable under %s", root))
	}
	if !info["lualatex_usable"].(bool) {
		logadd("lualatex", "capability-probe", 1, fmt.Sprint(info["probe_log"]))
	}
	if !info["standalone_lua_usable"].(bool) {
		logadd("standalone-lua+tex", "capability-probe", 1, fmt.Sprint(info["probe_log"]))
	}
	timeout := 120
	for _, k := range []string{"timeout", "compile_timeout"} {
		switch n := options[k].(type) {
		case int:
			if n > 0 {
				timeout = n
			}
		case float64:
			if n > 0 {
				timeout = int(n)
			}
		}
	}
	dur := time.Duration(timeout) * time.Second
	name := strOpt(options, "output_name", "figure")
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(name) {
		return fail(fmt.Errorf("output_name may contain only letters, digits, '_' and '-'"))
	}
	pdf := filepath.Join(work, name+".pdf")
	wrapper := filepath.Join(work, name+"_luadraw_wrapper.tex")
	tkz := filepath.Join(work, name+".tkz")
	for _, p := range []string{pdf, wrapper, tkz} {
		if filepath.Dir(abs(p)) != work {
			return fail(fmt.Errorf("output artifacts must remain inside workdir"))
		}
	}
	for _, p := range []string{pdf, tkz} {
		if p == src {
			return fail(fmt.Errorf("refusing to remove source file %s", p))
		}
		_ = os.Remove(p)
	}
	r["pdf"] = pdf
	r["path"] = pdf
	r["wrapper"] = wrapper
	r["generated_lua"] = lua
	cache := filepath.Join(work, ".luadraw-cache")
	env, e := texCacheEnv(envFor(root), cache)
	if e != nil {
		return fail(fmt.Errorf("TeX cache: %w", e))
	}
	pkgs := stringsOpt(options["tex_packages"])
	if pkgs == nil {
		pkgs = stringsOpt(options["packages"])
	}
	seen := map[string]bool{}
	preamble := ""
	if regexp.MustCompile(`[\x{2e80}-\x{9fff}]`).MatchString(raw) {
		preamble = "\\usepackage{ctex}\n"
		seen["ctex"] = true
	}
	for _, p := range pkgs {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(p) {
			return fail(fmt.Errorf("invalid tex package name %q", p))
		}
		if !seen[p] {
			preamble += "\\usepackage{" + p + "}\n"
			seen[p] = true
		}
	}
	preamble += strOpt(options, "preamble", "")
	if preamble != "" && !strings.HasSuffix(preamble, "\n") {
		preamble += "\n"
	}
	requested := strings.ToLower(strOpt(options, "engine", "auto"))
	if !map[string]bool{"auto": true, "lualatex": true, "xelatex": true, "pdflatex": true, "fallback": true}[requested] {
		return fail(fmt.Errorf("invalid engine %q", requested))
	}
	masked := map[string]bool{}
	for _, v := range stringsOpt(options["mask_executables"]) {
		masked[v] = true
	}
	if info["lualatex_usable"].(bool) && !masked["lualatex"] && requested != "xelatex" && requested != "pdflatex" && requested != "fallback" && options["force_fallback"] != true {
		rel, _ := filepath.Rel(work, lua)
		q := luaQuote(filepath.ToSlash(rel))
		body := "\\documentclass[tikz,border=2pt]{standalone}\n" + preamble + "\\usepackage[3d]{luadraw}\n\\begin{document}\n\\directlua{ld=luadraw; Z=luadraw.cpx.Z; M=luadraw.pt3d.M; pt3d=luadraw.pt3d; luadraw.graph_name=\"" + name + ".tkz\"; dofile(" + q + ")}\n\\end{document}\n"
		_ = os.WriteFile(wrapper, []byte(body), 0600)
		c, l := run(ctx, lookup("lualatex"), []string{"-interaction=nonstopmode", "-halt-on-error", "-jobname=" + name, filepath.Base(wrapper)}, work, env, dur)
		logadd("lualatex", "", c, l)
		if c == 0 && exists(pdf) {
			if exists(tkz) {
				r["tikz"] = tkz
				r["generated_tikz"] = []string{tkz}
			}
			r["ok"] = true
			r["engine"] = "lualatex"
			r["backend"] = "luadraw.sty"
			return r, nil
		}
	}
	var candidates []fb
	for _, m := range mapSlice(info["fallback_candidates"]) {
		if m["usable"] != true {
			continue
		}
		c := fb{Lua: fmt.Sprint(m["lua"]), Engine: fmt.Sprint(m["engine"]), Usable: true}
		if masked[c.Lua] || masked[filepath.Base(c.Lua)] || masked[c.Engine] {
			continue
		}
		if (requested == "xelatex" || requested == "pdflatex") && requested != c.Engine {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return fail(fmt.Errorf("no fallback combination passed real probes"))
	}
	pref := []string{fmt.Sprint(info["fallback_engine"]), "xelatex", "pdflatex"}
	if requested == "xelatex" || requested == "pdflatex" {
		pref = []string{requested}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return index(pref, candidates[i].Engine) < index(pref, candidates[j].Engine) })
	helper := helperPath
	if helper == "" {
		return fail(fmt.Errorf("helper_path is required; shared Lua helper not found"))
	}
	last := "LuaDraw fallback failed"
	for _, c := range candidates {
		_ = os.Remove(tkz)
		code, l := run(ctx, c.Lua, []string{helper, files, lua, work, name}, work, env, dur)
		logadd(filepath.Base(c.Lua), "LuaDraw-to-TikZ", code, l)
		if code != 0 || !exists(tkz) {
			last = "standalone LuaDraw TikZ generation failed"
			continue
		}
		r["tikz"] = tkz
		r["generated_tikz"] = []string{tkz}
		body := "\\documentclass[tikz,border=2pt]{standalone}\n\\usepackage{tikz}\n" + tikzRuntimeLibraries + preamble + "\\begin{document}\n\\input{" + filepath.Base(tkz) + "}\n\\end{document}\n"
		_ = os.WriteFile(wrapper, []byte(body), 0600)
		_ = os.Remove(pdf)
		code, l = run(ctx, lookup(c.Engine), []string{"-interaction=nonstopmode", "-halt-on-error", "-jobname=" + name, filepath.Base(wrapper)}, work, env, dur)
		logadd(c.Engine, "", code, l)
		if code == 0 && exists(pdf) {
			r["ok"] = true
			r["engine"] = c.Engine
			r["backend"] = "standalone-lua+generated-TikZ"
			return r, nil
		}
		last = c.Engine + " failed compiling generated TikZ"
	}
	return fail(fmt.Errorf("%s", last))
}
func index(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return 99
}
