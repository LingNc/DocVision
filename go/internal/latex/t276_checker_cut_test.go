package latex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mineru-tools/internal/retryq"
)

// T276：核对会话被判退化（复盘 truncate）时只作废"判定"，**不许**动转换产物与
// 工作树。以前的实现跟着调 applyWorkspaceCut——把已提交的 chapter_XXX.tex /
// 分组目录挪进 work/attempts/<章>/try<核对尝试号>/，于是一个坏掉的核对模型就能
// 把一章合格的产物从书里搬走，这一章下一轮整章重转，再交一份给它再被搬走。
func TestRescueCheckerTruncateKeepsProducts(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_010"
	submit := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(submit, 0o755); err != nil {
		t.Fatal(err)
	}
	texPath := filepath.Join(submit, base+".tex")
	partsPath := filepath.Join(submit, base)
	if err := os.WriteFile(texPath, []byte("\\chapter{x}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(partsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partsPath, "part_01.tex"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(texPath+".checker", []byte("checker 会话已退化，已排队从干净轮次继续核对"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 核对转录：足够多的消息行，让"截到第 3 条"真的有东西可截。
	trPath := filepath.Join(proj, "work", "sessions", "checker_"+base+".jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(trPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		line := map[string]any{"t": "msg"}
		if i%2 == 0 {
			line["role"] = "assistant"
			line["content"] = []map[string]any{{"type": "text", "text": "working"}}
		} else {
			line["role"] = "tool"
			line["text"] = "ok"
		}
		raw, _ := json.Marshal(line)
		if _, err := f.Write(append(raw, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	r := newRoundTestRunner(t)
	_, err = r.rescueQueueTruncate(rescueTarget{
		ProjDir: proj, Base: base, Role: "checker", TrPath: trPath,
	}, 3)
	if err != nil {
		t.Fatalf("核对侧截断不该失败：%v", err)
	}
	if !fileExists(texPath) {
		t.Fatal("核对侧截断把已提交的 .tex 搬走了（T276 回归）")
	}
	if !fileExists(filepath.Join(partsPath, "part_01.tex")) {
		t.Fatal("核对侧截断把分片目录搬走了")
	}
	if _, err := os.Stat(filepath.Join(proj, "work", "attempts")); !os.IsNotExist(err) {
		t.Fatalf("核对侧截断不该创建 work/attempts（err=%v）", err)
	}
	pending := retryq.Pending(proj)
	if len(pending) != 1 || pending[0].Request.Role != "checker" {
		t.Fatalf("应排一条 checker 重核请求，got %+v", pending)
	}
}

// 归档只认转换转录：别的角色拿着自己的尝试号来归档，一律拒绝（并且不搬文件）。
func TestArchiveChapterProductsOnlyFromConvert(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_003"
	submit := filepath.Join(proj, "work", "chapters")
	if err := os.MkdirAll(submit, 0o755); err != nil {
		t.Fatal(err)
	}
	texPath := filepath.Join(submit, base+".tex")
	if err := os.WriteFile(texPath, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newRoundTestRunner(t)

	archiveChapterProducts(r, proj, base, "checker_"+base+".jsonl")
	if !fileExists(texPath) {
		t.Fatal("非转换转录不该归档产物")
	}
	archiveChapterProducts(r, proj, base, "convert_"+base+".jsonl")
	if fileExists(texPath) {
		t.Fatal("转换转录应当归档产物")
	}
	if !fileExists(filepath.Join(proj, "work", "attempts", base, "try1", base+".tex")) {
		t.Fatal("产物应落在 work/attempts/<章>/try1/")
	}
}
