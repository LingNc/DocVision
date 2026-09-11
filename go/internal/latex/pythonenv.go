package latex

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// PythonEnv is the Python environment session bash sees.
//
// The sandbox runs with --unshare-net, so a session can NEVER install a
// package itself. Two real-run consequences: a style session burned ~10
// rounds and hand-wrote a PGM parser after `ModuleNotFoundError: No
// module named 'PIL'`, and another session wrote /tmp/p10-10.pgm and
// never got to read it. So the environment is provided by the HOST:
// the interpreter (system / venv / conda) is made visible read-only
// inside the sandbox with PATH pointing at it, and a missing module is
// installed host-side (which has network) with the model simply told to
// retry. When installation fails the operator gets the same treatment
// as missing fonts: a requirements.txt + README.md to install by hand.
type PythonEnv struct {
	cfg    config.ToolsPythonConfig
	log    *logger.Logger
	report string // <proj>/work/python (requirements.txt + README.md)

	mu        sync.Mutex
	prepared  bool
	prepErr   string
	installed map[string]bool
	failed    map[string]string

	// conda resolution has its own once/mutex: Interpreter() may be called
	// while mu is held (Prepare), so it must not take mu again.
	condaOnce sync.Once
	condaDir  string
}

// autoInstall reports the resolved tools.python.auto_install (default on).
func (p *PythonEnv) autoInstall() bool {
	if p == nil {
		return false
	}
	return p.cfg.AutoInstall == nil || *p.cfg.AutoInstall
}

// normalizePythonConfig applies the code defaults (mode=system,
// install_timeout=300) so the tool never has to special-case empties.
func normalizePythonConfig(cfg config.ToolsPythonConfig) config.ToolsPythonConfig {
	if cfg.Mode == "" {
		cfg.Mode = "system"
	}
	if cfg.InstallTimeout <= 0 {
		cfg.InstallTimeout = 300
	}
	return cfg
}

// NewPythonEnv builds the environment handle; nil when disabled.
func NewPythonEnv(cfg config.ToolsPythonConfig, reportDir string, log *logger.Logger) *PythonEnv {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	cfg = normalizePythonConfig(cfg)
	return &PythonEnv{
		cfg: cfg, log: log, report: reportDir,
		installed: map[string]bool{}, failed: map[string]string{},
	}
}

// Interpreter returns the python executable to put on PATH (after
// Prepare it is an absolute path inside the environment).
func (p *PythonEnv) Interpreter() string {
	if p == nil {
		return ""
	}
	if p.cfg.Interpreter != "" {
		if abs, err := exec.LookPath(p.cfg.Interpreter); err == nil {
			return abs
		}
		return p.cfg.Interpreter
	}
	if p.cfg.Mode == "conda" {
		if dir := p.condaPrefix(); dir != "" {
			return filepath.Join(dir, "bin", "python3")
		}
	}
	if p.cfg.EnvDir != "" {
		cand := filepath.Join(p.cfg.EnvDir, "bin", "python3")
		if pathExists(cand) {
			return cand
		}
	}
	if path, err := exec.LookPath("python3"); err == nil {
		return path
	}
	return ""
}

// condaPrefix resolves the conda environment prefix.
//
// mode=conda with NOTHING configured means the user's BASE environment
// (the common case: "我本地就有 conda 的 base"), so an empty config is a
// valid, working setup instead of a validation error. conda_env: base /
// root mean the same thing.
func (p *PythonEnv) condaPrefix() string {
	p.condaOnce.Do(func() {
		if p.cfg.EnvDir != "" {
			p.condaDir = p.cfg.EnvDir
			return
		}
		base := condaBaseDir()
		name := strings.TrimSpace(p.cfg.CondaEnv)
		if name == "" || name == "base" || name == "root" {
			p.condaDir = base
			return
		}
		if cand := filepath.Join(base, "envs", name); pathExists(cand) {
			p.condaDir = cand
			return
		}
		p.condaDir = condaEnvPrefix(name)
	})
	return p.condaDir
}

