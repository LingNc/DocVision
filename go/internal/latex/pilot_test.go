package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T97：styleDigest 对 cls/manual 的任何变化敏感（决定首章产物是否作废）。
func TestStyleDigestSensitivity(t *testing.T) {
	proj := t.TempDir()
	style := filepath.Join(proj, "style")
	if err := os.MkdirAll(style, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(cls, manual string) {
		_ = os.WriteFile(filepath.Join(style, "book.cls"), []byte(cls), 0o644)
		_ = os.WriteFile(filepath.Join(style, "manual.md"), []byte(manual), 0o644)
	}
	write("cls-v1", "manual-v1")
	d1 := styleDigest(proj)
	write("cls-v1", "manual-v1")
	if styleDigest(proj) != d1 {
		t.Fatal("内容没变指纹必须稳定")
	}
	write("cls-v2", "manual-v1")
	if styleDigest(proj) == d1 {
		t.Fatal("cls 变了指纹必须变")
	}
	d2 := styleDigest(proj)
	write("cls-v2", "manual-v2")
	if styleDigest(proj) == d2 {
		t.Fatal("manual 变了指纹必须变")
	}
}

// T97：pilot marker 存在即跳过（中断续跑不重复试转）。
func TestPilotMarkerSkips(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(pilotMarkerPath(proj)), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}
	r.pilotPhase(proj) // 无章节无样式：静默返回，不写 marker
	if fileExists(pilotMarkerPath(proj)) {
		t.Fatal("没有章节时不应写 marker")
	}
	if err := os.WriteFile(pilotMarkerPath(proj), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.pilotPhase(proj) // marker 存在：直接返回（无章节也不报错）
}
