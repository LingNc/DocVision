package latex

import "testing"

// T84：模型捕获 ImportError 后只打印 str(e)（无异常类名前缀）时也要识别。
func TestMissingModuleBareStr(t *testing.T) {
	p := &PythonEnv{}
	got := p.MissingModule("/usr/bin/mutool\nno pypdf No module named 'pypdf'\n")
	if got != "pypdf" {
		t.Fatalf("bare str(e) 应识别 pypdf，得到 %q", got)
	}
	// 标准 traceback 不受影响
	got = p.MissingModule("Traceback...\nModuleNotFoundError: No module named 'PIL'\n")
	if got != "PIL" {
		t.Fatalf("标准 traceback: %q", got)
	}
	// 普通输出不误伤
	if got := p.MissingModule("hello world\n"); got != "" {
		t.Fatalf("普通输出应返回空: %q", got)
	}
}