// condaBaseDir runs `conda info --base` once per process.
//
// `conda` is often NOT on PATH for the process that runs DocVision: the
// user's ~/.bashrc does `conda init` (so an interactive shell has it) but
// a service/tmux/screen/nohup start does not — and `mode: conda` then
// silently degraded to whatever `python3` PATH happened to hold (on this
// host: a Homebrew Python 3.14 that is PEP 668 externally-managed, so
// every auto-install died with "externally-managed-environment" even
// though the conda base env already had PIL and PyMuPDF). So: PATH first,
// then $CONDA_EXE, then the usual install prefixes.
func condaBaseDir() string {
	condaBaseOnce.Do(func() {
		bin := findCondaBin()
		if bin == "" {
			return
		}
		out, err := runHostTimeout(bin, 30*time.Second, "info", "--base")
		if err == nil {
			if line := firstLine(out); line != "" {
				if abs, aerr := filepath.Abs(line); aerr == nil {
					line = abs
				}
				condaBase = line
				return
			}
		}
		// `info --base` failed (broken install, slow first run): the
		// binary's own prefix is the conventional answer.
		condaBase = filepath.Dir(filepath.Dir(bin))
	})
	return condaBase
}

// findCondaBin locates the conda executable: PATH, $CONDA_EXE, then the
// standard install prefixes under $HOME and /opt.
func findCondaBin() string {
	if p, err := exec.LookPath("conda"); err == nil {
		return p
	}
	if exe := strings.TrimSpace(os.Getenv("CONDA_EXE")); exe != "" && pathExists(exe) {
		return exe
	}
	home, _ := os.UserHomeDir()
	var prefixes []string
	for _, name := range []string{"miniconda3", "anaconda3", "miniforge3", "mambaforge", "miniconda", "anaconda"} {
		if home != "" {
			prefixes = append(prefixes, filepath.Join(home, name))
		}
	}
	prefixes = append(prefixes, "/opt/conda", "/opt/miniconda3", "/opt/anaconda3", "/usr/local/miniconda3", "/usr/local/anaconda3")
	for _, pre := range prefixes {
		for _, rel := range []string{"bin/conda", "condabin/conda"} {
			cand := filepath.Join(pre, rel)
			if pathExists(cand) {
				return cand
			}
		}
	}
	return ""
}

// condaEnvPrefix finds a named environment through `conda env list --json`
// (needed when the environment lives outside <base>/envs, e.g. -p prefixes
// or envs_dirs).
func condaEnvPrefix(name string) string {
	bin := findCondaBin()
	if bin == "" {
		return ""
	}
	out, err := runHostTimeout(bin, 30*time.Second, "env", "list", "--json")
	if err != nil {
		return ""
	}
	var parsed struct {
		Envs []string `json:"envs"`
	}
	if jerr := json.Unmarshal([]byte(out), &parsed); jerr != nil {
		return ""
	}
	for _, dir := range parsed.Envs {
		if filepath.Base(dir) == name && pathExists(dir) {
			return dir
		}
	}
	return ""
}

var (
	condaBaseOnce sync.Once
	condaBase     string
)

// resetCondaCache clears the process-wide conda base cache (tests).
func resetCondaCache() {
	condaBaseOnce = sync.Once{}
	condaBase = ""
}

// BinDir is the directory that must be prepended to PATH inside the
// sandbox ("" for a plain system interpreter, which is already on PATH).
func (p *PythonEnv) BinDir() string {
	if p == nil {
		return ""
	}
	if prefix := p.PrefixDir(); prefix != "" {
		return filepath.Join(prefix, "bin")
	}
	return ""
}

// PrefixDir is the environment root bound into the sandbox at the SAME
// absolute path (a venv/conda prefix is not relocatable: its scripts
// hardcode the path they were created with).
func (p *PythonEnv) PrefixDir() string {
	if p == nil || p.cfg.Mode == "system" {
		return ""
	}
	if p.cfg.Mode == "conda" {
		return p.condaPrefix()
	}
	return p.cfg.EnvDir
}

