package latex

import (
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
	"mineru-tools/internal/retryq"
)

// T303：产物已经核对通过、之后没改过时，自动提的「接着核对 / 继续跑」要被认
// 出来作废；产物改了、或又被打了回，就不算"已通过"。
func TestChapterPassedAndUnchanged(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_007"
	chapDir := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(filepath.Join(chapDir, base), 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(chapDir, base+".tex")
	part := filepath.Join(chapDir, base, "part01.tex")
	for _, p := range []string{tex, part} {
		if err := os.WriteFile(p, []byte("body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if chapterPassedAndUnchanged(proj, base) {
		t.Fatal("没有通过记号时不该判定已通过")
	}
	writePassMarker(tex, productFingerprint(tex, filepath.Join(chapDir, base)))
	if !chapterPassedAndUnchanged(proj, base) {
		t.Fatal("产物与记号一致时应判定已通过")
	}
	// 分片改了：指纹对不上，这一版不再是"通过那一版"。
	if err := os.WriteFile(part, []byte("body changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if chapterPassedAndUnchanged(proj, base) {
		t.Fatal("分片改过之后不该判定已通过")
	}
	writePassMarker(tex, productFingerprint(tex, filepath.Join(chapDir, base)))
	if !chapterPassedAndUnchanged(proj, base) {
		t.Fatal("重新记下记号后应判定已通过")
	}
	// 又被打了回：.checker 在就不算通过（那串问题还要处理）。
	if err := os.WriteFile(tex+".checker", []byte("1. 缺一张表"), 0o644); err != nil {
		t.Fatal(err)
	}
	if chapterPassedAndUnchanged(proj, base) {
		t.Fatal("有 .checker 打回时不该判定已通过")
	}
}

// 队列里排着的自动重核请求，在产物已经通过之后要被作废；页面点的那条留着。
func TestPrepareCheckerRetriesVoidsAutoRequestsOnPassedChapter(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_007"
	chapDir := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(chapDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tex := filepath.Join(chapDir, base+".tex")
	if err := os.WriteFile(tex, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 章节 md 在、产物在（prepareCheckerRetries 的前置条件），并且这一版已通过。
	if err := os.MkdirAll(filepath.Join(proj, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "chapters", base+".md"), []byte("# 章\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePassMarker(tex, productFingerprint(tex, filepath.Join(chapDir, base)))

	for _, src := range []string{"no_verdict", "web"} {
		if _, err := retryq.Queue(proj, retryq.Request{
			Chapter: base, Role: "checker", Source: src, Reason: "测试",
		}); err != nil {
			t.Fatal(err)
		}
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	got := r.prepareCheckerRetries(proj)
	if len(got) != 1 || got[0] != base {
		t.Fatalf("只应留下页面提的那条请求，实际 %v", got)
	}
	left := retryq.Pending(proj)
	if len(left) != 0 {
		t.Fatalf("两条请求都该被消费掉（自动的作废、页面的交给批次处理），还剩 %d 条", len(left))
	}
}

// T304：重核通过之后要把暂存区里的产物晋级到 work/chapters/，否则下一次启动按
// "没有产物"整章重转。
func TestPromoteRecheckedProduct(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_007"
	stage := filepath.Join(proj, "work", "temp", "conv_"+base, "integrated-stage")
	writeFile := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	texPath := filepath.Join(stage, base+".tex")
	writeFile(texPath, "\\input{chapter_007/children/chapter_007_child_01}\n")
	writeFile(filepath.Join(stage, base, "children", base+"_child_01.tex"), "child body\n")
	writePassMarker(texPath, productFingerprint(texPath, filepath.Join(stage, base)))

	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	r.promoteRecheckedProduct(proj, base, texPath, filepath.Join(stage, base))

	dstTex := filepath.Join(proj, "work", "chapters", base+".tex")
	if !fileExists(dstTex) {
		t.Fatal("重核通过后主文件应已晋级到 work/chapters/")
	}
	dstChild := filepath.Join(proj, "work", "chapters", base, "children", base+"_child_01.tex")
	if !fileExists(dstChild) {
		t.Fatal("分片目录应一起晋级")
	}
	if !chapterPassedAndUnchanged(proj, base) {
		t.Fatal("晋级后应认出这一版已经通过（记号要跟着搬到新路径）")
	}
}
