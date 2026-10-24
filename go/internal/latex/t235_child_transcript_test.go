package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T235：递归子章节失败时，终态必须写到**这一次实际用的**那条转录上。
//
// 以前 `convertChapterNode` 按值收 chapterNodeConfig：里面的尝试号推导、转录名
// 改写都发生在副本上，调用方手里的 `childNode.TranscriptPath` 还是第 1 次尝试的
// 旧路径。于是"子章节失败"这句话被追加到一条早已结束的历史转录末尾（它的 mtime
// 因此变新，预览页按最新挑行，就把那一代显示成「会话错误」——实际上那一代早就
// 通过了）。这里直接钉住契约：函数返回之后，调用方手里的路径必须已经指向磁盘上
// 该用的那一次尝试。
func TestConvertChapterNodeRewritesCallerTranscriptPath(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_001"
	for _, d := range []string{
		filepath.Join(proj, "chapters"),
		filepath.Join(proj, "work", "sessions"),
		filepath.Join(proj, "work", "chapters"),
		filepath.Join(proj, "work", "reports"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	chapPath := filepath.Join(proj, "chapters", base+".md")
	if err := os.WriteFile(chapPath, []byte("# 章\n\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 磁盘上已经有到第 4 次尝试的转录：这一次要接着写 .try4.jsonl，而不是第 1 条。
	baseTr := filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
	if err := os.WriteFile(filepath.Join(proj, "work", "sessions", "convert_"+base+".try4.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := testRunner(t, true)
	node := chapterNodeConfig{
		Base: base, Depth: 1, InputMount: "project", InputRef: "chapters/" + base + ".md",
		NodeRoot:       filepath.Join(proj, "work", "temp", "conv_"+base),
		TranscriptPath: baseTr,
		SubmitRoot:     filepath.Join(proj, "work", "chapters"),
		ReportPath:     filepath.Join(proj, "work", "reports", base+".md"),
		Seq:            1,
	}
	// 没有模型客户端：函数会在这里就断掉（clientFor 直接 panic）。断言的点在它
	// **之前**——转录名已经按磁盘上的尝试号推导完并写回调用方那份配置。
	func() {
		defer func() { _ = recover() }()
		_ = r.convertChapterNode(proj, "cls", filepath.Join(proj, "manual.md"), chapPath,
			filepath.Join(proj, "work", "chapters"), 0, 0, true, &node)
	}()
	want := filepath.Join(proj, "work", "sessions", "convert_"+base+".try4.jsonl")
	if node.TranscriptPath != want {
		t.Fatalf("调用方拿到的转录路径 = %q, want %q（按值传参时这里会退回第 1 次尝试）", node.TranscriptPath, want)
	}
	if strings.Contains(node.TranscriptPath, ".try4.try") {
		t.Fatalf("转录名套娃: %q", node.TranscriptPath)
	}
}
