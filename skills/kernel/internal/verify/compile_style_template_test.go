package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// SK-I13：style/draw-style.tex 存在 → 主文件必须引用模板；不存在 → 跳过（老项目兼容）。
func TestStyleTemplateEnforcement(t *testing.T) {
	root := t.TempDir()
	tk := &task.Task{ID: "drw_x", Type: task.TypeDraw, Sandbox: "tasks/drw_x"}
	tk.Accept = map[string]any{"main_file": "main.tex", "require_compile": true}
	tk.Sandbox = "tasks/drw_x"
	os.MkdirAll(filepath.Join(root, tk.Sandbox, "out"), 0o755)
	os.WriteFile(filepath.Join(root, tk.Sandbox, "out", "main.tex"), []byte("\\documentclass{article}\n\\begin{document}x\\end{document}"), 0o644)
	c := &Checker{ProjectRoot: root}

	// 无模板：不拦
	if errs, _ := c.Run(tk); len(errs) != 0 {
		t.Fatalf("无模板不应拦: %v", errs)
	}
	// 有模板但不引用：打回
	os.MkdirAll(filepath.Join(root, "style"), 0o755)
	os.WriteFile(filepath.Join(root, "style", "draw-style.tex"), []byte("% tpl"), 0o644)
	errs, _ := c.Run(tk)
	found := false
	for _, e := range errs {
		if filepath.Clean(e) == filepath.Clean("绘图风格模板 style/draw-style.tex 已产出但主文件未引用（须 \\input 该模板或用其 \\dvLine 宏）——逐图散写一律打回") {
			found = true
		}
	}
	_ = found
	if len(errs) == 0 {
		t.Fatal("有模板未引用应打回")
	}
	// 引用了：放行
	os.WriteFile(filepath.Join(root, tk.Sandbox, "out", "main.tex"), []byte("\\input{draw-style}\n\\begin{document}\\dvLineMain\\end{document}"), 0o644)
	if errs, _ := c.Run(tk); len(errs) != 0 {
		t.Fatalf("引用模板后应放行: %v", errs)
	}
}