// BindArgs returns the bwrap arguments that make the environment
// visible read-only inside the sandbox (empty when there is nothing to
// bind, i.e. the plain system interpreter).
func (p *PythonEnv) BindArgs() []string {
	if p == nil {
		return nil
	}
	var args []string
	if prefix := p.PrefixDir(); prefix != "" {
		if abs, err := filepath.Abs(prefix); err == nil && pathExists(abs) {
			args = append(args, "--ro-bind", abs, abs)
		}
	} else if dir := p.BinDir(); dir != "" {
		if abs, err := filepath.Abs(dir); err == nil && pathExists(abs) {
			args = append(args, "--ro-bind", abs, abs)
		}
	}
	// A system interpreter is NOT necessarily under /usr: on this host
	// `python3` is /home/lingnc/.linuxbrew/bin/python3, and the sandbox
	// only binds /usr /bin /sbin /lib /lib64 /etc — so "just put it on
	// PATH" leaves bash either with `python3: command not found`, or
	// (worse) silently running a DIFFERENT system python that lacks the
	// packages installed for the configured one. Bind the interpreter's
	// own directories read-only, skipping the standard trees.
	for _, dir := range p.extraDirs() {
		args = append(args, "--ro-bind", dir, dir)
	}
	return args
}

// extraDirs lists host directories that must be visible for the
// configured interpreter to run at all: its own directory, sys.prefix,
// and (with auto_install) the site-packages trees — a package installed
// on the host must actually be importable inside the sandbox.
func (p *PythonEnv) extraDirs() []string {
	inter := p.Interpreter()
	if inter == "" {
		return nil
	}
	candidates := []string{filepath.Dir(inter)}
	// A venv's bin/python3 is a SYMLINK to the base interpreter: running
	// it inside the sandbox needs the real binary's directory too.
	base := inter
	if real, err := filepath.EvalSymlinks(inter); err == nil {
		base = real
		candidates = append(candidates, filepath.Dir(real))
	}
	if out, err := runHost(base, "-c", "import sys;print(sys.prefix)"); err == nil {
		if prefix := strings.TrimSpace(out); prefix != "" {
			candidates = append(candidates, prefix)
		}
	}
	// Shared libraries: the interpreter is a dynamic binary whose libpython
	// often lives OUTSIDE its prefix (Homebrew: bin/python3 →
	// Cellar/…/bin/python3.14 needing .linuxbrew/lib/libpython3.14.so.1.0).
	// Without those dirs the exec fails with ENOENT and — the nasty part —
	// bash's PATH search silently falls through to the NEXT python3, so the
	// session would quietly run /usr/bin/python3 with none of the
	// configured packages. Verified inside the real sandbox.
	// Every directory holding a link in the interpreter's symlink chain:
	// a Homebrew venv is bin/python3 -> python3.14 ->
	// ~/.linuxbrew/opt/python@3.14/bin/python3.14 -> Cellar/…, and a
	// missing intermediate (here ~/.linuxbrew/opt) makes the exec fail
	// with ENOENT — bash then falls through to the next python3 on PATH.
	candidates = append(candidates, linkChainDirs(inter)...)
	candidates = append(candidates, sharedLibDirs(base)...)
	if p.autoInstall() {
		candidates = append(candidates, systemSiteDirs(inter)...)
	}
	seen := map[string]bool{}
	var dirs []string
	add := func(path string) {
		if path == "" || !pathExists(path) || coveredBySandboxRoots(path) || seen[path] {
			return
		}
		seen[path] = true
		dirs = append(dirs, path)
	}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		// Bind the path LITERALLY (bwrap resolves nothing: a Homebrew venv
		// needs ~/.linuxbrew/opt/python@3.14/bin to exist as a link) AND
		// its target (site-packages are often links into Cellar).
		add(abs)
		if real, rerr := filepath.EvalSymlinks(abs); rerr == nil && real != abs {
			add(real)
		}
	}
	return dirs
}

// linkChainDirs walks a symlink chain and returns the directory of each
// link, so every hop is visible inside the sandbox.
func linkChainDirs(path string) []string {
	var dirs []string
	cur := path
	for i := 0; i < 16; i++ {
		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		target, err := os.Readlink(cur)
		if err != nil {
			break
		}
		dirs = append(dirs, filepath.Dir(cur))
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(cur), target)
		}
		cur = target
	}
	return dirs
}

