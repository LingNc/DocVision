package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/logger"
)

// T245：复盘会话把"最后一条好消息的下一行"当切口，而那一行几乎总是用量行。
// 以前四个截断点都硬拒这种 from，队列里的请求带着同一个数再派一遍、再拒一遍，
// 这一章就永远接不下去。现在往前挪到最近的消息行上。

// TestSnapCutLine：往前挪、前面没有才往后挪、整条没有消息行才报错。
func TestSnapCutLine(t *testing.T) {
	lines := []string{
		`{"t":"meta","kind":"system"}`,                  // 1
		`{"t":"msg","role":"user","text":"task"}`,       // 2
		`{"t":"msg","role":"assistant","text":"ok"}`,    // 3
		`{"t":"usage","finish":"tool_calls"}`,           // 4
		`{"t":"msg","role":"tool","text":"result"}`,     // 5
		`{"t":"usage","finish":"stop"}`,                 // 6
		`{"t":"msg","role":"assistant","text":"final"}`, // 7
	}
	cases := []struct {
		from int
		want int
	}{
		{4, 3}, // 用量行 → 前一条消息
		{1, 2}, // 第 1 行是 meta、前面没有消息 → 往后找
		{6, 5}, // 第二轮之后又是用量行
		{7, 7}, // 本来就是消息行，不动
		{5, 5},
	}
	for _, c := range cases {
		got, moved, err := snapCutLine(lines, c.from)
		if err != nil {
			t.Fatalf("from=%d 不该报错: %v", c.from, err)
		}
		if got != c.want {
			t.Fatalf("from=%d → %d, want %d", c.from, got, c.want)
		}
		if moved != (got != c.from) {
			t.Fatalf("from=%d moved=%v", c.from, moved)
		}
	}
	// 越界与 0 照旧报错。
	if _, _, err := snapCutLine(lines, 0); err == nil {
		t.Fatal("from=0 应报错")
	}
	if _, _, err := snapCutLine(lines, 99); err == nil {
		t.Fatal("越界应报错")
	}
	// 整条转录只有用量/元信息行：挪不过去，如实报错。
	only := []string{`{"t":"meta","kind":"system"}`, `{"t":"usage","finish":"stop"}`}
	if _, _, err := snapCutLine(only, 2); err == nil {
		t.Fatal("没有消息行时应报错")
	}
}

