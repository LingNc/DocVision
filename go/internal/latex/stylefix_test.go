package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T43：submit_problems 解析与校验。
func TestSubmitProblemsTool(t *testing.T) {
	tl := &SubmitProblemsTool{}
	res, err := tl.Execute(`{"problems":[{"id":"P1","title":"题号样式错","detail":"选择题编号应为粗体","chapters":["chapter_001","chapter_003"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.Set || len(tl.Problems) != 1 {
		t.Fatalf("problems = %+v", tl.Problems)
	}
	p := tl.Problems[0]
	if p.ID != "P1" || len(p.Chapters) != 2 {
		t.Errorf("problem = %+v", p)
	}
	_ = res

	// 空清单拒绝。
	tl2 := &SubmitProblemsTool{}
	r2, _ := tl2.Execute(`{"problems":[]}`)
	if tl2.Set {
		t.Error("empty problems must be rejected")
	}
	_ = r2
}

// T43：submit_blocks 解析与校验。
func TestSubmitBlocksTool(t *testing.T) {
	tl := &SubmitBlocksTool{}
	_, err := tl.Execute(`{"blocks":[{"title":"题号修复","problem_ids":["P1"],"chapters":["chapter_001","chapter_003"]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.Set || len(tl.Blocks) != 1 || len(tl.Blocks[0].Chapters) != 2 {
		t.Fatalf("blocks = %+v", tl.Blocks)
	}
}

// T43：分块提交逐章结论。
func TestSubmitBlockFixTool(t *testing.T) {
	tl := &SubmitBlockFixTool{}
	_, err := tl.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true,"note":"已修"},{"chapter":"chapter_003","resolved":false,"note":"仍有问题"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.Submitted {
		t.Fatal("not submitted")
	}
	if !tl.ChapterResults["chapter_001"] || tl.ChapterResults["chapter_003"] {
		t.Errorf("results = %+v", tl.ChapterResults)
	}
	if tl.Notes["chapter_003"] != "仍有问题" {
		t.Errorf("notes = %+v", tl.Notes)
	}
}

// T43：collectIssueReports 只统计「存在问题」的汇报。
func TestCollectIssueReports(t *testing.T) {
	proj := t.TempDir()
	dir := filepath.Join(proj, "work", "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "chapter_001.md"), []byte("- 结论: 存在问题\n\n题号样式错"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapter_002.md"), []byte("- 结论: 无问题\n"), 0o644)
	total, issues := collectIssueReports(proj)
	if total != 2 || len(issues) != 1 {
		t.Fatalf("total=%d issues=%d", total, len(issues))
	}
	if _, ok := issues["chapter_001"]; !ok {
		t.Errorf("issues = %v", issues)
	}
}
