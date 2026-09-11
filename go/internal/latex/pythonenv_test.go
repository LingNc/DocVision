package latex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
)

// A session bash hits ModuleNotFoundError (the sandbox has no network),
// the HOST installs the package and the model is told to retry.
func TestPythonEnvAutoInstallsMissingModule(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "pip.log")
	shim := filepath.Join(dir, "python3")
	// 假解释器：记录调用，import 永远失败（模拟没装），pip install 成功。
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"if [ \"$1\" = \"-c\" ]; then exit 1; fi\n" +
		"exit 0\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := NewPythonEnv(config.ToolsPythonConfig{
		Mode: "system", Interpreter: shim,
	}, filepath.Join(dir, "report"), nil)

	out := "Traceback (most recent call last):\n  File \"x.py\", line 1, in <module>\n    from PIL import Image\nModuleNotFoundError: No module named 'PIL'\n"
	if got := env.MissingModule(out); got != "PIL" {
		t.Fatalf("MissingModule = %q, want PIL", got)
	}
	note := env.NoteFor(out)
	if !strings.Contains(note, "已自动安装") || !strings.Contains(note, "重试") {
		t.Fatalf("note must tell the model it was installed and to retry, got %q", note)
	}
	calls, _ := os.ReadFile(log)
	// 模块名 PIL，PyPI 包名 Pillow：`pip install PIL` 永远装不上。
	if !strings.Contains(string(calls), "pip install --user Pillow") {
		t.Fatalf("host must run pip install --user Pillow, got:\n%s", calls)
	}
	// 同一模块第二次只提醒重试，不再重复安装。
	before := strings.Count(string(calls), "pip install")
	if again := env.NoteFor(out); !strings.Contains(again, "已安装") {
		t.Fatalf("second note = %q", again)
	}
	calls2, _ := os.ReadFile(log)
	if strings.Count(string(calls2), "pip install") != before {
		t.Fatalf("must not install twice:\n%s", calls2)
	}
	// 子模块按根包安装。
	if got := env.MissingModule("ModuleNotFoundError: No module named 'matplotlib.pyplot'"); got != "matplotlib.pyplot" {
		t.Fatalf("MissingModule = %q", got)
	}
	if note := env.NoteFor("ModuleNotFoundError: No module named 'matplotlib.pyplot'"); !strings.Contains(note, "matplotlib") {
		t.Fatalf("submodule note = %q", note)
	}
	if got := env.MissingModule("everything is fine"); got != "" {
		t.Fatalf("clean output must not report a module, got %q", got)
	}
}

// When installation fails the operator gets the fonts-style fallback:
// requirements.txt + README.md next to the project.
func TestPythonEnvWritesRequirementsOnFailure(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "python3")
	script := "#!/bin/sh\nif [ \"$1\" = \"-c\" ]; then exit 1; fi\necho 'ERROR: no network' >&2\nexit 1\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "work", "python")
	env := NewPythonEnv(config.ToolsPythonConfig{
		Mode: "system", Interpreter: shim, Packages: []string{"numpy"},
	}, report, nil)

	note := env.NoteFor("ModuleNotFoundError: No module named 'scipy'")
	if !strings.Contains(note, "自动安装失败") || !strings.Contains(note, "requirements.txt") || !strings.Contains(note, report) {
		t.Fatalf("note must report the failure and the fallback file, got %q", note)
	}
	req, err := os.ReadFile(filepath.Join(report, "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(req), "scipy") {
		t.Fatalf("requirements.txt must list the missing module, got %q", req)
	}
	readme, err := os.ReadFile(filepath.Join(report, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pip install", "unshare-net", "scipy", "python3"} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("README missing %q:\n%s", want, readme)
		}
	}
}

