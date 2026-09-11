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
	if !strings.Contains(string(calls), "pip install --user PIL") {
		t.Fatalf("host must run pip install --user PIL, got:\n%s", calls)
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
