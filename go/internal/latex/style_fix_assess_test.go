package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T64 同款：评估/分组会话的 blocks.json 提交路径——无内联参数时从工作区读。
func TestSubmitBlocksFromWorkspace(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "blocks.json"), []byte(
		`{"blocks":[{"title":"第一章块","problem_ids":["P1"],"chapters":["chapter_001"]}]}`), 0o644)
	submit := &SubmitBlocksTool{Workspace: ws}
	res, err := submit.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !submit.Set || len(submit.Blocks) != 1 || submit.Blocks[0].Title != "第一章块" {
		t.Fatalf("blocks.json 提交未生效: %q %+v", res.Text, submit.Blocks)
	}
	// 无内联且无文件 → REJECTED
	submit2 := &SubmitBlocksTool{Workspace: t.TempDir()}
	res2, _ := submit2.Execute(`{}`)
	if submit2.Set || !strings.Contains(res2.Text, "REJECTED") {
		t.Fatalf("空工作区应被拒: %q", res2.Text)
	}
	// blocks.json 坏 JSON → REJECTED 带修正提示
	ws3 := t.TempDir()
	os.WriteFile(filepath.Join(ws3, "blocks.json"), []byte(`{bad`), 0o644)
	submit3 := &SubmitBlocksTool{Workspace: ws3}
	res3, _ := submit3.Execute(`{}`)
	if submit3.Set || !strings.Contains(res3.Text, "REJECTED") {
		t.Fatalf("坏 JSON 应被拒: %q", res3.Text)
	}
}
