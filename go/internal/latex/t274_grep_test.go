package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T274：grep 工具以前把模式交给 grep(1) 当 BRE——`a|b` 里的 `|` 是字面量（整段
// 多选一模式静默 "NO MATCHES"）、`begin\{question\}` 被当成区间表达式（报
// "grep: 不匹配的 {"）。这里钉住三条：ERE/Go 正则语义、编译不过时按纯文本、
// 以及"路径不存在"必须如实报错而不是 "NO MATCHES"。
func TestRunGrepGoRegexpSemantics(t *testing.T) {
	dir := t.TempDir()
	tex := filepath.Join(dir, "a.tex")
	body := strings.Join([]string{
		`\newcounter{question}`,
		`\newenvironment{workedexample}{}{}`,
		`plain line`,
		`中文，标点；测试：结束。`,
	}, "\n") + "\n"
	if err := os.WriteFile(tex, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// ① 交替：BRE 会把 | 当字面量。
	out, err := runGrep(`question|workedexample`, dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n")+1 != 2 {
		t.Fatalf("交替模式应命中两行，got %q", out)
	}
	// ② 大括号转义：模型常照 grep/ERE 写法写 \{…\}（Go 正则里 = 字面量），
	//    必须照常命中，而不是像 grep(1) 那样报 "不匹配的 {"。
	out, err = runGrep(`\{question\}`, dir, 10)
	if err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if !strings.Contains(out, "newcounter{question}") {
		t.Fatalf("转义大括号没命中：%q", out)
	}
	// ②' 真正编译不过的模式（如未闭合的括号）退回纯文本，不报错。
	if out, err := runGrep(`workedexample}{}{`, dir, 10); err != nil {
		t.Fatalf("编译不过应退回纯文本：%v", err)
	} else if !strings.Contains(out, "workedexample") {
		t.Fatalf("纯文本兜底没命中：%q", out)
	}
	// ③ 中文标点交替。
	out, err = runGrep(`。|，|；|：`, dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "中文，标点") {
		t.Fatalf("中文标点交替没命中：%q", out)
	}
	// ④ 什么都不匹配时返回空（工具层据此回 NO MATCHES）。
	if out, err := runGrep(`绝不会出现的串`, dir, 10); err != nil || out != "" {
		t.Fatalf("无匹配应返回空串，got %q / %v", out, err)
	}
	// ⑤ 行号从 1 开始。
	out, _ = runGrep(`plain line`, dir, 10)
	if !strings.Contains(out, ":3:plain line") {
		t.Fatalf("行号不对：%q", out)
	}
}

// 路径不存在必须是错误（而不是 NO MATCHES），并给出最近一层的真实条目。
func TestGrepMissingPathIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chapter.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &GrepTool{Root: dir}
	res, err := tool.Execute(`{"pattern":"x","path":"nope"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "grep error") || !strings.Contains(res.Text, "路径不存在") {
		t.Fatalf("应回显路径不存在：%q", res.Text)
	}
	if !strings.Contains(res.Text, "chapter.md") || !strings.Contains(res.Text, "chapters/") {
		t.Fatalf("应列出同级真实条目：%q", res.Text)
	}
	if strings.Contains(res.Text, "NO MATCHES") {
		t.Fatalf("不该再说 NO MATCHES：%q", res.Text)
	}
}

// 二进制文件跳过、命中数按 max_matches 截断并提示。
func TestGrepSkipsBinaryAndCapsMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0x00, 0x01, 'h', 'i'}, 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < 20; i++ {
		b.WriteString("hit me\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGrep(`hit me`, dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "...(15 more, 收窄 pattern)") {
		t.Fatalf("截断提示缺失：%q", out)
	}
	if strings.Contains(out, "bin.dat") {
		t.Fatalf("二进制文件不该被搜：%q", out)
	}
}
