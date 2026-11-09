package latex

import (
	"os"
	"path/filepath"
	"testing"
)

// T285：交过的阶段就是做完了——重入时跳过它、直接进下一阶段，而不是另开一条
// 会话把那一步再跑一遍。
func TestStyleRoundDoneReadsTranscripts(t *testing.T) {
	proj := t.TempDir()
	dir := filepath.Join(proj, "work", "sessions")
	if styleRoundDone(proj, 1) {
		t.Fatal("还没有转录时不该算已完成")
	}
	writeAttemptTranscript(t, dir, "stylefix_style_r1.jsonl",
		`{"t":"msg","role":"user","text":"开始"}`,
		`{"t":"msg","role":"assistant","text":"改完","tool_calls":[{"id":"c1","type":"function","function":{"name":"submit_style","arguments":"{}"}}]}`)
	if !styleRoundDone(proj, 1) {
		t.Fatal("交过 submit_style 的一轮应当算已完成")
	}
	if styleRoundDone(proj, 2) {
		t.Fatal("别的轮次不该受影响")
	}

	// 第二次尝试（.2）交的也算。
	writeAttemptTranscript(t, dir, "stylefix_style_r2.jsonl", `{"t":"msg","role":"user","text":"开始"}`)
	writeAttemptTranscript(t, dir, "stylefix_style_r2.2.jsonl",
		`{"t":"msg","role":"assistant","text":"改完","tool_calls":[{"id":"c2","type":"function","function":{"name":"submit_style","arguments":"{}"}}]}`)
	if !styleRoundDone(proj, 2) {
		t.Fatal(".2 那条交过也算这一轮已完成")
	}
}

// T285：分块与各块结论落盘后可以沿用；工作汇报指纹变了就作废（旧分块不能拿来
// 糊新报告）。
func TestRoundBlocksAndResultsReuseBySHA(t *testing.T) {
	proj := t.TempDir()
	if _, ok := loadRoundBlocks(proj, 1, "sha-a"); ok {
		t.Fatal("没有落盘时不该沿用")
	}
	data := `{"sha":"sha-a","blocks":[{"title":"P1","problem_ids":["P1"],"chapters":["chapter_001","chapter_002"]}]}`
	if err := os.MkdirAll(filepath.Join(proj, "work", "style_fix"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(roundArtifact(proj, "blocks", 1), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := loadRoundBlocks(proj, 1, "sha-a")
	if !ok || len(got) != 1 || len(got[0].Chapters) != 2 {
		t.Fatalf("同指纹应当沿用分块: %v %v", ok, got)
	}
	if _, ok := loadRoundBlocks(proj, 1, "sha-b"); ok {
		t.Fatal("指纹变了就不能沿用旧分块")
	}

	// 各块结论：同指纹可读回，异指纹读不到。
	if err := saveBlockResult(proj, 1, 1, "sha-a", blockResult{
		Resolved: map[string]bool{"chapter_001": true, "chapter_002": false},
		Notes:    map[string]string{"chapter_001": "已改", "chapter_002": "还要再看"},
	}); err != nil {
		t.Fatal(err)
	}
	all := loadBlockResults(proj, 1, "sha-a")
	res, ok := all["1"]
	if !ok || !res.Resolved["chapter_001"] || res.Resolved["chapter_002"] || res.Notes["chapter_002"] != "还要再看" {
		t.Fatalf("结论应当原样读回: %v", all)
	}
	if len(loadBlockResults(proj, 1, "sha-b")) != 0 {
		t.Fatal("指纹变了就不能沿用旧结论")
	}

	// 第二块写进去不能把第一块挤掉。
	if err := saveBlockResult(proj, 1, 2, "sha-a", blockResult{Resolved: map[string]bool{"chapter_003": true}}); err != nil {
		t.Fatal(err)
	}
	if all := loadBlockResults(proj, 1, "sha-a"); len(all) != 2 || !all["1"].Resolved["chapter_001"] {
		t.Fatalf("追加写不该覆盖已有的块结论: %v", all)
	}
}
