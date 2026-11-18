package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

func sandboxWithVerdict(t *testing.T, content string) (string, *task.Task) {
	t.Helper()
	root := t.TempDir()
	tk := &task.Task{ID: "t1", Sandbox: "tasks/t1", Accept: map[string]any{}}
	outDir := filepath.Join(root, "tasks/t1/out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(outDir, "verdict.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, tk
}

func TestClassifySchema(t *testing.T) {
	c := &Checker{}

	// 缺 verdict.json
	root, tk := sandboxWithVerdict(t, "")
	tk.Accept["verdict_schema"] = "classify"
	errs, _ := c.Run(tk)
	if len(errs) == 0 || !contains(errs, "verdict.json") {
		t.Fatalf("缺 verdict.json 应打回: %v", errs)
	}

	// styled_text 空提取 → 打回（不降级判据）
	root, tk = sandboxWithVerdict(t, `{"class":"styled_text"}`)
	tk.Accept["verdict_schema"] = "classify"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "extracted") {
		t.Fatalf("空提取应打回: %v", errs)
	}

	// styled_text 有提取 → 过
	root, tk = sandboxWithVerdict(t, `{"class":"styled_text","extracted":"第一章 引论"}`)
	tk.Accept["verdict_schema"] = "classify"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("合法 styled_text 应通过: %v", errs)
	}

	// vector 无证据 → 打回
	root, tk = sandboxWithVerdict(t, `{"class":"vector"}`)
	tk.Accept["verdict_schema"] = "classify"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "reason") {
		t.Fatalf("vector 无证据应打回: %v", errs)
	}

	// 乱填 class → 打回（不合法值不被静默接受）
	root, tk = sandboxWithVerdict(t, `{"class":"huh"}`)
	tk.Accept["verdict_schema"] = "classify"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "不合法") {
		t.Fatalf("乱填 class 应打回: %v", errs)
	}
}

func TestCheckSchema(t *testing.T) {
	c := &Checker{}

	// pass → 过
	root, tk := sandboxWithVerdict(t, `{"result":"pass"}`)
	tk.Accept["verdict_schema"] = "check"
	c.ProjectRoot = root
	errs, _ := c.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("pass 应通过: %v", errs)
	}

	// issues 无位置指针 → 打回
	root, tk = sandboxWithVerdict(t, `{"result":"issues","issues":["图没画完"]}`)
	tk.Accept["verdict_schema"] = "check"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "位置指针") {
		t.Fatalf("无位置指针应打回: %v", errs)
	}

	// issues 带位置指针 → 过
	root, tk = sandboxWithVerdict(t, `{"result":"issues","issues":["第3节公式漏排（chapter_003.md 第3节 ↔ ch003.tex）"]}`)
	tk.Accept["verdict_schema"] = "check"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("带位置指针的 issues 应通过: %v", errs)
	}

	// issues 空清单 → 打回
	root, tk = sandboxWithVerdict(t, `{"result":"issues","issues":[]}`)
	tk.Accept["verdict_schema"] = "check"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "为空清单") {
		t.Fatalf("空 issues 应打回: %v", errs)
	}

	// no_verdict → 打回但允许重试（attempts 上限兜底）
	root, tk = sandboxWithVerdict(t, `{"result":"no_verdict"}`)
	tk.Accept["verdict_schema"] = "check"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if len(errs) == 0 {
		t.Fatal("no_verdict 不应记 done")
	}
}

func contains(errs []string, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

func TestImg2TextVerdict(t *testing.T) {
	c := &Checker{}

	// text：无 replacement.md 打回
	root, tk := sandboxWithVerdictImg(t, `{"result":"text"}`)
	tk.Accept["verdict_schema"] = "img2text"
	c.ProjectRoot = root
	errs, _ := c.Run(tk)
	if !contains(errs, "replacement.md") {
		t.Fatalf("text 无产物应打回: %v", errs)
	}
	// text：有产物 → 过
	root, tk = sandboxWithVerdictImg(t, `{"result":"text"}`)
	tk.Accept["verdict_schema"] = "img2text"
	os.MkdirAll(filepath.Join(root, tk.Sandbox, "out"), 0o755)
	os.WriteFile(filepath.Join(root, tk.Sandbox, "out", "replacement.md"), []byte("提取文字"), 0o644)
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("text 有产物应过: %v", errs)
	}
	// keep：无产物也过
	root, tk = sandboxWithVerdictImg(t, `{"result":"keep"}`)
	tk.Accept["verdict_schema"] = "img2text"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if len(errs) != 0 {
		t.Fatalf("keep 应过: %v", errs)
	}
	// 乱填 result 打回
	root, tk = sandboxWithVerdictImg(t, `{"result":"whatever"}`)
	tk.Accept["verdict_schema"] = "img2text"
	c.ProjectRoot = root
	errs, _ = c.Run(tk)
	if !contains(errs, "不合法") {
		t.Fatalf("乱填应打回: %v", errs)
	}
}

func sandboxWithVerdictImg(t *testing.T, verdict string) (string, *task.Task) {
	t.Helper()
	root := t.TempDir()
	tk := &task.Task{ID: "img2txt_9", Sandbox: "tasks/img2txt_9", Accept: map[string]any{}}
	outDir := filepath.Join(root, "tasks/img2txt_9/out")
	os.MkdirAll(outDir, 0o755)
	os.WriteFile(filepath.Join(outDir, "verdict.json"), []byte(verdict), 0o644)
	return root, tk
}