// sharedLibDirs parses `ldd <binary>` and returns the directories of the
// libraries it loads (the interpreter itself, its libpython, the loader
// path) so a sandboxed exec of a non-/usr interpreter can actually
// succeed.
func sharedLibDirs(binary string) []string {
	if binary == "" {
		return nil
	}
	out, err := runHost("ldd", binary)
	if err != nil && strings.TrimSpace(out) == "" {
		return nil
	}
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "not found") {
			continue
		}
		fields := strings.Fields(line)
		path := ""
		for i, f := range fields {
			if f == "=>" && i+1 < len(fields) {
				path = fields[i+1]
				break
			}
		}
		if path == "" && strings.HasPrefix(fields[0], "/") {
			path = fields[0] // "linux-vdso.so.1" and friends have no path
		}
		if path == "" || !strings.HasPrefix(path, "/") {
			continue
		}
		// Both the directory named in the loader entry AND its target:
		// Homebrew's .linuxbrew/lib/libpython3.14.so.1.0 is a link into
		// Cellar, but the binary asks for the .linuxbrew/lib path itself.
		if pathExists(path) {
			dirs = append(dirs, filepath.Dir(path))
		}
		if real, rerr := filepath.EvalSymlinks(path); rerr == nil && pathExists(real) {
			dirs = append(dirs, filepath.Dir(real))
		}
	}
	return dirs
}

// coveredBySandboxRoots reports whether a path already sits inside one
// of the trees every sandboxed bash receives.
func coveredBySandboxRoots(path string) bool {
	for _, root := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc"} {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

// EnvVars returns the environment variables the sandbox needs (PATH
// first entry = the environment's bin, PYTHONPATH for --user installs).
func (p *PythonEnv) EnvVars(defaultPath string) [][2]string {
	if p == nil {
		return nil
	}
	path := defaultPath
	if bin := p.BinDir(); bin != "" {
		path = bin + ":" + defaultPath
	}
	vars := [][2]string{{"PATH", path}}
	if p.cfg.Mode == "system" && p.autoInstall() {
		if dirs := systemSiteDirs(p.Interpreter()); len(dirs) > 0 {
			vars = append(vars, [2]string{"PYTHONPATH", strings.Join(dirs, ":")})
		}
	}
	if p.cfg.Mode != "" {
		vars = append(vars, [2]string{"DOCVISION_PYTHON_MODE", p.cfg.Mode})
	}
	return vars
}

// Describe is the one-line status shown in the bash tool definition, so
// the model knows what it can rely on without any prompt change.
func (p *PythonEnv) Describe() string {
	if p == nil {
		return ""
	}
	if p.prepErr != "" {
		return "Python is configured but NOT usable right now: " + p.prepErr +
			" — report it in your submission instead of working around it."
	}
	inter := p.Interpreter()
	if inter == "" {
		return "Python was requested but no interpreter was found on the host — do not rely on python."
	}
	mode := p.cfg.Mode
	if mode == "" {
		mode = "system"
	}
	desc := fmt.Sprintf("Python is available: %s (mode=%s)", inter, mode)
	if len(p.cfg.Packages) > 0 {
		desc += ", preinstalled: " + strings.Join(p.cfg.Packages, ", ")
	}
	if p.autoInstall() {
		desc += ". If an import fails, just retry after the tool reports the package was installed"
	}
	return desc + "."
}

// Prepare makes the environment real on the HOST before sessions start:
// create the venv/conda prefix when missing and ensure every configured
// package imports. Failures are recorded (never fatal): the session is
// told, and the operator gets requirements.txt + README.md.
func (p *PythonEnv) Prepare() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prepared {
		return
	}
	p.prepared = true

	if p.cfg.Mode == "venv" && p.cfg.EnvDir != "" && !pathExists(filepath.Join(p.cfg.EnvDir, "bin", "python3")) {
		base := p.cfg.Interpreter
		if base == "" {
			base = "python3"
		}
		out, err := runHostTimeout(base, 5*time.Minute, "-m", "venv", p.cfg.EnvDir)
		if err != nil {
			p.prepErr = fmt.Sprintf("创建虚拟环境失败（%s -m venv %s）: %v %s", base, p.cfg.EnvDir, err, firstLine(out))
			p.logf("[python] " + p.prepErr)
			return
		}
		p.logf("[python] 已创建虚拟环境:", p.cfg.EnvDir)
	}
	if p.cfg.Mode == "conda" {
		// 静默降级曾经掩盖了真实故障：配置写的是 conda base（那里
		// PIL/PyMuPDF 都在），实际却用了 PATH 上的 Homebrew python 3.14
		// （PEP 668 externally-managed），于是每次自动安装都失败、会话只能
		// 自己手搓 PGM 解析器。找不到 conda 就必须说清楚。
		if p.condaPrefix() == "" {
			p.logf("[python] 警告: 配置了 mode=conda 但没找到 conda（PATH/$CONDA_EXE/常见安装位置都没有）——"+
				"将回退到 PATH 上的 python3；想用 conda 请设 tools.python.interpreter 或 env_dir",
				"（已尝试:", findCondaBin(), "）")
		} else {
			p.logf("[python] conda 环境:", p.condaPrefix())
		}
	}
	if inter := p.Interpreter(); inter != "" {
		p.logf("[python] 会话 bash 使用:", inter)
	} else {
		p.prepErr = "宿主上没有找到 python3 解释器（检查 tools.python.interpreter / PATH）"
		p.logf("[python] " + p.prepErr)
		return
	}
	for _, mod := range p.cfg.Packages {
		if err := p.ensureModuleLocked(mod); err != nil {
			p.logf("[python] 预装依赖失败:", mod, err)
		}
	}
}

