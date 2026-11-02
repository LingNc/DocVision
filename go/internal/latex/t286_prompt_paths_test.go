package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/prompts"
)

// T286：评估会话的 bash 里有两个挂载点、两套 chapters——提示词必须把
// "可读的 .tex 在 project:work/chapters" 与 "/work 只是自己的草稿区" 说清楚，
// 否则会话会去 /work/chapters 找（真实运行里连试两次才转向 /project）。
func TestAssessPromptNamesBothMounts(t *testing.T) {
	text := prompts.Must(prompts.StyleFixAssessSystem)
	for _, want := range []string{
		"project:converted/<name>.tex",
		"/project/converted/",
		"/project/style",
		"/work is only your scratch area",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("评估提示词缺少 %q", want)
		}
	}
	// 不能再出现 work/chapters——那正是会话跑去 /work/chapters 找的来源。
	if strings.Contains(text, "work/chapters") {
		t.Fatal("评估提示词里不该再出现 work/chapters（窄视图里叫 converted）")
	}
}

// 分块会话：细分章节的正文都在它自己的工作区里（bash 的 /work），
// 说明里带上这个前缀，免得它去 /project 找原件。
func TestSubdividedBlockNoteNamesWorkspace(t *testing.T) {
	proj := t.TempDir()
	// 造一个"细分父章节"：<章>/children/ 存在。
	children := filepath.Join(proj, "work", "chapters", "chapter_001", "children")
	if err := os.MkdirAll(children, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(children, "child.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := subdividedBlockNote(proj, []string{"chapter_001", "chapter_002"})
	if !strings.Contains(note, "chapters/chapter_001/children/") {
		t.Fatalf("应当点到正文目录: %q", note)
	}
	if !strings.Contains(note, "/work/chapters/<name>/children/") {
		t.Fatalf("应当给出 bash 里可编辑副本的路径: %q", note)
	}
	if strings.Contains(note, "chapter_002") {
		t.Fatalf("不是细分父章节的不该被点到: %q", note)
	}
}
