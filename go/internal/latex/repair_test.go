package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 转录按 jsonl 写，测试也按 jsonl 造行：与真实的读法完全一致（解析、坏行、
// 行号都走同一条路）。
const (
	mountErrText = `TOOL ERROR: 未知挂载点 "source"（可用: work(rw), project(ro), input(ro)）`
	sourceGate   = "REJECTED: 还没有和原书对照过。这一章的 PDF 有 8 页，至少用 view_pdf 打开原书对应页 2 页" +
		"（source:part.pdf, page: 3），逐段核对——你已经看了 0 页。"
	compactedLine = `{"t":"msg","role":"user","text":"=== COMPRESSED SESSION CONTEXT (older turns summarized) ==="}`
)

func userLine(text string) string {
	return `{"t":"msg","role":"user","text":` + quoteJSON(text) + `}`
}

func assistantLine(text string) string {
	return `{"t":"msg","role":"assistant","text":` + quoteJSON(text) + `}`
}

func toolLine(text string) string {
	return `{"t":"msg","role":"tool","tool_call_id":"c0","text":` + quoteJSON(text) + `}`
}

func submitCallLine(id string) string {
	return `{"t":"msg","role":"assistant","tool_calls":[{"id":"` + id +
		`","type":"function","function":{"name":"submit","arguments":"{}"}}]}`
}

func submitReceiptLine(id, text string) string {
	return `{"t":"msg","role":"tool","tool_call_id":"` + id + `","text":` + quoteJSON(text) + `}`
}

const (
	runEndLine  = `{"t":"meta","kind":"run_end","ok":true}`
	queuedLine  = `{"t":"meta","kind":"queued"}`
	resultLine  = `{"t":"meta","kind":"checking_result"}`
	submittedOK = "SUBMITTED. chapters/x.tex and chapters/x/ (3 files) are in the book."
)

func rowsOf(t *testing.T, lines ...string) []repairRow {
	t.Helper()
	return parseRepairRows([]byte(strings.Join(lines, "\n") + "\n"))
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// 最常见的那一类：原书视图没起来的那一轮里，每一步都撞在"没有 source 挂载点"
// 上，模型于是写一段"做不到"的结论收尾。
func TestRepairCutsAtLastMessageBeforeMountError(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		assistantLine("我先读 md"),
		toolLine("file contents"),
		runEndLine, // 上一次跑到哪儿
		assistantLine("继续"),
		toolLine(mountErrText),
		assistantLine("本书没有原书 PDF，我无法继续。"),
	)
	cut, why := analyzeDerailedTranscript(rows)
	if cut != 4 {
		t.Fatalf("cut = %d, want 4（留到挂载错误之前那条 assistant）", cut)
	}
	if !strings.Contains(why, "挂载点") {
		t.Fatalf("why = %q", why)
	}
}

// 已经真的交进去了就不许切：历史里的报错只是过程，切了会毁掉已做的工作。
func TestRepairKeepsTranscriptThatSubmitted(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(mountErrText),
		submitCallLine("c1"),
		submitReceiptLine("c1", submittedOK),
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, want -1（提交过的不动）", cut)
	}
}

// 被门禁打回不算"提交过"：那正是卡住的形态。
func TestRepairTreatsRejectedSubmitAsUnsubmitted(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(mountErrText),
		submitCallLine("c1"),
		submitReceiptLine("c1", sourceGate),
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut != 0 {
		t.Fatalf("cut = %d, want 0（留到第一条 user）", cut)
	}
}

// 一条"还没和原书对照过"是正常的门禁行为（模型还能补救），只有同一条反复
// 出现（已看页数一直不变 = 一步没动）才算退化。
func TestRepairIgnoresSingleGateRejection(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		assistantLine("写 tex"),
		toolLine(sourceGate),
		assistantLine("好，我去看原书"),
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, want -1（单次打回不算退化）", cut)
	}
}

func TestRepairCutsOnRepeatedGateRejection(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		assistantLine("写 tex"),
		toolLine(sourceGate),
		assistantLine("再交一次"),
		toolLine(sourceGate),
		assistantLine("算了"),
	)
	cut, why := analyzeDerailedTranscript(rows)
	if cut != 1 {
		t.Fatalf("cut = %d, want 1（留到同一条打回第一次出现之前）", cut)
	}
	if !strings.Contains(why, "对照") {
		t.Fatalf("why = %q", why)
	}
}

// 页数变了就不算"同一条"：模型确实在换页看，只是还没凑够。
func TestRepairDistinguishesDifferentViewCounts(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(sourceGate),
		assistantLine("再看两页"),
		toolLine("REJECTED: 还没有和原书对照过。这一章的 PDF 有 8 页，至少用 view_pdf 打开原书对应页 2 页"+
			"（source:part.pdf, page: 5），逐段核对——你已经看了 1 页。"),
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, want -1（页数在涨，说明还在干活）", cut)
	}
}

