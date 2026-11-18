package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
)

func TestGenConvert(t *testing.T) {
	root := t.TempDir()
	chDir := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"chapter_001.md", "chapter_002.md"} {
		if err := os.WriteFile(filepath.Join(chDir, n), []byte("# 第一章\n正文"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	rep, err := GenConvert(q, root, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 2 || rep.TasksTotal != 2 {
		t.Fatalf("应新入队 2: %+v", rep)
	}
	tk, _ := q.Get("conv_chapter_001")
	if tk.Accept["main_file"] != "chapter_001.tex" || tk.Accept["require_compile"] != true {
		t.Fatalf("验收 spec 错: %+v", tk.Accept)
	}
	// 输入已拷进沙箱（覆盖率对照源）
	data, err := os.ReadFile(filepath.Join(root, "tasks/conv_chapter_001/inputs/chapter.md"))
	if err != nil || !strings.Contains(string(data), "第一章") {
		t.Fatalf("inputs/chapter.md 应已就位: %v", err)
	}
	// 幂等：重生成 Added=0、总数不变
	rep2, err := GenConvert(q, root, "chapters")
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Added != 0 || rep2.TasksTotal != 2 {
		t.Fatalf("重复生成应幂等: %+v", rep2)
	}
}

func TestGenConvertMissingDir(t *testing.T) {
	root := t.TempDir()
	q, _ := queue.Load(filepath.Join(root, "q.json"))
	rep, err := GenConvert(q, root, "nope/chapters")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 {
		t.Fatalf("缺目录应零新增: %+v", rep)
	}
}
