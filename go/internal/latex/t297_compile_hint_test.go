package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T297：插图名是 64 位哈希，模型手打错一个尾巴时，编译回执要直接给出
// 工作区里真实的名字，让它照抄，而不是再猜一次。
func TestMissingFileHintsPointsAtRealName(t *testing.T) {
	dir := t.TempDir()
	imgDir := filepath.Join(dir, "images", "book")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := "70e71ab70935e0ac710f9d99fe466f33dcc7701fea6b240d9efaad490c916217.jpg"
	if err := os.WriteFile(filepath.Join(imgDir, real), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrong := "70e71ab70935e0ac710f9d99fe46668f56b40b805d3943dd6662d797ce10de8e01.jpg"
	errText := "! LaTeX Error: File `images/book/" + wrong + "' not found.\n" +
		"LaTeX Warning: File `images/book/" + wrong + "' not found on input line 225.\n"

	hint := missingFileHints(errText, dir)
	if hint == "" {
		t.Fatal("应当给出候选名字")
	}
	if !strings.Contains(hint, real) {
		t.Fatalf("提示里应当有真实文件名，actual=%q", hint)
	}
	if strings.Count(hint, "→") != 1 {
		t.Fatalf("同一个缺失文件只该列一次，actual=%q", hint)
	}
}

// 目录里没有任何相近的名字时不要乱指认。
func TestMissingFileHintsSilentWithoutCandidate(t *testing.T) {
	dir := t.TempDir()
	imgDir := filepath.Join(dir, "images")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, "a.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	errText := "! LaTeX Error: File `images/zzzzzzzzzzzzzzzz.jpg' not found.\n"
	if hint := missingFileHints(errText, dir); hint != "" {
		t.Fatalf("没有相似候选时不该给提示，actual=%q", hint)
	}
}

// 目录本身不存在（比如挂载点写错）时也不能崩，只当没有提示。
func TestMissingFileHintsMissingDir(t *testing.T) {
	dir := t.TempDir()
	errText := "! I can't find file `images/nope/x.jpg'.\n"
	if hint := missingFileHints(errText, dir); hint != "" {
		t.Fatalf("目录不存在时不该给提示，actual=%q", hint)
	}
}
