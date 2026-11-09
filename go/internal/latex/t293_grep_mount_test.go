package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T293：样式/修复这类会话的 grep 只有 Root+AltRoots（没有完整挂载表）。
// 模型写 `project:source/book.md` 时，以前会把整串当成文件名拼到 Root 下，
// 找不到就静默回 "NO MATCHES"——模型因此断定原文里没有那些标记。
func TestGrepAltRootExplicitMount(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(proj, "source", "book.md"),
		"# t\n\n<!-- DOCVISION-VECTOR: grammar diagram -->\nLINK: [vector](images/x.png)\n")

	tool := &GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: proj}}}
	res, err := tool.Execute(`{"path":"project:source/book.md","pattern":"DOCVISION-VECTOR"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(res.Text, "NO MATCHES") {
		t.Fatalf("显式挂载点没被解析（T293）: %q", res.Text)
	}
	if !strings.Contains(res.Text, "DOCVISION-VECTOR") {
		t.Fatalf("没搜到命中行: %q", res.Text)
	}
	// 命中行的前缀要是模型写过的那个路径写法，不能是绝对路径。
	if !strings.Contains(res.Text, "project:source/book.md:3:") {
		t.Fatalf("命中行路径写法不对: %q", res.Text)
	}
}

// 路径确实不存在时要说"搜的路径不存在"，不能回一句 NO MATCHES。
func TestGrepAltRootMissingPathIsNotNoMatches(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	tool := &GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: proj}}}
	res, err := tool.Execute(`{"path":"project:source/nope.md","pattern":"X"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(res.Text, "NO MATCHES") {
		t.Fatalf("路径不存在不该回 NO MATCHES: %q", res.Text)
	}
	if !strings.Contains(res.Text, "grep error") {
		t.Fatalf("应当是明确的路径错误: %q", res.Text)
	}
}

// 真的搜到了路径但模式不匹配，仍然如实说 NO MATCHES。
func TestGrepAltRootRealNoMatch(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(proj, "source", "book.md"), "# t\nplain text\n")
	tool := &GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: proj}}}
	res, err := tool.Execute(`{"path":"project:source/book.md","pattern":"DOCVISION-VECTOR"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Text, "NO MATCHES") {
		t.Fatalf("文件在但没命中时应当 NO MATCHES: %q", res.Text)
	}
}

// 无前缀的相对路径仍然先搜工作区、再按备选根兜底（行为不能退化）。
func TestGrepBareRelativeFallsBackToAltRoots(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(proj, "chapters", "a.tex"), "\\section{HIT}\n")
	tool := &GrepTool{Root: work, AltRoots: []AltRoot{{Label: "project", Dir: proj}}}
	res, err := tool.Execute(`{"pattern":"HIT","path":"chapters"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "HIT") {
		t.Fatalf("无前缀路径应当能搜到 project 下的文件: %q", res.Text)
	}
}

// 有完整挂载表的会话（转换/核对）行为不变。
func TestGrepWithMountsStillWorks(t *testing.T) {
	work := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(work, "main.tex"), "HIT work\n")
	writeFile(t, filepath.Join(proj, "source", "book.md"), "HIT proj\n")
	tool := &GrepTool{Mounts: []Mount{{Name: "work", Dir: work, Writable: true}, {Name: "project", Dir: proj}}}
	res, err := tool.Execute(`{"path":"project:source/book.md","pattern":"HIT"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "HIT proj") {
		t.Fatalf("挂载表路径解析退化: %q", res.Text)
	}
	if os.Getenv("V") != "" {
		t.Log(res.Text)
	}
}