// The environment must be visible inside the sandbox at the SAME path
// (a venv/conda prefix is not relocatable) with PATH pointing at it.
func TestPythonEnvSandboxBindsInterpreter(t *testing.T) {
	envDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(envDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 真实的 venv：bin/python3 是指向基础解释器的软链。
	base, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	if err := os.Symlink(base, filepath.Join(envDir, "bin", "python3")); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	env := NewPythonEnv(config.ToolsPythonConfig{
		Mode: "venv", EnvDir: envDir,
	}, "", nil)
	args := env.BindArgs()
	if len(args) < 3 || args[0] != "--ro-bind" || args[1] != envDir || args[2] != envDir {
		t.Fatalf("venv must be bound read-only at its own path, got %v", args)
	}
	vars := env.EnvVars(defaultSandboxPath)
	if len(vars) == 0 || vars[0][0] != "PATH" || !strings.HasPrefix(vars[0][1], filepath.Join(envDir, "bin")+":") {
		t.Fatalf("PATH must start with the env bin, got %v", vars)
	}

	tool := &WorkBashTool{
		Root: work, Mounts: []Mount{{Name: "work", Dir: work, Writable: true}},
		Python: env,
	}
	sandbox := strings.Join(tool.sandboxArgs("python3 -c pass", false), " ")
	if !strings.Contains(sandbox, "--ro-bind "+envDir+" "+envDir) {
		t.Fatalf("sandbox must bind the environment:\n%s", sandbox)
	}
	if !strings.Contains(sandbox, "--setenv PATH "+filepath.Join(envDir, "bin")+":") {
		t.Fatalf("sandbox PATH must find the environment:\n%s", sandbox)
	}
	// 描述里必须出现 python 状态：模型靠它决定能不能用 python3。
	if desc := env.Describe(); !strings.Contains(desc, "Python is available") || !strings.Contains(desc, env.Interpreter()) {
		t.Fatalf("Describe = %q", desc)
	}
	if !strings.Contains(tool.Definition()["function"].(map[string]any)["description"].(string), "Python is available") {
		t.Fatal("bash definition must advertise the python environment")
	}
}

// tools.python.enabled=false must remove every trace of python.
func TestPythonEnvDisabled(t *testing.T) {
	off := false
	cfg := config.ToolsPythonConfig{Enabled: &off}
	if env := NewPythonEnv(cfg, "", nil); env != nil {
		t.Fatalf("disabled env must be nil, got %+v", env)
	}
	on := true
	cfg = config.ToolsPythonConfig{Enabled: &on, Interpreter: "/nonexistent/python3", Mode: "system"}
	env := NewPythonEnv(cfg, "", nil)
	if env == nil {
		t.Fatal("enabled env must exist")
	}
	if b := env.BindArgs(); len(b) != 0 {
		t.Fatalf("system interpreter needs no bind, got %v", b)
	}
	// 没有解释器时不编造能力：描述必须说清不可用。
	env.prepErr = "宿主上没有找到 python3 解释器"
	if desc := env.Describe(); !strings.Contains(desc, "NOT usable") {
		t.Fatalf("Describe = %q", desc)
	}
}

// End-to-end in the REAL sandbox: python3 must actually run inside
// bubblewrap (this host's interpreter is outside /usr, which the
// sandbox does not bind by default).
func TestPythonEnvRunsInsideRealSandbox(t *testing.T) {
	if !bwrapAvailable() {
		t.Skip("bwrap not installed")
	}
	inter, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	dir := t.TempDir()
	env := NewPythonEnv(config.ToolsPythonConfig{Mode: "system", Interpreter: inter}, "", nil)
	tool := &WorkBashTool{
		Root: dir, Mounts: []Mount{{Name: "work", Dir: dir, Writable: true}},
		Sandbox: true, Python: env, Log: nil, Tid: 1,
	}
	res, err := tool.Execute(`{"command":"python3 -c \"print('PY_OK')\"","timeout":60}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "PY_OK") {
		t.Fatalf("python3 must run inside the sandbox, got: %s", res.Text)
	}
}

// A real venv (created by python3 -m venv) must run inside the sandbox
// with its own interpreter — that is the isolation mode the environment
// is meant to provide, and a venv bin/python3 is a symlink to the base
// interpreter, which the sandbox does not bind by default.
func TestPythonEnvVenvRunsInsideRealSandbox(t *testing.T) {
	if !bwrapAvailable() {
		t.Skip("bwrap not installed")
	}
	base, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	dir := t.TempDir()
	envDir := filepath.Join(dir, "venv")
	if out, err := exec.Command(base, "-m", "venv", envDir).CombinedOutput(); err != nil {
		t.Skipf("cannot create a venv here: %v %s", err, firstLine(string(out)))
	}
	env := NewPythonEnv(config.ToolsPythonConfig{Mode: "venv", EnvDir: envDir}, filepath.Join(dir, "report"), nil)
	env.Prepare()
	if env.prepErr != "" {
		t.Fatalf("prepare failed: %s", env.prepErr)
	}
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &WorkBashTool{
		Root: work, Mounts: []Mount{{Name: "work", Dir: work, Writable: true}},
		Sandbox: true, Python: env, Tid: 1,
	}
	// sys.prefix 必须是这个 venv（证明跑的是配置的环境而不是系统 python）。
	res, err := tool.Execute(`{"command":"python3 -c \"import sys;print('VENV', sys.prefix, sys.executable)\" 2>&1","timeout":60}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "VENV "+envDir) {
		t.Fatalf("inside the sandbox python must be the configured venv, got: %s", res.Text)
	}
	// 宿主侧装的包必须真的能在沙箱里 import（否则"自动装好了"是假的）。
	res2, err := tool.Execute(`{"command":"python3 -c \"import setuptools;print('PKG_OK')\"","timeout":60}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res2.Text, "PKG_OK") {
		t.Fatalf("a package present in the venv must import inside the sandbox, got: %s", res2.Text)
	}
}

// mode=conda 全空 = 用 conda 的 base 环境（用户本地本来就有 base）。
// 这里用一个假 conda（PATH 上的 shim）验证解析：`conda info --base` 给前缀，
// 解释器就是 <base>/bin/python3。
func TestPythonEnvCondaBaseDefault(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "miniconda3")
	if err := os.MkdirAll(filepath.Join(base, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	py := filepath.Join(base, "bin", "python3")
	if err := os.WriteFile(py, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "shim")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\nif [ \"$1\" = info ]; then echo " + base + "; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "conda"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	resetCondaCache() // 进程级缓存：别让别的用例影响这一条

	env := NewPythonEnv(config.ToolsPythonConfig{Mode: "conda"}, filepath.Join(dir, "report"), nil)
	if got := env.Interpreter(); got != py {
		t.Fatalf("conda base 解释器 = %q, want %q", got, py)
	}
	if got := env.PrefixDir(); got != base {
		t.Fatalf("conda base 前缀 = %q, want %q", got, base)
	}
	if got := env.BinDir(); got != filepath.Join(base, "bin") {
		t.Fatalf("conda base bin = %q", got)
	}
	// conda_env: base 与留空等价。
	resetCondaCache()
	env2 := NewPythonEnv(config.ToolsPythonConfig{Mode: "conda", CondaEnv: "base"}, filepath.Join(dir, "report"), nil)
	if got := env2.PrefixDir(); got != base {
		t.Fatalf("conda_env=base 前缀 = %q, want %q", got, base)
	}
	// 有名字的环境走 <base>/envs/<name>。
	named := filepath.Join(base, "envs", "docvision")
	if err := os.MkdirAll(filepath.Join(named, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	resetCondaCache()
	env3 := NewPythonEnv(config.ToolsPythonConfig{Mode: "conda", CondaEnv: "docvision"}, filepath.Join(dir, "report"), nil)
	if got := env3.PrefixDir(); got != named {
		t.Fatalf("conda_env=docvision 前缀 = %q, want %q", got, named)
	}
}

// 模块名 ≠ PyPI 包名：`pip install PIL` 报 "No matching distribution
// found for PIL"（真名 Pillow），`pip install fitz` 装的是一个无关的旧包
// （真名 PyMuPDF）。现场样式会话就是这么把 PIL 装失败的。
func TestPipPackageNameMapping(t *testing.T) {
	cases := map[string]string{
		"PIL": "Pillow", "PIL.Image": "Pillow", "PIL.ImageDraw": "Pillow",
		"fitz": "PyMuPDF", "cv2": "opencv-python", "yaml": "PyYAML",
		"sklearn": "scikit-learn", "bs4": "beautifulsoup4",
		"google.protobuf": "protobuf", "matplotlib.pyplot": "matplotlib",
		"numpy": "numpy", "sympy": "sympy", "requests": "requests",
	}
	for mod, want := range cases {
		if got := pipPackageName(mod); got != want {
			t.Errorf("pipPackageName(%q) = %q, want %q", mod, got, want)
		}
	}
	if !needsBreakSystemPackages("error: externally-managed-environment") {
		t.Error("PEP 668 拒绝必须被识别")
	}
	if needsBreakSystemPackages("ERROR: No matching distribution found for PIL") {
		t.Error("普通失败不该被当成 PEP 668")
	}
}

// PEP 668（Homebrew/Debian 的 python）拒绝写入：改用
// --break-system-packages 重试一次，并且用映射后的包名。
func TestPythonEnvBreakSystemPackagesRetry(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "pip.log")
	shim := filepath.Join(dir, "python3")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"if [ \"$1\" = \"-c\" ]; then exit 1; fi\n" +
		"case \"$*\" in *break-system-packages*) exit 0;; esac\n" +
		"echo 'error: externally-managed-environment' >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := NewPythonEnv(config.ToolsPythonConfig{Mode: "system", Interpreter: shim},
		filepath.Join(dir, "report"), nil)
	note := env.NoteFor("ModuleNotFoundError: No module named 'fitz'\n")
	if !strings.Contains(note, "已自动安装") {
		t.Fatalf("PEP 668 之后必须重试成功并通知模型，note = %q", note)
	}
	calls, _ := os.ReadFile(log)
	txt := string(calls)
	if !strings.Contains(txt, "pip install --user PyMuPDF") {
		t.Fatalf("第一次应安装映射后的包名 PyMuPDF，got:\n%s", txt)
	}
	if !strings.Contains(txt, "--break-system-packages PyMuPDF") {
		t.Fatalf("第二次应带 --break-system-packages 重试，got:\n%s", txt)
	}
}

// conda 不在 PATH 上（服务方式启动没有 ~/.bashrc 的 conda init）时，仍要
// 在常见安装位置找到它——现场就是在这里静默降级成了 Homebrew python 3.14。
func TestCondaFoundOutsidePATH(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "miniconda3")
	if err := os.MkdirAll(filepath.Join(base, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\nif [ \"$1\" = info ]; then echo " + base + "; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(base, "bin", "conda"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	py := filepath.Join(base, "bin", "python3")
	if err := os.WriteFile(py, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// PATH 里没有 conda；HOME 指向假家目录。
	empty := filepath.Join(dir, "emptybin")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", empty)
	t.Setenv("HOME", dir)
	t.Setenv("CONDA_EXE", "")
	resetCondaCache()

	if got := findCondaBin(); got != filepath.Join(base, "bin", "conda") {
		t.Fatalf("findCondaBin = %q, want %q", got, filepath.Join(base, "bin", "conda"))
	}
	env := NewPythonEnv(config.ToolsPythonConfig{Mode: "conda"}, filepath.Join(dir, "report"), nil)
	if got := env.Interpreter(); got != py {
		t.Fatalf("conda 解释器 = %q, want %q", got, py)
	}
	if got := env.PrefixDir(); got != base {
		t.Fatalf("conda 前缀 = %q, want %q", got, base)
	}
	// 完全找不到 conda 时必须能被上层识别为"没解析出来"（好打印警告）。
	resetCondaCache()
	none := filepath.Join(dir, "nohome")
	if err := os.MkdirAll(none, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", none)
	resetCondaCache()
	if got := findCondaBin(); got != "" {
		t.Fatalf("没有 conda 时 findCondaBin = %q, want 空", got)
	}
	env2 := NewPythonEnv(config.ToolsPythonConfig{Mode: "conda"}, filepath.Join(dir, "report"), nil)
	if got := env2.condaPrefix(); got != "" {
		t.Fatalf("没找到 conda 时前缀应为空（触发警告），got %q", got)
	}
}