// MissingModule extracts the module name from a traceback in the command
// output ("" when the failure is not a missing module).
func (p *PythonEnv) MissingModule(output string) string {
	if p == nil {
		return ""
	}
	for _, line := range strings.Split(output, "\n") {
		if m := moduleErrorRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

var moduleErrorRe = regexp.MustCompile(`(?:ModuleNotFoundError|ImportError): No module named '?([A-Za-z0-9_.\-]+)'?`)

// NoteFor installs a module the session was missing and returns the text
// appended to the tool result ("" when nothing was done).
func (p *PythonEnv) NoteFor(output string) string {
	if p == nil {
		return ""
	}
	mod := p.MissingModule(output)
	if mod == "" {
		return ""
	}
	// Sub-modules ("PIL.Image") are installed through their root package.
	root := mod
	if i := strings.Index(root, "."); i > 0 {
		root = root[:i]
	}
	p.mu.Lock()
	already := p.installed[root]
	p.mu.Unlock()
	if already {
		return fmt.Sprintf("\n\n[python] %s 已安装（宿主侧）——请重试同一条命令。", root)
	}
	if !p.autoInstall() {
		path := p.WriteRequirements(fmt.Sprintf("会话缺少 Python 包 %s，且 tools.python.auto_install 为 false。", root))
		return fmt.Sprintf("\n\n[python] 缺少模块 %s；自动安装已关闭。请改用其它方式完成这一步，或让用户按 %s 安装。", root, path)
	}
	if err := p.ensureModule(root); err != nil {
		path := p.WriteRequirements(fmt.Sprintf("自动安装 %s 失败：%v", root, err))
		return fmt.Sprintf("\n\n[python] 缺少模块 %s，宿主侧自动安装失败：%v\n已写入 %s（requirements.txt + README.md，按其中的说明手动安装后重试）。",
			root, err, filepath.Dir(path))
	}
	return fmt.Sprintf("\n\n[python] 缺少模块 %s —— 宿主侧已自动安装（沙箱断网，会话自己装不了），请重试同一条命令。", root)
}

func (p *PythonEnv) ensureModule(mod string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ensureModuleLocked(mod)
}

func (p *PythonEnv) ensureModuleLocked(mod string) error {
	if p.installed[mod] {
		return nil
	}
	inter := p.Interpreter()
	if inter == "" {
		return fmt.Errorf("没有可用的 python 解释器")
	}
	// Verify first: the package may already be there (system python).
	if out, err := runHost(inter, "-c", "import "+mod); err == nil {
		p.installed[mod] = true
		_ = out
		return nil
	}
	base := []string{"-m", "pip", "install"}
	if p.cfg.Mode == "system" {
		base = append(base, "--user") // no root, and it is visible in the sandbox
	}
	if idx := strings.TrimSpace(p.cfg.PipIndexURL); idx != "" {
		// 显式源：不依赖宿主 ~/.config/pip/pip.conf 是否被改成可用的镜像。
		base = append(base, "-i", idx)
	}
	// The module name is NOT always the PyPI name: `pip install PIL` fails
	// with "No matching distribution found for PIL" (the project is
	// Pillow), and `pip install fitz` fetches an unrelated legacy package
	// (the real one is PyMuPDF). Map before installing.
	pkg := pipPackageName(mod)
	timeout := time.Duration(p.cfg.InstallTimeout) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	args := append(append([]string{}, base...), pkg)
	out, err := runHostTimeout(inter, timeout, args...)
	if err != nil && needsBreakSystemPackages(out) {
		// PEP 668 (Homebrew / Debian / most distro pythons): pip refuses to
		// touch the interpreter's site-packages. This interpreter is the
		// user's own brew python, so allow it explicitly instead of failing
		// — otherwise the session can never get the module and burns rounds
		// hand-rolling a PGM parser (observed in a real run).
		p.logf("[python] 解释器是 externally-managed，改用 --break-system-packages 重试:", pkg)
		args = append(append([]string{}, base...), "--break-system-packages", pkg)
		out, err = runHostTimeout(inter, timeout, args...)
	}
	if err != nil {
		p.failed[mod] = firstLine(out)
		p.logf("[python] 安装失败:", pkg, "(模块 "+mod+")", err, firstLine(out))
		return fmt.Errorf("%v %s", err, firstLine(out))
	}
	p.installed[mod] = true
	if pkg != mod {
		p.logf("[python] 已自动安装:", pkg, "(模块 "+mod+")")
	} else {
		p.logf("[python] 已自动安装:", mod)
	}
	return nil
}

// needsBreakSystemPackages reports the PEP 668 refusal that makes a
// plain `pip install` impossible on a managed interpreter.
func needsBreakSystemPackages(out string) bool {
	return strings.Contains(out, "externally-managed-environment")
}

// pipPackageName maps an importable module name to its PyPI distribution
// name. Only names that genuinely differ are listed; everything else is
// installed under its own name.
func pipPackageName(mod string) string {
	// 先整名查（google.protobuf 这类点号名本身就是包名），再退到根模块
	// （PIL.Image → PIL），最后按点号取第一段。
	if pkg, ok := pipPackageAliases[mod]; ok {
		return pkg
	}
	top := mod
	if i := strings.IndexAny(top, ".["); i > 0 {
		top = top[:i]
	}
	if pkg, ok := pipPackageAliases[top]; ok {
		return pkg
	}
	return top
}

// pipPackageAliases: module -> PyPI distribution (the classic traps).
var pipPackageAliases = map[string]string{
	"PIL":             "Pillow",
	"fitz":            "PyMuPDF",
	"cv2":             "opencv-python",
	"yaml":            "PyYAML",
	"sklearn":         "scikit-learn",
	"bs4":             "beautifulsoup4",
	"dateutil":        "python-dateutil",
	"dotenv":          "python-dotenv",
	"serial":          "pyserial",
	"OpenSSL":         "pyOpenSSL",
	"Cryptodome":      "pycryptodome",
	"Crypto":          "pycryptodome",
	"pkg_resources":   "setuptools",
	"google.protobuf": "protobuf",
	"mpl_toolkits":    "matplotlib",
	"pytesseract":     "pytesseract",
	"docx":            "python-docx",
	"pptx":            "python-pptx",
	"fpdf":            "fpdf2",
}

// WriteRequirements writes the manual-install fallback (same shape as
// the fonts README) and returns its path.
func (p *PythonEnv) WriteRequirements(reason string) string {
	if p == nil || p.report == "" {
		return ""
	}
	_ = os.MkdirAll(p.report, 0o755)
	p.mu.Lock()
	mods := make([]string, 0, len(p.installed)+len(p.failed))
	for m := range p.installed {
		mods = append(mods, m)
	}
	for m := range p.failed {
		if !p.installed[m] {
			mods = append(mods, m)
		}
	}
	failed := make(map[string]string, len(p.failed))
	for k, v := range p.failed {
		failed[k] = v
	}
	p.mu.Unlock()
	sort.Strings(mods)
	if len(p.cfg.Packages) > 0 {
		for _, m := range p.cfg.Packages {
			found := false
			for _, have := range mods {
				if have == m {
					found = true
					break
				}
			}
			if !found {
				mods = append(mods, m)
			}
		}
	}
	var b strings.Builder
	for _, m := range mods {
		b.WriteString(m + "\n")
	}
	reqPath := filepath.Join(p.report, "requirements.txt")
	_ = os.WriteFile(reqPath, []byte(b.String()), 0o644)

	inter := p.Interpreter()
	if inter == "" {
		inter = "python3"
	}
	var readme strings.Builder
	readme.WriteString("# Python 环境（会话 bash 用）\n\n")
	readme.WriteString(reason + "\n\n")
	readme.WriteString("会话 bash 在沙箱里运行（`--unshare-net` 断网），**会话自己无法安装任何包**；\n")
	readme.WriteString("DocVision 会在宿主侧按 `tools.python` 的配置自动安装缺失模块，失败时把清单写在这里。\n\n")
	readme.WriteString("## 手动安装\n\n```bash\n")
	if p.cfg.Mode == "venv" && p.cfg.EnvDir != "" {
		readme.WriteString(filepath.Join(p.cfg.EnvDir, "bin", "python3") + " -m pip install -r " + reqPath + "\n")
	} else if p.cfg.Mode == "conda" && p.cfg.EnvDir != "" {
		readme.WriteString("conda run -p " + p.cfg.EnvDir + " python -m pip install -r " + reqPath + "\n")
	} else {
		readme.WriteString(inter + " -m pip install --user -r " + reqPath + "\n")
	}
	readme.WriteString("```\n\n## 本次记录\n\n")
	readme.WriteString("- 解释器：`" + inter + "`\n")
	readme.WriteString("- 模式：`" + p.cfg.Mode + "`\n")
	if p.cfg.EnvDir != "" {
		readme.WriteString("- 环境目录：`" + p.cfg.EnvDir + "`\n")
	}
	if len(failed) > 0 {
		readme.WriteString("\n### 安装失败\n\n")
		names := make([]string, 0, len(failed))
		for m := range failed {
			names = append(names, m)
		}
		sort.Strings(names)
		for _, m := range names {
			readme.WriteString("- `" + m + "`：" + failed[m] + "\n")
		}
	}
	readmePath := filepath.Join(p.report, "README.md")
	_ = os.WriteFile(readmePath, []byte(readme.String()), 0o644)
	return readmePath
}

// systemSiteDirs lists the site-packages directories of a system
// interpreter (including the --user one), so --user installs are visible
// inside the sandbox.
func systemSiteDirs(inter string) []string {
	if inter == "" {
		return nil
	}
	out, err := runHost(inter, "-c", "import site,sys;print('\\n'.join(site.getsitepackages()+[site.getusersitepackages()]))")
	if err != nil {
		return nil
	}
	var dirs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !pathExists(line) {
			continue
		}
		if real, err := filepath.EvalSymlinks(line); err == nil {
			line = real
		}
		dirs = append(dirs, line)
	}
	return dirs
}

func (p *PythonEnv) logf(parts ...any) {
	if p != nil && p.log != nil {
		p.log.Log(1, append([]any{"[python]"}, parts...)...)
	}
}

func runHost(bin string, args ...string) (string, error) {
	return runHostTimeout(bin, 2*time.Minute, args...)
}

func runHostTimeout(bin string, timeout time.Duration, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(out), err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return string(out), fmt.Errorf("超时（%s）", timeout)
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
