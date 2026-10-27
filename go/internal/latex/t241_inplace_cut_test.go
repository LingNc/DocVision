package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T241：复盘截断不在历史里长出新尝试，但"切口别落在半截工具调用上"这件事必须
// 由代码保证——模型看到自己上一轮调了工具却没有回执，下一句就是"这个会话里
// 工具不可用"，然后同样的退化再演一遍（实测一族章节连切十次全是这句）。
func TestBalancedCutFrom(t *testing.T) {
	lines := []string{
		`{"t":"meta","kind":"system"}`,
		`{"t":"msg","role":"user","text":"go"}`,
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"a"},{"id":"b"}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"a","text":"ok"}`,
		`{"t":"msg","role":"tool","tool_call_id":"b","text":"ok"}`,
		`{"t":"msg","role":"assistant","text":"done"}`,
	}
	cases := []struct {
		name       string
		from, want int
	}{
		{"切口在平衡处不动", 6, 6},
		{"切口在 user 行", 2, 2},
		{"切口落在等回执的 assistant 后面就前移", 3, 2},
		{"只回了一条回执也前移", 4, 2},
		{"两条回执齐了就可以切", 5, 5},
		{"切口在 meta 行按前一条消息算", 1, 0},
	}
	for _, c := range cases {
		if got := balancedCutFrom(lines, c.from); got != c.want {
			t.Fatalf("%s：from=%d 应得 %d，got %d", c.name, c.from, c.want, got)
		}
	}
	// 老转录没有 tool_call_id：按先后顺序一对一销。
	legacy := []string{
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":""}]}`,
		`{"t":"msg","role":"tool","text":"ok"}`,
		`{"t":"msg","role":"assistant","text":"done"}`,
	}
	if got := balancedCutFrom(legacy, 2); got != 2 {
		t.Fatalf("老转录按顺序销账：from=2 的回执就算数，got %d", got)
	}
	if got := balancedCutFrom(legacy, 3); got != 3 {
		t.Fatalf("回执到了就该能切：got %d", got)
	}
}

// T241：原地截断——同一条文件、同一个尝试号、丢掉的那一段留档。
func TestTrimTranscriptInPlace(t *testing.T) {
	proj := t.TempDir()
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_002.try3.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"t":"msg","role":"user","text":"1"}`,
		`{"t":"msg","role":"assistant","text":"2"}`,
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"x"}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"x","text":"ok"}`,
		`{"t":"msg","role":"assistant","text":"5"}`,
		`{"t":"msg","role":"user","text":"6"}`,
	}
	if err := os.WriteFile(trPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	kept, dropped, err := r.trimTranscriptInPlace(proj, "chapter_002", trPath, 5, "复盘判为退化")
	if err != nil {
		t.Fatal(err)
	}
	if kept != 5 || dropped != 1 {
		t.Fatalf("应保留 5 条、丢掉 1 条，got %d/%d", kept, dropped)
	}
	all, err := os.ReadFile(trPath)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimRight(string(all), "\n"), "\n")
	if len(rows) != 6 {
		t.Fatalf("应是 5 条记录 + 1 条 cut 记录，got %d", len(rows))
	}
	if rows[0] != lines[0] || rows[4] != lines[4] {
		t.Fatalf("保留的那一段应逐字不变")
	}
	if !strings.Contains(rows[5], `"kind":"cut"`) || !strings.Contains(rows[5], `"dropped":1`) {
		t.Fatalf("末尾应有 cut 记录：%s", rows[5])
	}
	// 不能顺手多长出下一条尝试。
	if fileExists(attemptTranscriptPath(transcriptStem(trPath), 4)) {
		t.Fatalf("原地截断不该写新尝试")
	}
	// 证据留档（按尝试号不变、证据目录只作查看）。
	dirs, err := os.ReadDir(filepath.Join(proj, "work", "rescue", "cuts", "chapter_002"))
	if err != nil || len(dirs) != 1 {
		t.Fatalf("留档文件缺失：%v %v", dirs, err)
	}
	if !strings.Contains(dirs[0].Name(), "convert_chapter_002.try3") {
		t.Fatalf("留档文件名应带原来的转录名与尝试号：%s", dirs[0].Name())
	}
}

// T241：切口落在等回执的那一条上时，真正切的位置要往前挪。
func TestTrimTranscriptInPlaceWalksBack(t *testing.T) {
	proj := t.TempDir()
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_003.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"t":"msg","role":"user","text":"1"}`,
		`{"t":"msg","role":"assistant","text":"2"}`,
		`{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"a"},{"id":"b"},{"id":"c"},{"id":"d"}]}`,
		`{"t":"msg","role":"assistant","text":"4"}`,
	}
	if err := os.WriteFile(trPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	kept, _, err := r.trimTranscriptInPlace(proj, "chapter_003", trPath, 4, "复盘判为退化")
	if err != nil {
		t.Fatal(err)
	}
	if kept != 2 {
		t.Fatalf("第 3 条还在等 4 个回执，切口应挪到第 2 条，got %d", kept)
	}
	cuts, err := os.ReadDir(filepath.Join(proj, "work", "rescue", "cuts", "chapter_003"))
	if err != nil || len(cuts) != 1 {
		t.Fatalf("被前移切掉的那两条也要留档：%v %v", cuts, err)
	}
	tail, err := os.ReadFile(filepath.Join(proj, "work", "rescue", "cuts", "chapter_003", cuts[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Split(strings.TrimRight(string(tail), "\n"), "\n")); got != 2 {
		t.Fatalf("留档应是被丢掉的 2 条，got %d", got)
	}
}

// 整段前缀都不平衡时如实失败，别切出一个接不下去的转录。
func TestTrimTranscriptInPlaceRefusesUnbalancedPrefix(t *testing.T) {
	proj := t.TempDir()
	trPath := filepath.Join(proj, "work", "sessions", "convert_chapter_004.jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"t":"msg","role":"assistant","text":"","tool_calls":[{"id":"a"}]}` + "\n" +
		`{"t":"msg","role":"assistant","text":"2"}` + "\n"
	if err := os.WriteFile(trPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	if _, _, err := r.trimTranscriptInPlace(proj, "chapter_004", trPath, 2, "x"); err == nil {
		t.Fatalf("前缀里没有任何一步是回执齐的，应拒绝")
	}
	if got, err := os.ReadFile(trPath); err != nil || string(got) != body {
		t.Fatalf("拒绝时一个字都不该动：%v", err)
	}
}
