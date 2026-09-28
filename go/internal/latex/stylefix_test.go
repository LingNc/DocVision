package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T64（重构）：submit_problems 无参收账——读共享存储，空清单拒绝。
func TestSubmitProblemsTool(t *testing.T) {
	st := loadProblemStore(filepath.Join(t.TempDir(), "problems.json"))
	tl := &SubmitProblemsTool{Store: st}
	res, err := tl.Execute(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Set {
		t.Fatal("空清单不应提交成功")
	}
	if !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("空清单应打回: %q", res.Text)
	}

	// 存储里有一条后即可提交。
	st.Problems = []StyleProblem{{ID: "P1", Title: "题号样式错", Detail: "选择题编号应为粗体", Chapters: []string{"chapter_001"}}}
	res, err = tl.Execute(`{"report":"done"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tl.Set || !strings.Contains(res.Text, "1 problems") {
		t.Fatalf("应提交成功: %q", res.Text)
	}

	// 计划清单未完成时打回。
	g := NewGoalTool(filepath.Join(t.TempDir(), "plan.md"))
	tl2 := &SubmitProblemsTool{Store: st, Goal: g}
	if _, err := g.Execute(`{"op":"add","text":"再核对一遍 chapter_003"}`); err != nil {
		t.Fatal(err)
	}
	res, _ = tl2.Execute(`{}`)
	if tl2.Set {
		t.Fatal("计划未完成不应提交成功")
	}
	if !strings.Contains(res.Text, "再核对一遍") {
		t.Fatalf("打回应点名未完成项: %q", res.Text)
	}
	if _, err := g.Execute(`{"op":"check","n":1}`); err != nil {
		t.Fatal(err)
	}
	if _, err := tl2.Execute(`{}`); err != nil || !tl2.Set {
		t.Fatal("计划完成后应提交成功")
	}
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