// TestTrimInPlaceSnapsOffUsageLine：事故原形——切口落在紧跟最后一条好消息的
// 用量行上，原地截断必须照做，而不是判"第 N 行不是一条消息记录"。
func TestTrimInPlaceSnapsOffUsageLine(t *testing.T) {
	proj := t.TempDir()
	base := "chapter_009"
	trPath := filepath.Join(proj, "work", "sessions", "convert_"+base+".jsonl")
	if err := os.MkdirAll(filepath.Dir(trPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"t":"meta","kind":"system"}`,                     // 1
		`{"t":"msg","role":"user","text":"task"}`,          // 2
		`{"t":"msg","role":"assistant","text":"step 1"}`,   // 3
		`{"t":"usage","finish":"tool_calls"}`,              // 4
		`{"t":"msg","role":"tool","text":"result"}`,        // 5
		`{"t":"msg","role":"assistant","text":"step 2"}`,   // 6
		`{"t":"usage","finish":"stop"}`,                    // 7 ← 复盘填的就是这一行
		`{"t":"msg","role":"assistant","text":"refusals"}`, // 8 后面的都丢掉
	}, "\n") + "\n"
	if err := os.WriteFile(trPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger("", "", 4)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: &config.Config{}, log: log}
	kept, dropped, err := r.trimTranscriptInPlace(proj, base, trPath, 7, "复盘判为退化")
	if err != nil {
		t.Fatalf("切口在用量行上应往前挪一条照切: %v", err)
	}
	if kept != 6 || dropped != 2 {
		t.Fatalf("切口应落在第 6 条消息上（保留 6、丢掉 2），got %d/%d", kept, dropped)
	}
	data, err := os.ReadFile(trPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "refusals") {
		t.Fatalf("退化那一段应被切掉：%q", text)
	}
	if !strings.Contains(text, "step 2") {
		t.Fatalf("最后一条好消息要保住：%q", text)
	}
	if strings.Count(strings.TrimRight(text, "\n"), "\n") < 6 {
		t.Fatalf("前缀少了行：%q", text)
	}
	// 切掉的那一段留档不删，切口留一条 kind=cut 记录。
	cuts, err := os.ReadDir(filepath.Join(proj, "work", "rescue", "cuts", base))
	if err != nil || len(cuts) != 1 {
		t.Fatalf("被切掉的那一段要留档：%v %v", cuts, err)
	}
	tail, err := os.ReadFile(filepath.Join(proj, "work", "rescue", "cuts", base, cuts[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tail), "refusals") {
		t.Fatalf("留档里应有被丢掉的那两条：%q", string(tail))
	}
	if !strings.Contains(text, `"kind":"cut"`) {
		t.Fatalf("切口应留一条 meta cut 记录：%q", text)
	}
}

// TestRescueSubmitSnapsFromUsageLine：复盘会话自己交的数就落位，队列里带的是
// 挪好的行号（派发端再挪也不会挪第二次）。
func TestRescueSubmitSnapsFromUsageLine(t *testing.T) {
	lines := []transcriptLine{
		{Line: 1, Kind: "meta"},
		{Line: 2, Kind: "msg", Role: "user"},
		{Line: 3, Kind: "msg", Role: "assistant"},
		{Line: 4, Kind: "usage"},
	}
	tool := &RescueSubmitTool{Lines: lines}
	res, err := tool.Execute(`{"verdict":"truncate","from":4,"reason":"refusal loop"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !tool.Submitted || tool.From != 3 || tool.RawFrom != 4 {
		t.Fatalf("from 应落到第 3 条：%+v", tool)
	}
	if !strings.Contains(res.Text, "LINE 3") || !strings.Contains(res.Text, "WILL TRUNCATE") {
		t.Fatalf("回执要说清切在哪儿：%s", res.Text)
	}
	if !strings.Contains(res.Text, "last message line 3") {
		t.Fatalf("回执要说明为什么挪：%s", res.Text)
	}
	// 本来就是消息行：一个数都不改。
	plain := &RescueSubmitTool{Lines: lines}
	if res, _ := plain.Execute(`{"verdict":"truncate","from":2,"reason":"x"}`); plain.From != 2 || plain.RawFrom != 2 {
		t.Fatalf("消息行不该被挪：%+v %s", plain, res.Text)
	}
	// 没有转录（单测/老调用方）：原样收下，不试图纠正。
	bare := &RescueSubmitTool{}
	if _, err := bare.Execute(`{"verdict":"truncate","from":9,"reason":"x"}`); err != nil || bare.From != 9 {
		t.Fatalf("没有转录时不该改数：%+v %v", bare, err)
	}
}

// TestCutChapterTranscriptForRetrySnapsUsageLine：派发端（队列里的 continue/网页
// ⤓ 请求）拿到一个落在用量行上的行号时同样往前挪一条，并且日志报的是真正切到
// 的那一行——以前这里硬拒，于是同一个坏行号在队列里来回撞、这一章永远续不上。
func TestCutChapterTranscriptForRetrySnapsUsageLine(t *testing.T) {
	proj := t.TempDir()
	dir := filepath.Join(proj, "work", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "convert_chapter_009.jsonl")
	body := `{"ts":"1","t":"meta","kind":"system"}
{"ts":"2","t":"msg","role":"user","content":"task"}
{"ts":"3","t":"msg","role":"assistant","content":"好"}
{"ts":"4","t":"usage","kind":"request"}
{"ts":"5","t":"msg","role":"assistant","content":"乱改一通"}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, true)
	got := r.cutChapterTranscriptForRetry(proj, "chapter_009", "convert_chapter_009.jsonl", 4)
	if got == "" {
		t.Fatal("切口落在用量行上应当先挪到消息行再切，而不是报错")
	}
	data, err := os.ReadFile(filepath.Join(dir, got))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimRight(string(data), "\n"), "\n") + 1; n != 3 {
		t.Fatalf("挪到第 3 行后新转录应是 3 行, got %d：%q", n, string(data))
	}
	if strings.Contains(string(data), "乱改一通") {
		t.Fatalf("坏尾巴不该跟过来：%q", string(data))
	}
}