// 压缩检查点之前的行本来就不回放：切在那儿等于没切，宁可不修。
func TestRepairSkipsWhenCutFallsBeforeCompactionMarker(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(mountErrText),
		runEndLine,
		compactedLine,
		assistantLine("摘要之后继续"),
		runEndLine,
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, want -1（切点落在检查点之前）", cut)
	}
}

// 尾巴上只剩元信息行时照样修：证据在上一次跑完的那一段里，回放窗口里它仍然在。
func TestRepairCutsInEarlierRunWhenTailIsMetaOnly(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		assistantLine("我先读 md"),
		runEndLine,
		toolLine(mountErrText),
		runEndLine,
		queuedLine,
		resultLine,
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut != 1 {
		t.Fatalf("cut = %d, want 1（证据在前一段的最后一条消息之前）", cut)
	}
}

// 后来真的把原书页看进去了，就说明那一轮自己走出去了：之前的报错只是过程，
// 不能再拿它当截断理由（截了等于白扔那段干活记录）。
func TestRepairLeavesSessionThatLaterSawOriginalPagesAlone(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(mountErrText),
		assistantLine("换一种办法"),
		toolLine("PDF page source:book_part1.pdf p12 (crop 0%,0%-100%,100%, width 1400px) (page 12/200) attached."),
		assistantLine("看清了，继续对"),
	)
	if cut, why := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, why = %q, want -1（已经看过原书页）", cut, why)
	}
}

// 别的挂载点（figures/parts 那些）报错不是这条判据：它们不影响"和原书对照"
// 那道门，也不会把模型带进"这本书不存在"的结论里。
func TestRepairIgnoresOtherMountErrors(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		toolLine(`TOOL ERROR: 未知挂载点 "figures"（可用: work(rw), source(ro)）`),
		assistantLine("那就直接写 tikz"),
	)
	if cut, _ := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, want -1（不是原书挂载点）", cut)
	}
}

// 干净但没跑完的会话（只是被中断）绝对不能动：那种续跑本来就能接上。
func TestRepairLeavesCleanUnfinishedTranscriptAlone(t *testing.T) {
	rows := rowsOf(t,
		userLine("convert chapter"),
		assistantLine("读 md"),
		toolLine("file contents"),
		assistantLine("开始写 tex"),
	)
	if cut, why := analyzeDerailedTranscript(rows); cut >= 0 {
		t.Fatalf("cut = %d, why = %q, want -1", cut, why)
	}
}

// 落盘那一半：源转录一字不动，新尝试正好是它的前缀。
func TestRepairDerailedTranscriptWritesPrefixAttempt(t *testing.T) {
	dir := t.TempDir()
	nodeWork := filepath.Join(dir, "conv", "work")
	if err := os.MkdirAll(nodeWork, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "conv", "sessions", "convert_chapter_900.jsonl")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		userLine("convert"),
		assistantLine("ok"),
		toolLine(mountErrText),
		assistantLine("做不到"),
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dst, attempt, why, ok := repairDerailedTranscript(src, nodeWork)
	if !ok {
		t.Fatalf("没修：%q", why)
	}
	if attempt != 2 {
		t.Fatalf("attempt = %d, want 2", attempt)
	}
	if !strings.HasSuffix(dst, ".try2.jsonl") {
		t.Fatalf("dst = %s", dst)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(lines[:2], "\n") + "\n"; string(got) != want {
		t.Fatalf("新尝试内容不对:\n got %q\nwant %q", got, want)
	}
	if now, err := os.ReadFile(src); err != nil || string(now) != body {
		t.Fatalf("源转录被改了（err=%v）", err)
	}
	// 再跑一次不该再切：新尝试里已经没有证据。
	if _, _, _, ok := repairDerailedTranscript(dst, nodeWork); ok {
		t.Fatal("新尝试又被切了一次")
	}
}

// 已经有过旧尝试时，新号要接着往下排，不能覆盖。
func TestRepairDerailedTranscriptPicksNextAttemptNumber(t *testing.T) {
	dir := t.TempDir()
	nodeWork := filepath.Join(dir, "work")
	for _, p := range []string{nodeWork, filepath.Join(dir, "sessions")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	src := filepath.Join(dir, "sessions", "convert_chapter_910.jsonl")
	body := strings.Join([]string{userLine("x"), toolLine(mountErrText)}, "\n") + "\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions", "convert_chapter_910.try2.jsonl"),
		[]byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	dst, attempt, why, ok := repairDerailedTranscript(src, nodeWork)
	if !ok {
		t.Fatalf("没修：%q", why)
	}
	if attempt != 3 || !strings.HasSuffix(dst, ".try3.jsonl") {
		t.Fatalf("attempt = %d dst = %s, want 3 / *.try3.jsonl", attempt, dst)
	}
}

// 没有工作树的转录不修：那种会话续跑不了，切了也没人接。
func TestRepairSkipsWithoutWorkTree(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "convert_chapter_901.jsonl")
	body := strings.Join([]string{userLine("x"), toolLine(mountErrText)}, "\n") + "\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := repairDerailedTranscript(src, filepath.Join(dir, "missing")); ok {
		t.Fatal("没有工作树却修了")
	}
}
