package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T201：子章节转换完、父节点还没整合时，产物在**父工作区的临时树**里，不在
// work/chapters 下。以前 chapterProductsFor 只认 work/chapters，于是"核对也跑完了，
// 点重新核对却说这一章还没有提交产物"，产品明明在临时树里等父节点整合。
func TestChapterProductsForChildInTempTree(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_014_child_04"
	parentWork := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work")

	// ① 父节点收集孩子产物的暂存池。
	staged := filepath.Join(parentWork, "chapter_014", "children", base+".tex")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("\\section{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tex, parts := chapterProductsFor(proj, base); tex != staged {
		t.Fatalf("应找到暂存池里的产物，得到 %q（分片 %q）", tex, parts)
	}

	// ② 自己那一支节点里的产物（这一支还要继续细分时落在 integrated-stage）。
	if err := os.Remove(staged); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(parentWork, "_subdivision", "child_04", "node", "integrated-stage", base+".tex")
	if err := os.MkdirAll(filepath.Dir(node), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(node, []byte("\\section{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tex, parts := chapterProductsFor(proj, base); tex != node || parts != filepath.Dir(node)+"/"+base {
		t.Fatalf("应找到节点产物，得到 %q（分片 %q）", tex, parts)
	}

	// ③ children.tmp 里的半成品不算（暂存目录里也有同名副本）。
	if err := os.Remove(node); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(parentWork, "chapter_014", "children.tmp", base+".tex")
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("half\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tex, _ := chapterProductsFor(proj, base); fileExists(tex) {
		t.Fatalf("children.tmp 的半成品不该当成已提交: %q", tex)
	}
}

// T201：派发前置条件对子章节要认"细分树里的 md/产物"——子章节没有
// chapters/<章>.md，以前请求会永远停在"章节 markdown 还不存在"。
func TestRetryPreconditionChildInTempTree(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_014_child_04"
	if err := os.MkdirAll(filepath.Join(proj, "style"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "style", "bookstyle.cls"), []byte("\\ProvidesClass{bookstyle}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parentWork := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work")
	sub := filepath.Join(parentWork, "_subdivision", "child_04")
	if err := os.MkdirAll(filepath.Join(sub, "input"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "input", "chapter.md"), []byte("# 子章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(sub, "node", "integrated-stage", base+".tex")
	if err := os.MkdirAll(filepath.Dir(tex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tex, []byte("\\section{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Runner{}
	if msg := r.retryPrecondition(proj, "checker", base); msg != "" {
		t.Fatalf("子章节有产物时应通过前置检查，得到 %q", msg)
	}
}

// T201：同一棵细分树里不同层级都可能有 `child_04`。章节 id 的层级链必须与目录
// 链对上——真数据里 `chapter_014_child_04` 曾经被认成 child_01 底下那一支的
// child_04（只比目录名就会这样），核对的输入 md 就串了。
func TestFindChildMarkdownDepthExact(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "work", "temp", "conv_chapter_014", "work", "_subdivision")
	write := func(rel, content string) {
		p := filepath.Join(sub, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("child_04/input/chapter.md", "top-level child_04\n")
	write("child_01/node/work/_subdivision/child_04/input/chapter.md", "nested child_04\n")
	write("child_03/node/work/_subdivision/child_02/input/chapter.md", "nested child_02\n")

	got := findChildMarkdown(proj, "chapter_014", "chapter_014_child_04")
	if !strings.HasSuffix(filepath.ToSlash(got), "/_subdivision/child_04/input/chapter.md") ||
		strings.Contains(filepath.ToSlash(got), "child_01") {
		t.Fatalf("顶层 child_04 的 md 认错了: %q", got)
	}
	got = findChildMarkdown(proj, "chapter_014_child_03", "chapter_014_child_03_child_02")
	if !strings.HasSuffix(filepath.ToSlash(got), "/_subdivision/child_03/node/work/_subdivision/child_02/input/chapter.md") {
		t.Fatalf("嵌套 child_02 的 md 认错了: %q", got)
	}
}
