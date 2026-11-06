package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// 续跑判定（T300/T301）：样式阶段已经收过账、试转已经交过产物时不要重来。
// 两个判据都只认磁盘事实——转录收账 + 产物在盘上。
func TestStylePackageDone(t *testing.T) {
	proj := t.TempDir()
	r := &Runner{}
	styleDir := filepath.Join(proj, "style")
	if err := os.MkdirAll(styleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		p := filepath.Join(proj, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("style/demo.cls", "\\ProvidesClass{demo}\n")
	write("style/manual.md", "# manual\n")
	write("style/example.tex", "\\documentclass{demo}\n")

	// 只有包、试点还没开始：样式阶段仍应正常跑（可能是上一轮把包交了、试点没起来）。
	if r.stylePackageDone(proj) {
		t.Fatal("试点没开始时不该判定样式阶段已完成")
	}
	write("work/style/pilot/spec.json", "{}")
	if !r.stylePackageDone(proj) {
		t.Fatal("包齐 + 试点已开始时应判定样式阶段已完成")
	}
	// pilot.done（试点整轮跑完）也算同样证据。
	os.Remove(filepath.Join(proj, "work", "style", "pilot", "spec.json"))
	write("work/style/pilot.done", "done\n")
	if !r.stylePackageDone(proj) {
		t.Fatal("pilot.done 在时应判定样式阶段已完成")
	}
	// 包缺件就不算：不能拿半份包往下走。
	os.Remove(filepath.Join(styleDir, "example.tex"))
	if r.stylePackageDone(proj) {
		t.Fatal("样式包缺 example.tex 时不该判定已完成")
	}
}

// 没有任何试转转录时直接判否，不去碰编译器。
func TestPilotTrialPrevRunWithoutTranscript(t *testing.T) {
	proj := t.TempDir()
	r := &Runner{}
	if r.pilotTrialPrevRun(proj, filepath.Join(proj, "work", "style", "pilot", "work")) {
		t.Fatal("没有试转转录时不该判定上一场试转可沿用")
	}
}
